package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type InstalledModRecord struct {
	InstanceID  string    `json:"instance_id"`
	ModID       string    `json:"mod_id"`
	FileName    string    `json:"file_name"`
	Source      string    `json:"source"`
	VersionID   string    `json:"version_id"`
	ReleaseType string    `json:"release_type"`
	InstalledAt time.Time `json:"installed_at"`
}

type InstalledModsRepository struct {
	db *sql.DB
}

func NewInstalledModsRepository(db *Database) *InstalledModsRepository {
	if db == nil || db.DB() == nil {
		return nil
	}
	return &InstalledModsRepository{db: db.DB()}
}

func NewInstalledModsRepositoryFromDB(sqlDB *sql.DB) *InstalledModsRepository {
	if sqlDB == nil {
		return nil
	}
	return &InstalledModsRepository{db: sqlDB}
}

func (r *InstalledModsRepository) Save(ctx context.Context, rec InstalledModRecord) error {
	if r == nil || r.db == nil {
		return nil
	}
	query := `
	INSERT INTO installed_mods (instance_id, mod_id, file_name, source, version_id, release_type, installed_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(instance_id, file_name) DO UPDATE SET
		mod_id = excluded.mod_id,
		source = excluded.source,
		version_id = excluded.version_id,
		release_type = excluded.release_type,
		installed_at = excluded.installed_at;
	`
	_, err := r.db.ExecContext(ctx, query,
		rec.InstanceID, rec.ModID, rec.FileName, rec.Source, rec.VersionID, rec.ReleaseType, rec.InstalledAt,
	)
	if err != nil {
		return fmt.Errorf("save installed mod: %w", err)
	}
	return nil
}

func (r *InstalledModsRepository) Delete(ctx context.Context, instanceID, fileName string) error {
	if r == nil || r.db == nil {
		return nil
	}
	query := `DELETE FROM installed_mods WHERE instance_id = ? AND file_name = ?;`
	_, err := r.db.ExecContext(ctx, query, instanceID, fileName)
	if err != nil {
		return fmt.Errorf("delete installed mod: %w", err)
	}
	return nil
}

// DeleteByCleanName removes any records matching either .jar or .jar.disabled for cleanName.
func (r *InstalledModsRepository) DeleteByCleanName(ctx context.Context, instanceID, cleanName string) error {
	if r == nil || r.db == nil {
		return nil
	}
	clean := strings.TrimSuffix(strings.TrimSuffix(cleanName, ".disabled"), ".jar")
	query := `DELETE FROM installed_mods WHERE instance_id = ? AND (file_name = ? OR file_name = ? OR file_name LIKE ?);`
	_, err := r.db.ExecContext(ctx, query, instanceID, clean+".jar", clean+".jar.disabled", clean+"%")
	if err != nil {
		return fmt.Errorf("delete installed mod by clean name: %w", err)
	}
	return nil
}

func (r *InstalledModsRepository) UpdateFileName(ctx context.Context, instanceID, oldFileName, newFileName string) error {
	if r == nil || r.db == nil {
		return nil
	}
	query := `UPDATE installed_mods SET file_name = ? WHERE instance_id = ? AND file_name = ?;`
	_, err := r.db.ExecContext(ctx, query, newFileName, instanceID, oldFileName)
	if err != nil {
		return fmt.Errorf("update installed mod file name: %w", err)
	}
	return nil
}

func (r *InstalledModsRepository) GetByInstance(ctx context.Context, instanceID string) ([]InstalledModRecord, error) {
	if r == nil || r.db == nil {
		return nil, nil
	}
	query := `SELECT instance_id, mod_id, file_name, source, version_id, release_type, installed_at FROM installed_mods WHERE instance_id = ?;`
	rows, err := r.db.QueryContext(ctx, query, instanceID)
	if err != nil {
		return nil, fmt.Errorf("query installed mods: %w", err)
	}
	defer rows.Close()

	var records []InstalledModRecord
	for rows.Next() {
		var rec InstalledModRecord
		if err := rows.Scan(&rec.InstanceID, &rec.ModID, &rec.FileName, &rec.Source, &rec.VersionID, &rec.ReleaseType, &rec.InstalledAt); err != nil {
			return nil, fmt.Errorf("scan installed mod: %w", err)
		}
		records = append(records, rec)
	}
	return records, nil
}

func (r *InstalledModsRepository) SyncInstance(ctx context.Context, instanceID string, activeFileNames []string) error {
	if r == nil || r.db == nil {
		return nil
	}
	if len(activeFileNames) == 0 {
		_, err := r.db.ExecContext(ctx, `DELETE FROM installed_mods WHERE instance_id = ?;`, instanceID)
		if err != nil {
			return fmt.Errorf("purge installed mods for instance: %w", err)
		}
		return nil
	}

	placeholders := make([]string, len(activeFileNames))
	args := make([]interface{}, len(activeFileNames)+1)
	args[0] = instanceID
	for i, f := range activeFileNames {
		placeholders[i] = "?"
		args[i+1] = f
	}

	query := fmt.Sprintf(`DELETE FROM installed_mods WHERE instance_id = ? AND file_name NOT IN (%s);`, strings.Join(placeholders, ","))
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("sync prune installed mods: %w", err)
	}
	return nil
}
