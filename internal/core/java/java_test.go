package java_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/java"
	"github.com/nord-launcher/launcher/internal/core/netutil"
)

func TestResolveJavaMajor(t *testing.T) {
	tests := []struct {
		mcVersion     string
		expectedMajor int
	}{
		// Modern 26.1+ -> Java 25
		{"26.3", 25},
		{"26.1", 25},

		// Modern 1.20.5 - 26.0 -> Java 21
		{"26.0", 21},
		{"1.21.1", 21},
		{"1.21", 21},
		{"1.20.6", 21},
		{"1.20.5", 21},

		// 1.17 to 1.20.4 -> Java 17
		{"1.20.4", 17},
		{"1.20.1", 17},
		{"1.19.4", 17},
		{"1.18.2", 17},
		{"1.17.1", 17},
		{"1.17", 17},

		// Legacy <= 1.16.5 -> Java 8
		{"1.16.5", 8},
		{"1.16.1", 8},
		{"1.12.2", 8},
		{"1.7.10", 8},
		{"1.2.5", 8},
	}

	for _, tt := range tests {
		t.Run("MC_"+tt.mcVersion, func(t *testing.T) {
			major, err := java.ResolveJavaMajor(tt.mcVersion)
			if err != nil {
				t.Fatalf("unexpected error resolving java for %s: %v", tt.mcVersion, err)
			}
			if major != tt.expectedMajor {
				t.Errorf("mc %s: expected java %d, got %d", tt.mcVersion, tt.expectedMajor, major)
			}
		})
	}
}

func TestAdoptiumClient_GetLatestRelease_Java25(t *testing.T) {
	mockResponse := `[
		{
			"binaries": [
				{
					"image_type": "jdk",
					"os": "windows",
					"architecture": "x64",
					"package": {
						"name": "OpenJDK25U-jdk_x64_windows_hotspot_25.0.0_1.zip",
						"link": "https://github.com/adoptium/temurin25-binaries/releases/download/jdk-25.0.0%2B1/OpenJDK25U-jdk_x64_windows_hotspot_25.0.0_1.zip",
						"checksum": "e25a273b4eb8dc6a5ef4ffec6cfcf7918a5956a643ee1c6f932ea439062ee960",
						"size": 215000000
					}
				}
			],
			"version_data": {
				"semver": "25.0.0+1"
			}
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mockResponse)) // errcheck:ok mock response write
	}))
	defer srv.Close()

	client := java.NewAdoptiumClient(srv.URL, srv.Client())
	asset, err := client.GetLatestRelease(context.Background(), 25)
	if err != nil {
		t.Fatalf("failed to fetch release for java 25: %v", err)
	}

	if asset.Name != "OpenJDK25U-jdk_x64_windows_hotspot_25.0.0_1.zip" {
		t.Errorf("unexpected asset name: %s", asset.Name)
	}
	if asset.Version != "25.0.0+1" {
		t.Errorf("unexpected version: %s", asset.Version)
	}
}

func TestAdoptiumClient_GetLatestRelease(t *testing.T) {
	mockResponse := `[
		{
			"binaries": [
				{
					"image_type": "jdk",
					"os": "windows",
					"architecture": "x64",
					"package": {
						"name": "OpenJDK21U-jdk_x64_windows_hotspot_21.0.2_13.zip",
						"link": "https://github.com/adoptium/temurin21-binaries/releases/download/jdk-21.0.2%2B13/OpenJDK21U-jdk_x64_windows_hotspot_21.0.2_13.zip",
						"checksum": "d50a273b4eb8dc6a5ef4ffec6cfcf7918a5956a643ee1c6f932ea439062ee960",
						"size": 204910240
					}
				}
			],
			"version_data": {
				"semver": "21.0.2+13"
			}
		}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(mockResponse))
	}))
	defer srv.Close()

	client := java.NewAdoptiumClient(srv.URL, srv.Client())
	asset, err := client.GetLatestRelease(context.Background(), 21)
	if err != nil {
		t.Fatalf("failed to fetch release: %v", err)
	}

	if asset.Name != "OpenJDK21U-jdk_x64_windows_hotspot_21.0.2_13.zip" {
		t.Errorf("unexpected asset name: %s", asset.Name)
	}
	if asset.SHA256 != "d50a273b4eb8dc6a5ef4ffec6cfcf7918a5956a643ee1c6f932ea439062ee960" {
		t.Errorf("unexpected checksum: %s", asset.SHA256)
	}
	if asset.Size != 204910240 {
		t.Errorf("unexpected size: %d", asset.Size)
	}
	if asset.Version != "21.0.2+13" {
		t.Errorf("unexpected version: %s", asset.Version)
	}
}

func TestMapGoPlatformToAdoptium(t *testing.T) {
	cases := []struct {
		goos, goarch string
		wantOS       string
		wantArch     string
	}{
		{"windows", "amd64", "windows", "x64"},
		{"linux", "arm64", "linux", "aarch64"},
		{"darwin", "amd64", "mac", "x64"},
		{"freebsd", "riscv64", "freebsd", "riscv64"},
	}

	for _, c := range cases {
		gotOS, gotArch := java.MapGoPlatformToAdoptium(c.goos, c.goarch)
		if gotOS != c.wantOS || gotArch != c.wantArch {
			t.Errorf("MapGoPlatformToAdoptium(%s, %s) = (%s, %s); want (%s, %s)",
				c.goos, c.goarch, gotOS, gotArch, c.wantOS, c.wantArch)
		}
	}
}

func TestResolveJavaMajor_Invalid(t *testing.T) {
	_, err := java.ResolveJavaMajor("")
	if err == nil {
		t.Errorf("expected error for empty version, got nil")
	}
}

func TestAdoptiumClient_ErrorCases(t *testing.T) {
	// 500 error
	errSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer errSrv.Close()

	client := java.NewAdoptiumClient(errSrv.URL, errSrv.Client())
	_, err := client.GetLatestRelease(context.Background(), 21)
	if err == nil {
		t.Errorf("expected error on 500 server response, got nil")
	}

	// Empty array
	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer emptySrv.Close()

	emptyClient := java.NewAdoptiumClient(emptySrv.URL, emptySrv.Client())
	_, err = emptyClient.GetLatestRelease(context.Background(), 21)
	if err == nil {
		t.Errorf("expected error on empty response array, got nil")
	}
}

func TestAdoptiumClient_Headers_UserAgentAndAccept(t *testing.T) {
	var receivedUA string
	var receivedAccept string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		receivedAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	expectedUA := netutil.FormatUserAgent("0.6.0")

	// 1. Client with custom http.Client (wraps Transport)
	client := java.NewAdoptiumClient(srv.URL, srv.Client())
	_, _ = client.GetLatestRelease(context.Background(), 21) // errcheck:ok testing headers on mock response

	if receivedUA != expectedUA {
		t.Fatalf("expected User-Agent %q, got %q", expectedUA, receivedUA)
	}
	if receivedAccept != "application/json" {
		t.Fatalf("expected Accept application/json, got %q", receivedAccept)
	}

	// 2. Client with default http.Client (nil passed, uses netutil.NewHTTPClient)
	receivedUA = ""
	receivedAccept = ""
	defaultClient := java.NewAdoptiumClient(srv.URL, nil)
	_, _ = defaultClient.GetLatestRelease(context.Background(), 21) // errcheck:ok testing headers on mock response

	if receivedUA != expectedUA {
		t.Fatalf("expected default User-Agent %q, got %q", expectedUA, receivedUA)
	}
	if receivedAccept != "application/json" {
		t.Fatalf("expected default Accept application/json, got %q", receivedAccept)
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected bool
	}{
		{"21.0.2", "21.0.3+9", true},
		{"21.0.2+13", "21.0.2+14", true},
		{"21.0.2-13", "21.0.2-14", true},
		{"21.0.2", "21.0.2+13", true},
		{"21.0.2+13", "21.0.2", false},
		{"21.0.2+13", "21.0.2+13", false},
		{"21.0.2", "21.0.2", false},
		{"17.0.10", "17.0.9", false},
		{"8u391", "8u402", true},
		{"", "21.0.2", false},
		{"21.0.2", "", false},
	}

	for _, tt := range tests {
		got := java.IsNewerVersion(tt.current, tt.latest)
		if got != tt.expected {
			t.Errorf("IsNewerVersion(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.expected)
		}
	}
}

