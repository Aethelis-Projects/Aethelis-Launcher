package auth

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

const (
	DefaultClientID = "00000000402b5328" // Microsoft standard client ID for Minecraft
	KeyringService  = "nord-launcher"
)

type AuthService struct {
	clientID    string
	apiClient   *APIClient
	accountRepo ports.AccountRepository
	keyring     ports.Keyring

	sessionsMu sync.RWMutex
	sessions   map[string]*AuthSession // UUID -> in-memory active session with AccessToken
}

func NewAuthService(
	clientID string,
	apiClient *APIClient,
	accountRepo ports.AccountRepository,
	keyring ports.Keyring,
) *AuthService {
	if clientID == "" {
		clientID = DefaultClientID
	}
	if apiClient == nil {
		apiClient = NewAPIClient(nil, DefaultEndpoints())
	}
	return &AuthService{
		clientID:    clientID,
		apiClient:   apiClient,
		accountRepo: accountRepo,
		keyring:     keyring,
		sessions:    make(map[string]*AuthSession),
	}
}

// StartInteractiveLogin starts a local loopback callback server, launches browser, and handles login.
func (s *AuthService) StartInteractiveLogin(
	ctx context.Context,
	openBrowser func(url string) error,
) (*domain.Account, error) {
	// 1. Listen on ephemeral loopback port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen on loopback: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// 2. Generate PKCE verifier, challenge, and CSRF state
	codeVerifier, err := GenerateCodeVerifier()
	if err != nil {
		return nil, err
	}
	codeChallenge := GenerateCodeChallenge(codeVerifier)
	expectedState, err := GenerateRandomState()
	if err != nil {
		return nil, err
	}

	authURLVals := url.Values{
		"client_id":             {s.clientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {"XboxLive.signin offline_access"},
		"state":                 {expectedState},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
		"prompt":                {"select_account"},
	}
	fullAuthURL := s.apiClient.endpoints.MSAuthorizeURL + "?" + authURLVals.Encode()

	// 3. Channel to receive auth code from HTTP callback
	type callbackResult struct {
		code string
		err  error
	}
	resultChan := make(chan callbackResult, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		state := q.Get("state")
		if state != expectedState {
			http.Error(w, "State mismatch", http.StatusBadRequest)
			resultChan <- callbackResult{err: ErrStateMismatch}
			return
		}

		errParam := q.Get("error")
		if errParam != "" {
			errDesc := q.Get("error_description")
			http.Error(w, "OAuth Error: "+errDesc, http.StatusBadRequest)
			resultChan <- callbackResult{err: fmt.Errorf("oauth error %s: %s", errParam, errDesc)}
			return
		}

		code := q.Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			resultChan <- callbackResult{err: errors.New("missing authorization code")}
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>Nord Launcher - Authorized</title>
<style>body{background:#09090b;color:#fafafa;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;}
.box{text-align:center;padding:2rem;background:#18181b;border:1px solid #27272a;border-radius:8px;}
h1{color:#00D4B2;margin-bottom:0.5rem;}p{color:#a1a1aa;}</style></head>
<body><div class="box"><h1>Authorization Successful</h1><p>You may safely close this browser window and return to Nord Launcher.</p></div></body>
</html>`))

		resultChan <- callbackResult{code: code}
	})

	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(listener)
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	// 4. Trigger browser open
	if openBrowser != nil {
		if err := openBrowser(fullAuthURL); err != nil {
			return nil, fmt.Errorf("open browser: %w", err)
		}
	}

	// 5. Wait for callback or timeout/cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			return nil, res.err
		}
		return s.completeAuthPipeline(ctx, res.code, redirectURI, codeVerifier)
	}
}

// completeAuthPipeline executes the full token exchange chain.
func (s *AuthService) completeAuthPipeline(
	ctx context.Context,
	authCode string,
	redirectURI string,
	codeVerifier string,
) (*domain.Account, error) {
	// A. Exchange code for MS tokens
	msTokens, err := s.apiClient.ExchangeMSToken(ctx, s.clientID, authCode, redirectURI, codeVerifier)
	if err != nil {
		return nil, fmt.Errorf("exchange ms token: %w", err)
	}

	return s.finishMinecraftLogin(ctx, msTokens)
}

// RefreshSession uses stored refresh token to obtain a new valid Minecraft session.
func (s *AuthService) RefreshSession(ctx context.Context, uuid string) (*domain.Account, error) {
	if s.keyring == nil {
		return nil, errors.New("keyring not configured for refresh token storage")
	}

	refreshToken, err := s.keyring.Get(KeyringService, uuid)
	if err != nil {
		return nil, fmt.Errorf("fetch refresh token from keyring: %w", err)
	}

	msTokens, err := s.apiClient.RefreshMSToken(ctx, s.clientID, refreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh ms token: %w", err)
	}

	return s.finishMinecraftLogin(ctx, msTokens)
}

// finishMinecraftLogin performs Xbox Live -> XSTS -> Minecraft Services login and persists.
func (s *AuthService) finishMinecraftLogin(ctx context.Context, msTokens *MSTokenResponse) (*domain.Account, error) {
	// 1. Xbox Live authentication
	xblResp, err := s.apiClient.AuthenticateXboxLive(ctx, msTokens.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("xbox live auth: %w", err)
	}
	userHash := xblResp.DisplayClaims.XUI[0].UHS

	// 2. XSTS authorization
	xstsResp, err := s.apiClient.AuthorizeXSTS(ctx, xblResp.Token)
	if err != nil {
		return nil, fmt.Errorf("xsts auth: %w", err)
	}

	// 3. Minecraft Services login
	mcResp, err := s.apiClient.LoginMinecraft(ctx, userHash, xstsResp.Token)
	if err != nil {
		return nil, fmt.Errorf("minecraft login: %w", err)
	}

	// 4. Fetch Minecraft Profile
	profile, err := s.apiClient.FetchMinecraftProfile(ctx, mcResp.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("fetch minecraft profile: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(mcResp.ExpiresIn) * time.Second)

	account := &domain.Account{
		UUID:        profile.ID,
		Username:    profile.Name,
		Type:        domain.AccountMicrosoft,
		AccessToken: mcResp.AccessToken,
		ExpiresAt:   expiresAt,
		IsActive:    true,
	}

	// 5. Store refresh token securely in keyring
	if s.keyring != nil && msTokens.RefreshToken != "" {
		if err := s.keyring.Set(KeyringService, profile.ID, msTokens.RefreshToken); err != nil {
			return nil, fmt.Errorf("save refresh token to keyring: %w", err)
		}
	}

	// 6. Store account metadata in repository
	if s.accountRepo != nil {
		if err := s.accountRepo.Save(ctx, account); err != nil {
			return nil, fmt.Errorf("save account to repository: %w", err)
		}
		if err := s.accountRepo.SetActive(ctx, account.UUID); err != nil {
			return nil, fmt.Errorf("set active account: %w", err)
		}
	}

	// 7. Retain session in memory
	s.sessionsMu.Lock()
	s.sessions[account.UUID] = &AuthSession{
		Account:      *account,
		RefreshToken: msTokens.RefreshToken,
		ExpiresAt:    expiresAt,
	}
	s.sessionsMu.Unlock()

	return account, nil
}

// CreateOfflineAccount creates an unauthenticated local account for offline play.
func (s *AuthService) CreateOfflineAccount(ctx context.Context, username string) (*domain.Account, error) {
	if username == "" {
		return nil, errors.New("offline username cannot be empty")
	}

	// Generate deterministic offline UUID v3 (MD5 of "OfflinePlayer:" + username)
	h := md5.Sum([]byte("OfflinePlayer:" + username))
	h[6] = (h[6] & 0x0f) | 0x30 // Version 3
	h[8] = (h[8] & 0x3f) | 0x80 // Variant RFC 4122
	uuid := hex.EncodeToString(h[:])

	account := &domain.Account{
		UUID:        uuid,
		Username:    username,
		Type:        domain.AccountOffline,
		AccessToken: "0",
		ExpiresAt:   time.Now().Add(100 * 365 * 24 * time.Hour), // 100 years
		IsActive:    true,
	}

	if s.accountRepo != nil {
		if err := s.accountRepo.Save(ctx, account); err != nil {
			return nil, fmt.Errorf("save offline account: %w", err)
		}
		if err := s.accountRepo.SetActive(ctx, account.UUID); err != nil {
			return nil, fmt.Errorf("set active account: %w", err)
		}
	}

	s.sessionsMu.Lock()
	s.sessions[account.UUID] = &AuthSession{
		Account:   *account,
		ExpiresAt: account.ExpiresAt,
	}
	s.sessionsMu.Unlock()

	return account, nil
}

// GetActiveSession returns the active session for an account UUID if valid.
func (s *AuthService) GetActiveSession(uuid string) (*AuthSession, bool) {
	s.sessionsMu.RLock()
	defer s.sessionsMu.RUnlock()
	sess, ok := s.sessions[uuid]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}