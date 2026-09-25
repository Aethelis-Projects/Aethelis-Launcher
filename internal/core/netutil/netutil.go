package netutil

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	DefaultVersion = "0.6.1"
	RepoURL        = "https://github.com/Aethelis-Projects/Aethelis-Launcher"
)

// FormatUserAgent constructs the standard NordLauncher User-Agent header value.
func FormatUserAgent(version string) string {
	v := strings.TrimSpace(strings.TrimPrefix(version, "v"))
	if v == "" {
		v = DefaultVersion
	}
	return fmt.Sprintf("NordLauncher/%s (+%s)", v, RepoURL)
}

// UserAgentTransport wraps an http.RoundTripper and ensures every outgoing
// HTTP request carries the authentic launcher User-Agent and JSON accept header.
type UserAgentTransport struct {
	Base      http.RoundTripper
	UserAgent string
}

// RoundTrip injects User-Agent and default Accept headers if missing.
func (t *UserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	if cloned.Header.Get("User-Agent") == "" {
		ua := t.UserAgent
		if ua == "" {
			ua = FormatUserAgent("")
		}
		cloned.Header.Set("User-Agent", ua)
	}
	if cloned.Header.Get("Accept") == "" {
		cloned.Header.Set("Accept", "application/json")
	}
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(cloned)
}

// NewTransport wraps an existing RoundTripper with standard User-Agent headers.
func NewTransport(version string, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &UserAgentTransport{
		Base:      base,
		UserAgent: FormatUserAgent(version),
	}
}

// NewHTTPClient creates an *http.Client configured with standard UserAgentTransport and timeout.
func NewHTTPClient(version string, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{
		Transport: NewTransport(version, nil),
		Timeout:   timeout,
	}
}
