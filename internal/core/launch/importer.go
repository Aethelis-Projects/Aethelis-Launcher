package launch

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nord-launcher/launcher/internal/core/domain"
)

var (
	ErrInvalidSourceDir        = errors.New("invalid source directory")
	ErrPathTraversal           = errors.New("path traversal detected")
	ErrInstanceNotFoundInPrism = errors.New("no prism or multimc instance found at path")
)

// MinecraftImportSummary contains metadata scanned from official .minecraft folder.
type MinecraftImportSummary struct {
	Path           string   `json:"path"`
	Versions       []string `json:"versions"`
	DefaultVersion string   `json:"default_version"`
	WorldCount     int      `json:"world_count"`
	ResourcePacks  int      `json:"resource_packs"`
	Screenshots    int      `json:"screenshots"`
	ModCount       int      `json:"mod_count"`
	HasOptions     bool     `json:"has_options"`
	HasServers     bool     `json:"has_servers"`
}

// ImportOfficialRequest specifies parameters for importing from .minecraft.
type ImportOfficialRequest struct {
	SourceDir         string `json:"source_dir"`
	InstanceName      string `json:"instance_name"`
	GameVersion       string `json:"game_version"`
	Loader            string `json:"loader"`
	CopySaves         bool   `json:"copy_saves"`
	CopyResourcePacks bool   `json:"copy_resource_packs"`
	CopyScreenshots   bool   `json:"copy_screenshots"`
	CopyMods          bool   `json:"copy_mods"`
	CopyOptions       bool   `json:"copy_options"`
	CopyServers       bool   `json:"copy_servers"`
}

// PrismImportSummary contains metadata scanned from a Prism or MultiMC instance.
type PrismImportSummary struct {
	Path          string `json:"path"`
	InstanceName  string `json:"instance_name"`
	GameVersion   string `json:"game_version"`
	Loader        string `json:"loader"`
	LoaderVersion string `json:"loader_version,omitempty"`
	WorldCount    int    `json:"world_count"`
	ResourcePacks int    `json:"resource_packs"`
	Screenshots   int    `json:"screenshots"`
	ModCount      int    `json:"mod_count"`
	HasOptions    bool   `json:"has_options"`
	HasServers    bool   `json:"has_servers"`
}

// ImportPrismRequest specifies parameters for importing from a Prism / MultiMC instance.
type ImportPrismRequest struct {
	SourceDir         string `json:"source_dir"`
	InstanceName      string `json:"instance_name"`
	CopySaves         bool   `json:"copy_saves"`
	CopyResourcePacks bool   `json:"copy_resource_packs"`
	CopyScreenshots   bool   `json:"copy_screenshots"`
	CopyMods          bool   `json:"copy_mods"`
	CopyOptions       bool   `json:"copy_options"`
	CopyServers       bool   `json:"copy_servers"`
}

type mmcPack struct {
	Components []struct {
		CachedName    string `json:"cachedName"`
		CachedVersion string `json:"cachedVersion"`
		UID           string `json:"uid"`
		Version       string `json:"version"`
	} `json:"components"`
}

// InstanceImporter handles migrating content from official Minecraft and Prism Launcher.
type InstanceImporter struct {
	svc          *InstanceService
	instancesDir string
}

func NewInstanceImporter(svc *InstanceService, instancesDir string) *InstanceImporter {
	return &InstanceImporter{
		svc:          svc,
		instancesDir: instancesDir,
	}
}

// DefaultOfficialMinecraftPath detects default .minecraft path for the current OS.
func DefaultOfficialMinecraftPath() string {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			return filepath.Join(appData, ".minecraft")
		}
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "AppData", "Roaming", ".minecraft")
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "Application Support", "minecraft")
		}
	default:
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".minecraft")
		}
	}
	return ""
}

// ScanOfficialMinecraft inspects the given path (or default path) for Minecraft content.
func (imp *InstanceImporter) ScanOfficialMinecraft(dirPath string) (*MinecraftImportSummary, error) {
	if dirPath == "" {
		dirPath = DefaultOfficialMinecraftPath()
	}
	if dirPath == "" {
		return nil, fmt.Errorf("%w: default path could not be resolved", ErrInvalidSourceDir)
	}

	cleanPath := filepath.Clean(dirPath)
	info, err := os.Stat(cleanPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: directory does not exist: %s", ErrInvalidSourceDir, cleanPath)
	}

	summary := &MinecraftImportSummary{
		Path: cleanPath,
	}

	// 1. Scan versions
	versionsDir := filepath.Join(cleanPath, "versions")
	if entries, err := os.ReadDir(versionsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				summary.Versions = append(summary.Versions, e.Name())
			}
		}
	}
	if len(summary.Versions) > 0 {
		summary.DefaultVersion = summary.Versions[len(summary.Versions)-1]
	}

	// 2. Count worlds in saves/
	savesDir := filepath.Join(cleanPath, "saves")
	if entries, err := os.ReadDir(savesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				levelDat := filepath.Join(savesDir, e.Name(), "level.dat")
				if _, err := os.Stat(levelDat); err == nil {
					summary.WorldCount++
				}
			}
		}
	}

	// 3. Count resourcepacks/
	rpDir := filepath.Join(cleanPath, "resourcepacks")
	if entries, err := os.ReadDir(rpDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
				summary.ResourcePacks++
			}
		}
	}

	// 4. Count screenshots/
	screenshotsDir := filepath.Join(cleanPath, "screenshots")
	if entries, err := os.ReadDir(screenshotsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
				summary.Screenshots++
			}
		}
	}

	// 5. Count mods/
	modsDir := filepath.Join(cleanPath, "mods")
	if entries, err := os.ReadDir(modsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(strings.ToLower(e.Name()), ".jar") || strings.HasSuffix(strings.ToLower(e.Name()), ".jar.disabled")) {
				summary.ModCount++
			}
		}
	}

	// 6. Check options and servers
	if _, err := os.Stat(filepath.Join(cleanPath, "options.txt")); err == nil {
		summary.HasOptions = true
	}
	if _, err := os.Stat(filepath.Join(cleanPath, "servers.dat")); err == nil {
		summary.HasServers = true
	}

	return summary, nil
}

// ImportOfficialMinecraft creates an isolated instance and copies selected content from .minecraft.
func (imp *InstanceImporter) ImportOfficialMinecraft(ctx context.Context, req ImportOfficialRequest) (*domain.Instance, error) {
	if req.SourceDir == "" {
		req.SourceDir = DefaultOfficialMinecraftPath()
	}
	cleanSrc := filepath.Clean(req.SourceDir)
	srcInfo, err := os.Stat(cleanSrc)
	if err != nil || !srcInfo.IsDir() {
		return nil, fmt.Errorf("%w: source path not found: %s", ErrInvalidSourceDir, cleanSrc)
	}

	if req.InstanceName == "" {
		req.InstanceName = "Imported Minecraft"
	}
	if req.GameVersion == "" {
		req.GameVersion = "1.21.1"
	}

	loaderType := domain.LoaderVanilla
	if req.Loader != "" {
		loaderType = domain.LoaderType(req.Loader)
	}

	inst, err := imp.svc.CreateInstance(req.InstanceName, req.GameVersion, loaderType)
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}

	destDir := filepath.Join(imp.instancesDir, inst.ID)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create instance destination directory: %w", err)
	}

	// Copy requested components
	if req.CopySaves {
		srcSaves := filepath.Join(cleanSrc, "saves")
		destSaves := filepath.Join(destDir, "saves")
		if err := copyDirectorySafely(srcSaves, destSaves); err != nil {
			return nil, fmt.Errorf("copy saves: %w", err)
		}
	}

	if req.CopyResourcePacks {
		srcRP := filepath.Join(cleanSrc, "resourcepacks")
		destRP := filepath.Join(destDir, "resourcepacks")
		if err := copyDirectorySafely(srcRP, destRP); err != nil {
			return nil, fmt.Errorf("copy resourcepacks: %w", err)
		}
	}

	if req.CopyScreenshots {
		srcScreenshots := filepath.Join(cleanSrc, "screenshots")
		destScreenshots := filepath.Join(destDir, "screenshots")
		if err := copyDirectorySafely(srcScreenshots, destScreenshots); err != nil {
			return nil, fmt.Errorf("copy screenshots: %w", err)
		}
	}

	if req.CopyMods {
		srcMods := filepath.Join(cleanSrc, "mods")
		destMods := filepath.Join(destDir, "mods")
		if err := copyDirectorySafely(srcMods, destMods); err != nil {
			return nil, fmt.Errorf("copy mods: %w", err)
		}
	}

	if req.CopyOptions {
		srcOptions := filepath.Join(cleanSrc, "options.txt")
		destOptions := filepath.Join(destDir, "options.txt")
		if err := copyFileSafely(srcOptions, destOptions); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("copy options.txt: %w", err)
		}
	}

	if req.CopyServers {
		srcServers := filepath.Join(cleanSrc, "servers.dat")
		destServers := filepath.Join(destDir, "servers.dat")
		if err := copyFileSafely(srcServers, destServers); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("copy servers.dat: %w", err)
		}
	}

	return inst, nil
}

// ScanPrismInstance inspects a Prism or MultiMC instance folder.
func (imp *InstanceImporter) ScanPrismInstance(dirPath string) (*PrismImportSummary, error) {
	cleanPath := filepath.Clean(dirPath)
	info, err := os.Stat(cleanPath)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("%w: path is not a directory: %s", ErrInvalidSourceDir, cleanPath)
	}

	summary := &PrismImportSummary{
		Path:         cleanPath,
		InstanceName: filepath.Base(cleanPath),
		GameVersion:  "1.21.1",
		Loader:       "vanilla",
	}

	// 1. Read instance.cfg if present
	cfgPath := filepath.Join(cleanPath, "instance.cfg")
	if f, err := os.Open(cfgPath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			k := strings.TrimSpace(parts[0])
			v := strings.TrimSpace(parts[1])
			switch k {
			case "name":
				if v != "" {
					summary.InstanceName = v
				}
			case "IntendedVersion", "MinecraftVersion":
				if v != "" {
					summary.GameVersion = v
				}
			}
		}
	}

	// 2. Read mmc-pack.json if present
	packPath := filepath.Join(cleanPath, "mmc-pack.json")
	if data, err := os.ReadFile(packPath); err == nil {
		var pack mmcPack
		if err := json.Unmarshal(data, &pack); err == nil {
			for _, comp := range pack.Components {
				switch comp.UID {
				case "net.minecraft":
					if comp.Version != "" {
						summary.GameVersion = comp.Version
					}
				case "net.fabricmc.fabric-loader":
					summary.Loader = "fabric"
					summary.LoaderVersion = comp.Version
				case "net.minecraftforge":
					summary.Loader = "forge"
					summary.LoaderVersion = comp.Version
				case "net.neoforged", "net.neoforged.neoforge":
					summary.Loader = "neoforge"
					summary.LoaderVersion = comp.Version
				case "org.quiltmc.quilt-loader":
					summary.Loader = "quilt"
					summary.LoaderVersion = comp.Version
				}
			}
		}
	}

	// 3. Locate the game content directory (.minecraft or minecraft or root)
	contentDir := cleanPath
	if sub := filepath.Join(cleanPath, ".minecraft"); dirExists(sub) {
		contentDir = sub
	} else if sub := filepath.Join(cleanPath, "minecraft"); dirExists(sub) {
		contentDir = sub
	}

	// Count saves
	savesDir := filepath.Join(contentDir, "saves")
	if entries, err := os.ReadDir(savesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				levelDat := filepath.Join(savesDir, e.Name(), "level.dat")
				if _, err := os.Stat(levelDat); err == nil {
					summary.WorldCount++
				}
			}
		}
	}

	// Count resourcepacks
	rpDir := filepath.Join(contentDir, "resourcepacks")
	if entries, err := os.ReadDir(rpDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
				summary.ResourcePacks++
			}
		}
	}

	// Count screenshots
	screenshotsDir := filepath.Join(contentDir, "screenshots")
	if entries, err := os.ReadDir(screenshotsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
				summary.Screenshots++
			}
		}
	}

	// Count mods
	modsDir := filepath.Join(contentDir, "mods")
	if entries, err := os.ReadDir(modsDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(strings.ToLower(e.Name()), ".jar") || strings.HasSuffix(strings.ToLower(e.Name()), ".jar.disabled")) {
				summary.ModCount++
			}
		}
	}

	// Options & servers
	if _, err := os.Stat(filepath.Join(contentDir, "options.txt")); err == nil {
		summary.HasOptions = true
	}
	if _, err := os.Stat(filepath.Join(contentDir, "servers.dat")); err == nil {
		summary.HasServers = true
	}

	return summary, nil
}

// ImportPrismInstance creates an instance and copies components from a Prism / MultiMC instance.
func (imp *InstanceImporter) ImportPrismInstance(ctx context.Context, req ImportPrismRequest) (*domain.Instance, error) {
	cleanSrc := filepath.Clean(req.SourceDir)
	srcInfo, err := os.Stat(cleanSrc)
	if err != nil || !srcInfo.IsDir() {
		return nil, fmt.Errorf("%w: source path not found: %s", ErrInvalidSourceDir, cleanSrc)
	}

	// Scan first to extract game version and loader
	scanned, err := imp.ScanPrismInstance(cleanSrc)
	if err != nil {
		return nil, fmt.Errorf("scan prism instance: %w", err)
	}

	instanceName := req.InstanceName
	if instanceName == "" {
		instanceName = scanned.InstanceName
	}
	if instanceName == "" {
		instanceName = "Imported Prism Instance"
	}

	loaderType := domain.LoaderType(scanned.Loader)
	if loaderType == "" {
		loaderType = domain.LoaderVanilla
	}

	inst, err := imp.svc.CreateInstance(instanceName, scanned.GameVersion, loaderType)
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}

	if scanned.LoaderVersion != "" {
		if updated, err := imp.svc.UpdateInstance(ctx, UpdateInstanceParams{
			ID: scanned.LoaderVersion,
		}); err == nil {
			inst = updated
		}
	}

	// Resolve game content dir
	contentDir := cleanSrc
	if sub := filepath.Join(cleanSrc, ".minecraft"); dirExists(sub) {
		contentDir = sub
	} else if sub := filepath.Join(cleanSrc, "minecraft"); dirExists(sub) {
		contentDir = sub
	}

	destDir := filepath.Join(imp.instancesDir, inst.ID)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("create instance destination directory: %w", err)
	}

	if req.CopySaves {
		srcSaves := filepath.Join(contentDir, "saves")
		destSaves := filepath.Join(destDir, "saves")
		if err := copyDirectorySafely(srcSaves, destSaves); err != nil {
			return nil, fmt.Errorf("copy saves: %w", err)
		}
	}

	if req.CopyResourcePacks {
		srcRP := filepath.Join(contentDir, "resourcepacks")
		destRP := filepath.Join(destDir, "resourcepacks")
		if err := copyDirectorySafely(srcRP, destRP); err != nil {
			return nil, fmt.Errorf("copy resourcepacks: %w", err)
		}
	}

	if req.CopyScreenshots {
		srcScreenshots := filepath.Join(contentDir, "screenshots")
		destScreenshots := filepath.Join(destDir, "screenshots")
		if err := copyDirectorySafely(srcScreenshots, destScreenshots); err != nil {
			return nil, fmt.Errorf("copy screenshots: %w", err)
		}
	}

	if req.CopyMods {
		srcMods := filepath.Join(contentDir, "mods")
		destMods := filepath.Join(destDir, "mods")
		if err := copyDirectorySafely(srcMods, destMods); err != nil {
			return nil, fmt.Errorf("copy mods: %w", err)
		}
	}

	if req.CopyOptions {
		srcOptions := filepath.Join(contentDir, "options.txt")
		destOptions := filepath.Join(destDir, "options.txt")
		if err := copyFileSafely(srcOptions, destOptions); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("copy options.txt: %w", err)
		}
	}

	if req.CopyServers {
		srcServers := filepath.Join(contentDir, "servers.dat")
		destServers := filepath.Join(destDir, "servers.dat")
		if err := copyFileSafely(srcServers, destServers); err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("copy servers.dat: %w", err)
		}
	}

	return inst, nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// copyFileSafely copies a single file with path traversal check and credential sanitization.
func copyFileSafely(src, dst string) error {
	cleanSrc := filepath.Clean(src)
	cleanDst := filepath.Clean(dst)

	// Block credentials/token leakage
	baseName := strings.ToLower(filepath.Base(cleanSrc))
	if baseName == "launcher_accounts.json" ||
		baseName == "launcher_msa_credentials.bin" ||
		baseName == "usercache.json" ||
		strings.HasSuffix(baseName, ".token") ||
		strings.HasSuffix(baseName, ".auth") {
		return nil
	}

	info, err := os.Stat(cleanSrc)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("source is a directory, not a file: %s", cleanSrc)
	}

	if err := os.MkdirAll(filepath.Dir(cleanDst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(cleanSrc)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(cleanDst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}

// copyDirectorySafely recursively copies a directory with traversal guards.
func copyDirectorySafely(srcDir, dstDir string) error {
	cleanSrc := filepath.Clean(srcDir)
	cleanDst := filepath.Clean(dstDir)

	info, err := os.Stat(cleanSrc)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("source is not a directory: %s", cleanSrc)
	}

	return filepath.Walk(cleanSrc, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(cleanSrc, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if strings.Contains(rel, "..") {
			return ErrPathTraversal
		}

		target := filepath.Join(cleanDst, rel)

		// Sanitize credentials
		baseName := strings.ToLower(filepath.Base(path))
		if baseName == "launcher_accounts.json" ||
			baseName == "launcher_msa_credentials.bin" ||
			baseName == "usercache.json" ||
			strings.HasSuffix(baseName, ".token") ||
			strings.HasSuffix(baseName, ".auth") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		return copyFileSafely(path, target)
	})
}
