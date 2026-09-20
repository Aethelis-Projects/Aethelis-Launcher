package netutil_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/core/netutil"
)

func TestFormatUserAgent(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0.5.0", "NordLauncher/0.5.0 (+https://github.com/Aethelis-Projects/Aethelis-Launcher)"},
		{"v0.5.0", "NordLauncher/0.5.0 (+https://github.com/Aethelis-Projects/Aethelis-Launcher)"},
		{"", "NordLauncher/0.5.0 (+https://github.com/Aethelis-Projects/Aethelis-Launcher)"},
		{"   ", "NordLauncher/0.5.0 (+https://github.com/Aethelis-Projects/Aethelis-Launcher)"},
	}

	for _, tc := range tests {
		got := netutil.FormatUserAgent(tc.input)
		if got != tc.expected {
			t.Errorf("FormatUserAgent(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestNewHTTPClient_HeadersInjected(t *testing.T) {
	var capturedUA string
	var capturedAccept string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		capturedAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := netutil.NewHTTPClient("0.5.0", 5*time.Second)
	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("client.Get failed: %v", err)
	}
	defer resp.Body.Close()

	expectedUA := "NordLauncher/0.5.0 (+https://github.com/Aethelis-Projects/Aethelis-Launcher)"
	if capturedUA != expectedUA {
		t.Errorf("captured User-Agent = %q; want %q", capturedUA, expectedUA)
	}
	if capturedAccept != "application/json" {
		t.Errorf("captured Accept = %q; want %q", capturedAccept, "application/json")
	}
}

func TestNewHTTPClient_PreservesExistingHeaders(t *testing.T) {
	var capturedUA string
	var capturedAccept string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		capturedAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := netutil.NewHTTPClient("0.5.0", 5*time.Second)
	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatalf("http.NewRequest failed: %v", err)
	}
	req.Header.Set("User-Agent", "CustomUA/1.0")
	req.Header.Set("Accept", "application/octet-stream")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do failed: %v", err)
	}
	defer resp.Body.Close()

	if capturedUA != "CustomUA/1.0" {
		t.Errorf("captured User-Agent = %q; want %q", capturedUA, "CustomUA/1.0")
	}
	if capturedAccept != "application/octet-stream" {
		t.Errorf("captured Accept = %q; want %q", capturedAccept, "application/octet-stream")
	}
}
