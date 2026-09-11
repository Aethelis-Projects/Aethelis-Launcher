package wails

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/nord-launcher/launcher/internal/core/auth"
	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/content/modrinth"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

// WailsAdapter connects Wails IPC layer to the Hexagonal Core.
type WailsAdapter struct {
	svc         *launch.InstanceService
	authSvc     *auth.AuthService
	accountRepo ports.AccountRepository
	modrinth    *modrinth.Client
	curseforge  *curseforge.Client
	fileSys     ports.FileSystem
	instancesDir string

	lastCrashes map[string]*CrashReportDTO
	mu          sync.RWMutex
}

func NewWailsAdapter(svc *launch.InstanceService) *WailsAdapter {
	return &WailsAdapter{
		svc:         svc,
		lastCrashes: make(map[string]*CrashReportDTO),
	}
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