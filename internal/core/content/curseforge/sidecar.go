package curseforge

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveSidecarKey resolves the CurseForge API key using the following priority:
// 1. Environment variable NORD_CF_KEY or CURSEFORGE_API_KEY.
// 2. cf.key adjacent to the specified executable path (or os.Executable() if exePath is empty).
// 3. cf.key in the current working directory.
func ResolveSidecarKey(exePath string) (string, error) {
	// 1. Environment variable override
	if envKey := strings.TrimSpace(os.Getenv("NORD_CF_KEY")); envKey != "" {
		return envKey, nil
	}
	if envKey := strings.TrimSpace(os.Getenv("CURSEFORGE_API_KEY")); envKey != "" {
		return envKey, nil
	}

	// 2. Adjacent to specified or current executable
	if exePath == "" {
		var err error
		exePath, err = os.Executable()
		if err != nil {
			exePath = ""
		}
	}
	if exePath != "" {
		sidecar := filepath.Join(filepath.Dir(exePath), "cf.key")
		if data, err := os.ReadFile(sidecar); err == nil {
			k := strings.TrimSpace(string(data))
			if k != "" {
				return k, nil
			}
		}
	}

	// 3. Current working directory
	if data, err := os.ReadFile("cf.key"); err == nil {
		k := strings.TrimSpace(string(data))
		if k != "" {
			return k, nil
		}
	}

	return "", nil
}
