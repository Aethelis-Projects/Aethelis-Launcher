package game

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

// Integrity verification (Feature D'2, v0.7.1).
//
// Scope: Mojang-managed cache files - version JSON, client jar, rule-eligible
// libraries (with native classifiers) and the asset index plus its objects.
// Loader-side (Fabric) libraries and installed mods keep their own lifecycle
// (existence check in Provision, self-heal in the mods engine) and are
// deliberately not re-hashed here; the UI states this scope honestly.
//
// Virtual asset indexes (pre-1.13) are reported as skipped-by-size only:
// their objects live under virtual paths that only a full Provision expands,
// and re-running Provision repairs them.

const maxIntegrityProblemDetails = 25

type integrityReason string

const (
	integrityReasonMissing          integrityReason = "Missing"
	integrityReasonChecksumMismatch integrityReason = "ChecksumMismatch"
	integrityReasonSizeMismatch     integrityReason = "SizeMismatch"
)

type integrityFile struct {
	label        string
	path         string
	url          string
	sha1         string
	sizeBytes    int64
	sizeOnlyHash bool // hash unknown: verify by declared size only
}

type integrityProblem struct {
	file   integrityFile
	reason integrityReason
}

type integrityFinding = ports.IntegrityFinding

// IntegrityFinding re-exports the contract finding type for consumers.
type IntegrityFinding = ports.IntegrityFinding

// IntegrityResult is the domain-level outcome of a verify (or verify+repair) pass.
type IntegrityResult = ports.IntegrityResult

// collectIntegrityFiles resolves the version metadata and builds the full list
// of cacheable files for the instance's game version.
func (s *GameService) collectIntegrityFiles(ctx context.Context, inst *domain.Instance) ([]integrityFile, *AssetIndexMeta, error) {
	manifest, err := FetchVersionManifest(ctx, s.http, s.manifestURL)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch version manifest: %w", err)
	}
	entry, err := FindVersionEntry(manifest, inst.GameVersion)
	if err != nil {
		return nil, nil, err
	}

	versionDir := filepath.Join(s.dataDir, "versions", inst.GameVersion)
	versionFilePath := filepath.Join(versionDir, inst.GameVersion+".json")
	files := []integrityFile{{
		label: "Version JSON", path: versionFilePath, url: entry.URL, sha1: entry.SHA1,
	}}

	// The client jar and asset index are described inside the version JSON;
	// without it there is nothing to check, so fetch it through the normal
	// verify-and-download path first.
	if !s.verifyFileSHA1(versionFilePath, entry.SHA1) {
		if err := s.fs.MkdirAll(versionDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("mkdir versions dir: %w", err)
		}
		if err := s.http.DownloadFile(ctx, entry.URL, versionFilePath, entry.SHA1, nil); err != nil {
			return nil, nil, fmt.Errorf("download version metadata: %w", err)
		}
	}
	versionBytes, err := s.fs.ReadFile(versionFilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("read version json: %w", err)
	}
	var versionMeta domain.VersionJSON
	if err := json.Unmarshal(versionBytes, &versionMeta); err != nil {
		return nil, nil, fmt.Errorf("unmarshal version json: %w", err)
	}

	if versionMeta.Downloads.Client != nil && versionMeta.Downloads.Client.URL != "" {
		files = append(files, integrityFile{
			label: "Client JAR",
			path:  filepath.Join(versionDir, inst.GameVersion+".jar"),
			url:   versionMeta.Downloads.Client.URL,
			sha1:  versionMeta.Downloads.Client.SHA1,
		})
	}

	librariesDir := filepath.Join(s.dataDir, "libraries")
	for _, lib := range versionMeta.Libraries {
		if lib.ServerReq {
			continue
		}
		if !domain.EvaluateRules(lib.Rules, s.currentOS, s.currentArch, nil) {
			continue
		}
		var relPath, url, sha1 string
		if lib.Downloads.Artifact != nil && lib.Downloads.Artifact.URL != "" {
			relPath, url, sha1 = lib.Downloads.Artifact.Path, lib.Downloads.Artifact.URL, lib.Downloads.Artifact.SHA1
		} else if lib.Name != "" {
			relPath = MavenCoordinatesToPath(lib.Name)
			url = fmt.Sprintf("%s/%s", strings.TrimRight(s.librariesBaseURL, "/"), relPath)
		}
		if relPath != "" {
			files = append(files, integrityFile{
				label: "Library " + lib.Name,
				path:  filepath.Join(librariesDir, filepath.FromSlash(relPath)),
				url:   url, sha1: sha1,
			})
		}
		if len(lib.Natives) == 0 {
			continue
		}
		osKey := s.currentOS
		if osKey == "darwin" {
			osKey = "osx"
		}
		classifierTemplate, ok := lib.Natives[osKey]
		if !ok {
			continue
		}
		archKey := "64"
		if s.currentArch == "386" {
			archKey = "32"
		}
		classifier := strings.ReplaceAll(classifierTemplate, "${arch}", archKey)
		var nativeRel, nativeURL, nativeSHA1 string
		if nativeArt, ok := lib.Downloads.Classifiers[classifier]; ok && nativeArt.URL != "" {
			nativeRel, nativeURL, nativeSHA1 = nativeArt.Path, nativeArt.URL, nativeArt.SHA1
		} else if lib.Name != "" {
			nativeRel = MavenCoordinatesToPath(lib.Name + ":" + classifier)
			nativeURL = fmt.Sprintf("%s/%s", strings.TrimRight(s.librariesBaseURL, "/"), nativeRel)
		}
		if nativeRel != "" {
			files = append(files, integrityFile{
				label: "Native " + classifier + " of " + lib.Name,
				path:  filepath.Join(librariesDir, filepath.FromSlash(nativeRel)),
				url:   nativeURL, sha1: nativeSHA1,
			})
		}
	}

	meta := &AssetIndexMeta{}
	if versionMeta.AssetIndex.URL != "" {
		indexPath := filepath.Join(s.dataDir, "assets", "indexes", versionMeta.AssetIndex.ID+".json")
		files = append(files, integrityFile{
			label: "Asset Index", path: indexPath, url: versionMeta.AssetIndex.URL, sha1: versionMeta.AssetIndex.SHA1,
		})
		meta.indexPath = indexPath
	}
	return files, meta, nil
}

type AssetIndexMeta struct {
	indexPath string
}

// CheckInstanceFiles is the exported integrity entry point (verify only).
func (s *GameService) CheckInstanceFiles(ctx context.Context, inst *domain.Instance) (*IntegrityResult, error) {
	return s.runIntegrityPass(ctx, inst, false)
}

// FixInstanceFiles is the exported integrity entry point (verify + atomic repair).
func (s *GameService) FixInstanceFiles(ctx context.Context, inst *domain.Instance) (*IntegrityResult, error) {
	return s.runIntegrityPass(ctx, inst, true)
}

// VerifyInstanceFiles checks every Mojang-managed cache file of the instance
// against official SHA-1 (or declared size where the index provides no hash).
// It performs no downloads.
func (s *GameService) VerifyInstanceFiles(ctx context.Context, inst *domain.Instance) (*IntegrityResult, error) {
	return s.runIntegrityPass(ctx, inst, false)
}

// RepairInstanceFiles verifies and then re-downloads exactly the broken or
// missing files through the standard atomic downloader (sha1-enforced).
func (s *GameService) RepairInstanceFiles(ctx context.Context, inst *domain.Instance) (*IntegrityResult, error) {
	return s.runIntegrityPass(ctx, inst, true)
}

func (s *GameService) runIntegrityPass(ctx context.Context, inst *domain.Instance, repair bool) (*IntegrityResult, error) {
	if inst == nil {
		return nil, fmt.Errorf("instance cannot be nil")
	}
	files, meta, err := s.collectIntegrityFiles(ctx, inst)
	if err != nil {
		return nil, err
	}

	res := &IntegrityResult{GameVersion: inst.GameVersion}
	var problems []integrityProblem

	// Repair pass one: files known upfront (version json, client jar, libs).
	for _, f := range files {
		res.CheckedCount++
		problem, isProblem := s.checkIntegrityFile(f)
		if !isProblem {
			continue
		}
		if repair {
			if err := s.repairIntegrityFile(f); err == nil {
				res.RepairedCount++
				continue
			}
		}
		problems = append(problems, integrityProblem{file: f, reason: problem})
	}

	// Expand the asset index once, so its entries can be verified against the
	// same pass (previously a freshly repaired index was only visible to the
	// next run).
	var assetIndex AssetIndex
	indexUsable := false
	if meta.indexPath != "" {
		indexBytes, err := s.fs.ReadFile(meta.indexPath)
		if err == nil {
			indexUsable = json.Unmarshal(indexBytes, &assetIndex) == nil
			if assetIndex.Virtual {
				res.VirtualAssetsSkipped = true
			}
		}
	}

	if indexUsable {
		assetsDir := filepath.Join(s.dataDir, "assets")
		for _, obj := range assetIndex.Objects {
			if len(obj.Hash) < 2 || assetIndex.Virtual {
				continue
			}
			sub := obj.Hash[:2]
			objFile := integrityFile{
				label:     "Asset " + obj.Hash,
				path:      filepath.Join(assetsDir, "objects", sub, obj.Hash),
				url:       fmt.Sprintf("%s/%s/%s", strings.TrimRight(s.resourcesBaseURL, "/"), sub, obj.Hash),
				sha1:      obj.Hash,
				sizeBytes: obj.Size,
			}
			res.CheckedCount++
			problem, isProblem := s.checkIntegrityFile(objFile)
			if !isProblem {
				continue
			}
			if repair {
				if err := s.repairIntegrityFile(objFile); err == nil {
					res.RepairedCount++
					continue
				}
			}
			problems = append(problems, integrityProblem{file: objFile, reason: problem})
		}
	}

	sort.Slice(problems, func(i, j int) bool { return problems[i].file.path < problems[j].file.path })
	res.ProblemsCount = len(problems)
	if len(problems) > maxIntegrityProblemDetails {
		problems = problems[:maxIntegrityProblemDetails]
		res.ProblemsCapped = true
	}
	for _, p := range problems {
		res.Findings = append(res.Findings, integrityFinding{Path: p.file.label, Reason: string(p.reason)})
	}
	return res, nil
}

func (s *GameService) checkIntegrityFile(f integrityFile) (integrityReason, bool) {
	info, err := s.fs.Stat(f.path)
	if err != nil || info == nil {
		return integrityReasonMissing, true
	}
	if f.sha1 != "" {
		if !s.verifyFileSHA1(f.path, f.sha1) {
			return integrityReasonChecksumMismatch, true
		}
		return "", false
	}
	if f.sizeBytes > 0 && info.Size() != f.sizeBytes {
		return integrityReasonSizeMismatch, true
	}
	return "", false
}

func (s *GameService) repairIntegrityFile(f integrityFile) error {
	if f.url == "" {
		return fmt.Errorf("no repair URL for %s", f.path)
	}
	if err := s.fs.MkdirAll(filepath.Dir(f.path), 0755); err != nil {
		return err
	}
	return s.http.DownloadFile(context.Background(), f.url, f.path, f.sha1, nil)
}
