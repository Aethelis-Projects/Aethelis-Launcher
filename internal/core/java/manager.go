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
	client      *AdoptiumClient
	mu          sync.RWMutex
}

func NewJavaManager(
	managedDir string,
	detector ports.JavaDetector,
	instRepo ports.InstanceRepository,
	httpClient *http.Client,
) *JavaManager {
	client := NewAdoptiumClient(DefaultAdoptiumBaseURL, httpClient)
	provisioner := NewAdoptiumRuntimeService(managedDir, client, httpClient)
	return &JavaManager{
		managedDir:  managedDir,
		detector:    detector,
		instRepo:    instRepo,
		provisioner: provisioner,
		client:      client,
	}
}

func (m *JavaManager) SetProvisioner(p *AdoptiumRuntimeService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.provisioner = p
}

func (m *JavaManager) SetClient(c *AdoptiumClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.client = c
	if m.provisioner != nil {
		m.provisioner.SetClient(c)
	}
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

// CheckRuntimeUpdates inspects installed managed Java runtimes and checks if newer releases
// are available from Adoptium.
func (m *JavaManager) CheckRuntimeUpdates(ctx context.Context) ([]JavaRuntimeUpdate, error) {
	runtimes, err := m.ListRuntimes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: %w", err)
	}

	m.mu.RLock()
	client := m.client
	if client == nil && m.provisioner != nil {
		client = m.provisioner.Client()
	}
	m.mu.RUnlock()

	if client == nil {
		return nil, fmt.Errorf("adoptium client not initialized")
	}

	managedByMajor := make(map[int]ports.JavaInstallation)
	for _, r := range runtimes {
		if r.Kind == "managed" && r.MajorVersion > 0 {
			existing, exists := managedByMajor[r.MajorVersion]
			if !exists || IsNewerVersion(existing.FullVersion, r.FullVersion) {
				managedByMajor[r.MajorVersion] = r
			}
		}
	}

	updates := make([]JavaRuntimeUpdate, 0, len(managedByMajor))
	for major, inst := range managedByMajor {
		rel, err := client.GetLatestRelease(ctx, major)
		if err != nil {
			continue
		}
		isNewer := IsNewerVersion(inst.FullVersion, rel.Version)
		updates = append(updates, JavaRuntimeUpdate{
			MajorVersion:    major,
			CurrentVersion:  inst.FullVersion,
			LatestVersion:   rel.Version,
			UpdateAvailable: isNewer,
			DownloadURL:     rel.DownloadURL,
		})
	}

	sort.Slice(updates, func(i, j int) bool {
		return updates[i].MajorVersion > updates[j].MajorVersion
	})

	return updates, nil
}

// UpgradeRuntime provisions the latest release of the given major Java version, relinks all
// instances currently pointing to older managed runtimes of that major version, and removes obsolete runtimes.
// Refuses to upgrade if ANY instance using that runtime is currently running or launching (R5).
func (m *JavaManager) UpgradeRuntime(ctx context.Context, major int) (string, error) {
	if major <= 0 {
		return "", fmt.Errorf("invalid major version: %d", major)
	}

	m.mu.RLock()
	prov := m.provisioner
	cleanManaged := filepath.Clean(m.managedDir)
	m.mu.RUnlock()

	if prov == nil {
		return "", fmt.Errorf("java provisioner not initialized")
	}

	// 1. Guard against upgrading if ANY instance using a managed runtime of this major is running or launching
	if m.instRepo != nil {
		instances, err := m.instRepo.ListAll(ctx)
		if err == nil {
			for _, inst := range instances {
				if inst.JavaPath != "" {
					cleanPath := filepath.Clean(inst.JavaPath)
					if isSubpath(cleanPath, cleanManaged) {
						if strings.Contains(cleanPath, fmt.Sprintf("adoptium-%d-", major)) ||
							strings.Contains(cleanPath, fmt.Sprintf("adoptium-%d", major)) {
							if inst.State == domain.StateRunning || inst.State == domain.StateLaunching {
								return "", fmt.Errorf("cannot upgrade runtime: instance %q is currently %s with Java %d", inst.Name, inst.State, major)
							}
						}
					}
				}
			}
		}
	}

	// 2. Discover existing managed runtimes for this major
	var oldManagedPaths []string
	if runtimes, err := m.ListRuntimes(ctx); err == nil {
		for _, r := range runtimes {
			if r.Kind == "managed" && r.MajorVersion == major {
				oldManagedPaths = append(oldManagedPaths, filepath.Clean(r.Path))
			}
		}
	}

	// 3. Download and provision the latest release
	newJavaBin, err := prov.Download(ctx, major)
	if err != nil {
		return "", fmt.Errorf("failed to download upgraded runtime: %w", err)
	}
	cleanNewBin := filepath.Clean(newJavaBin)

	// 4. Relink instances in the repository
	if m.instRepo != nil {
		instances, err := m.instRepo.ListAll(ctx)
		if err == nil {
			for _, inst := range instances {
				if inst.JavaPath != "" {
					cleanPath := filepath.Clean(inst.JavaPath)
					shouldRelink := false
					for _, oldPath := range oldManagedPaths {
						if cleanPath == oldPath {
							shouldRelink = true
							break
						}
					}
					// Also relink if the instance points to an older adoptium directory for this major
					if !shouldRelink && isSubpath(cleanPath, cleanManaged) &&
						(strings.Contains(cleanPath, fmt.Sprintf("adoptium-%d-", major)) || strings.Contains(cleanPath, fmt.Sprintf("adoptium-%d", major))) {
						shouldRelink = true
					}

					if shouldRelink {
						inst.JavaPath = cleanNewBin
						_ = m.instRepo.Save(ctx, inst) // errcheck:ok relink instance java path
					}
				}
			}
		}
	}

	// 5. Clean up old runtime folders
	newDir := filepath.Dir(cleanNewBin)
	if strings.EqualFold(filepath.Base(newDir), "bin") {
		newDir = filepath.Dir(newDir)
	}

	for _, oldPath := range oldManagedPaths {
		if oldPath == cleanNewBin {
			continue
		}
		oldDir := filepath.Dir(oldPath)
		if strings.EqualFold(filepath.Base(oldDir), "bin") {
			oldDir = filepath.Dir(oldDir)
		}
		if oldDir != newDir && isSubpath(oldDir, cleanManaged) {
			_ = os.RemoveAll(oldDir) // errcheck:ok best-effort cleanup of superseded runtime
		}
	}

	return cleanNewBin, nil
}

// CleanUnusedRuntimes removes all managed runtimes that are not referenced by any instance,
// preserving safety guards against active instances and detected runtimes (R5).
func (m *JavaManager) CleanUnusedRuntimes(ctx context.Context) ([]string, error) {
	runtimes, err := m.ListRuntimes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list runtimes: %w", err)
	}

	var removed []string
	for _, r := range runtimes {
		if r.Kind == "managed" && len(r.UsedBy) == 0 {
			if err := m.RemoveRuntime(r.Path); err == nil {
				removed = append(removed, r.Path)
			}
		}
	}
	return removed, nil
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
