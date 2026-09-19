package wails

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
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
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/ports"
	"github.com/nord-launcher/launcher/internal/core/storage"
	"github.com/nord-launcher/launcher/internal/core/updater"
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

	lastCrashes map[string]*CrashReportDTO
	mu          sync.RWMutex
}

func NewWailsAdapter(svc *launch.InstanceService) *WailsAdapter {
	a := &WailsAdapter{
		svc:         svc,
		relauncher:  updater.DefaultRelauncher,
		lastCrashes: make(map[string]*CrashReportDTO),
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

func (a *WailsAdapter) ListInstances() []InstanceDTO {
	coreList := a.svc.ListInstances()
	dtoList := make([]InstanceDTO, 0, len(coreList))
	for _, inst := range coreList {
		dtoList = append(dtoList, InstanceDTO{
			ID:               inst.ID,
			Name:             inst.Name,
			GameVersion:      inst.GameVersion,
			Loader:           string(inst.Loader),
			LoaderVersion:    inst.LoaderVer,
			IconPath:         inst.IconPath,
			State:            string(inst.State),
			TotalPlaySeconds: inst.TotalPlaySec,
		})
	}
	return dtoList
}

func (a *WailsAdapter) CreateInstance(req CreateInstanceRequest) (*InstanceDTO, error) {
	inst, err := a.svc.CreateInstance(req.Name, req.GameVersion, domain.LoaderType(req.Loader))
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}

	return &InstanceDTO{
		ID:               inst.ID,
		Name:             inst.Name,
		GameVersion:      inst.GameVersion,
		Loader:           string(inst.Loader),
		LoaderVersion:    inst.LoaderVer,
		IconPath:         inst.IconPath,
		State:            string(inst.State),
		TotalPlaySeconds: inst.TotalPlaySec,
	}, nil
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

func (a *WailsAdapter) SearchMods(req SearchModsRequest) ([]ModItemDTO, error) {
	if req.Source == "curseforge" {
		if a.curseforge == nil {
			return nil, fmt.Errorf("curseforge client not initialized")
		}
		items, _, err := a.curseforge.SearchMods(context.Background(), req.Query, req.GameVersion, req.Loader, req.Limit, req.Offset)
		if err != nil {
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
		return dtos, nil
	}

	// Default to Modrinth
	if a.modrinth == nil {
		return nil, fmt.Errorf("modrinth client not initialized")
	}
	items, _, err := a.modrinth.SearchMods(context.Background(), req.Query, req.GameVersion, req.Loader, req.Limit, req.Offset)
	if err != nil {
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
	return dtos, nil
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

	var res []InstalledModDTO
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
			cleanName := strings.TrimSuffix(strings.TrimSuffix(name, ".disabled"), ".jar")

			res = append(res, InstalledModDTO{
				FileName:  name,
				Name:      cleanName,
				Enabled:   enabled,
				SizeBytes: size,
			})
		}
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
	return os.Rename(oldPath, newPath)
}

func (a *WailsAdapter) DeleteMod(req DeleteModRequest) error {
	modsDir := a.getModsDir(req.InstanceID)
	target := filepath.Join(modsDir, req.FileName)
	return os.Remove(target)
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

func (a *WailsAdapter) InstallMod(req InstallModRequest) (*InstallModResponse, error) {
	if strings.TrimSpace(req.InstanceID) == "" {
		return nil, errors.New("instance_id is required")
	}
	if strings.TrimSpace(req.ModID) == "" {
		return nil, errors.New("mod_id is required")
	}

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

	var fileToDownload *content.ModFile

	switch source {
	case "modrinth":
		a.mu.RLock()
		mr := a.modrinth
		a.mu.RUnlock()
		if mr == nil {
			return nil, errors.New("modrinth client not initialized")
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
			return nil, fmt.Errorf("fetch modrinth versions: %w", err)
		}
		if len(versions) == 0 {
			return nil, errors.New("no compatible versions found for mod")
		}

		for _, v := range versions {
			if len(v.Files) == 0 {
				continue
			}
			for i := range v.Files {
				if v.Files[i].Primary {
					fileToDownload = &v.Files[i]
					break
				}
			}
			if fileToDownload == nil {
				fileToDownload = &v.Files[0]
			}
			break
		}
		if fileToDownload == nil {
			return nil, errors.New("no downloadable files found for mod")
		}

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

		ctx := context.Background()
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
		if len(files) == 0 {
			return nil, errors.New("no compatible files found for mod")
		}

		fileToDownload = &files[0]

	default:
		return nil, fmt.Errorf("unsupported mod source: %s", req.Source)
	}

	if fileToDownload == nil || fileToDownload.URL == "" {
		return nil, errors.New("mod file download URL is missing")
	}

	modsDir := a.getModsDir(req.InstanceID)
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		return nil, fmt.Errorf("create mods directory: %w", err)
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
		return &InstallModResponse{
			Success:  true,
			FileName: fileName,
			Message:  "Mod already installed",
		}, nil
	}

	tempFile, err := os.CreateTemp(modsDir, ".tmp-*.jar")
	if err != nil {
		return nil, fmt.Errorf("create temporary download file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close() // errcheck:ok best effort close if still open
		if _, statErr := os.Stat(tempPath); statErr == nil {
			_ = os.Remove(tempPath) // errcheck:ok best effort cleanup of temporary file
		}
	}()

	client := a.getHTTPClient()
	httpReq, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fileToDownload.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("create download request: %w", err)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("download mod file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	hSha1 := sha1.New()
	hSha512 := sha512.New()
	mw := io.MultiWriter(tempFile, hSha1, hSha512)

	if _, err := io.Copy(mw, resp.Body); err != nil {
		return nil, fmt.Errorf("write mod file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}

	// Verify checksums
	if fileToDownload.SHA512 != "" {
		actualSha512 := hex.EncodeToString(hSha512.Sum(nil))
		if !strings.EqualFold(actualSha512, fileToDownload.SHA512) {
			return nil, fmt.Errorf("sha512 mismatch: expected %s, got %s", fileToDownload.SHA512, actualSha512)
		}
	} else if fileToDownload.SHA1 != "" {
		actualSha1 := hex.EncodeToString(hSha1.Sum(nil))
		if !strings.EqualFold(actualSha1, fileToDownload.SHA1) {
			return nil, fmt.Errorf("sha1 mismatch: expected %s, got %s", fileToDownload.SHA1, actualSha1)
		}
	}

	// Atomic rename
	if err := os.Rename(tempPath, destPath); err != nil {
		return nil, fmt.Errorf("finalize mod install: %w", err)
	}

	return &InstallModResponse{
		Success:  true,
		FileName: fileName,
		Message:  fmt.Sprintf("Mod %s installed successfully", fileName),
	}, nil
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
				resolvedKey = curseforge.BuiltinAPIKey
			}
		}
		cf.SetAPIKey(resolvedKey)
	}

	return nil
}