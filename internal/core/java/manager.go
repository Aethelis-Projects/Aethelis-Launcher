package java

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

type JavaManager struct {
	managedDir  string
	detector    ports.JavaDetector
	instRepo    ports.InstanceRepository
	provisioner *AdoptiumRuntimeService
	mu          sync.RWMutex
}

func NewJavaManager(
	managedDir string,
	detector ports.JavaDetector,
	instRepo ports.InstanceRepository,
	httpClient *http.Client,
) *JavaManager {
	provisioner := NewAdoptiumRuntimeService(managedDir, nil, httpClient)
	return &JavaManager{
		managedDir:  managedDir,
		detector:    detector,
		instRepo:    instRepo,
		provisioner: provisioner,
	}
}

func (m *JavaManager) SetProvisioner(p *AdoptiumRuntimeService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provisioner = p
}

func (m *JavaManager) GetDownloadStatus() JavaDownloadStatusDTO {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.provisioner == nil {
		return JavaDownloadStatusDTO{Status: "idle"}
	}
	return m.provisioner.GetDownloadStatus()
}

// DownloadRuntime provisions an Adoptium JDK runtime for the requested major version.
// If major <= 0, it dynamically resolves the recommended version from registered instances (fallback 21) per R8.
func (m *JavaManager) DownloadRuntime(ctx context.Context, major int) error {
	if major <= 0 {
		major = 21
		if m.instRepo != nil {
			if instances, err := m.instRepo.ListAll(ctx); err == nil && len(instances) > 0 {
				for _, inst := range instances {
					if inst.GameVersion != "" {
						if resolved, resErr := ResolveJavaMajor(inst.GameVersion); resErr == nil && resolved > 0 {
							major = resolved
							break
						}
					}
				}
			}
		}
	}

	m.mu.RLock()
	prov := m.provisioner
	m.mu.RUnlock()

	if prov == nil {
		return fmt.Errorf("java provisioner not initialized")
	}

	_, err := prov.Download(ctx, major)
	return err
}

// ListRuntimes discovers all Java installations across system and managed directories,
// deduplicates them, and maps instance usage (UsedBy) per J1.
func (m *JavaManager) ListRuntimes(ctx context.Context) ([]ports.JavaInstallation, error) {
	m.mu.RLock()
	managedDirClean := filepath.Clean(m.managedDir)
	m.mu.RUnlock()

	installsMap := make(map[string]ports.JavaInstallation)

	// 1. Detect installations from system detector
	if m.detector != nil {
		detected, err := m.detector.DetectInstallations(ctx)
		if err == nil {
			for _, install := range detected {
				cleanPath := filepath.Clean(install.Path)
				install.Path = cleanPath
				install.HomeDir = filepath.Clean(install.HomeDir)
				if isSubpath(cleanPath, managedDirClean) {
					install.Kind = "managed"
				} else {
					install.Kind = "detected"
				}
				installsMap[cleanPath] = install
			}
		}
	}

	// 2. Scan managed directory directly for any nested managed runtimes
	if entries, err := os.ReadDir(managedDirClean); err == nil {
		javaExe := "java"
		if filepath.Separator == '\\' {
			javaExe = "java.exe"
		}

		for _, entry := range entries {
			if entry.IsDir() {
				homeDir := filepath.Join(managedDirClean, entry.Name())
				// Try parsing release file first
				if releaseInstall, relErr := ParseReleaseFile(homeDir); relErr == nil {
					cleanPath := filepath.Clean(releaseInstall.Path)
					releaseInstall.Path = cleanPath
					releaseInstall.HomeDir = filepath.Clean(releaseInstall.HomeDir)
					releaseInstall.Kind = "managed"
					installsMap[cleanPath] = *releaseInstall
				} else {
					// Check bin/java directly
					binPath := filepath.Join(homeDir, "bin", javaExe)
					if _, statErr := os.Stat(binPath); statErr == nil {
						cleanPath := filepath.Clean(binPath)
						if _, exists := installsMap[cleanPath]; !exists {
							cmd := exec.CommandContext(ctx, cleanPath, "-version")
							out, runErr := cmd.CombinedOutput()
							major := 0
							if runErr == nil {
								major, _ = ParseJavaMajorFromOutput(string(out))
							}
							installsMap[cleanPath] = ports.JavaInstallation{
								Path:         cleanPath,
								HomeDir:      homeDir,
								MajorVersion: major,
								Kind:         "managed",
							}
						}
					}
				}
			}
		}
	}

	// 3. Map instance usage (UsedBy) from repository
	var instances []*domain.Instance
	if m.instRepo != nil {
		if list, err := m.instRepo.ListAll(ctx); err == nil {
			instances = list
		}
	}

	for key, install := range installsMap {
		install.UsedBy = make([]string, 0)
		for _, inst := range instances {
			if inst.JavaPath != "" && filepath.Clean(inst.JavaPath) == key {
				install.UsedBy = append(install.UsedBy, inst.Name)
			}
		}
		installsMap[key] = install
	}

	// 4. Flatten and sort stably (managed first, then MajorVersion desc, path asc)
	results := make([]ports.JavaInstallation, 0, len(installsMap))
	for _, install := range installsMap {
		if install.UsedBy == nil {
			install.UsedBy = []string{}
		}
		results = append(results, install)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Kind != results[j].Kind {
			return results[i].Kind == "managed"
		}
		if results[i].MajorVersion != results[j].MajorVersion {
			return results[i].MajorVersion > results[j].MajorVersion
		}
		return results[i].Path < results[j].Path
	})

	return results, nil
}

// RemoveRuntime deletes a managed Java runtime directory.
// Enforces: 1) only managed runtimes can be removed; 2) cannot remove if used by any instance;
// 3) cannot remove if an instance is currently running with it (R8).
func (m *JavaManager) RemoveRuntime(path string) error {
	if path == "" {
		return fmt.Errorf("java path cannot be empty")
	}

	m.mu.RLock()
	cleanManaged := filepath.Clean(m.managedDir)
	m.mu.RUnlock()

	cleanPath := filepath.Clean(path)
	if !isSubpath(cleanPath, cleanManaged) {
		return fmt.Errorf("cannot remove system runtime: only managed runtimes in %s can be removed", cleanManaged)
	}

	// Guard against removal if used by any instances or if any instance is running
	if m.instRepo != nil {
		instances, err := m.instRepo.ListAll(context.Background())
		if err == nil {
			var usedBy []string
			var runningInstance string
			for _, inst := range instances {
				if inst.JavaPath != "" && filepath.Clean(inst.JavaPath) == cleanPath {
					usedBy = append(usedBy, inst.Name)
					if inst.State == domain.StateRunning || inst.State == domain.StateLaunching {
						runningInstance = inst.Name
					}
				}
			}

			if runningInstance != "" {
				return fmt.Errorf("cannot remove runtime: instance %q is currently running with this runtime", runningInstance)
			}

			if len(usedBy) > 0 {
				return fmt.Errorf("cannot remove runtime: used by %d instance(s) (%s); change instance Java settings first", len(usedBy), strings.Join(usedBy, ", "))
			}
		}
	}

	// Resolve the runtime root directory containing the binary
	// e.g. <managedDir>/adoptium-21/bin/java.exe -> <managedDir>/adoptium-21
	runtimeDir := filepath.Dir(cleanPath)
	if strings.EqualFold(filepath.Base(runtimeDir), "bin") {
		runtimeDir = filepath.Dir(runtimeDir)
	}

	if !isSubpath(runtimeDir, cleanManaged) && runtimeDir != cleanManaged {
		return fmt.Errorf("resolved runtime directory %s is outside managed directory %s", runtimeDir, cleanManaged)
	}
	if runtimeDir == cleanManaged {
		return fmt.Errorf("refusing to delete root managed directory %s", cleanManaged)
	}

	if err := os.RemoveAll(runtimeDir); err != nil {
		return fmt.Errorf("delete runtime directory: %w", err)
	}

	return nil
}

// AddRuntime validates an external Java installation path by running -version and returns its installation details.
func (m *JavaManager) AddRuntime(ctx context.Context, path string) (*ports.JavaInstallation, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("java path does not exist: %w", err)
	}

	execPath := cleanPath
	homeDir := filepath.Dir(cleanPath)

	if info.IsDir() {
		javaExe := "java"
		if filepath.Separator == '\\' {
			javaExe = "java.exe"
		}
		candidate := filepath.Join(cleanPath, "bin", javaExe)
		if _, statErr := os.Stat(candidate); statErr == nil {
			execPath = candidate
			homeDir = cleanPath
		} else {
			return nil, fmt.Errorf("could not find bin/%s in directory %s", javaExe, cleanPath)
		}
	} else if strings.EqualFold(filepath.Base(homeDir), "bin") {
		homeDir = filepath.Dir(homeDir)
	}

	// Try reading release file if present
	if relInstall, relErr := ParseReleaseFile(homeDir); relErr == nil {
		relInstall.Kind = "detected"
		relInstall.UsedBy = []string{}
		return relInstall, nil
	}

	// Fallback to running java -version
	cmd := exec.CommandContext(ctx, execPath, "-version")
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		return nil, fmt.Errorf("failed to execute Java at %s: %w", execPath, runErr)
	}

	major, fullVer := ParseJavaMajorFromOutput(string(out))
	if major == 0 {
		return nil, fmt.Errorf("could not parse Java major version from output of %s", execPath)
	}

	return &ports.JavaInstallation{
		Path:         execPath,
		HomeDir:      homeDir,
		MajorVersion: major,
		FullVersion:  fullVer,
		Vendor:       "Custom",
		Kind:         "detected",
		UsedBy:       []string{},
	}, nil
}

func isSubpath(path, parent string) bool {
	cleanPath := filepath.Clean(path)
	cleanParent := filepath.Clean(parent)
	if cleanPath == cleanParent {
		return false
	}
	return strings.HasPrefix(cleanPath, cleanParent+string(filepath.Separator))
}

// ParseJavaMajorFromOutput extracts the major version and full version string from 'java -version' output.
func ParseJavaMajorFromOutput(output string) (int, string) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "version") {
			parts := strings.Split(line, "\"")
			if len(parts) >= 2 {
				fullVersion := parts[1]
				major, _ := ParseJavaMajor(fullVersion) // errcheck:ok fallback to 0 on parse error
				return major, fullVersion
			}
		}
	}
	return 0, ""
}
