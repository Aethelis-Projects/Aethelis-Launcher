package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/core/auth"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/storage"
)

type mockAuthServers struct {
	server       *httptest.Server
	endpoints    auth.Endpoints
	msTokenCalls int
	xblCalls     int
	xstsCalls    int
	mcLoginCalls int
	profileCalls int
	xstsErrCode  int64
}

func setupMockAuthServers(t *testing.T) *mockAuthServers {
	m := &mockAuthServers{}

	mux := http.NewServeMux()

	// 1. MS Token
	mux.HandleFunc("/oauth20_token.srf", func(w http.ResponseWriter, r *http.Request) {
		m.msTokenCalls++
		_ = r.ParseForm()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auth.MSTokenResponse{
			AccessToken:  "mock-ms-access-token",
			RefreshToken: "mock-ms-refresh-token-rotated",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
		})
	})

	// 2. XBL Authenticate
	mux.HandleFunc("/user/authenticate", func(w http.ResponseWriter, r *http.Request) {
		m.xblCalls++
		w.Header().Set("Content-Type", "application/json")
		resp := auth.XBLResponse{
			Token: "mock-xbl-token",
		}
		resp.DisplayClaims.XUI = []struct {
			UHS string `json:"uhs"`
		}{
			{UHS: "12345678"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// 3. XSTS Authorize
	mux.HandleFunc("/xsts/authorize", func(w http.ResponseWriter, r *http.Request) {
		m.xstsCalls++
		if m.xstsErrCode != 0 {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"XErr":    m.xstsErrCode,
				"Message": "Child account constraint violation",
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		resp := auth.XSTSResponse{
			Token: "mock-xsts-token",
		}
		resp.DisplayClaims.XUI = []struct {
			UHS string `json:"uhs"`
		}{
			{UHS: "12345678"},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	// 4. Minecraft Login
	mux.HandleFunc("/launcher/login", func(w http.ResponseWriter, r *http.Request) {
		m.mcLoginCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auth.MinecraftAuthResponse{
			Username:    "NordPlayer",
			AccessToken: "mock-mc-jwt-token",
			TokenType:   "Bearer",
			ExpiresIn:   86400,
		})
	})

	// 5. Minecraft Profile
	mux.HandleFunc("/minecraft/profile", func(w http.ResponseWriter, r *http.Request) {
		m.profileCalls++
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer mock-mc-jwt-token" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(auth.MinecraftProfile{
			ID:   "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
			Name: "NordPlayer",
		})
	})

	m.server = httptest.NewServer(mux)
	m.endpoints = auth.Endpoints{
		MSAuthorizeURL: m.server.URL + "/oauth20_authorize.srf",
		MSTokenURL:     m.server.URL + "/oauth20_token.srf",
		XBLAuthURL:     m.server.URL + "/user/authenticate",
		XSTSAuthURL:    m.server.URL + "/xsts/authorize",
		MCLoginURL:     m.server.URL + "/launcher/login",
		MCProfileURL:   m.server.URL + "/minecraft/profile",
	}

	return m
}

func TestAuth_InteractiveLoginSuccess(t *testing.T) {
	mockNet := setupMockAuthServers(t)
	defer mockNet.server.Close()

	tempDir := t.TempDir()
	db, err := storage.OpenDatabase(filepath.Join(tempDir, "auth.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	accountRepo := storage.NewAccountRepository(db)
	kr := keyring.NewMemoryKeyring()

	apiClient := auth.NewAPIClient(mockNet.server.Client(), mockNet.endpoints)
	authSvc := auth.NewAuthService("test-client-id", apiClient, accountRepo, kr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	acc, err := authSvc.StartInteractiveLogin(ctx, func(authURL string) error {
		// Parse the redirectURI and state from authURL and simulate browser callback
		u, parseErr := url.Parse(authURL)
		if parseErr != nil {
			return parseErr
		}
		redirectURI := u.Query().Get("redirect_uri")
		state := u.Query().Get("state")

		// Hit local callback server asynchronously
		go func() {
			time.Sleep(50 * time.Millisecond)
			callbackURL := fmt.Sprintf("%s?code=mock-auth-code&state=%s", redirectURI, state)
			resp, httpErr := http.Get(callbackURL)
			if httpErr != nil {
				t.Errorf("callback request failed: %v", httpErr)
				return
			}
			_ = resp.Body.Close()
		}()

		return nil
	})

	if err != nil {
		t.Fatalf("interactive login failed: %v", err)
	}

	if acc.Username != "NordPlayer" {
		t.Fatalf("expected username NordPlayer, got %s", acc.Username)
	}
	if acc.UUID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Fatalf("unexpected uuid: %s", acc.UUID)
	}
	if acc.Type != domain.AccountMicrosoft {
		t.Fatalf("expected AccountMicrosoft, got %s", acc.Type)
	}
	if !acc.IsActive {
		t.Fatal("expected account to be active")
	}

	// Verify refresh token saved in Keyring
	refreshToken, err := kr.Get(auth.KeyringService, acc.UUID)
	if err != nil {
		t.Fatalf("failed to retrieve refresh token from keyring: %v", err)
	}
	if refreshToken != "mock-ms-refresh-token-rotated" {
		t.Fatalf("unexpected refresh token in keyring: %s", refreshToken)
	}

	// Verify account saved in SQLite repository
	dbAcc, err := accountRepo.GetActive(context.Background())
	if err != nil {
		t.Fatalf("failed to retrieve active account from db: %v", err)
	}
	if dbAcc.UUID != acc.UUID || dbAcc.Username != acc.Username {
		t.Fatalf("mismatched db account: %+v", dbAcc)
	}

	// Verify all 5 API endpoints were touched
	if mockNet.msTokenCalls != 1 || mockNet.xblCalls != 1 || mockNet.xstsCalls != 1 || mockNet.mcLoginCalls != 1 || mockNet.profileCalls != 1 {
		t.Fatalf("unexpected call counts: ms=%d, xbl=%d, xsts=%d, mc=%d, prof=%d",
			mockNet.msTokenCalls, mockNet.xblCalls, mockNet.xstsCalls, mockNet.mcLoginCalls, mockNet.profileCalls)
	}
}

func TestAuth_TokenRefresh(t *testing.T) {
	mockNet := setupMockAuthServers(t)
	defer mockNet.server.Close()

	tempDir := t.TempDir()
	db, err := storage.OpenDatabase(filepath.Join(tempDir, "auth_refresh.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	accountRepo := storage.NewAccountRepository(db)
	kr := keyring.NewMemoryKeyring()

	apiClient := auth.NewAPIClient(mockNet.server.Client(), mockNet.endpoints)
	authSvc := auth.NewAuthService("test-client-id", apiClient, accountRepo, kr)

	uuid := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	_ = kr.Set(auth.KeyringService, uuid, "initial-refresh-token")

	refreshed, err := authSvc.RefreshSession(context.Background(), uuid)
	if err != nil {
		t.Fatalf("refresh session failed: %v", err)
	}

	if refreshed.UUID != uuid || refreshed.Username != "NordPlayer" {
		t.Fatalf("unexpected refreshed account: %+v", refreshed)
	}

	// Verify rotated token in keyring
	rotated, err := kr.Get(auth.KeyringService, uuid)
	if err != nil {
		t.Fatalf("fetch rotated token: %v", err)
	}
	if rotated != "mock-ms-refresh-token-rotated" {
		t.Fatalf("unexpected rotated token: %s", rotated)
	}
}

func TestAuth_OfflineAccountCreation(t *testing.T) {
	tempDir := t.TempDir()
	db, err := storage.OpenDatabase(filepath.Join(tempDir, "auth_offline.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	accountRepo := storage.NewAccountRepository(db)
	kr := keyring.NewMemoryKeyring()

	authSvc := auth.NewAuthService("test-client-id", nil, accountRepo, kr)

	acc, err := authSvc.CreateOfflineAccount(context.Background(), "OfflineSteve")
	if err != nil {
		t.Fatalf("failed to create offline account: %v", err)
	}

	if acc.Username != "OfflineSteve" {
		t.Fatalf("expected username OfflineSteve, got %s", acc.Username)
	}
	if acc.Type != domain.AccountOffline {
		t.Fatalf("expected AccountOffline, got %s", acc.Type)
	}
	if acc.UUID == "" {
		t.Fatal("expected non-empty UUID")
	}

	active, err := accountRepo.GetActive(context.Background())
	if err != nil {
		t.Fatalf("failed to get active offline account: %v", err)
	}
	if active.UUID != acc.UUID {
		t.Fatalf("expected active uuid %s, got %s", acc.UUID, active.UUID)
	}
}

func TestAuth_OfflineUUIDV3_VanillaParity(t *testing.T) {
	// Canonical vectors from Mojang Java client:
	// UUID.nameUUIDFromBytes(("OfflinePlayer:" + username).getBytes(StandardCharsets.UTF_8))
	cases := []struct {
		username     string
		expectedUUID string
	}{
		{"Steve", "5627dd98e6be3c21b8a8e92344183641"},
		{"Alex", "36532b5ec4423dbba24cc7e55d0f979a"},
	}

	authSvc := auth.NewAuthService("test-id", nil, nil, nil)
	for _, tc := range cases {
		acc, err := authSvc.CreateOfflineAccount(context.Background(), tc.username)
		if err != nil {
			t.Fatalf("failed to create account for %s: %v", tc.username, err)
		}
		if acc.UUID != tc.expectedUUID {
			t.Errorf("vanilla parity failure for %s: expected %s, got %s", tc.username, tc.expectedUUID, acc.UUID)
		}
	}
}

func TestAuth_XSTSError_ChildAccount(t *testing.T) {
	mockNet := setupMockAuthServers(t)
	defer mockNet.server.Close()
	mockNet.xstsErrCode = 2148916238 // Child account error

	tempDir := t.TempDir()
	db, _ := storage.OpenDatabase(filepath.Join(tempDir, "auth_child.db"))
	defer db.Close()
	accountRepo := storage.NewAccountRepository(db)
	kr := keyring.NewMemoryKeyring()

	apiClient := auth.NewAPIClient(mockNet.server.Client(), mockNet.endpoints)
	authSvc := auth.NewAuthService("test-client-id", apiClient, accountRepo, kr)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := authSvc.StartInteractiveLogin(ctx, func(authURL string) error {
		u, _ := url.Parse(authURL)
		go func() {
			time.Sleep(50 * time.Millisecond)
			callbackURL := fmt.Sprintf("%s?code=mock-code&state=%s", u.Query().Get("redirect_uri"), u.Query().Get("state"))
			resp, _ := http.Get(callbackURL)
			if resp != nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	})

	if err == nil {
		t.Fatal("expected child account error, got nil")
	}

	var xstsErr *auth.XSTSError
	if !errors.As(err, &xstsErr) {
		t.Fatalf("expected XSTSError, got %T: %v", err, err)
	}
	if !strings.Contains(xstsErr.Error(), "child under 18") {
		t.Fatalf("expected child account message, got: %s", xstsErr.Error())
	}
}

func TestAuth_GetActiveSession_And_XSTSErrorBranches(t *testing.T) {
	// 1. Test XSTSError branches
	errNoProfile := &auth.XSTSError{Code: 2148916233, Message: "no profile"}
	if !strings.Contains(errNoProfile.Error(), "does not have an Xbox profile") {
		t.Errorf("unexpected error string: %s", errNoProfile.Error())
	}

	errChild := &auth.XSTSError{Code: 2148916238, Message: "child"}
	if !strings.Contains(errChild.Error(), "child under 18") {
		t.Errorf("unexpected error string: %s", errChild.Error())
	}

	errOther := &auth.XSTSError{Code: 99999, Message: "unknown failure"}
	if !strings.Contains(errOther.Error(), "99999") {
		t.Errorf("unexpected error string: %s", errOther.Error())
	}

	// 2. Test GetActiveSession
	dbPath := filepath.Join(t.TempDir(), "auth_sess.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer db.Close()
	accountRepo := storage.NewAccountRepository(db)
	kr := keyring.NewMemoryKeyring()

	authSvc := auth.NewAuthService("test-client-id", nil, accountRepo, kr)

	// Non-existent session
	if _, ok := authSvc.GetActiveSession("non-existent"); ok {
		t.Errorf("expected false for non-existent session, got true")
	}

	// Create offline account and verify active session
	acc, err := authSvc.CreateOfflineAccount(context.Background(), "PlayerOne")
	if err != nil {
		t.Fatalf("failed to create offline account: %v", err)
	}
	sess, ok := authSvc.GetActiveSession(acc.UUID)
	if !ok || sess == nil {
		t.Fatalf("expected active session for offline account %s, got false", acc.UUID)
	}
	if sess.Account.Username != "PlayerOne" {
		t.Errorf("expected username PlayerOne, got %s", sess.Account.Username)
	}
}