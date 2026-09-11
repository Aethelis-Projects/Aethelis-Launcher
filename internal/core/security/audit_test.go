package security

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

type mockKeyring struct {
	data map[string]string
}

func newMockKeyring() *mockKeyring {
	return &mockKeyring{data: make(map[string]string)}
}

func (m *mockKeyring) Set(service, user, password string) error {
	m.data[service+":"+user] = password
	return nil
}

func (m *mockKeyring) Get(service, user string) (string, error) {
	val, ok := m.data[service+":"+user]
	if !ok {
		return "", sql.ErrNoRows
	}
	return val, nil
}

func (m *mockKeyring) Delete(service, user string) error {
	delete(m.data, service+":"+user)
	return nil
}

func TestSecurityAuditor_CleanDatabase(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Create clean schema similar to launcher storage
	schema := `
	CREATE TABLE accounts (
		uuid TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		type TEXT NOT NULL,
		expires_at DATETIME NOT NULL,
		is_active INTEGER NOT NULL DEFAULT 0
	);
	INSERT INTO accounts VALUES ('uuid-1', 'PlayerOne', 'microsoft', '2026-09-12', 1);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	auditor := NewSecurityAuditor(newMockKeyring())
	findings, err := auditor.AuditDatabase(context.Background(), db)
	if err != nil {
		t.Fatalf("AuditDatabase failed: %v", err)
	}

	for _, f := range findings {
		if f.Severity == SeverityCritical {
			t.Errorf("unexpected critical finding in clean database: %s (%s)", f.Target, f.Description)
		}
	}
}

func TestSecurityAuditor_ForbiddenColumnName(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	// Prohibited column: access_token
	schema := `
	CREATE TABLE bad_table (
		id INTEGER PRIMARY KEY,
		access_token TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	auditor := NewSecurityAuditor(newMockKeyring())
	findings, err := auditor.AuditDatabase(context.Background(), db)
	if err != nil {
		t.Fatalf("AuditDatabase failed: %v", err)
	}

	var foundCritical bool
	for _, f := range findings {
		if f.Severity == SeverityCritical && strings.Contains(f.Target, "access_token") {
			foundCritical = true
		}
	}
	if !foundCritical {
		t.Errorf("expected critical finding for forbidden column access_token, got none: %+v", findings)
	}
}

func TestSecurityAuditor_LeakedTokenInRow(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	schema := `
	CREATE TABLE metadata (
		id INTEGER PRIMARY KEY,
		value TEXT NOT NULL
	);
	INSERT INTO metadata VALUES (1, 'M.R3_mock_oauth_secret_data_here');
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}

	auditor := NewSecurityAuditor(newMockKeyring())
	findings, err := auditor.AuditDatabase(context.Background(), db)
	if err != nil {
		t.Fatalf("AuditDatabase failed: %v", err)
	}

	var foundCritical bool
	for _, f := range findings {
		if f.Severity == SeverityCritical && strings.Contains(f.Description, "Microsoft OAuth token detected") {
			foundCritical = true
		}
	}
	if !foundCritical {
		t.Errorf("expected critical finding for leaked Microsoft OAuth token, got none: %+v", findings)
	}
}

func TestSecurityAuditor_AuditKeyring(t *testing.T) {
	ring := newMockKeyring()
	auditor := NewSecurityAuditor(ring)

	findings, err := auditor.AuditKeyring("test-uuid-42")
	if err != nil {
		t.Fatalf("AuditKeyring failed: %v", err)
	}

	for _, f := range findings {
		if f.Severity == SeverityCritical {
			t.Errorf("unexpected critical keyring finding: %+v", f)
		}
	}
}

func TestSanitizeLogs(t *testing.T) {
	raw := `
2026-09-12 12:00:00 [INFO] Request headers: Authorization: Bearer secret_bearer_token_12345
2026-09-12 12:00:01 [DEBUG] User directory: C:\Users\Alice\AppData\Roaming\.minecraft
2026-09-12 12:00:02 [DEBUG] Linux directory: /home/bob/.local/share/nord-launcher
2026-09-12 12:00:03 [INFO] Account email user.name+tag@example.com logged in
2026-09-12 12:00:04 [DEBUG] Xbox token: M.R3_abcdef1234567890
`
	sanitized := SanitizeLogs(raw)

	if strings.Contains(sanitized, "secret_bearer_token_12345") {
		t.Errorf("bearer token was not sanitized: %s", sanitized)
	}
	if strings.Contains(sanitized, "Alice") {
		t.Errorf("Windows username Alice was not sanitized: %s", sanitized)
	}
	if strings.Contains(sanitized, "/home/bob") {
		t.Errorf("Linux home path /home/bob was not sanitized: %s", sanitized)
	}
	if strings.Contains(sanitized, "user.name+tag@example.com") {
		t.Errorf("email was not sanitized: %s", sanitized)
	}
	if strings.Contains(sanitized, "M.R3_abcdef1234567890") {
		t.Errorf("Microsoft token was not sanitized: %s", sanitized)
	}
}
