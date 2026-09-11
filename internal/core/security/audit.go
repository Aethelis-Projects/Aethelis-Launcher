package security

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/nord-launcher/launcher/internal/core/ports"
)

type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityCritical Severity = "CRITICAL"
)

type SecurityFinding struct {
	Severity    Severity `json:"severity"`
	Target      string   `json:"target"`
	Description string   `json:"description"`
}

var (
	prohibitedColumnNames = []string{
		"token",
		"access_token",
		"refresh_token",
		"password",
		"secret",
		"private_key",
		"api_key",
	}

	jwtRegex          = regexp.MustCompile(`eyJ[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{8,}`)
	msTokenRegex      = regexp.MustCompile(`M\.(R3|C5)_[A-Za-z0-9_\-\.]+`)
	bearerHeaderRegex = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9_\-\.]+`)
	emailRegex        = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)
	winUserPathRegex  = regexp.MustCompile(`(?i)[a-zA-Z]:\\Users\\[^\\]+`)
	unixUserPathRegex = regexp.MustCompile(`/home/[^/]+`)
)

type SecurityAuditor struct {
	keyring ports.Keyring
}

func NewSecurityAuditor(keyring ports.Keyring) *SecurityAuditor {
	return &SecurityAuditor{keyring: keyring}
}

// AuditDatabase inspects SQLite database schema and rows to ensure zero sensitive tokens/secrets are stored in plaintext.
func (a *SecurityAuditor) AuditDatabase(ctx context.Context, db *sql.DB) ([]SecurityFinding, error) {
	if db == nil {
		return nil, errors.New("database connection is nil")
	}

	var findings []SecurityFinding

	// 1. Get all user tables
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != 'goose_db_version'")
	if err != nil {
		return nil, fmt.Errorf("query sqlite tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table name: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// 2. Audit each table
	for _, tbl := range tables {
		tableFindings, err := a.auditTable(ctx, db, tbl)
		if err != nil {
			return nil, fmt.Errorf("audit table %s: %w", tbl, err)
		}
		findings = append(findings, tableFindings...)
	}

	return findings, nil
}

func (a *SecurityAuditor) auditTable(ctx context.Context, db *sql.DB, tableName string) ([]SecurityFinding, error) {
	var findings []SecurityFinding

	// Inspect columns
	infoRows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, fmt.Errorf("pragma table_info: %w", err)
	}
	defer infoRows.Close()

	var columns []string
	for infoRows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := infoRows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return nil, fmt.Errorf("scan column info: %w", err)
		}
		columns = append(columns, name)

		// Check prohibited column names
		lowerCol := strings.ToLower(name)
		for _, prohibited := range prohibitedColumnNames {
			if strings.Contains(lowerCol, prohibited) {
				findings = append(findings, SecurityFinding{
					Severity:    SeverityCritical,
					Target:      fmt.Sprintf("%s.%s", tableName, name),
					Description: fmt.Sprintf("prohibited sensitive column name %q found in database schema", name),
				})
			}
		}
	}

	// Inspect sample rows for secret leak patterns
	selectQuery := fmt.Sprintf("SELECT %s FROM %s LIMIT 100", strings.Join(columns, ", "), tableName)
	dataRows, err := db.QueryContext(ctx, selectQuery)
	if err != nil {
		return findings, fmt.Errorf("query table data: %w", err)
	}
	defer dataRows.Close()

	vals := make([]interface{}, len(columns))
	valPtrs := make([]interface{}, len(columns))
	for i := range vals {
		valPtrs[i] = &vals[i]
	}

	for dataRows.Next() {
		if err := dataRows.Scan(valPtrs...); err != nil {
			return findings, fmt.Errorf("scan table row: %w", err)
		}

		for i, val := range vals {
			if val == nil {
				continue
			}
			strVal := fmt.Sprintf("%v", val)

			if jwtRegex.MatchString(strVal) {
				findings = append(findings, SecurityFinding{
					Severity:    SeverityCritical,
					Target:      fmt.Sprintf("%s.%s", tableName, columns[i]),
					Description: "JWT token detected in plaintext database row",
				})
			}
			if msTokenRegex.MatchString(strVal) {
				findings = append(findings, SecurityFinding{
					Severity:    SeverityCritical,
					Target:      fmt.Sprintf("%s.%s", tableName, columns[i]),
					Description: "Microsoft OAuth token detected in plaintext database row",
				})
			}
		}
	}

	return findings, nil
}

// AuditKeyring verifies that secrets can be safely stored, retrieved, and deleted from Keyring.
func (a *SecurityAuditor) AuditKeyring(accountUUID string) ([]SecurityFinding, error) {
	if a.keyring == nil {
		return []SecurityFinding{
			{
				Severity:    SeverityCritical,
				Target:      "Keyring",
				Description: "system keyring port is not configured",
			},
		}, nil
	}

	testSecret := "audit-test-secret-" + accountUUID
	key := "test_" + accountUUID
	service := "nord-launcher"

	// Write
	if err := a.keyring.Set(service, key, testSecret); err != nil {
		return []SecurityFinding{
			{
				Severity:    SeverityCritical,
				Target:      "Keyring.Set",
				Description: fmt.Sprintf("failed to store secret in keyring: %v", err),
			},
		}, nil
	}

	// Read
	got, err := a.keyring.Get(service, key)
	if err != nil || got != testSecret {
		return []SecurityFinding{
			{
				Severity:    SeverityCritical,
				Target:      "Keyring.Get",
				Description: fmt.Sprintf("failed to retrieve verified secret from keyring: %v", err),
			},
		}, nil
	}

	// Clean up
	_ = a.keyring.Delete(service, key)

	return []SecurityFinding{
		{
			Severity:    SeverityInfo,
			Target:      "Keyring",
			Description: "system keyring verified for isolated credential storage",
		},
	}, nil
}

// SanitizeLogs removes PII, paths, OAuth tokens, and emails from log strings before telemetry or export.
func SanitizeLogs(raw string) string {
	res := bearerHeaderRegex.ReplaceAllString(raw, "Bearer [REDACTED]")
	res = msTokenRegex.ReplaceAllString(res, "M.R3_[REDACTED]")
	res = jwtRegex.ReplaceAllString(res, "eyJ[REDACTED]")
	res = emailRegex.ReplaceAllString(res, "[EMAIL_REDACTED]")
	res = winUserPathRegex.ReplaceAllString(res, `C:\Users\[REDACTED]`)
	res = unixUserPathRegex.ReplaceAllString(res, `/home/[REDACTED]`)
	return res
}
