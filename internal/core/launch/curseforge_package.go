package launch

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

// CurseForge modpack .zip import (Feature D'4b, v0.7.1).
//
// Supported layouts:
//   - manifest.json (CurseForge export format): files + overrides + version;
//   - modlist.html (instances exported by the official launcher): only the
//     anchor list is parsed; project/file IDs are unavailable, so the plan
//     resolves nothing and the UI must present that honestly.
//
// Security invariants inherited from D'4a:
//   - zip entries are never written without a containment re-check (zip-slip);
//   - override names pass through domain.IsCredentialFilename (shared blocklist);
//   - an unavailable CurseForge resolver never yields a silent partial
//     install: every unresolvable file is reported in the plan and result.

const curseForgeOverridesPrefix = "overrides/"

var (
	ErrNotCurseForgeZip = errors.New("not a curseforge modpack zip (manifest.json or modlist.html missing/invalid)")
	ErrPackFileBlocked  = errors.New("pack file blocked by credential policy")
)

// CurseForgePackFile is one file entry from manifest.json, optionally enriched
// with a resolved download during Scan.
type CurseForgePackFile struct {
	ProjectID   int64  `json:"project_id"`
	FileID      int64  `json:"file_id"`
	FileName    string `json:"file_name,omitempty"`
	Required    bool   `json:"required"`
	DownloadURL string `json:"download_url,omitempty"`
	SHA1        string `json:"sha1,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	ResolveErr  string `json:"resolve_error,omitempty"`
}

// CurseForgePackPlan is the pre-commit scan result for the import modal.
type CurseForgePackPlan struct {
	ZipPath        string               `json:"zip_path"`
	Format         string               `json:"format"` // "manifest" | "modlist-html"
	InstanceName   string               `json:"instance_name"`
	GameVersion    string               `json:"game_version"`
	Loader         string               `json:"loader"`
	LoaderVersion  string               `json:"loader_version,omitempty"`
	Files          []CurseForgePackFile `json:"files"`
	Unresolved     []CurseForgePackFile `json:"unresolved"`
	OverrideNames  []string             `json:"override_names"`
	BlockedNames   []string             `json:"blocked_names"`
	RequiredTotal  int                  `json:"required_total"`
	RequiredFailed int                  `json:"required_failed"`
}

// CurseForgePackImportResult is the commit outcome.
type CurseForgePackImportResult struct {
	InstanceID    string   `json:"instance_id"`
	Downloaded    int      `json:"downloaded"`
	OverrideFiles int      `json:"override_files"`
	SkippedCred   []string `json:"skipped_credentials"`
	FailedFiles   []string `json:"failed_files"`
	Unresolved    []string `json:"unresolved"`
}

// CurseForgePackFileResolver maps (projectID, fileID) to a concrete download.
// Implemented by an adapter-side shim over the CurseForge API client.
type CurseForgePackFileResolver interface {
	ResolvePackFile(ctx context.Context, projectID, fileID int64) (url string, sha1 string, size int64, fileName string, err error)
}

// CurseForgePackImporter owns the scan + commit pipeline.
type CurseForgePackImporter struct {
	svc          *InstanceService
	http         ports.HTTPClient
	instancesDir string
	resolver     CurseForgePackFileResolver
}

func NewCurseForgePackImporter(svc *InstanceService, httpClient ports.HTTPClient, instancesDir string, resolver CurseForgePackFileResolver) *CurseForgePackImporter {
	if instancesDir == "" {
		instancesDir = "instances"
	}
	return &CurseForgePackImporter{svc: svc, http: httpClient, instancesDir: instancesDir, resolver: resolver}
}

type cfManifest struct {
	ManifestType string `json:"manifestType"`
	Name         string `json:"name"`
	Version      int    `json:"version"`
	Minecraft    struct {
		Version    string `json:"version"`
		ModLoaders []struct {
			ID string `json:"id"`
		} `json:"modLoaders"`
	} `json:"minecraft"`
	Files []struct {
		ProjectID int64 `json:"projectID"`
		FileID    int64 `json:"fileID"`
		Required  *bool `json:"required"`
	} `json:"files"`
}

var cfModlistAnchor = regexp.MustCompile(`href="https://www\.curseforge\.com/minecraft/mc-mods/([a-z0-9\-]+)"`)

// ScanCurseForgeZip reads the pack, and, when a resolver is attached, tries
// to resolve every file so the UI can show the real resolvable count upfront.
func (imp *CurseForgePackImporter) ScanCurseForgeZip(ctx context.Context, zipPath string) (*CurseForgePackPlan, error) {
	clean := filepath.Clean(zipPath)
	r, err := zip.OpenReader(clean)
	if err != nil {
		return nil, fmt.Errorf("open pack zip: %w", err)
	}
	defer r.Close()

	var manifestData []byte
	var modlistData []byte
	var overrideNames []string
	var blockedNames []string

	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)
		switch {
		case name == "manifest.json":
			if f.UncompressedSize64 > 8<<20 {
				return nil, fmt.Errorf("%w: manifest.json too large", ErrNotCurseForgeZip)
			}
			manifestData, err = readZipEntry(f)
			if err != nil {
				return nil, err
			}
		case name == "modlist.html":
			modlistData, err = readZipEntry(f)
			if err != nil {
				return nil, err
			}
		case strings.HasPrefix(name, curseForgeOverridesPrefix):
			rel := strings.TrimPrefix(name, curseForgeOverridesPrefix)
			if rel == "" || strings.HasSuffix(rel, "/") {
				continue
			}
			if err := validateOverrideRel(rel); err != nil {
				return nil, err
			}
			if domain.IsCredentialFilename(filepath.Base(rel)) {
				blockedNames = append(blockedNames, rel)
				continue
			}
			overrideNames = append(overrideNames, rel)
		}
	}

	plan := &CurseForgePackPlan{ZipPath: clean, OverrideNames: overrideNames, BlockedNames: blockedNames}

	if len(manifestData) > 0 {
		var m cfManifest
		if err := json.Unmarshal(manifestData, &m); err != nil || m.Minecraft.Version == "" {
			return nil, fmt.Errorf("%w: %v", ErrNotCurseForgeZip, err)
		}
		plan.Format = "manifest"
		plan.InstanceName = firstNonEmptyStr(m.Name, strings.TrimSuffix(filepath.Base(clean), filepath.Ext(clean)))
		plan.GameVersion = m.Minecraft.Version
		if len(m.Minecraft.ModLoaders) > 0 {
			loaderID := m.Minecraft.ModLoaders[0].ID
			if idx := strings.Index(loaderID, "-"); idx > 0 {
				plan.Loader = loaderID[:idx]
				plan.LoaderVersion = loaderID[idx+1:]
			} else {
				plan.Loader = loaderID
			}
		}
		for _, f := range m.Files {
			pf := CurseForgePackFile{ProjectID: f.ProjectID, FileID: f.FileID, Required: f.Required == nil || *f.Required}
			if pf.ProjectID <= 0 || pf.FileID <= 0 {
				pf.ResolveErr = "malformed entry ids"
				plan.Unresolved = append(plan.Unresolved, pf)
				continue
			}
			if imp.resolver != nil {
				url, sha1, size, fileName, rerr := imp.resolver.ResolvePackFile(ctx, f.ProjectID, f.FileID)
				if rerr != nil {
					pf.ResolveErr = rerr.Error()
				} else {
					pf.DownloadURL, pf.SHA1, pf.SizeBytes, pf.FileName = url, sha1, size, fileName
				}
			} else {
				pf.ResolveErr = "curseforge client not initialized"
			}
			if pf.DownloadURL == "" {
				plan.RequiredFailed++
				plan.Unresolved = append(plan.Unresolved, pf)
			} else {
				plan.Files = append(plan.Files, pf)
			}
			if pf.Required {
				plan.RequiredTotal++
			}
		}
		return plan, nil
	}

	if len(modlistData) > 0 {
		modNames := cfModlistAnchor.FindAllStringSubmatch(string(modlistData), -1)
		plan.Format = "modlist-html"
		plan.InstanceName = strings.TrimSuffix(filepath.Base(clean), filepath.Ext(clean))
		for _, mm := range modNames {
			plan.Unresolved = append(plan.Unresolved, CurseForgePackFile{
				FileName:   mm[1],
				Required:   true,
				ResolveErr: "modlist.html carries no project/file ids; pick manifest.json packs for automatic download",
			})
		}
		if len(modNames) == 0 {
			return nil, fmt.Errorf("%w: no manifest.json and no parsable modlist.html", ErrNotCurseForgeZip)
		}
		return plan, nil
	}

	return nil, fmt.Errorf("%w: no manifest.json or modlist.html in archive root", ErrNotCurseForgeZip)
}

// ImportCurseForgeZip commits a scanned plan: create instance, download the
// resolved files into mods/, extract overrides (blocklist re-applied).
// Required-file download failures are aborted loudly (error), unresolved
// entries are reported in the result instead of failing the import.
func (imp *CurseForgePackImporter) ImportCurseForgeZip(ctx context.Context, plan *CurseForgePackPlan) (*CurseForgePackImportResult, error) {
	if plan == nil {
		return nil, errors.New("nil plan")
	}
	if imp.svc == nil {
		return nil, errors.New("instance service not initialized")
	}
	if plan.Format != "manifest" {
		return nil, fmt.Errorf("plan format %q is not importable automatically", plan.Format)
	}
	if plan.GameVersion == "" {
		return nil, errors.New("plan has no game version")
	}

	loader := domain.LoaderVanilla
	switch plan.Loader {
	case "forge":
		loader = domain.LoaderForge
	case "neoforge":
		loader = domain.LoaderNeoForge
	case "fabric":
		loader = domain.LoaderFabric
	case "quilt":
		loader = domain.LoaderQuilt
	}

	inst, err := imp.svc.CreateInstance(plan.InstanceName, plan.GameVersion, loader)
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}
	destDir := filepath.Join(imp.instancesDir, inst.ID)
	if err := os.MkdirAll(filepath.Join(destDir, "mods"), 0o755); err != nil {
		return nil, fmt.Errorf("create mods dir: %w", err)
	}

	res := &CurseForgePackImportResult{InstanceID: inst.ID}
	for i := range plan.Unresolved {
		res.Unresolved = append(res.Unresolved, unresolvedLabel(&plan.Unresolved[i]))
	}

	// Extract overrides first so manual config tweaks survive even if a
	// later download fails and aborts the import.
	if err := imp.extractOverrides(plan, destDir, res); err != nil {
		return nil, err
	}

	for _, f := range plan.Files {
		fileName := sanitizePackFileName(f.FileName)
		if fileName == "" {
			fileName = fmt.Sprintf("cf-%d-%d.jar", f.ProjectID, f.FileID)
		}
		dest := filepath.Join(destDir, "mods", fileName)
		if err := imp.http.DownloadFile(ctx, f.DownloadURL, dest, f.SHA1, nil); err != nil {
			res.FailedFiles = append(res.FailedFiles, fileName)
			if f.Required {
				return res, fmt.Errorf("required mod %s download failed: %w", fileName, err)
			}
			continue
		}
		res.Downloaded++
	}

	return res, nil
}

func (imp *CurseForgePackImporter) extractOverrides(plan *CurseForgePackPlan, destDir string, res *CurseForgePackImportResult) error {
	r, err := zip.OpenReader(plan.ZipPath)
	if err != nil {
		return fmt.Errorf("reopen pack zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)
		if !strings.HasPrefix(name, curseForgeOverridesPrefix) {
			continue
		}
		rel := strings.TrimPrefix(name, curseForgeOverridesPrefix)
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}
		if err := validateOverrideRel(rel); err != nil {
			return err
		}
		if domain.IsCredentialFilename(filepath.Base(rel)) {
			res.SkippedCred = append(res.SkippedCred, rel)
			continue
		}
		target := filepath.Join(destDir, filepath.FromSlash(rel))
		if !strings.HasPrefix(target, destDir+string(os.PathSeparator)) && target != destDir {
			return fmt.Errorf("%w: override %q escapes instance dir", ErrPathTraversal, rel)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("mkdir override dir: %w", err)
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open override entry: %w", err)
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			rc.Close()
			return fmt.Errorf("create override file: %w", err)
		}
		_, copyErr := io.Copy(out, io.LimitReader(rc, 64<<20))
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return fmt.Errorf("write override %s: %w", rel, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close override %s: %w", rel, closeErr)
		}
		res.OverrideFiles++
	}
	sort.Strings(res.SkippedCred)
	return nil
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read zip entry %s: %w", f.Name, err)
	}
	return data, nil
}

func validateOverrideRel(rel string) error {
	slashed := filepath.ToSlash(rel)
	if strings.HasPrefix(slashed, "/") {
		return fmt.Errorf("%w: absolute override path %q", ErrPathTraversal, rel)
	}
	// Component-wise gate: ".." anywhere is fatal (Clean("/..") would
	// silently collapse the escape and hide the attack).
	for _, part := range strings.Split(slashed, "/") {
		if part == ".." {
			return fmt.Errorf("%w: parent-directory component in override %q", ErrPathTraversal, rel)
		}
	}
	clean := strings.TrimPrefix(filepath.Clean("/"+slashed), "/")
	if clean == "" {
		return fmt.Errorf("%w: empty override path %q", ErrPathTraversal, rel)
	}
	return nil
}

func sanitizePackFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	if name == "." || name == "/" || name == string(filepath.Separator) {
		return ""
	}
	if strings.Contains(name, "..") {
		return ""
	}
	return name
}

func unresolvedLabel(f *CurseForgePackFile) string {
	if f.FileName != "" {
		return f.FileName + " (" + f.ResolveErr + ")"
	}
	return fmt.Sprintf("cf:%d/%d (%s)", f.ProjectID, f.FileID, f.ResolveErr)
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
