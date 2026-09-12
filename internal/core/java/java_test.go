package java_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/java"
)

func TestResolveJavaMajor(t *testing.T) {
	tests := []struct {
		mcVersion     string
		expectedMajor int
	}{
		// Modern 1.20.5+ -> Java 21
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
