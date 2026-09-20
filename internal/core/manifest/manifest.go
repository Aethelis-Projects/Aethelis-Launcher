package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	CurrentSchemaVersion = 1
	ManifestFileName     = "nord-installs.json"
)

var (
	ErrUnsupportedSchemaVersion = errors.New("unsupported manifest schema version")
)

type ModRecord struct {
	ModID       string    `json:"mod_id"`
	ModSlug     string    `json:"mod_slug,omitempty"`
	ModName     string    `json:"mod_name,omitempty"`
	FileName    string    `json:"file_name"`
	Source      string    `json:"source"`
	VersionID   string    `json:"version_id,omitempty"`
	ReleaseType string    `json:"release_type,omitempty"`
	InstalledAt time.Time `json:"installed_at"`
}

type InstallsManifest struct {
	SchemaVersion int                   `json:"schema_version"`
	Mods          map[string]*ModRecord `json:"mods"`
	mu            sync.RWMutex
}

func NewManifest() *InstallsManifest {
	return &InstallsManifest{
		SchemaVersion: CurrentSchemaVersion,
		Mods:          make(map[string]*ModRecord),
	}
}

// CleanModKey produces a normalized canonical lookup key by stripping .disabled and .jar extensions.
func CleanModKey(fileName string) string {
	name := strings.TrimSuffix(fileName, ".disabled")
	name = strings.TrimSuffix(name, ".jar")
	return strings.ToLower(strings.TrimSpace(name))
}

// LoadManifest reads and parses nord-installs.json in the specified mods directory.
// If the file does not exist, an empty manifest with CurrentSchemaVersion is returned without error.
// If the schema version exceeds CurrentSchemaVersion, ErrUnsupportedSchemaVersion is returned.
func LoadManifest(modsDir string) (*InstallsManifest, error) {
	manifestPath := filepath.Join(modsDir, ManifestFileName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewManifest(), nil
		}
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m InstallsManifest
	if err := json.Unmarshal(data, &m); err != nil {
		// Corrupted or malformed manifest: return fresh instance rather than crashing
		return NewManifest(), nil
	}

	if m.SchemaVersion > CurrentSchemaVersion {
		return nil, fmt.Errorf("%w: version %d (max supported %d)", ErrUnsupportedSchemaVersion, m.SchemaVersion, CurrentSchemaVersion)
	}

	if m.Mods == nil {
		m.Mods = make(map[string]*ModRecord)
	}

	return &m, nil
}

// Save atomically writes the manifest to nord-installs.json in modsDir using a temporary file.
func (m *InstallsManifest) Save(modsDir string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if err := os.MkdirAll(modsDir, 0755); err != nil {
		return fmt.Errorf("create mods dir: %w", err)
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}

	tempFile, err := os.CreateTemp(modsDir, ".nord-installs-tmp-*.json")
	if err != nil {
		return fmt.Errorf("create temp manifest file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close() // errcheck:ok safe to ignore close error if already closed
		if _, statErr := os.Stat(tempPath); statErr == nil {
			_ = os.Remove(tempPath) // errcheck:ok cleanup temp file on failure
		}
	}()

	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("write temp manifest: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("sync temp manifest: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp manifest: %w", err)
	}

	destPath := filepath.Join(modsDir, ManifestFileName)
	// On Windows, rename over an existing file may fail if not removed first in older Go
	if _, err := os.Stat(destPath); err == nil {
		_ = os.Remove(destPath) // errcheck:ok best effort remove before atomic rename
	}

	if err := os.Rename(tempPath, destPath); err != nil {
		return fmt.Errorf("finalize manifest rename: %w", err)
	}

	return nil
}

// AddOrUpdate inserts or updates a mod record keyed by its canonical clean name.
func (m *InstallsManifest) AddOrUpdate(rec *ModRecord) {
	if rec == nil || rec.FileName == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Mods == nil {
		m.Mods = make(map[string]*ModRecord)
	}
	key := CleanModKey(rec.FileName)
	m.Mods[key] = rec
}

// Remove removes the record matching the clean name from the manifest.
func (m *InstallsManifest) Remove(fileName string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Mods == nil {
		return
	}
	key := CleanModKey(fileName)
	delete(m.Mods, key)
}

// GetRecord returns a copy of the record matching the filename or nil if not found.
func (m *InstallsManifest) GetRecord(fileName string) *ModRecord {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.Mods == nil {
		return nil
	}
	key := CleanModKey(fileName)
	if rec, ok := m.Mods[key]; ok && rec != nil {
		copy := *rec
		return &copy
	}
	return nil
}

// ReconcileWithDisk audits the manifest against the actual files on disk in modsDir.
// The filesystem is the source of truth:
// 1. Files on disk not tracked in the manifest are synthesized and added.
// 2. Tracked records missing from disk (neither .jar nor .jar.disabled) are removed.
// 3. Status changes (.jar <-> .jar.disabled) update the record's FileName.
// Returns true if any changes were made to the manifest.
func (m *InstallsManifest) ReconcileWithDisk(modsDir string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read mods directory for reconcile: %w", err)
	}

	changed := false
	diskFiles := make(map[string]os.DirEntry)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".jar") || strings.HasSuffix(name, ".jar.disabled") {
			key := CleanModKey(name)
			diskFiles[key] = entry
		}
	}

	if m.Mods == nil {
		m.Mods = make(map[string]*ModRecord)
	}

	// 1. Reconcile existing manifest entries against disk
	for key, rec := range m.Mods {
		entry, existsOnDisk := diskFiles[key]
		if !existsOnDisk {
			// File removed from disk externally -> remove from manifest
			delete(m.Mods, key)
			changed = true
		} else {
			// File exists: check if filename toggled (.jar <-> .jar.disabled)
			actualName := entry.Name()
			if rec.FileName != actualName {
				rec.FileName = actualName
				changed = true
			}
		}
	}

	// 2. Discover unmanifested files on disk (manual drops)
	for key, entry := range diskFiles {
		if _, tracked := m.Mods[key]; !tracked {
			name := entry.Name()
			cleanName := strings.TrimSuffix(strings.TrimSuffix(name, ".disabled"), ".jar")
			modTime := time.Now()
			if info, err := entry.Info(); err == nil {
				modTime = info.ModTime()
			}
			m.Mods[key] = &ModRecord{
				ModID:       cleanName,
				ModName:     cleanName,
				FileName:    name,
				Source:      "local",
				InstalledAt: modTime,
			}
			changed = true
		}
	}

	return changed, nil
}
