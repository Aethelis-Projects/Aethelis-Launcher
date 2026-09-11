package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
)

var (
	ErrNoMinecraftLicense = errors.New("microsoft account does not own a valid minecraft license")
	ErrStateMismatch      = errors.New("oauth state mismatch / potential CSRF attempt")
	ErrAuthTimedOut       = errors.New("authentication flow timed out waiting for user response")
	ErrInvalidXSTSToken   = errors.New("failed to acquire XSTS token")
)

type XSTSError struct {
	Code    int64
	Message string
}

func (e *XSTSError) Error() string {
	switch e.Code {
	case 2148916233:
		return "this Microsoft account does not have an Xbox profile. Please create one on xbox.com"
	case 2148916238:
		return "account belongs to a child under 18; parent must link it to a Microsoft Family to allow access"
	default:
		return fmt.Sprintf("Xbox Live authorization failed with XErr code %d: %s", e.Code, e.Message)
	}
}

// MSTokenResponse represents response from login.live.com oauth20_token.srf.
type MSTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

// XBLResponse represents response from user.auth.xboxlive.com.
type XBLResponse struct {
	IssueInstant  string `json:"IssueInstant"`
	NotAfter      string `json:"NotAfter"`
	Token         string `json:"Token"`
	DisplayClaims struct {
		XUI []struct {
			UHS string `json:"uhs"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
}

// XSTSResponse represents response from xsts.auth.xboxlive.com.
type XSTSResponse struct {
	IssueInstant  string `json:"IssueInstant"`
	NotAfter      string `json:"NotAfter"`
	Token         string `json:"Token"`
	DisplayClaims struct {
		XUI []struct {
			UHS string `json:"uhs"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
	XErr int64 `json:"XErr,omitempty"`
}

// MinecraftAuthResponse represents response from api.minecraftservices.com/launcher/login.
type MinecraftAuthResponse struct {
	Username    string `json:"username"`
	Roles       []any  `json:"roles"`
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// MinecraftProfile represents player profile from api.minecraftservices.com/minecraft/profile.
type MinecraftProfile struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Skins []struct {
		ID      string `json:"id"`
		State   string `json:"state"`
		URL     string `json:"url"`
		Variant string `json:"variant"`
	} `json:"skins"`
	Capes []struct {
		ID    string `json:"id"`
		State string `json:"state"`
		URL   string `json:"url"`
		Alias string `json:"alias"`
	} `json:"capes"`
}

// AuthSession represents an active in-memory session.
type AuthSession struct {
	Account      domain.Account
	RefreshToken string
	ExpiresAt    time.Time
}