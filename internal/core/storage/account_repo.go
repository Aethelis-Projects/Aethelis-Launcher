package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
)

type AccountRepository struct {
	db *sql.DB
}

func NewAccountRepository(db *Database) *AccountRepository {
	return &AccountRepository{db: db.DB()}
}

func (r *AccountRepository) Save(ctx context.Context, acc *domain.Account) error {
	query := `
	INSERT INTO accounts (uuid, username, type, expires_at, is_active)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(uuid) DO UPDATE SET
		username = excluded.username,
		type = excluded.type,
		expires_at = excluded.expires_at,
		is_active = excluded.is_active;
	`
	isActiveInt := 0
	if acc.IsActive {
		isActiveInt = 1
	}

	_, err := r.db.ExecContext(ctx, query,
		acc.UUID,
		acc.Username,
		string(acc.Type),
		acc.ExpiresAt.Format(time.RFC3339),
		isActiveInt,
	)
	if err != nil {
		return fmt.Errorf("save account: %w", err)
	}
	return nil
}

func (r *AccountRepository) GetByUUID(ctx context.Context, uuid string) (*domain.Account, error) {
	query := `SELECT uuid, username, type, expires_at, is_active FROM accounts WHERE uuid = ?;`
	var (
		u, username, accType, expiresAtStr string
		isActiveInt                        int
	)
	err := r.db.QueryRowContext(ctx, query, uuid).Scan(&u, &username, &accType, &expiresAtStr, &isActiveInt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("get account by uuid: %w", err)
	}

	expiresAt, _ := time.Parse(time.RFC3339, expiresAtStr)
	return &domain.Account{
		UUID:      u,
		Username:  username,
		Type:      domain.AccountType(accType),
		ExpiresAt: expiresAt,
		IsActive:  isActiveInt == 1,
	}, nil
}

func (r *AccountRepository) GetActive(ctx context.Context) (*domain.Account, error) {
	query := `SELECT uuid, username, type, expires_at, is_active FROM accounts WHERE is_active = 1 LIMIT 1;`
	var (
		u, username, accType, expiresAtStr string
		isActiveInt                        int
	)
	err := r.db.QueryRowContext(ctx, query).Scan(&u, &username, &accType, &expiresAtStr, &isActiveInt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrAccountNotFound
		}
		return nil, fmt.Errorf("get active account: %w", err)
	}

	expiresAt, _ := time.Parse(time.RFC3339, expiresAtStr)
	return &domain.Account{
		UUID:      u,
		Username:  username,
		Type:      domain.AccountType(accType),
		ExpiresAt: expiresAt,
		IsActive:  true,
	}, nil
}

func (r *AccountRepository) ListAll(ctx context.Context) ([]*domain.Account, error) {
	query := `SELECT uuid, username, type, expires_at, is_active FROM accounts ORDER BY is_active DESC;`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	var res []*domain.Account
	for rows.Next() {
		var (
			u, username, accType, expiresAtStr string
			isActiveInt                        int
		)
		if err := rows.Scan(&u, &username, &accType, &expiresAtStr, &isActiveInt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		expiresAt, _ := time.Parse(time.RFC3339, expiresAtStr)
		res = append(res, &domain.Account{
			UUID:      u,
			Username:  username,
			Type:      domain.AccountType(accType),
			ExpiresAt: expiresAt,
			IsActive:  isActiveInt == 1,
		})
	}
	return res, nil
}

func (r *AccountRepository) SetActive(ctx context.Context, uuid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "UPDATE accounts SET is_active = 0;"); err != nil {
		return fmt.Errorf("reset active accounts: %w", err)
	}

	res, err := tx.ExecContext(ctx, "UPDATE accounts SET is_active = 1 WHERE uuid = ?;", uuid)
	if err != nil {
		return fmt.Errorf("set active account: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return domain.ErrAccountNotFound
	}

	return tx.Commit()
}

func (r *AccountRepository) Delete(ctx context.Context, uuid string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM accounts WHERE uuid = ?;", uuid)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	return nil
}