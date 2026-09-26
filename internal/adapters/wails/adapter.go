package wails

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nord-launcher/launcher/internal/core/auth"
	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/content/modrinth"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/java"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/manifest"
	"github.com/nord-launcher/launcher/internal/core/netutil"
	"github.com/nord-launcher/launcher/internal/core/ports"
	"github.com/nord-launcher/launcher/internal/core/storage"
	"github.com/nord-launcher/launcher/internal/core/updater"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// WailsAdapter connects Wails IPC layer to the Hexagonal Core.
type WailsAdapter struct {
	svc          *launch.InstanceService
	authSvc      *auth.AuthService
	accountRepo  ports.AccountRepository
	modrinth     *modrinth.Client
	curseforge   *curseforge.Client
	fileSys      ports.FileSystem
	version      string
	instancesDir string
	updater      *updater.AutoUpdater
	relauncher   updater.RelauncherFunc
	javaDetector ports.JavaDetector
	settingsRepo *storage.SettingsRepository
	httpClient   *http.Client
	allowedHosts []string
	javaMgr      *java.JavaManager

	installedModsRepo *storage.InstalledModsRepository
	db                *sql.DB

	lastCrashes        map[string]*CrashReportDTO
	modVersionsCache   map[string]modVersionCacheEntry
	modInstallProgress map[string]*ModInstallProgressDTO
	mrpackImporter     *content.MrPackImporter
	mrpackExporter     *content.MrPackExporter
	mrpackProgress     map[string]*MrPackImportStatusDTO
	filePickerFn       func() (string, error)
	mu                 sync.RWMutex
}

type modVersionCacheEntry struct {
	files     []ModFileDTO
	timestamp time.Time
}

func (a *WailsAdapter) setInstallProgress(instanceID string, p *ModInstallProgressDTO) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.modInstallProgress == nil {
		a.modInstallProgress = make(map[string]*ModInstallProgressDTO)
	}
	a.modInstallProgress[instanceID] = p
}

func (a *WailsAdapter) SetJavaManager(jm *java.JavaManager) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.javaMgr = jm
}

func (a *WailsAdapter) SetInstalledModsRepo(repo *storage.InstalledModsRepository) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.installedModsRepo = repo
}

func (a *WailsAdapter) SetDB(db *sql.DB) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.db = db
	if db != nil {
		a.installedModsRepo = storage.NewInstalledModsRepositoryFromDB(db)
	}
}

func (a *WailsAdapter) SetMrPackImporter(importer *content.MrPackImporter) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mrpackImporter = importer
}

func (a *WailsAdapter) SetMrPackExporter(exporter *content.MrPackExporter) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mrpackExporter = exporter
}

func (a *WailsAdapter) SetFilePicker(fn func() (string, error)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.filePickerFn = fn
}

type wailsHTTPClientWrapper struct {
	client *http.Client
}

func (w *wailsHTTPClientWrapper) Get(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := w.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("http error %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func (w *wailsHTTPClientWrapper) DownloadFile(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := w.client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download error %d", resp.StatusCode)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	totalBytes := resp.ContentLength
	var bytesRead int64
	buf := make([]byte, 32*1024)
	for {
		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := f.Write(buf[:n]); wErr != nil {
				return wErr
			}
			bytesRead += int64(n)
			if onProgress != nil {
				onProgress(bytesRead, totalBytes)
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			return rErr
		}
	}
	return nil
}

func (a *WailsAdapter) getMrPackImporter() *content.MrPackImporter {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mrpackImporter == nil {
		var repo ports.InstanceRepository
		if a.svc != nil {
			repo = a.svc.Repo()
		}
		a.mrpackImporter = content.NewMrPackImporter(&wailsHTTPClientWrapper{client: a.httpClient}, repo, a.instancesDir)
	}
	return a.mrpackImporter
}

func (a *WailsAdapter) getMrPackExporter() *content.MrPackExporter {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.mrpackExporter == nil {
		var repo ports.InstanceRepository
		var cache ports.ContentCache
		if a.svc != nil {
			repo = a.svc.Repo()
		}
		if a.db != nil {
			cache = storage.NewContentCacheRepositoryFromDB(a.db)
		}
		exportDir := filepath.Join(filepath.Dir(a.instancesDir), "exports")
		a.mrpackExporter = content.NewMrPackExporter(repo, cache, a.instancesDir, exportDir)
	}
	return a.mrpackExporter
}

func NewWailsAdapter(svc *launch.InstanceService) *WailsAdapter {
	a := &WailsAdapter{
		svc:                svc,
		relauncher:         updater.DefaultRelauncher,
		lastCrashes:        make(map[string]*CrashReportDTO),
		modVersionsCache:   make(map[string]modVersionCacheEntry),
		modInstallProgress: make(map[string]*ModInstallProgressDTO),
	}
	if svc != nil {
		svc.SetOnCrash(func(instanceID string, report *launch.CrashReport) {
			a.RecordCrash(instanceID, report)
		})
	}
	return a
}

func (a *WailsAdapter) SetVersion(v string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.version = strings.TrimPrefix(v, "v")
}

func (a *WailsAdapter) GetCurrentVersion() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.version != "" {
		return a.version
	}
	if a.updater != nil {
		return a.updater.CurrentVersion()
	}
	return ""
}

func (a *WailsAdapter) SetUpdater(u *updater.AutoUpdater) {
	a.updater = u
}

func (a *WailsAdapter) SetRelauncher(fn updater.RelauncherFunc) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.relauncher = fn
}

func (a *WailsAdapter) SetJavaDetector(jd ports.JavaDetector) {
	a.javaDetector = jd
}

func (a *WailsAdapter) SetAuth(authSvc *auth.AuthService, accountRepo ports.AccountRepository) {
	a.authSvc = authSvc
	a.accountRepo = accountRepo
}

func (a *WailsAdapter) SetContent(mr *modrinth.Client, cf *curseforge.Client) {
	a.modrinth = mr
	a.curseforge = cf
}

func (a *WailsAdapter) SetFileSystem(fs ports.FileSystem, instancesDir string) {
	a.fileSys = fs
	a.instancesDir = instancesDir
}

func (a *WailsAdapter) SetSettings(repo *storage.SettingsRepository) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.settingsRepo = repo
}

func toInstanceDTO(inst *domain.Instance) InstanceDTO {
	jvmArgs := inst.JVMArgs
	if jvmArgs == nil {
		jvmArgs = []string{}
	}
	return InstanceDTO{
		ID:               inst.ID,
		Name:             inst.Name,
		GameVersion:      inst.GameVersion,
		Loader:           string(inst.Loader),
		LoaderVersion:    inst.LoaderVer,
		IconPath:         inst.IconPath,
		JavaPath:         inst.JavaPath,
		MinRAMMB:         inst.MinRAMMB,
		MaxRAMMB:         inst.MaxRAMMB,
		JVMArgs:          jvmArgs,
		SkipJavaCheck:    inst.SkipJavaCheck,
		State:            string(inst.State),
		LastPlayedAt:     inst.LastPlayedAt,
		TotalPlaySeconds: inst.TotalPlaySec,
	}
}

func (a *WailsAdapter) ListInstances() []InstanceDTO {
	coreList := a.svc.ListInstances()
	dtoList := make([]InstanceDTO, 0, len(coreList))
	for _, inst := range coreList {
		dtoList = append(dtoList, toInstanceDTO(inst))
	}
	return dtoList
}

func (a *WailsAdapter) CreateInstance(req CreateInstanceRequest) (*InstanceDTO, error) {
	inst, err := a.svc.CreateInstance(req.Name, req.GameVersion, domain.LoaderType(req.Loader))
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}
	if req.JavaPath != "" {
		if updated, err := a.svc.UpdateInstance(context.Background(), launch.UpdateInstanceParams{
			ID:       inst.ID,
			JavaPath: req.JavaPath,
		}); err == nil {
			inst = updated
		}
	}

	dto := toInstanceDTO(inst)
	return &dto, nil
}

func (a *WailsAdapter) UpdateInstance(req UpdateInstanceRequest) (*InstanceDTO, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("instance ID cannot be empty")
	}
	inst, err := a.svc.UpdateInstance(context.Background(), launch.UpdateInstanceParams{
		ID:            req.ID,
		Name:          req.Name,
		JavaPath:      req.JavaPath,
		ClearJavaPath: req.ClearJavaPath,
		MinRAMMB:      req.MinRAMMB,
		MaxRAMMB:      req.MaxRAMMB,
		JVMArgs:       req.JVMArgs,
		SkipJavaCheck: req.SkipJavaCheck,
	})
	if err != nil {
		return nil, fmt.Errorf("update instance: %w", err)
	}

	dto := toInstanceDTO(inst)
	return &dto, nil
}

func (a *WailsAdapter) LaunchInstance(id string) (*LaunchResponse, error) {
	pid, err := a.svc.Launch(context.Background(), id)
	if err != nil {
		return &LaunchResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &LaunchResponse{
		Success: true,
		PID:     pid,
	}, nil
}

func (a *WailsAdapter) GetLogTail(instanceID string, n int) ([]string, error) {
	if a.svc == nil {
		return []string{}, nil
	}
	return a.svc.GetLogTail(instanceID, n)
}

func (a *WailsAdapter) ListAccounts() ([]AccountDTO, error) {
	if a.accountRepo == nil {
		return []AccountDTO{}, nil
	}
	accs, err := a.accountRepo.ListAll(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}

	dtos := make([]AccountDTO, 0, len(accs))
	for _, acc := range accs {
		dtos = append(dtos, AccountDTO{
			UUID:     acc.UUID,
			Username: acc.Username,
			Type:     string(acc.Type),
			IsActive: acc.IsActive,
		})
	}
	return dtos, nil
}

func (a *WailsAdapter) SetActiveAccount(uuid string) error {
	if a.accountRepo == nil {
		return nil
	}
	return a.accountRepo.SetActive(context.Background(), uuid)
}

func (a *WailsAdapter) LoginOffline(username string) (*AccountDTO, error) {
	if a.authSvc == nil {
		return nil, fmt.Errorf("auth service not initialized")
	}
	acc, err := a.authSvc.CreateOfflineAccount(context.Background(), username)
	if err != nil {
		return nil, err
	}
	return &AccountDTO{
		UUID:     acc.UUID,
		Username: acc.Username,
		Type:     string(acc.Type),
		IsActive: acc.IsActive,
	}, nil
}

func (a *WailsAdapter) LoginMicrosoft() (*AccountDTO, error) {
	if a.authSvc == nil {
		return nil, fmt.Errorf("auth service not initialized")
	}
	acc, err := a.authSvc.StartInteractiveLogin(context.Background(), openBrowserCrossPlatform)
	if err != nil {
		return nil, err
	}
	return &AccountDTO{
		UUID:     acc.UUID,
		Username: acc.Username,
		Type:     string(acc.Type),
		IsActive: acc.IsActive,
	}, nil
}

func openBrowserCrossPlatform(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func (a *WailsAdapter) SearchMods(req SearchModsRequest) (*SearchModsResultDTO, error) {
	if req.Source == "curseforge" {
		if a.curseforge == nil {
			return nil, fmt.Errorf("curseforge client not initialized")
		}
		items, total, err := a.curseforge.SearchMods(context.Background(), req.Query, req.GameVersion, req.Loader, req.Limit, req.Offset, req.Sort, req.Category)
		if err != nil {
			var rateErr *curseforge.RateLimitError
			if errors.As(err, &rateErr) {
				wait := rateErr.RetryAfterSeconds
				if wait <= 0 {
					wait = 5
				}
				return &SearchModsResultDTO{
					Items:             []ModItemDTO{},
					TotalCount:        0,
					Reason:            "rate_limited",
					RetryAfterSeconds: wait,
				}, nil
			}
			if errors.Is(err, curseforge.ErrCurseForgeKeyInvalid) {
				return &SearchModsResultDTO{
					Items:             []ModItemDTO{},
					TotalCount:        0,
					Reason:            "key_invalid",
					RetryAfterSeconds: 0,
				}, nil
			}
			if errors.Is(err, curseforge.ErrCurseForgeRateLimited) {
				return &SearchModsResultDTO{
					Items:             []ModItemDTO{},
					TotalCount:        0,
					Reason:            "rate_limited",
					RetryAfterSeconds: 5,
				}, nil
			}
			var netErr net.Error
			if errors.As(err, &netErr) || strings.Contains(strings.ToLower(err.Error()), "dial") || strings.Contains(strings.ToLower(err.Error()), "connect") || strings.Contains(strings.ToLower(err.Error()), "no such host") {
				return &SearchModsResultDTO{
					Items:             []ModItemDTO{},
					TotalCount:        0,
					Reason:            "unreachable",
					RetryAfterSeconds: 0,
				}, nil
			}
			return nil, err
		}
		dtos := make([]ModItemDTO, 0, len(items))
		for _, it := range items {
			dtos = append(dtos, ModItemDTO{
				ID:         it.ID,
				Slug:       it.Slug,
				Source:     string(it.Source),
				Name:       it.Name,
				Author:     it.Author,
				Summary:    it.Summary,
				IconURL:    it.IconURL,
				Downloads:  it.Downloads,
				Categories: it.Categories,
			})
		}
		return &SearchModsResultDTO{
			Items:      dtos,
			TotalCount: total,
		}, nil
	}

	// Default to Modrinth
	if a.modrinth == nil {
		return nil, fmt.Errorf("modrinth client not initialized")
	}
	items, total, err := a.modrinth.SearchMods(context.Background(), req.Query, req.GameVersion, req.Loader, req.Limit, req.Offset, req.Sort, req.Category)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) || strings.Contains(strings.ToLower(err.Error()), "dial") || strings.Contains(strings.ToLower(err.Error()), "connect") || strings.Contains(strings.ToLower(err.Error()), "no such host") {
			return &SearchModsResultDTO{
				Items:             []ModItemDTO{},
				TotalCount:        0,
				Reason:            "unreachable",
				RetryAfterSeconds: 0,
			}, nil
		}
		return nil, err
	}
	dtos := make([]ModItemDTO, 0, len(items))
	for _, it := range items {
		dtos = append(dtos, ModItemDTO{
			ID:         it.ID,
			Slug:       it.Slug,
			Source:     string(it.Source),
			Name:       it.Name,
			Author:     it.Author,
			Summary:    it.Summary,
			IconURL:    it.IconURL,
			Downloads:  it.Downloads,
			Categories: it.Categories,
		})
	}
	return &SearchModsResultDTO{
		Items:      dtos,
		TotalCount: int64(total),
	}, nil
}

func (a *WailsAdapter) ListModVersions(req ListModVersionsRequest) ([]ModFileDTO, error) {
	if strings.TrimSpace(req.ModID) == "" {
		return nil, errors.New("mod_id is required")
	}

	gameVersion := req.GameVersion
	loader := req.Loader
	if (gameVersion == "" || loader == "") && req.InstanceID != "" && a.svc != nil {
		if inst, err := a.svc.GetInstance(req.InstanceID); err == nil && inst != nil {
			if gameVersion == "" {
				gameVersion = inst.GameVersion
			}
			if loader == "" {
				loader = string(inst.Loader)
			}
		}
	}

	source := strings.ToLower(strings.TrimSpace(req.Source))
	if source == "" {
		source = "modrinth"
	}

	cacheKey := fmt.Sprintf("%s|%s|%s|%s", source, req.ModID, gameVersion, loader)
	a.mu.RLock()
	if entry, ok := a.modVersionsCache[cacheKey]; ok {
		if time.Since(entry.timestamp) < 10*time.Minute {
			a.mu.RUnlock()
			return entry.files, nil
		}
	}
	a.mu.RUnlock()

	var result []ModFileDTO
	ctx := context.Background()

	switch source {
	case "curseforge":
		a.mu.RLock()
		cf := a.curseforge
		a.mu.RUnlock()
		if cf == nil {
			return nil, errors.New("curseforge client not initialized")
		}

		cfModID, err := strconv.ParseInt(req.ModID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid curseforge mod id: %w", err)
		}

		files, err := cf.GetModFiles(ctx, cfModID, gameVersion, loader)
		if (err != nil || len(files) == 0) && (gameVersion != "" || loader != "") {
			if fFallback, err2 := cf.GetModFiles(ctx, cfModID, "", ""); err2 == nil && len(fFallback) > 0 {
				files = fFallback
				err = nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("fetch curseforge files: %w", err)
		}

		for _, f := range files {
			relType := f.ReleaseType
			if relType == "" {
				relType = "release"
			}
			fDate := ""
			if !f.FileDate.IsZero() {
				fDate = f.FileDate.Format(time.RFC3339)
			}
			var deps []string
			for _, d := range f.Dependencies {
				if d.ProjectID != "" {
					deps = append(deps, d.ProjectID)
				}
			}
			result = append(result, ModFileDTO{
				ID:           f.ID,
				ModID:        req.ModID,
				FileName:     f.FileName,
				DisplayName:  f.FileName,
				ReleaseType:  relType,
				FileSize:     f.Size,
				FileDate:     fDate,
				GameVersions: f.GameVersions,
				Loaders:      f.Loaders,
				DownloadURL:  f.URL,
				Dependencies: deps,
			})
		}

	case "modrinth":
		a.mu.RLock()
		mr := a.modrinth
		a.mu.RUnlock()
		if mr == nil {
			return nil, errors.New("modrinth client not initialized")
		}

		versions, err := mr.GetProjectVersions(ctx, req.ModID, gameVersion, loader)
		if (err != nil || len(versions) == 0) && (gameVersion != "" || loader != "") {
			if vFallback, err2 := mr.GetProjectVersions(ctx, req.ModID, "", ""); err2 == nil && len(vFallback) > 0 {
				versions = vFallback
				err = nil
			}
		}
		if err != nil {
			return nil, fmt.Errorf("fetch modrinth versions: %w", err)
		}

		for _, v := range versions {
			relType := v.VersionType
			if relType == "" {
				relType = "release"
			}
			dateStr := ""
			if !v.ReleaseDate.IsZero() {
				dateStr = v.ReleaseDate.Format(time.RFC3339)
			}
			for _, f := range v.Files {
				dispName := f.FileName
				if v.Name != "" && v.Name != f.FileName {
					dispName = fmt.Sprintf("%s (%s)", v.Name, f.FileName)
				}
				var deps []string
				for _, d := range f.Dependencies {
					if d.ProjectID != "" {
						deps = append(deps, d.ProjectID)
					}
				}
				result = append(result, ModFileDTO{
					ID:           f.ID,
					ModID:        req.ModID,
					FileName:     f.FileName,
					DisplayName:  dispName,
					ReleaseType:  relType,
					FileSize:     f.Size,
					FileDate:     dateStr,
					GameVersions: v.GameVersions,
					Loaders:      v.Loaders,
					DownloadURL:  f.URL,
					Changelog:    v.Changelog,
					Dependencies: deps,
				})
			}
		}

	default:
		return nil, fmt.Errorf("unsupported mod source: %s", req.Source)
	}

	a.mu.Lock()
	if a.modVersionsCache == nil {
		a.modVersionsCache = make(map[string]modVersionCacheEntry)
	}
	a.modVersionsCache[cacheKey] = modVersionCacheEntry{
		files:     result,
		timestamp: time.Now(),
	}
	a.mu.Unlock()

	return result, nil
}

func (a *WailsAdapter) GetModInstallStatus(instanceID string) (*ModInstallProgressDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.modInstallProgress != nil {
		if p, ok := a.modInstallProgress[instanceID]; ok && p != nil {
			copy := *p
			return &copy, nil
		}
	}
	return &ModInstallProgressDTO{
		Status:     "idle",
		InstanceID: instanceID,
	}, nil
}

func (a *WailsAdapter) ListInstalledMods(instanceID string) ([]InstalledModDTO, error) {
	modsDir := a.getModsDir(instanceID)
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []InstalledModDTO{}, nil
		}
		return nil, fmt.Errorf("read mods dir: %w", err)
	}

	// Reconcile manifest with disk: filesystem is source of truth
	m, err := manifest.LoadManifest(modsDir)
	if err != nil {
		m = manifest.NewManifest()
	}
	recRes, _ := m.ReconcileWithDisk(modsDir)
	if recRes != nil && recRes.Changed {
		_ = m.Save(modsDir) // errcheck:ok best effort manifest save on reconcile
		if updatedEntries, err := os.ReadDir(modsDir); err == nil {
			entries = updatedEntries
		}
	}

	var res []InstalledModDTO
	var activeFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".jar") || strings.HasSuffix(name, ".jar.disabled") {
			info, err := e.Info()
			size := int64(0)
			if err == nil {
				size = info.Size()
			}
			enabled := strings.HasSuffix(name, ".jar")
			cleanKey := manifest.CleanModKey(name)
			cleanDisplayName := strings.TrimSuffix(strings.TrimSuffix(name, ".disabled"), ".jar")

			item := InstalledModDTO{
				FileName:  name,
				Name:      cleanDisplayName,
				Enabled:   enabled,
				SizeBytes: size,
				Source:    "local",
			}

			if rec := m.GetRecord(cleanKey); rec != nil {
				if rec.ModName != "" {
					item.Name = rec.ModName
				}
				item.ModID = rec.ModID
				item.Version = rec.VersionID
				if rec.Source != "" {
					item.Source = rec.Source
				}
				item.ReleaseType = rec.ReleaseType
			}

			res = append(res, item)
			activeFiles = append(activeFiles, name)
		}
	}

	// Sync with SQLite cache if repo configured
	a.mu.RLock()
	repo := a.installedModsRepo
	a.mu.RUnlock()
	if repo != nil {
		for _, item := range res {
			_ = repo.Save(context.Background(), storage.InstalledModRecord{ // errcheck:ok best effort cache update
				InstanceID:  instanceID,
				ModID:       item.ModID,
				FileName:    item.FileName,
				Source:      item.Source,
				VersionID:   item.Version,
				ReleaseType: item.ReleaseType,
				InstalledAt: time.Now(),
			})
		}
		_ = repo.SyncInstance(context.Background(), instanceID, activeFiles) // errcheck:ok best effort cache sync
	}

	return res, nil
}

func (a *WailsAdapter) ToggleMod(req ToggleModRequest) error {
	modsDir := a.getModsDir(req.InstanceID)
	oldPath := filepath.Join(modsDir, req.FileName)

	var newName string
	if req.Enable && strings.HasSuffix(req.FileName, ".disabled") {
		newName = strings.TrimSuffix(req.FileName, ".disabled")
	} else if !req.Enable && !strings.HasSuffix(req.FileName, ".disabled") {
		newName = req.FileName + ".disabled"
	} else {
		return nil // already in desired state
	}

	newPath := filepath.Join(modsDir, newName)
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}

	// Update manifest
	m, err := manifest.LoadManifest(modsDir)
	if err == nil {
		cleanKey := manifest.CleanModKey(req.FileName)
		if rec := m.GetRecord(cleanKey); rec != nil {
			rec.FileName = newName
			m.AddOrUpdate(rec)
			_ = m.Save(modsDir) // errcheck:ok best effort manifest save on toggle
		}
	}

	// Update SQLite
	a.mu.RLock()
	repo := a.installedModsRepo
	a.mu.RUnlock()
	if repo != nil {
		_ = repo.UpdateFileName(context.Background(), req.InstanceID, req.FileName, newName) // errcheck:ok best effort cache update
	}

	return nil
}

func (a *WailsAdapter) DeleteMod(req DeleteModRequest) error {
	modsDir := a.getModsDir(req.InstanceID)
	cleanKey := manifest.CleanModKey(req.FileName)
	rawClean := strings.TrimSuffix(strings.TrimSuffix(req.FileName, ".disabled"), ".jar")

	jarPath := filepath.Join(modsDir, rawClean+".jar")
	disabledPath := filepath.Join(modsDir, rawClean+".jar.disabled")
	targetPath := filepath.Join(modsDir, req.FileName)

	// A2: Remove both candidate filenames (X.jar and X.jar.disabled) as well as explicit target
	_ = os.Remove(jarPath)      // errcheck:ok best effort candidate removal
	_ = os.Remove(disabledPath) // errcheck:ok best effort candidate removal
	_ = os.Remove(targetPath)   // errcheck:ok best effort target removal

	// Remove from manifest
	m, err := manifest.LoadManifest(modsDir)
	if err == nil {
		m.Remove(cleanKey)
		_ = m.Save(modsDir) // errcheck:ok best effort manifest save on delete
	}

	// Remove from SQLite
	a.mu.RLock()
	repo := a.installedModsRepo
	a.mu.RUnlock()
	if repo != nil {
		_ = repo.DeleteByCleanName(context.Background(), req.InstanceID, rawClean) // errcheck:ok best effort cache delete
	}

	return nil
}

func (a *WailsAdapter) RecordCrash(instanceID string, report *launch.CrashReport) {
	if report == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastCrashes[instanceID] = &CrashReportDTO{
		Category:      string(report.Category),
		Summary:       report.Summary,
		Remedy:        report.Remedy,
		Details:       report.Details,
		RelevantLines: report.RelevantLines,
		ExitCode:      report.ExitCode,
	}
}

func (a *WailsAdapter) GetLastCrashReport(instanceID string) *CrashReportDTO {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.lastCrashes[instanceID]
}

func (a *WailsAdapter) getModsDir(instanceID string) string {
	if a.instancesDir != "" {
		return filepath.Join(a.instancesDir, instanceID, "mods")
	}
	return filepath.Join("instances", instanceID, "mods")
}

func (a *WailsAdapter) CheckForUpdates() (*UpdateInfoDTO, error) {
	if a.updater == nil {
		return nil, fmt.Errorf("auto-updater not initialized")
	}
	info, err := a.updater.CheckForUpdates(context.Background())
	if err != nil {
		return nil, err
	}
	return &UpdateInfoDTO{
		HasUpdate:      info.Available,
		Version:        info.Version,
		CurrentVersion: info.CurrentVer,
		ReleaseDate:    info.ReleaseDate,
		ReleaseNotes:   info.Changelog,
		DownloadURL:    info.Asset.URL,
		SHA256:         info.Asset.SHA256,
		Size:           info.Asset.Size,
	}, nil
}

func (a *WailsAdapter) ApplyUpdate() (*UpdateApplyResultDTO, error) {
	if a.updater == nil {
		return nil, fmt.Errorf("auto-updater not initialized")
	}
	info, err := a.updater.CheckForUpdates(context.Background())
	if err != nil {
		return nil, fmt.Errorf("check update before apply: %w", err)
	}
	if !info.Available {
		return &UpdateApplyResultDTO{
			Success: false,
			Message: "No update available",
		}, nil
	}
	if err := a.updater.ApplyUpdate(context.Background(), info); err != nil {
		return &UpdateApplyResultDTO{
			Success: false,
			Message: err.Error(),
		}, nil
	}
	return &UpdateApplyResultDTO{
		Success:         true,
		Message:         fmt.Sprintf("Update %s applied successfully. Restart required for changes to take effect.", info.Version),
		RestartRequired: true,
	}, nil
}

func (a *WailsAdapter) RestartApplication() error {
	a.mu.RLock()
	fn := a.relauncher
	a.mu.RUnlock()
	if fn != nil {
		return fn()
	}
	return updater.Relaunch()
}

func (a *WailsAdapter) SetHTTPClient(client *http.Client) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.httpClient = client
}

func (a *WailsAdapter) getHTTPClient() *http.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.httpClient != nil {
		return a.httpClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (a *WailsAdapter) SetAllowedHosts(hosts []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowedHosts = hosts
}

func (a *WailsAdapter) isAllowedDownloadHost(rawHost string) bool {
	h := rawHost
	if strings.Contains(h, ":") {
		if hostOnly, _, err := net.SplitHostPort(rawHost); err == nil {
			h = hostOnly
		}
	}
	h = strings.ToLower(strings.TrimSpace(h))
	if h == "" {
		return false
	}

	if h == "cdn.modrinth.com" || h == "edge.forgecdn.net" {
		return true
	}
	if strings.HasSuffix(h, ".forgecdn.net") {
		return true
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, allowed := range a.allowedHosts {
		if strings.EqualFold(h, allowed) {
			return true
		}
	}
	return false
}

func (a *WailsAdapter) InstallMod(req InstallModRequest) (*InstallModResponse, error) {
	if strings.TrimSpace(req.InstanceID) == "" {
		return nil, errors.New("instance_id is required")
	}
	if strings.TrimSpace(req.ModID) == "" {
		return nil, errors.New("mod_id is required")
	}

	a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
		TaskID:     fmt.Sprintf("install-%s", req.ModID),
		InstanceID: req.InstanceID,
		ModID:      req.ModID,
		Status:     "resolving_dependencies",
		Percentage: 10,
	})

	var installErr error
	defer func() {
		if installErr != nil {
			a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
				TaskID:     fmt.Sprintf("install-%s", req.ModID),
				InstanceID: req.InstanceID,
				ModID:      req.ModID,
				Status:     "failed",
				Error:      installErr.Error(),
			})
		}
	}()

	source := strings.ToLower(strings.TrimSpace(req.Source))
	if source == "" {
		source = "modrinth"
	}

	var gameVersion, loader string
	if a.svc != nil {
		if inst, err := a.svc.GetInstance(req.InstanceID); err == nil && inst != nil {
			gameVersion = inst.GameVersion
			loader = string(inst.Loader)
		}
	}
	if req.GameVersion != "" {
		gameVersion = req.GameVersion
	}
	if req.Loader != "" {
		loader = req.Loader
	}

	var fileToDownload *content.ModFile

	switch source {
	case "modrinth":
		a.mu.RLock()
		mr := a.modrinth
		a.mu.RUnlock()
		if mr == nil {
			installErr = errors.New("modrinth client not initialized")
			return nil, installErr
		}

		ctx := context.Background()
		versions, err := mr.GetProjectVersions(ctx, req.ModID, gameVersion, loader)
		if (err != nil || len(versions) == 0) && (gameVersion != "" || loader != "") {
			if vFallback, err2 := mr.GetProjectVersions(ctx, req.ModID, "", ""); err2 == nil && len(vFallback) > 0 {
				versions = vFallback
				err = nil
			}
		}
		if err != nil {
			installErr = fmt.Errorf("fetch modrinth versions: %w", err)
			return nil, installErr
		}
		if len(versions) == 0 {
			installErr = errors.New("no compatible versions found for mod")
			return nil, installErr
		}

		var selectedVersion *content.ModVersion
		if req.VersionID != "" {
			for i := range versions {
				v := &versions[i]
				if v.ID == req.VersionID {
					selectedVersion = v
					for j := range v.Files {
						if v.Files[j].Primary {
							fileToDownload = &v.Files[j]
							break
						}
					}
					if fileToDownload == nil && len(v.Files) > 0 {
						fileToDownload = &v.Files[0]
					}
					break
				}
				for j := range v.Files {
					if v.Files[j].ID == req.VersionID {
						selectedVersion = v
						fileToDownload = &v.Files[j]
						break
					}
				}
				if fileToDownload != nil {
					break
				}
			}
		}

		if fileToDownload == nil {
			selRes, err := content.SelectBestModVersion(versions, gameVersion, loader)
			if err != nil {
				installErr = err
				return nil, installErr
			}
			selectedVersion = selRes.Version
			fileToDownload = selRes.File
		}

		// Dependency resolution: recursively install required dependencies (DepRequired)
		if selectedVersion != nil && len(selectedVersion.Dependencies) > 0 {
			modsDir := a.getModsDir(req.InstanceID)
			for _, dep := range selectedVersion.Dependencies {
				if dep.Type == content.DepRequired && dep.ProjectID != "" {
					depInstalled := false
					if existing, err := os.ReadDir(modsDir); err == nil {
						for _, entry := range existing {
							nameLower := strings.ToLower(entry.Name())
							if strings.Contains(nameLower, strings.ToLower(dep.ProjectID)) {
								depInstalled = true
								break
							}
						}
					}
					if !depInstalled {
						_, _ = a.InstallMod(InstallModRequest{ // errcheck:ok best effort dependency installation
							InstanceID:  req.InstanceID,
							ModID:       dep.ProjectID,
							Source:      "modrinth",
							GameVersion: gameVersion,
							Loader:      loader,
						})
					}
				}
			}
		}

	case "curseforge":
		a.mu.RLock()
		cf := a.curseforge
		a.mu.RUnlock()
		if cf == nil {
			installErr = errors.New("curseforge client not initialized")
			return nil, installErr
		}

		cfModID, err := strconv.ParseInt(req.ModID, 10, 64)
		if err != nil {
			installErr = fmt.Errorf("invalid curseforge mod id: %w", err)
			return nil, installErr
		}

		ctx := context.Background()
		files, err := cf.GetModFiles(ctx, cfModID, gameVersion, loader)
		if (err != nil || len(files) == 0) && (gameVersion != "" || loader != "") {
			if fFallback, err2 := cf.GetModFiles(ctx, cfModID, "", ""); err2 == nil && len(fFallback) > 0 {
				files = fFallback
				err = nil
			}
		}
		if err != nil {
			installErr = fmt.Errorf("fetch curseforge files: %w", err)
			return nil, installErr
		}
		if len(files) == 0 {
			installErr = errors.New("no compatible files found for mod")
			return nil, installErr
		}

		if req.VersionID != "" {
			for i := range files {
				if files[i].ID == req.VersionID {
					fileToDownload = &files[i]
					break
				}
			}
		}

		if fileToDownload == nil {
			selRes, err := content.SelectBestModFile(files, gameVersion, loader)
			if err != nil {
				installErr = err
				return nil, installErr
			}
			fileToDownload = selRes.File
		}

	default:
		installErr = fmt.Errorf("unsupported mod source: %s", req.Source)
		return nil, installErr
	}

	if fileToDownload == nil || fileToDownload.URL == "" {
		installErr = errors.New("mod file download URL is missing")
		return nil, installErr
	}

	parsedURL, err := url.Parse(fileToDownload.URL)
	if err != nil {
		installErr = fmt.Errorf("invalid download URL: %w", err)
		return nil, installErr
	}
	if !a.isAllowedDownloadHost(parsedURL.Host) {
		installErr = fmt.Errorf("download host not allowed: %s", parsedURL.Host)
		return nil, installErr
	}

	modsDir := a.getModsDir(req.InstanceID)
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		installErr = fmt.Errorf("create mods directory: %w", err)
		return nil, installErr
	}

	fileName := filepath.Base(fileToDownload.FileName)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = filepath.Base(fileToDownload.URL)
	}
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = fmt.Sprintf("%s.jar", req.ModID)
	}
	if !strings.HasSuffix(fileName, ".jar") {
		fileName = fileName + ".jar"
	}

	destPath := filepath.Join(modsDir, fileName)

	// Idempotency: if mod file already exists, return success
	if _, err := os.Stat(destPath); err == nil {
		a.recordInstalledMod(req.InstanceID, req.ModID, fileName, source, fileToDownload, req.VersionID)
		a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
			TaskID:     fmt.Sprintf("install-%s", req.ModID),
			InstanceID: req.InstanceID,
			ModID:      req.ModID,
			FileName:   fileName,
			Status:     "completed",
			Percentage: 100,
		})
		return &InstallModResponse{
			Success:  true,
			FileName: fileName,
			Message:  "Mod already installed",
		}, nil
	}

	a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
		TaskID:     fmt.Sprintf("install-%s", req.ModID),
		InstanceID: req.InstanceID,
		ModID:      req.ModID,
		FileName:   fileName,
		Status:     "downloading",
		Percentage: 50,
	})

	tempFile, err := os.CreateTemp(modsDir, ".tmp-*.jar")
	if err != nil {
		installErr = fmt.Errorf("create temporary download file: %w", err)
		return nil, installErr
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close() // errcheck:ok best effort close if still open
		if _, statErr := os.Stat(tempPath); statErr == nil {
			_ = os.Remove(tempPath) // errcheck:ok best effort cleanup of temporary file
		}
	}()

	baseClient := a.getHTTPClient()
	timeout := 60 * time.Second
	if baseClient.Timeout > 0 {
		timeout = baseClient.Timeout
	}
	downloadClient := &http.Client{
		Timeout:   timeout,
		Transport: baseClient.Transport,
		CheckRedirect: func(redirectReq *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if !a.isAllowedDownloadHost(redirectReq.URL.Host) {
				return fmt.Errorf("redirect to non-allowlisted host rejected: %s", redirectReq.URL.Host)
			}
			return nil
		},
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fileToDownload.URL, nil)
	if err != nil {
		installErr = fmt.Errorf("create download request: %w", err)
		return nil, installErr
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", netutil.FormatUserAgent(a.GetCurrentVersion()))
	}
	if httpReq.Header.Get("Accept") == "" {
		httpReq.Header.Set("Accept", "*/*")
	}
	if req.Source == "modrinth" {
		metaObj := map[string]string{
			"reason":       "standalone",
			"game_version": gameVersion,
			"loader":       loader,
		}
		if metaBytes, err := json.Marshal(metaObj); err == nil {
			httpReq.Header.Set("modrinth-download-meta", string(metaBytes))
		}
	}

	resp, err := downloadClient.Do(httpReq)
	if err != nil {
		installErr = fmt.Errorf("download mod file: %w", err)
		return nil, installErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		installErr = fmt.Errorf("download returned HTTP %d", resp.StatusCode)
		return nil, installErr
	}

	a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
		TaskID:     fmt.Sprintf("install-%s", req.ModID),
		InstanceID: req.InstanceID,
		ModID:      req.ModID,
		FileName:   fileName,
		Status:     "verifying",
		Percentage: 90,
	})

	hSha1 := sha1.New()
	hSha512 := sha512.New()
	mw := io.MultiWriter(tempFile, hSha1, hSha512)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		installErr = fmt.Errorf("write mod file: %w", err)
		return nil, installErr
	}

	if err := tempFile.Close(); err != nil {
		installErr = fmt.Errorf("close temp file: %w", err)
		return nil, installErr
	}

	// Verify checksums
	if fileToDownload.SHA512 != "" {
		actualSha512 := hex.EncodeToString(hSha512.Sum(nil))
		if !strings.EqualFold(actualSha512, fileToDownload.SHA512) {
			installErr = fmt.Errorf("sha512 mismatch: expected %s, got %s", fileToDownload.SHA512, actualSha512)
			return nil, installErr
		}
	} else if fileToDownload.SHA1 != "" {
		actualSha1 := hex.EncodeToString(hSha1.Sum(nil))
		if !strings.EqualFold(actualSha1, fileToDownload.SHA1) {
			installErr = fmt.Errorf("sha1 mismatch: expected %s, got %s", fileToDownload.SHA1, actualSha1)
			return nil, installErr
		}
	}

	// Atomic rename
	if err := os.Rename(tempPath, destPath); err != nil {
		installErr = fmt.Errorf("finalize mod install: %w", err)
		return nil, installErr
	}

	a.recordInstalledMod(req.InstanceID, req.ModID, fileName, source, fileToDownload, req.VersionID)

	a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
		TaskID:     fmt.Sprintf("install-%s", req.ModID),
		InstanceID: req.InstanceID,
		ModID:      req.ModID,
		FileName:   fileName,
		Status:     "completed",
		Percentage: 100,
	})

	return &InstallModResponse{
		Success:  true,
		FileName: fileName,
		Message:  fmt.Sprintf("Mod %s installed successfully", fileName),
	}, nil
}

func (a *WailsAdapter) UpdateMod(req UpdateModRequest) (*InstallModResponse, error) {
	if strings.TrimSpace(req.InstanceID) == "" {
		return nil, errors.New("instance_id is required")
	}
	if strings.TrimSpace(req.ModID) == "" {
		return nil, errors.New("mod_id is required")
	}
	if strings.TrimSpace(req.OldFileName) == "" {
		return nil, errors.New("old_file_name is required")
	}

	a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
		TaskID:     fmt.Sprintf("update-%s", req.ModID),
		InstanceID: req.InstanceID,
		ModID:      req.ModID,
		Status:     "resolving_dependencies",
		Percentage: 10,
	})

	var updateErr error
	defer func() {
		if updateErr != nil {
			a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
				TaskID:     fmt.Sprintf("update-%s", req.ModID),
				InstanceID: req.InstanceID,
				ModID:      req.ModID,
				Status:     "failed",
				Error:      updateErr.Error(),
			})
		}
	}()

	source := strings.ToLower(strings.TrimSpace(req.Source))
	if source == "" {
		source = "modrinth"
	}

	var gameVersion, loader string
	if a.svc != nil {
		if inst, err := a.svc.GetInstance(req.InstanceID); err == nil && inst != nil {
			gameVersion = inst.GameVersion
			loader = string(inst.Loader)
		}
	}
	if req.GameVersion != "" {
		gameVersion = req.GameVersion
	}
	if req.Loader != "" {
		loader = req.Loader
	}

	var fileToDownload *content.ModFile

	switch source {
	case "modrinth":
		a.mu.RLock()
		mr := a.modrinth
		a.mu.RUnlock()
		if mr == nil {
			updateErr = errors.New("modrinth client not initialized")
			return nil, updateErr
		}

		ctx := context.Background()
		versions, err := mr.GetProjectVersions(ctx, req.ModID, gameVersion, loader)
		if (err != nil || len(versions) == 0) && (gameVersion != "" || loader != "") {
			if vFallback, err2 := mr.GetProjectVersions(ctx, req.ModID, "", ""); err2 == nil && len(vFallback) > 0 {
				versions = vFallback
				err = nil
			}
		}
		if err != nil {
			updateErr = fmt.Errorf("fetch modrinth versions: %w", err)
			return nil, updateErr
		}
		if len(versions) == 0 {
			updateErr = errors.New("no compatible versions found for mod")
			return nil, updateErr
		}

		if req.TargetVersionID != "" {
			for i := range versions {
				v := &versions[i]
				if v.ID == req.TargetVersionID {
					for j := range v.Files {
						if v.Files[j].Primary {
							fileToDownload = &v.Files[j]
							break
						}
					}
					if fileToDownload == nil && len(v.Files) > 0 {
						fileToDownload = &v.Files[0]
					}
					break
				}
				for j := range v.Files {
					if v.Files[j].ID == req.TargetVersionID {
						fileToDownload = &v.Files[j]
						break
					}
				}
				if fileToDownload != nil {
					break
				}
			}
		}

		if fileToDownload == nil {
			selRes, err := content.SelectBestModVersion(versions, gameVersion, loader)
			if err != nil {
				updateErr = err
				return nil, updateErr
			}
			fileToDownload = selRes.File
		}

	case "curseforge":
		a.mu.RLock()
		cf := a.curseforge
		a.mu.RUnlock()
		if cf == nil {
			updateErr = errors.New("curseforge client not initialized")
			return nil, updateErr
		}

		cfModID, err := strconv.ParseInt(req.ModID, 10, 64)
		if err != nil {
			updateErr = fmt.Errorf("invalid curseforge mod id: %w", err)
			return nil, updateErr
		}

		ctx := context.Background()
		files, err := cf.GetModFiles(ctx, cfModID, gameVersion, loader)
		if (err != nil || len(files) == 0) && (gameVersion != "" || loader != "") {
			if fFallback, err2 := cf.GetModFiles(ctx, cfModID, "", ""); err2 == nil && len(fFallback) > 0 {
				files = fFallback
				err = nil
			}
		}
		if err != nil {
			updateErr = fmt.Errorf("fetch curseforge files: %w", err)
			return nil, updateErr
		}
		if len(files) == 0 {
			updateErr = errors.New("no compatible files found for mod")
			return nil, updateErr
		}

		if req.TargetVersionID != "" {
			for i := range files {
				if files[i].ID == req.TargetVersionID {
					fileToDownload = &files[i]
					break
				}
			}
		}

		if fileToDownload == nil {
			selRes, err := content.SelectBestModFile(files, gameVersion, loader)
			if err != nil {
				updateErr = err
				return nil, updateErr
			}
			fileToDownload = selRes.File
		}

	default:
		updateErr = fmt.Errorf("unsupported mod source: %s", req.Source)
		return nil, updateErr
	}

	if fileToDownload == nil || fileToDownload.URL == "" {
		updateErr = errors.New("mod file download URL is missing")
		return nil, updateErr
	}

	parsedURL, err := url.Parse(fileToDownload.URL)
	if err != nil {
		updateErr = fmt.Errorf("invalid download URL: %w", err)
		return nil, updateErr
	}
	if !a.isAllowedDownloadHost(parsedURL.Host) {
		updateErr = fmt.Errorf("download host not allowed: %s", parsedURL.Host)
		return nil, updateErr
	}

	modsDir := a.getModsDir(req.InstanceID)
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		updateErr = fmt.Errorf("create mods directory: %w", err)
		return nil, updateErr
	}

	fileName := filepath.Base(fileToDownload.FileName)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = filepath.Base(fileToDownload.URL)
	}
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = fmt.Sprintf("%s.jar", req.ModID)
	}
	if !strings.HasSuffix(fileName, ".jar") {
		fileName = fileName + ".jar"
	}

	destPath := filepath.Join(modsDir, fileName)

	// Idempotency check: if destination file already exists and matches requested version
	if fileName == req.OldFileName {
		if _, err := os.Stat(destPath); err == nil {
			a.recordInstalledMod(req.InstanceID, req.ModID, fileName, source, fileToDownload, req.TargetVersionID)
			a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
				TaskID:     fmt.Sprintf("update-%s", req.ModID),
				InstanceID: req.InstanceID,
				ModID:      req.ModID,
				FileName:   fileName,
				Status:     "completed",
				Percentage: 100,
			})
			return &InstallModResponse{
				Success:  true,
				FileName: fileName,
				Message:  "Mod already up to date",
			}, nil
		}
	}

	a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
		TaskID:     fmt.Sprintf("update-%s", req.ModID),
		InstanceID: req.InstanceID,
		ModID:      req.ModID,
		FileName:   fileName,
		Status:     "downloading",
		Percentage: 50,
	})

	tempFile, err := os.CreateTemp(modsDir, ".tmp-update-*.jar")
	if err != nil {
		updateErr = fmt.Errorf("create temporary download file: %w", err)
		return nil, updateErr
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close() // errcheck:ok safe to ignore close error
		if _, statErr := os.Stat(tempPath); statErr == nil {
			_ = os.Remove(tempPath) // errcheck:ok cleanup temp file on failure
		}
	}()

	baseClient := a.getHTTPClient()
	timeout := 60 * time.Second
	if baseClient.Timeout > 0 {
		timeout = baseClient.Timeout
	}
	downloadClient := &http.Client{
		Timeout:   timeout,
		Transport: baseClient.Transport,
		CheckRedirect: func(redirectReq *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			if !a.isAllowedDownloadHost(redirectReq.URL.Host) {
				return fmt.Errorf("redirect to non-allowlisted host rejected: %s", redirectReq.URL.Host)
			}
			return nil
		},
	}

	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fileToDownload.URL, nil)
	if err != nil {
		updateErr = fmt.Errorf("create download request: %w", err)
		return nil, updateErr
	}
	if httpReq.Header.Get("User-Agent") == "" {
		httpReq.Header.Set("User-Agent", netutil.FormatUserAgent(a.GetCurrentVersion()))
	}
	if httpReq.Header.Get("Accept") == "" {
		httpReq.Header.Set("Accept", "*/*")
	}
	if req.Source == "modrinth" {
		metaObj := map[string]string{
			"reason":       "standalone",
			"game_version": gameVersion,
			"loader":       loader,
		}
		if metaBytes, err := json.Marshal(metaObj); err == nil {
			httpReq.Header.Set("modrinth-download-meta", string(metaBytes))
		}
	}

	resp, err := downloadClient.Do(httpReq)
	if err != nil {
		updateErr = fmt.Errorf("download mod file: %w", err)
		return nil, updateErr
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		updateErr = fmt.Errorf("download returned HTTP %d", resp.StatusCode)
		return nil, updateErr
	}

	a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
		TaskID:     fmt.Sprintf("update-%s", req.ModID),
		InstanceID: req.InstanceID,
		ModID:      req.ModID,
		FileName:   fileName,
		Status:     "verifying",
		Percentage: 90,
	})

	hSha1 := sha1.New()
	hSha512 := sha512.New()
	mw := io.MultiWriter(tempFile, hSha1, hSha512)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		updateErr = fmt.Errorf("write mod file: %w", err)
		return nil, updateErr
	}

	if err := tempFile.Close(); err != nil {
		updateErr = fmt.Errorf("close temp file: %w", err)
		return nil, updateErr
	}

	// Checksum validation
	if fileToDownload.SHA512 != "" {
		actualSha512 := hex.EncodeToString(hSha512.Sum(nil))
		if !strings.EqualFold(actualSha512, fileToDownload.SHA512) {
			updateErr = fmt.Errorf("sha512 mismatch: expected %s, got %s", fileToDownload.SHA512, actualSha512)
			return nil, updateErr
		}
	} else if fileToDownload.SHA1 != "" {
		actualSha1 := hex.EncodeToString(hSha1.Sum(nil))
		if !strings.EqualFold(actualSha1, fileToDownload.SHA1) {
			updateErr = fmt.Errorf("sha1 mismatch: expected %s, got %s", fileToDownload.SHA1, actualSha1)
			return nil, updateErr
		}
	}

	// Atomic replacement: remove old mod file from disk if different
	if fileName != req.OldFileName {
		oldPath := filepath.Join(modsDir, req.OldFileName)
		_ = os.Remove(oldPath) // errcheck:ok best effort removal of old active file
		rawClean := strings.TrimSuffix(strings.TrimSuffix(req.OldFileName, ".disabled"), ".jar")
		_ = os.Remove(filepath.Join(modsDir, rawClean+".jar.disabled")) // errcheck:ok
	}

	// On Windows, remove destination file if it already exists before renaming
	if _, err := os.Stat(destPath); err == nil {
		_ = os.Remove(destPath) // errcheck:ok
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		updateErr = fmt.Errorf("finalize mod update: %w", err)
		return nil, updateErr
	}

	// Update manifest
	m, err := manifest.LoadManifest(modsDir)
	if err == nil {
		if fileName != req.OldFileName {
			m.Remove(req.OldFileName)
			rawClean := strings.TrimSuffix(strings.TrimSuffix(req.OldFileName, ".disabled"), ".jar")
			m.Remove(rawClean)
		}
		versionStr := req.TargetVersionID
		releaseTypeStr := ""
		modTitle := req.ModID
		if fileToDownload != nil {
			if versionStr == "" {
				versionStr = fileToDownload.ID
			}
			releaseTypeStr = fileToDownload.ReleaseType
			if fileToDownload.FileName != "" {
				modTitle = fileToDownload.FileName
			}
		}
		m.AddOrUpdate(&manifest.ModRecord{
			ModID:       req.ModID,
			ModName:     modTitle,
			FileName:    fileName,
			Source:      source,
			VersionID:   versionStr,
			ReleaseType: releaseTypeStr,
			InstalledAt: time.Now(),
		})

		// Reconcile remaining duplicates on disk
		recRes, _ := m.ReconcileWithDisk(modsDir)
		_ = m.Save(modsDir) // errcheck:ok best effort manifest save on update

		// Update SQLite
		a.mu.RLock()
		repo := a.installedModsRepo
		a.mu.RUnlock()
		if repo != nil {
			if fileName != req.OldFileName {
				rawClean := strings.TrimSuffix(strings.TrimSuffix(req.OldFileName, ".disabled"), ".jar")
				_ = repo.DeleteByCleanName(context.Background(), req.InstanceID, rawClean) // errcheck:ok best effort cache delete on update
			}
			_ = repo.Save(context.Background(), storage.InstalledModRecord{ // errcheck:ok best effort cache save on update
				InstanceID:  req.InstanceID,
				ModID:       req.ModID,
				FileName:    fileName,
				Source:      source,
				VersionID:   versionStr,
				ReleaseType: releaseTypeStr,
				InstalledAt: time.Now(),
			})
		}

		var removedDuplicates []string
		if recRes != nil && len(recRes.RemovedDuplicates) > 0 {
			removedDuplicates = recRes.RemovedDuplicates
		}

		a.setInstallProgress(req.InstanceID, &ModInstallProgressDTO{
			TaskID:     fmt.Sprintf("update-%s", req.ModID),
			InstanceID: req.InstanceID,
			ModID:      req.ModID,
			FileName:   fileName,
			Status:     "completed",
			Percentage: 100,
		})

		return &InstallModResponse{
			Success:           true,
			FileName:          fileName,
			Message:           fmt.Sprintf("Mod %s updated successfully", fileName),
			RemovedDuplicates: removedDuplicates,
		}, nil
	}

	return &InstallModResponse{
		Success:  true,
		FileName: fileName,
		Message:  fmt.Sprintf("Mod %s updated successfully", fileName),
	}, nil
}

func (a *WailsAdapter) recordInstalledMod(instanceID, modID, fileName, source string, fileToDownload *content.ModFile, reqVersionID string) {
	modsDir := a.getModsDir(instanceID)
	versionStr := reqVersionID
	releaseTypeStr := ""
	modTitle := modID
	if fileToDownload != nil {
		if versionStr == "" {
			versionStr = fileToDownload.ID
		}
		releaseTypeStr = fileToDownload.ReleaseType
		if fileToDownload.FileName != "" {
			modTitle = fileToDownload.FileName
		}
	}

	m, err := manifest.LoadManifest(modsDir)
	if err == nil {
		m.AddOrUpdate(&manifest.ModRecord{
			ModID:       modID,
			ModName:     modTitle,
			FileName:    fileName,
			Source:      source,
			VersionID:   versionStr,
			ReleaseType: releaseTypeStr,
			InstalledAt: time.Now(),
		})
		_ = m.Save(modsDir) // errcheck:ok best effort manifest save on install
	}

	a.mu.RLock()
	repo := a.installedModsRepo
	a.mu.RUnlock()
	if repo != nil {
		_ = repo.Save(context.Background(), storage.InstalledModRecord{ // errcheck:ok best effort sqlite sync on install
			InstanceID:  instanceID,
			ModID:       modID,
			FileName:    fileName,
			Source:      source,
			VersionID:   versionStr,
			ReleaseType: releaseTypeStr,
			InstalledAt: time.Now(),
		})
	}
}

func (a *WailsAdapter) GetSettings() (*GetSettingsResponse, error) {
	a.mu.RLock()
	repo := a.settingsRepo
	a.mu.RUnlock()

	if repo == nil {
		return &GetSettingsResponse{Settings: map[string]string{}}, nil
	}

	all, err := repo.GetAll(context.Background())
	if err != nil {
		return nil, fmt.Errorf("get settings: %w", err)
	}
	if all == nil {
		all = make(map[string]string)
	}
	if val, ok := all["curseforge_api_key"]; ok && val != "" {
		all["has_curseforge_api_key"] = "true"
		all["curseforge_api_key"] = ""
	}
	return &GetSettingsResponse{Settings: all}, nil
}

func (a *WailsAdapter) SetSetting(req SetSettingRequest) error {
	a.mu.RLock()
	repo := a.settingsRepo
	cf := a.curseforge
	a.mu.RUnlock()

	if repo != nil {
		if err := repo.Set(context.Background(), req.Key, req.Value); err != nil {
			return fmt.Errorf("save setting: %w", err)
		}
	}

	if req.Key == "curseforge_api_key" && cf != nil {
		resolvedKey := req.Value
		if resolvedKey == "" {
			resolvedKey = os.Getenv("CURSEFORGE_API_KEY")
			if resolvedKey == "" {
				resolvedKey = curseforge.GetBuiltinAPIKey()
			}
		}
		cf.SetAPIKey(resolvedKey)
	}

	return nil
}

func (a *WailsAdapter) HasBuiltinCurseForgeKey() (bool, error) {
	key, _ := curseforge.ResolveSidecarKey("") // errcheck:ok fallback to empty if sidecar not found
	return key != "" || curseforge.HasBuiltinKey(), nil
}

func toJavaInstallationDTO(inst ports.JavaInstallation) JavaInstallationDTO {
	usedBy := inst.UsedBy
	if usedBy == nil {
		usedBy = []string{}
	}
	return JavaInstallationDTO{
		Path:         inst.Path,
		HomeDir:      inst.HomeDir,
		MajorVersion: inst.MajorVersion,
		FullVersion:  inst.FullVersion,
		Vendor:       inst.Vendor,
		Kind:         inst.Kind,
		UsedBy:       usedBy,
	}
}

func (a *WailsAdapter) ListJavaRuntimes() ([]JavaInstallationDTO, error) {
	a.mu.RLock()
	jm := a.javaMgr
	a.mu.RUnlock()

	if jm == nil {
		return []JavaInstallationDTO{}, nil
	}

	installs, err := jm.ListRuntimes(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list java runtimes: %w", err)
	}

	dtos := make([]JavaInstallationDTO, 0, len(installs))
	for _, inst := range installs {
		dtos = append(dtos, toJavaInstallationDTO(inst))
	}
	return dtos, nil
}

func (a *WailsAdapter) DownloadJavaRuntime(major int) error {
	a.mu.RLock()
	jm := a.javaMgr
	a.mu.RUnlock()

	if jm == nil {
		return fmt.Errorf("java manager not configured")
	}

	go func() {
		_ = jm.DownloadRuntime(context.Background(), major) // errcheck:ok async download error captured in GetJavaDownloadStatus
	}()
	return nil
}

func (a *WailsAdapter) GetJavaDownloadStatus() JavaDownloadStatusDTO {
	a.mu.RLock()
	jm := a.javaMgr
	a.mu.RUnlock()

	if jm == nil {
		return JavaDownloadStatusDTO{Status: "idle"}
	}
	status := jm.GetDownloadStatus()
	return JavaDownloadStatusDTO{
		TaskID:     status.TaskID,
		Major:      status.Major,
		Status:     status.Status,
		BytesRead:  status.BytesRead,
		TotalBytes: status.TotalBytes,
		Percentage: status.Percentage,
		Error:      status.Error,
	}
}

func (a *WailsAdapter) RemoveJavaRuntime(path string) error {
	a.mu.RLock()
	jm := a.javaMgr
	a.mu.RUnlock()

	if jm == nil {
		return fmt.Errorf("java manager not configured")
	}
	return jm.RemoveRuntime(path)
}

func (a *WailsAdapter) AddJavaRuntime(path string) (*JavaInstallationDTO, error) {
	a.mu.RLock()
	jm := a.javaMgr
	a.mu.RUnlock()

	if jm == nil {
		return nil, fmt.Errorf("java manager not configured")
	}
	install, err := jm.AddRuntime(context.Background(), path)
	if err != nil {
		return nil, err
	}
	dto := toJavaInstallationDTO(*install)
	return &dto, nil
}

func (a *WailsAdapter) CheckModUpdates(instanceID string) ([]ModUpdateItemDTO, error) {
	if strings.TrimSpace(instanceID) == "" {
		return nil, errors.New("instance_id is required")
	}

	installedMods, err := a.ListInstalledMods(instanceID)
	if err != nil {
		return nil, fmt.Errorf("list installed mods: %w", err)
	}

	var updates []ModUpdateItemDTO
	var gameVersion, loader string
	if a.svc != nil {
		if inst, err := a.svc.GetInstance(instanceID); err == nil && inst != nil {
			gameVersion = inst.GameVersion
			loader = string(inst.Loader)
		}
	}

	for _, mod := range installedMods {
		if mod.ModID == "" {
			continue
		}
		src := strings.ToLower(strings.TrimSpace(mod.Source))
		if src != "modrinth" && src != "curseforge" {
			continue
		}

		files, err := a.ListModVersions(ListModVersionsRequest{
			InstanceID:  instanceID,
			ModID:       mod.ModID,
			Source:      src,
			GameVersion: gameVersion,
			Loader:      loader,
		})
		if err != nil || len(files) == 0 {
			continue
		}

		latest := files[0]
		isNewer := false
		cleanInstalled := strings.TrimSuffix(mod.FileName, ".disabled")

		if latest.FileName != "" && cleanInstalled != "" && latest.FileName != cleanInstalled {
			isNewer = true
		} else if latest.FileName != "" && cleanInstalled != "" && latest.FileName == cleanInstalled {
			isNewer = false
		} else if mod.Version != "" && latest.ID != "" && latest.ID != mod.Version {
			isNewer = true
		}

		if isNewer {
			updates = append(updates, ModUpdateItemDTO{
				FileName:        mod.FileName,
				ModID:           mod.ModID,
				Source:          src,
				CurrentVersion:  mod.Version,
				LatestVersion:   latest.DisplayName,
				LatestVersionID: latest.ID,
				ReleaseType:     latest.ReleaseType,
				Dependencies:    latest.Dependencies,
				Changelog:       latest.Changelog,
			})
		}
	}

	if updates == nil {
		updates = []ModUpdateItemDTO{}
	}
	return updates, nil
}

func (a *WailsAdapter) GetDiagnosticReport(instanceID string) (string, error) {
	a.mu.RLock()
	ver := a.version
	a.mu.RUnlock()
	if ver == "" {
		ver = "0.6.1"
	}

	var b strings.Builder
	b.WriteString("=== Nord Launcher Diagnostic Report ===\n")
	b.WriteString(fmt.Sprintf("Generated At: %s\n", time.Now().UTC().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("Launcher Version: %s\n", ver))
	b.WriteString(fmt.Sprintf("OS: %s\n", runtime.GOOS))
	b.WriteString(fmt.Sprintf("Architecture: %s\n", runtime.GOARCH))
	b.WriteString(fmt.Sprintf("Go Version: %s\n", runtime.Version()))
	b.WriteString(fmt.Sprintf("NumCPU: %d\n\n", runtime.NumCPU()))

	b.WriteString("--- Java Runtimes ---\n")
	runtimes, err := a.ListJavaRuntimes()
	if err != nil || len(runtimes) == 0 {
		b.WriteString("  (no java runtimes detected)\n")
	} else {
		for _, rt := range runtimes {
			b.WriteString(fmt.Sprintf("- Java %d (%s, vendor: %s) at %s\n",
				rt.MajorVersion, rt.Kind, rt.Vendor, anonymizeDiagnosticText(rt.Path)))
		}
	}
	b.WriteString("\n")

	if strings.TrimSpace(instanceID) != "" {
		b.WriteString(fmt.Sprintf("--- Instance: %s ---\n", instanceID))
		if a.svc != nil {
			if inst, err := a.svc.GetInstance(instanceID); err == nil && inst != nil {
				b.WriteString(fmt.Sprintf("Name: %s\n", inst.Name))
				b.WriteString(fmt.Sprintf("Game Version: %s\n", inst.GameVersion))
				b.WriteString(fmt.Sprintf("Loader: %s (version: %s)\n", inst.Loader, inst.LoaderVer))
				b.WriteString(fmt.Sprintf("Memory: Min %d MB / Max %d MB\n", inst.MinRAMMB, inst.MaxRAMMB))
			} else {
				b.WriteString("  (instance not found in service)\n")
			}
		}

		b.WriteString("\n--- Installed Mods ---\n")
		mods, err := a.ListInstalledMods(instanceID)
		if err != nil {
			b.WriteString(fmt.Sprintf("  (error reading mods: %v)\n", err))
		} else if len(mods) == 0 {
			b.WriteString("  (no mods installed)\n")
		} else {
			enabledCount := 0
			for _, m := range mods {
				if m.Enabled {
					enabledCount++
				}
			}
			b.WriteString(fmt.Sprintf("Total: %d (Enabled: %d, Disabled: %d)\n", len(mods), enabledCount, len(mods)-enabledCount))
			for _, m := range mods {
				status := "disabled"
				if m.Enabled {
					status = "enabled"
				}
				b.WriteString(fmt.Sprintf("- %s [%s] (Name: %s, ID: %s, Version: %s, Source: %s)\n",
					m.FileName, status, m.Name, m.ModID, m.Version, m.Source))
			}
		}
		b.WriteString("\n")

		b.WriteString("--- Recent Logs ---\n")
		logTail, err := a.GetLogTail(instanceID, 30)
		if err != nil || len(logTail) == 0 {
			b.WriteString("  (no recent log entries)\n")
		} else {
			for _, line := range logTail {
				b.WriteString(anonymizeDiagnosticText(line) + "\n")
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("--- Settings (Anonymized) ---\n")
	settingsResp, err := a.GetSettings()
	if err != nil || settingsResp == nil || len(settingsResp.Settings) == 0 {
		b.WriteString("  (no custom settings configured)\n")
	} else {
		for k, v := range settingsResp.Settings {
			if strings.Contains(strings.ToLower(k), "key") || strings.Contains(strings.ToLower(k), "token") || strings.Contains(strings.ToLower(k), "secret") {
				if v != "" {
					b.WriteString(fmt.Sprintf("%s: [SET]\n", k))
				} else {
					b.WriteString(fmt.Sprintf("%s: [NOT SET]\n", k))
				}
			} else {
				b.WriteString(fmt.Sprintf("%s: %s\n", k, anonymizeDiagnosticText(v)))
			}
		}
	}

	return b.String(), nil
}

func anonymizeDiagnosticText(text string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		text = strings.ReplaceAll(text, home, "~")
		text = strings.ReplaceAll(text, filepath.ToSlash(home), "~")
	}
	return text
}

func (a *WailsAdapter) PickMrPackFile() (string, error) {
	a.mu.RLock()
	picker := a.filePickerFn
	a.mu.RUnlock()
	if picker != nil {
		return picker()
	}

	app := application.Get()
	if app == nil || app.Dialog == nil {
		return "", fmt.Errorf("file dialog not available")
	}
	dialog := app.Dialog.OpenFile().
		SetTitle("Select Modpack (.mrpack)").
		AddFilter("Modrinth Modpack (*.mrpack)", "*.mrpack").
		CanChooseFiles(true).
		CanChooseDirectories(false)
	return dialog.PromptForSingleSelection()
}

func (a *WailsAdapter) GetMrPackImportPlan(mrpackPath string) (*MrPackImportPlanDTO, error) {
	if strings.TrimSpace(mrpackPath) == "" {
		return nil, errors.New("mrpack path cannot be empty")
	}
	f, err := os.Open(mrpackPath)
	if err != nil {
		return nil, fmt.Errorf("open mrpack: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat mrpack: %w", err)
	}

	var existingNames []string
	if a.svc != nil {
		instances := a.svc.ListInstances()
		for _, inst := range instances {
			existingNames = append(existingNames, inst.Name)
		}
	}

	plan, err := content.GetMrPackImportPlan(f, fi.Size(), existingNames)
	if err != nil {
		return nil, fmt.Errorf("get mrpack import plan: %w", err)
	}

	return &MrPackImportPlanDTO{
		Name:         plan.Name,
		Summary:      plan.Summary,
		GameVersion:  plan.GameVersion,
		Loader:       plan.Loader,
		LoaderVer:    plan.LoaderVersion,
		TotalFiles:   plan.TotalFiles,
		TotalSize:    plan.TotalBytes,
		Dependencies: plan.Dependencies,
	}, nil
}

func (a *WailsAdapter) ImportMrPack(req ImportMrPackRequest) (string, error) {
	if strings.TrimSpace(req.MrPackPath) == "" {
		return "", errors.New("mrpack path cannot be empty")
	}
	importer := a.getMrPackImporter()
	if importer == nil {
		return "", errors.New("mrpack importer not available")
	}

	opts := content.ImportMrPackOptions{
		InstanceName: req.InstanceName,
	}

	instanceID, err := importer.ImportMrPack(context.Background(), req.MrPackPath, opts, func(p content.MrPackImportProgress) {
		a.mu.Lock()
		if a.mrpackProgress == nil {
			a.mrpackProgress = make(map[string]*MrPackImportStatusDTO)
		}
		var pct float64
		if p.TotalFiles > 0 {
			pct = float64(p.DownloadedFiles) / float64(p.TotalFiles) * 100
		}
		statusItem := &MrPackImportStatusDTO{
			TaskID:      req.InstanceName,
			Status:      p.Status,
			CurrentFile: p.CurrentFile,
			FilesDone:   p.DownloadedFiles,
			TotalFiles:  p.TotalFiles,
			BytesRead:   p.DownloadedBytes,
			TotalBytes:  p.TotalBytes,
			Percentage:  pct,
			Error:       p.Error,
		}
		a.mrpackProgress[req.InstanceName] = statusItem
		a.mu.Unlock()
	})
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	if a.mrpackProgress != nil && a.mrpackProgress[req.InstanceName] != nil {
		a.mrpackProgress[instanceID] = a.mrpackProgress[req.InstanceName]
	}
	a.mu.Unlock()

	return instanceID, nil
}

func (a *WailsAdapter) GetMrPackImportStatus(instanceID string) (*MrPackImportStatusDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.mrpackProgress != nil {
		if p, ok := a.mrpackProgress[instanceID]; ok && p != nil {
			cp := *p
			return &cp, nil
		}
	}
	if a.mrpackImporter != nil {
		if p, ok := a.mrpackImporter.GetStatus(instanceID); ok && p != nil {
			var pct float64
			if p.TotalFiles > 0 {
				pct = float64(p.DownloadedFiles) / float64(p.TotalFiles) * 100
			}
			return &MrPackImportStatusDTO{
				TaskID:      instanceID,
				Status:      p.Status,
				CurrentFile: p.CurrentFile,
				FilesDone:   p.DownloadedFiles,
				TotalFiles:  p.TotalFiles,
				BytesRead:   p.DownloadedBytes,
				TotalBytes:  p.TotalBytes,
				Percentage:  pct,
				Error:       p.Error,
			}, nil
		}
	}
	return &MrPackImportStatusDTO{
		Status: "idle",
	}, nil
}

func (a *WailsAdapter) ExportMrPack(req ExportMrPackRequest) (string, error) {
	if strings.TrimSpace(req.InstanceID) == "" {
		return "", errors.New("instance id cannot be empty")
	}
	exporter := a.getMrPackExporter()
	if exporter == nil {
		return "", errors.New("mrpack exporter not available")
	}

	opts := content.ExportMrPackOptions{
		InstanceID:       req.InstanceID,
		PackName:         req.Name,
		PackVersion:      req.Version,
		Summary:          req.Summary,
		ExportPath:       req.OutputPath,
		IncludeShaders:   req.IncludeShaders,
		IncludeResources: req.IncludeResources,
	}

	return exporter.ExportMrPack(context.Background(), opts)
}

func (a *WailsAdapter) CheckJavaRuntimeUpdates() ([]JavaRuntimeUpdateDTO, error) {
	a.mu.RLock()
	jm := a.javaMgr
	a.mu.RUnlock()

	if jm == nil {
		return []JavaRuntimeUpdateDTO{}, nil
	}

	updates, err := jm.CheckRuntimeUpdates(context.Background())
	if err != nil {
		return nil, fmt.Errorf("check java runtime updates: %w", err)
	}

	dtos := make([]JavaRuntimeUpdateDTO, 0, len(updates))
	for _, u := range updates {
		dtos = append(dtos, JavaRuntimeUpdateDTO{
			MajorVersion:    u.MajorVersion,
			CurrentVersion:  u.CurrentVersion,
			LatestVersion:   u.LatestVersion,
			UpdateAvailable: u.UpdateAvailable,
			DownloadURL:     u.DownloadURL,
		})
	}
	return dtos, nil
}

func (a *WailsAdapter) UpgradeJavaRuntime(major int) (*JavaDownloadStatusDTO, error) {
	a.mu.RLock()
	jm := a.javaMgr
	a.mu.RUnlock()

	if jm == nil {
		return nil, fmt.Errorf("java manager not configured")
	}

	go func() {
		_, _ = jm.UpgradeRuntime(context.Background(), major) // errcheck:ok async upgrade captured in GetJavaDownloadStatus
	}()

	status := jm.GetDownloadStatus()
	return &JavaDownloadStatusDTO{
		TaskID:     status.TaskID,
		Major:      status.Major,
		Status:     status.Status,
		BytesRead:  status.BytesRead,
		TotalBytes: status.TotalBytes,
		Percentage: status.Percentage,
		Error:      status.Error,
	}, nil
}

var openPathExec = func(cleanPath string, isDir bool) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if isDir {
			cmd = exec.Command("explorer", cleanPath)
		} else {
			cmd = exec.Command("explorer", fmt.Sprintf("/select,%s", cleanPath))
		}
	case "darwin":
		if isDir {
			cmd = exec.Command("open", cleanPath)
		} else {
			cmd = exec.Command("open", "-R", cleanPath)
		}
	default:
		if isDir {
			cmd = exec.Command("xdg-open", cleanPath)
		} else {
			cmd = exec.Command("xdg-open", filepath.Dir(cleanPath))
		}
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open path: %w", err)
	}
	return nil
}

// OpenPath opens a file or directory in the native file manager (explorer, open, xdg-open).
// If targetPath is a file, it reveals the file in its containing folder.
// If targetPath is an instance ID or relative path in instancesDir, it resolves to the instance folder.
func (a *WailsAdapter) OpenPath(targetPath string) error {
	trimmed := strings.TrimSpace(targetPath)
	if trimmed == "" {
		return errors.New("empty path provided")
	}
	cleanPath := filepath.Clean(trimmed)
	fi, err := os.Stat(cleanPath)
	if err != nil {
		if a.instancesDir != "" {
			instPath := filepath.Join(a.instancesDir, cleanPath)
			if ifi, ierr := os.Stat(instPath); ierr == nil {
				return openPathExec(instPath, ifi.IsDir())
			}
		}
		return fmt.Errorf("path not accessible: %w", err)
	}
	return openPathExec(cleanPath, fi.IsDir())
}
