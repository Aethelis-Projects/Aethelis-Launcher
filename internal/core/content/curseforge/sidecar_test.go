package curseforge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSidecarKey(t *testing.T) {
	// Clean env during test
	oldNord := os.Getenv("NORD_CF_KEY")
	oldCF := os.Getenv("CURSEFORGE_API_KEY")
	defer func() {
		_ = os.Setenv("NORD_CF_KEY", oldNord)        // errcheck:ok restore env after test
		_ = os.Setenv("CURSEFORGE_API_KEY", oldCF)   // errcheck:ok restore env after test
	}()

	_ = os.Unsetenv("NORD_CF_KEY")      // errcheck:ok clean env for test isolation
	_ = os.Unsetenv("CURSEFORGE_API_KEY") // errcheck:ok clean env for test isolation

	tempDir := t.TempDir()
	fakeExe := filepath.Join(tempDir, "NordLauncher.exe")
	sidecarFile := filepath.Join(tempDir, "cf.key")

	// 1. None present
	key, err := ResolveSidecarKey(fakeExe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != "" {
		t.Errorf("expected empty key, got %q", key)
	}

	// 2. Adjacent to exe
	expectedKey := "test-sidecar-key-value-12345"
	if err := os.WriteFile(sidecarFile, []byte(expectedKey+"\n"), 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	key, err = ResolveSidecarKey(fakeExe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != expectedKey {
		t.Errorf("expected %q, got %q", expectedKey, key)
	}

	// 3. Env override takes precedence
	envKey := "env-override-key-999"
	_ = os.Setenv("NORD_CF_KEY", envKey) // errcheck:ok test env override
	key, err = ResolveSidecarKey(fakeExe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != envKey {
		t.Errorf("expected env key %q, got %q", envKey, key)
	}

	// 4. UserConfigDir fallback when adjacent is missing
	_ = os.Unsetenv("NORD_CF_KEY") // errcheck:ok clean env
	_ = os.Remove(sidecarFile)     // errcheck:ok remove adjacent file
	cfgDir := t.TempDir()
	oldAppData := os.Getenv("AppData")
	oldXDG := os.Getenv("XDG_CONFIG_HOME")
	defer func() {
		_ = os.Setenv("AppData", oldAppData)          // errcheck:ok restore env
		_ = os.Setenv("XDG_CONFIG_HOME", oldXDG)      // errcheck:ok restore env
	}()
	_ = os.Setenv("AppData", cfgDir)             // errcheck:ok override config dir for test
	_ = os.Setenv("XDG_CONFIG_HOME", cfgDir)     // errcheck:ok override config dir for test

	nordCfgDir := filepath.Join(cfgDir, "nord-launcher")
	_ = os.MkdirAll(nordCfgDir, 0755) // errcheck:ok test dir setup
	cfgKey := "cfg-sidecar-key-555"
	_ = os.WriteFile(filepath.Join(nordCfgDir, "cf.key"), []byte(cfgKey), 0644) // errcheck:ok write test key

	key, err = ResolveSidecarKey(fakeExe)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if key != cfgKey {
		t.Errorf("expected config dir key %q, got %q", cfgKey, key)
	}
}
