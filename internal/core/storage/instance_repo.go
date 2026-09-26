package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
)

type InstanceRepository struct {
	db *sql.DB
}

func NewInstanceRepository(db *Database) *InstanceRepository {
	return &InstanceRepository{db: db.DB()}
}

func (r *InstanceRepository) Save(ctx context.Context, inst *domain.Instance) error {
	jvmArgsJSON, err := json.Marshal(inst.JVMArgs)
	if err != nil {
		return fmt.Errorf("marshal jvm args: %w", err)
	}

	skipCheckInt := 0
	if inst.SkipJavaCheck {
		skipCheckInt = 1
	}

	favInt := 0
	if inst.IsFavorite {
		favInt = 1
	}

	query := `
	INSERT INTO instances (
		id, name, game_version, loader, loader_version, icon_path, java_path,
		min_ram_mb, max_ram_mb, jvm_args, skip_java_check, group_name, is_favorite, state, last_played_at, total_play_seconds,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		game_version = excluded.game_version,
		loader = excluded.loader,
		loader_version = excluded.loader_version,
		icon_path = excluded.icon_path,
		java_path = excluded.java_path,
		min_ram_mb = excluded.min_ram_mb,
		max_ram_mb = excluded.max_ram_mb,
		jvm_args = excluded.jvm_args,
		skip_java_check = excluded.skip_java_check,
		group_name = excluded.group_name,
		is_favorite = excluded.is_favorite,
		state = excluded.state,
		last_played_at = excluded.last_played_at,
		total_play_seconds = excluded.total_play_seconds,
		updated_at = excluded.updated_at;
	`

	var lastPlayed *string
	if inst.LastPlayedAt != nil {
		s := inst.LastPlayedAt.Format(time.RFC3339)
		lastPlayed = &s
	}

	_, err = r.db.ExecContext(ctx, query,
		inst.ID,
		inst.Name,
		inst.GameVersion,
		string(inst.Loader),
		inst.LoaderVer,
		inst.IconPath,
		inst.JavaPath,
		inst.MinRAMMB,
		inst.MaxRAMMB,
		string(jvmArgsJSON),
		skipCheckInt,
		inst.Group,
		favInt,
		string(inst.State),
		lastPlayed,
		inst.TotalPlaySec,
		inst.CreatedAt.Format(time.RFC3339),
		inst.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("execute save instance query: %w", err)
	}
	return nil
}

func (r *InstanceRepository) GetByID(ctx context.Context, id string) (*domain.Instance, error) {
	query := `
	SELECT
		id, name, game_version, loader, loader_version, icon_path, java_path,
		min_ram_mb, max_ram_mb, jvm_args, skip_java_check, group_name, is_favorite, state, last_played_at, total_play_seconds,
		created_at, updated_at
	FROM instances WHERE id = ?;
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanInstance(row)
}

func (r *InstanceRepository) ListAll(ctx context.Context) ([]*domain.Instance, error) {
	query := `
	SELECT
		id, name, game_version, loader, loader_version, icon_path, java_path,
		min_ram_mb, max_ram_mb, jvm_args, skip_java_check, group_name, is_favorite, state, last_played_at, total_play_seconds,
		created_at, updated_at
	FROM instances ORDER BY created_at DESC;
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query instances: %w", err)
	}
	defer rows.Close()

	var result []*domain.Instance
	for rows.Next() {
		inst, err := r.scanInstance(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return result, nil
}

func (r *InstanceRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM instances WHERE id = ?;", id)
	if err != nil {
		return fmt.Errorf("delete instance: %w", err)
	}
	return nil
}

func (r *InstanceRepository) UpdateState(ctx context.Context, id string, state domain.InstanceState) error {
	_, err := r.db.ExecContext(ctx, "UPDATE instances SET state = ?, updated_at = ? WHERE id = ?;",
		string(state), time.Now().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("update instance state: %w", err)
	}
	return nil
}

func (r *InstanceRepository) SetFavorite(ctx context.Context, id string, isFavorite bool) error {
	favInt := 0
	if isFavorite {
		favInt = 1
	}
	_, err := r.db.ExecContext(ctx, "UPDATE instances SET is_favorite = ?, updated_at = ? WHERE id = ?;",
		favInt, time.Now().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("update instance favorite: %w", err)
	}
	return nil
}

func (r *InstanceRepository) SetGroup(ctx context.Context, id string, group string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE instances SET group_name = ?, updated_at = ? WHERE id = ?;",
		group, time.Now().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("update instance group: %w", err)
	}
	return nil
}

// EnsureDefaultInstance seeds the default "Nordic Fabric 1.21" instance strictly when COUNT(instances) == 0 (N8).
func (r *InstanceRepository) EnsureDefaultInstance(ctx context.Context) (*domain.Instance, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM instances;").Scan(&count); err != nil {
		return nil, fmt.Errorf("count instances: %w", err)
	}

	if count > 0 {
		return nil, nil
	}

	now := time.Now()
	defaultInst := &domain.Instance{
		ID:           "default-fabric-1-21",
		Name:         "Nordic Fabric 1.21",
		GameVersion:  "1.21.1",
		Loader:       domain.LoaderFabric,
		LoaderVer:    "0.16.5",
		MinRAMMB:     2048,
		MaxRAMMB:     4096,
		JVMArgs:      []string{},
		State:        domain.StateIdle,
		TotalPlaySec: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := r.Save(ctx, defaultInst); err != nil {
		return nil, fmt.Errorf("save default instance: %w", err)
	}

	return defaultInst, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func (r *InstanceRepository) scanInstance(s rowScanner) (*domain.Instance, error) {
	var (
		id, name, gameVersion, loader, loaderVer, iconPath, javaPath, jvmArgsStr, stateStr string
		minRAM, maxRAM, skipCheckInt, favInt                                                int
		groupName                                                                           string
		totalPlaySec                                                                        int64
		lastPlayedStr                                                                       sql.NullString
		createdAtStr, updatedAtStr                                                          string
	)

	err := s.Scan(
		&id, &name, &gameVersion, &loader, &loaderVer, &iconPath, &javaPath,
		&minRAM, &maxRAM, &jvmArgsStr, &skipCheckInt, &groupName, &favInt, &stateStr, &lastPlayedStr, &totalPlaySec,
		&createdAtStr, &updatedAtStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrInstanceNotFound
		}
		return nil, fmt.Errorf("scan instance: %w", err)
	}

	var jvmArgs []string
	if err := json.Unmarshal([]byte(jvmArgsStr), &jvmArgs); err != nil {
		jvmArgs = []string{}
	}

	createdAt, _ := time.Parse(time.RFC3339, createdAtStr)
	updatedAt, _ := time.Parse(time.RFC3339, updatedAtStr)

	var lastPlayed *time.Time
	if lastPlayedStr.Valid && lastPlayedStr.String != "" {
		t, err := time.Parse(time.RFC3339, lastPlayedStr.String)
		if err == nil {
			lastPlayed = &t
		}
	}

	return &domain.Instance{
		ID:            id,
		Name:          name,
		GameVersion:   gameVersion,
		Loader:        domain.LoaderType(loader),
		LoaderVer:     loaderVer,
		IconPath:      iconPath,
		JavaPath:      javaPath,
		MinRAMMB:      minRAM,
		MaxRAMMB:      maxRAM,
		JVMArgs:       jvmArgs,
		SkipJavaCheck: skipCheckInt != 0,
		Group:         groupName,
		IsFavorite:    favInt != 0,
		State:         domain.InstanceState(stateStr),
		LastPlayedAt:  lastPlayed,
		TotalPlaySec:  totalPlaySec,
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}, nil
}