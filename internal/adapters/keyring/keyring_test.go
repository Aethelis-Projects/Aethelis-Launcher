package keyring_test

import (
	"errors"
	"testing"

	"github.com/nord-launcher/launcher/internal/adapters/keyring"
)

func TestInMemKeyring_CRUD(t *testing.T) {
	kr := keyring.NewMemoryKeyring()

	// 1. Initial Get should return ErrKeyNotFound
	_, err := kr.Get("test-svc", "alice")
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound, got: %v", err)
	}

	// 2. Set key
	if err := kr.Set("test-svc", "alice", "secret-token-123"); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	// 3. Get key
	val, err := kr.Get("test-svc", "alice")
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if val != "secret-token-123" {
		t.Fatalf("expected 'secret-token-123', got '%s'", val)
	}

	// 4. Overwrite
	if err := kr.Set("test-svc", "alice", "new-token-456"); err != nil {
		t.Fatalf("failed to overwrite key: %v", err)
	}
	val, err = kr.Get("test-svc", "alice")
	if err != nil {
		t.Fatalf("failed to get overwritten key: %v", err)
	}
	if val != "new-token-456" {
		t.Fatalf("expected 'new-token-456', got '%s'", val)
	}

	// 5. Delete key
	if err := kr.Delete("test-svc", "alice"); err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}

	// 6. Get after Delete should return ErrKeyNotFound
	_, err = kr.Get("test-svc", "alice")
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound after delete, got: %v", err)
	}
}

func TestSystemKeyring_Contract(t *testing.T) {
	kr := keyring.NewSystemKeyring()

	svc := "nord-launcher-test"
	user := "test-user-system"
	token := "system-refresh-token-xyz"

	// Cleanup in case of previous interrupted test
	_ = kr.Delete(svc, user)

	// Verify not found initially
	_, err := kr.Get(svc, user)
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound initially, got: %v", err)
	}

	// Set
	if err := kr.Set(svc, user, token); err != nil {
		t.Fatalf("failed to set key: %v", err)
	}

	// Get
	got, err := kr.Get(svc, user)
	if err != nil {
		t.Fatalf("failed to get key: %v", err)
	}
	if got != token {
		t.Fatalf("expected '%s', got '%s'", token, got)
	}

	// Delete
	if err := kr.Delete(svc, user); err != nil {
		t.Fatalf("failed to delete key: %v", err)
	}

	// Verify deleted
	_, err = kr.Get(svc, user)
	if !errors.Is(err, keyring.ErrKeyNotFound) {
		t.Fatalf("expected ErrKeyNotFound after deletion, got: %v", err)
	}
}
