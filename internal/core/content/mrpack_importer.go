package content

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/manifest"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

// ImportMrPackOptions configures the import operation.
type ImportMrPackOptions struct {
	InstanceID      string `json:"instance_id,omitempty"`
	InstanceName    string `json:"instance_name,omitempty"`
	IncludeOptional bool   `json:"include_optional"`
}

// MrPackImportProgress provides real-time progress information during mrpack import.
type MrPackImportProgress struct {
	TotalFiles      int    `json:"total_files"`
	DownloadedFiles int    `json:"downloaded_files"`
	TotalBytes      int64  `json:"total_bytes"`
	DownloadedBytes int64  `json:"downloaded_bytes"`
	CurrentFile     string `json:"current_file"`
	Status          string `json:"status"` // "downloading", "extracting", "complete", "failed"
	Error           string `json:"error,omitempty"`
}

// MrPackImporter handles importing .mrpack archives into isolated game instances.
type MrPackImporter struct {
	httpClient   ports.HTTPClient
	instRepo     ports.InstanceRepository
	instancesDir string
	mu           sync.RWMutex
	statuses     map[string]*MrPackImportProgress
}

// NewMrPackImporter creates a new MrPackImporter instance.
func NewMrPackImporter(httpClient ports.HTTPClient, instRepo ports.InstanceRepository, instancesDir string) *MrPackImporter {
	if instancesDir == "" {
		instancesDir = "instances"
	}
	return &MrPackImporter{
		httpClient:   httpClient,
		instRepo:     instRepo,
		instancesDir: instancesDir,
		statuses:     make(map[string]*MrPackImportProgress),
	}
}

// GetStatus returns the current import progress for a given instance ID.
func (m *MrPackImporter) GetStatus(instanceID string) (*MrPackImportProgress, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	status, ok := m.statuses[instanceID]
	if !ok || status == nil {
		return nil, false
	}
	cp := *status
	return &cp, true
}

func (m *MrPackImporter) setStatus(instanceID string, p *MrPackImportProgress) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *p
	m.statuses[instanceID] = &cp
}

// CalculateFileSHA1 computes the lowercase hex-encoded SHA-1 checksum of a file.
func CalculateFileSHA1(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha1.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// CalculateFileSHA512 computes the lowercase hex-encoded SHA-512 checksum of a file.
func CalculateFileSHA512(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha512.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// ImportMrPack executes the full modpack import pipeline: metadata validation, instance provisioning,
// idempotent download with checksum verification, overrides extraction, and manifest recording.
func (m *MrPackImporter) ImportMrPack(
	ctx context.Context,
	mrpackPath string,
	opts ImportMrPackOptions,
	onProgress func(p MrPackImportProgress),
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	fi, err := os.Stat(mrpackPath)
	if err != nil {
		return "", fmt.Errorf("stat mrpack file: %w", err)
	}
	if fi.Size() > MaxMrPackSizeBytes {
		return "", fmt.Errorf("mrpack archive size (%d bytes) exceeds maximum limit (%d bytes)", fi.Size(), MaxMrPackSizeBytes)
	}

	f, err := os.Open(mrpackPath)
	if err != nil {
		return "", fmt.Errorf("open mrpack file: %w", err)
	}
	defer f.Close()

	index, err := ParseMrPack(f, fi.Size())
	if err != nil {
		return "", err
	}

	gameVersion := index.Dependencies["minecraft"]
	if gameVersion == "" {
		return "", fmt.Errorf("missing minecraft version in .mrpack dependencies")
	}

	loader := domain.LoaderVanilla
	loaderVer := ""
	for depKey, depVer := range index.Dependencies {
		switch strings.ToLower(depKey) {
		case "fabric-loader", "fabric":
			loader = domain.LoaderFabric
			loaderVer = depVer
		case "quilt-loader", "quilt":
			loader = domain.LoaderQuilt
			loaderVer = depVer
		case "forge":
			loader = domain.LoaderForge
			loaderVer = depVer
		case "neoforge":
			loader = domain.LoaderNeoForge
			loaderVer = depVer
		}
	}

	var inst *domain.Instance
	instanceID := opts.InstanceID
	if instanceID != "" && m.instRepo != nil {
		var getErr error
		inst, getErr = m.instRepo.GetByID(ctx, instanceID)
		if getErr != nil {
			inst = nil
		}
	}

	if inst == nil {
		name := opts.InstanceName
		if name == "" {
			name = index.Name
		}
		if name == "" {
			base := filepath.Base(mrpackPath)
			name = strings.TrimSuffix(base, filepath.Ext(base))
		}
		if instanceID == "" {
			instanceID = fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
		}
		inst = &domain.Instance{
			ID:          instanceID,
			Name:        name,
			GameVersion: gameVersion,
			Loader:      loader,
			LoaderVer:   loaderVer,
			MinRAMMB:    2048,
			MaxRAMMB:    4096,
			State:       domain.StateImporting,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if m.instRepo != nil {
			if err := m.instRepo.Save(ctx, inst); err != nil {
				return "", fmt.Errorf("save instance: %w", err)
			}
		}
	} else {
		if m.instRepo != nil {
			_ = m.instRepo.UpdateState(ctx, inst.ID, domain.StateImporting) // errcheck:ok best effort state update
		}
	}

	instanceDir := filepath.Join(m.instancesDir, inst.ID)
	if err := os.MkdirAll(instanceDir, 0755); err != nil {
		if m.instRepo != nil {
			_ = m.instRepo.UpdateState(ctx, inst.ID, domain.StateError) // errcheck:ok best effort error state update
		}
		return inst.ID, fmt.Errorf("create instance dir: %w", err)
	}

	type fileTask struct {
		file MrPackFile
		dest string
	}
	var tasks []fileTask
	var totalBytes int64

	for _, fileEntry := range index.Files {
		if fileEntry.Env != nil {
			if fileEntry.Env.Client == "unsupported" {
				continue
			}
			if fileEntry.Env.Client == "optional" && !opts.IncludeOptional {
				continue
			}
		}

		targetPath, err := SafeRelPath(instanceDir, fileEntry.Path)
		if err != nil {
			if m.instRepo != nil {
				_ = m.instRepo.UpdateState(ctx, inst.ID, domain.StateError) // errcheck:ok best effort error state update
			}
			return inst.ID, err
		}

		tasks = append(tasks, fileTask{file: fileEntry, dest: targetPath})
		totalBytes += fileEntry.FileSize
	}

	progress := MrPackImportProgress{
		TotalFiles: len(tasks),
		TotalBytes: totalBytes,
		Status:     "downloading",
	}

	reportProgress := func() {
		m.setStatus(inst.ID, &progress)
		if onProgress != nil {
			onProgress(progress)
		}
	}
	reportProgress()

	for _, task := range tasks {
		if err := ctx.Err(); err != nil {
			if m.instRepo != nil {
				_ = m.instRepo.UpdateState(ctx, inst.ID, domain.StateError) // errcheck:ok best effort error state update
			}
			progress.Status = "failed"
			progress.Error = err.Error()
			reportProgress()
			return inst.ID, err
		}

		expectedSHA1 := strings.ToLower(task.file.Hashes["sha1"])
		expectedSHA512 := strings.ToLower(task.file.Hashes["sha512"])

		validOnDisk := false
		if _, statErr := os.Stat(task.dest); statErr == nil && expectedSHA1 != "" {
			actualSHA1, hashErr := CalculateFileSHA1(task.dest)
			if hashErr == nil && strings.EqualFold(actualSHA1, expectedSHA1) {
				if expectedSHA512 != "" {
					actualSHA512, hash512Err := CalculateFileSHA512(task.dest)
					if hash512Err == nil && strings.EqualFold(actualSHA512, expectedSHA512) {
						validOnDisk = true
					}
				} else {
					validOnDisk = true
				}
			}
		}

		if validOnDisk {
			progress.DownloadedFiles++
			progress.DownloadedBytes += task.file.FileSize
			progress.CurrentFile = task.file.Path
			reportProgress()
			continue
		}

		if len(task.file.Downloads) == 0 {
			err := fmt.Errorf("no download URLs available for %s", task.file.Path)
			if m.instRepo != nil {
				_ = m.instRepo.UpdateState(ctx, inst.ID, domain.StateError) // errcheck:ok best effort error state update
			}
			progress.Status = "failed"
			progress.Error = err.Error()
			reportProgress()
			return inst.ID, err
		}

		if err := os.MkdirAll(filepath.Dir(task.dest), 0755); err != nil {
			if m.instRepo != nil {
				_ = m.instRepo.UpdateState(ctx, inst.ID, domain.StateError) // errcheck:ok best effort error state update
			}
			progress.Status = "failed"
			progress.Error = err.Error()
			reportProgress()
			return inst.ID, fmt.Errorf("create parent directory for %s: %w", task.dest, err)
		}

		var dlErr error
		for _, dlURL := range task.file.Downloads {
			if ctx.Err() != nil {
				dlErr = ctx.Err()
				break
			}
			dlErr = m.httpClient.DownloadFile(ctx, dlURL, task.dest, expectedSHA1, nil)
			if dlErr == nil {
				break
			}
		}

		if dlErr != nil {
			if m.instRepo != nil {
				_ = m.instRepo.UpdateState(ctx, inst.ID, domain.StateError) // errcheck:ok best effort error state update
			}
			progress.Status = "failed"
			progress.Error = dlErr.Error()
			reportProgress()
			return inst.ID, fmt.Errorf("download %s: %w", task.file.Path, dlErr)
		}

		if expectedSHA512 != "" {
			actualSHA512, hashErr := CalculateFileSHA512(task.dest)
			if hashErr != nil || !strings.EqualFold(actualSHA512, expectedSHA512) {
				_ = os.Remove(task.dest) // errcheck:ok best effort cleanup of corrupted file
				mismatchErr := fmt.Errorf("checksum mismatch: sha512 mismatch: expected %s, got %s", expectedSHA512, actualSHA512)
				if m.instRepo != nil {
					_ = m.instRepo.UpdateState(ctx, inst.ID, domain.StateError) // errcheck:ok best effort error state update
				}
				progress.Status = "failed"
				progress.Error = mismatchErr.Error()
				reportProgress()
				return inst.ID, mismatchErr
			}
		}

		progress.DownloadedFiles++
		progress.DownloadedBytes += task.file.FileSize
		progress.CurrentFile = task.file.Path
		reportProgress()
	}

	// Extract overrides on top of downloaded files so overrides take precedence
	progress.Status = "extracting"
	reportProgress()

	if err := ExtractMrPackOverrides(f, fi.Size(), instanceDir); err != nil {
		if m.instRepo != nil {
			_ = m.instRepo.UpdateState(ctx, inst.ID, domain.StateError) // errcheck:ok best effort error state update
		}
		progress.Status = "failed"
		progress.Error = err.Error()
		reportProgress()
		return inst.ID, fmt.Errorf("extract overrides: %w", err)
	}

	// Record installed mods in nord-installs.json
	modsDir := filepath.Join(instanceDir, "mods")
	if _, err := os.Stat(modsDir); err == nil {
		instManifest, loadErr := manifest.LoadManifest(modsDir)
		if loadErr != nil || instManifest == nil {
			instManifest = manifest.NewManifest()
		}

		for _, task := range tasks {
			normPath := filepath.ToSlash(task.file.Path)
			if strings.HasPrefix(normPath, "mods/") && strings.HasSuffix(normPath, ".jar") {
				fileName := filepath.Base(task.file.Path)
				instManifest.AddOrUpdate(&manifest.ModRecord{
					ModID:       fileName,
					FileName:    fileName,
					Source:      "modrinth",
					InstalledAt: time.Now(),
				})
			}
		}
		_ = instManifest.Save(modsDir) // errcheck:ok best effort save manifest
	}

	// Finalize instance state
	if m.instRepo != nil {
		if err := m.instRepo.UpdateState(ctx, inst.ID, domain.StateIdle); err != nil {
			return inst.ID, fmt.Errorf("update instance state to idle: %w", err)
		}
	}

	progress.Status = "complete"
	progress.CurrentFile = ""
	reportProgress()

	return inst.ID, nil
}
