package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Endpoints holds the API URLs used for authentication.
type Endpoints struct {
	MSAuthorizeURL string
	MSTokenURL     string
	XBLAuthURL     string
	XSTSAuthURL    string
	MCLoginURL     string
	MCProfileURL   string
}

// DefaultEndpoints returns standard production Microsoft and Minecraft endpoints.
func DefaultEndpoints() Endpoints {
	return Endpoints{
		MSAuthorizeURL: "https://login.live.com/oauth20_authorize.srf",
		MSTokenURL:     "https://login.live.com/oauth20_token.srf",
		XBLAuthURL:     "https://user.auth.xboxlive.com/user/authenticate",
		XSTSAuthURL:    "https://xsts.auth.xboxlive.com/xsts/authorize",
		MCLoginURL:     "https://api.minecraftservices.com/launcher/login",
		MCProfileURL:   "https://api.minecraftservices.com/minecraft/profile",
	}
}

// APIClient executes HTTP requests for the Microsoft-Xbox-Minecraft authentication chain.
type APIClient struct {
	httpClient *http.Client
	endpoints  Endpoints
}

func NewAPIClient(client *http.Client, endpoints Endpoints) *APIClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &APIClient{
		httpClient: client,
		endpoints:  endpoints,
	}
}

// ExchangeMSToken exchanges the authorization code for a Microsoft access & refresh token.
func (c *APIClient) ExchangeMSToken(
	ctx context.Context,
	clientID string,
	code string,
	redirectURI string,
	codeVerifier string,
) (*MSTokenResponse, error) {
	data := url.Values{
		"client_id":     {clientID},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints.MSTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute token exchange request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ms token exchange returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp MSTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token exchange response: %w", err)
	}

	return &tokenResp, nil
}

// RefreshMSToken exchanges a refresh token for fresh Microsoft tokens.
func (c *APIClient) RefreshMSToken(
	ctx context.Context,
	clientID string,
	refreshToken string,
) (*MSTokenResponse, error) {
	data := url.Values{
		"client_id":     {clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints.MSTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute refresh request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ms token refresh returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp MSTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode refresh response: %w", err)
	}

	return &tokenResp, nil
}

// AuthenticateXboxLive exchanges a Microsoft access token for an Xbox Live user token.
func (c *APIClient) AuthenticateXboxLive(ctx context.Context, msAccessToken string) (*XBLResponse, error) {
	payload := map[string]any{
		"Properties": map[string]any{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  "d=" + msAccessToken,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints.XBLAuthURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("create xbl auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute xbl auth request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("xbl auth returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var xblResp XBLResponse
	if err := json.NewDecoder(resp.Body).Decode(&xblResp); err != nil {
		return nil, fmt.Errorf("decode xbl response: %w", err)
	}

	if len(xblResp.DisplayClaims.XUI) == 0 || xblResp.DisplayClaims.XUI[0].UHS == "" {
		return nil, fmt.Errorf("xbl response did not contain user hash (uhs)")
	}

	return &xblResp, nil
}

// AuthorizeXSTS exchanges an Xbox Live token for an XSTS token for the Minecraft RelyingParty.
func (c *APIClient) AuthorizeXSTS(ctx context.Context, xblToken string) (*XSTSResponse, error) {
	payload := map[string]any{
		"Properties": map[string]any{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{xblToken},
		},
		"RelyingParty": "rp://api.minecraftservices.com/",
		"TokenType":    "JWT",
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints.XSTSAuthURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("create xsts request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute xsts request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			XErr    int64  `json:"XErr"`
			Message string `json:"Message"`
		}
		_ = json.Unmarshal(bodyBytes, &errResp) // slop:ok best-effort error decoding, fallback follows below
		if errResp.XErr != 0 {
			return nil, &XSTSError{Code: errResp.XErr, Message: errResp.Message}
		}
		return nil, fmt.Errorf("xsts auth returned HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var xstsResp XSTSResponse
	if err := json.Unmarshal(bodyBytes, &xstsResp); err != nil {
		return nil, fmt.Errorf("decode xsts response: %w", err)
	}

	if xstsResp.Token == "" {
		return nil, ErrInvalidXSTSToken
	}

	return &xstsResp, nil
}

// LoginMinecraft exchanges XSTS Token and user hash for a Minecraft Services access token.
func (c *APIClient) LoginMinecraft(ctx context.Context, uhs string, xstsToken string) (*MinecraftAuthResponse, error) {
	payload := map[string]string{
		"identityToken": fmt.Sprintf("XBL3.0 x=%s;%s", uhs, xstsToken),
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints.MCLoginURL, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("create mc login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute mc login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mc login returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var mcResp MinecraftAuthResponse
	if err := json.NewDecoder(resp.Body).Decode(&mcResp); err != nil {
		return nil, fmt.Errorf("decode mc login response: %w", err)
	}

	return &mcResp, nil
}

// FetchMinecraftProfile retrieves player UUID, username, skins, and capes.
func (c *APIClient) FetchMinecraftProfile(ctx context.Context, mcAccessToken string) (*MinecraftProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoints.MCProfileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+mcAccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute profile request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoMinecraftLicense
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch profile returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var profile MinecraftProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode profile response: %w", err)
	}

	return &profile, nil
}