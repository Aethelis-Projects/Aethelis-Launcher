//go:build windows

package java

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/nord-launcher/launcher/internal/core/ports"
)

type WindowsJavaDetector struct {
	customDirs []string
}

func NewJavaDetector(customDirs ...string) ports.JavaDetector {
	return &WindowsJavaDetector{
		customDirs: customDirs,
	}
}

func (d *WindowsJavaDetector) DetectInstallations(ctx context.Context) ([]ports.JavaInstallation, error) {
	var results []ports.JavaInstallation
	seen := make(map[string]bool)

	addIfValid := func(dir string) {
		clean := filepath.Clean(dir)
		if seen[clean] {
			return
		}
		if inst, err := ParseReleaseFile(clean); err == nil {
			if _, statErr := os.Stat(inst.Path); statErr == nil {
				seen[clean] = true
				results = append(results, *inst)
			}
		}
	}

	// 1. Custom and Nord Launcher runtime directories
	for _, dir := range d.customDirs {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					addIfValid(filepath.Join(dir, entry.Name()))
				}
			}
		}
	}

	// 2. Check %LOCALAPPDATA%\NordLauncher\runtimes
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		runtimeDir := filepath.Join(localAppData, "NordLauncher", "runtimes")
		if entries, err := os.ReadDir(runtimeDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					addIfValid(filepath.Join(runtimeDir, entry.Name()))
				}
			}
		}
	}

	// 3. Check JAVA_HOME
	if javaHome := os.Getenv("JAVA_HOME"); javaHome != "" {
		addIfValid(javaHome)
	}

	// 4. Check Registry: HKLM\SOFTWARE\JavaSoft and HKLM\SOFTWARE\Eclipse Adoptium
	regKeys := []string{
		`SOFTWARE\JavaSoft\JDK`,
		`SOFTWARE\JavaSoft\Java Development Kit`,
		`SOFTWARE\JavaSoft\Java Runtime Environment`,
		`SOFTWARE\Eclipse Adoptium\JDK`,
		`SOFTWARE\Eclipse Adoptium\JRE`,
	}

	for _, keyPath := range regKeys {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.READ)
		if err != nil {
			continue
		}
		subkeys, err := k.ReadSubKeyNames(-1)
		_ = k.Close()
		if err != nil {
			continue
		}

		for _, sub := range subkeys {
			subKey, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath+`\`+sub, registry.READ)
			if err != nil {
				continue
			}
			home, _, err := subKey.GetStringValue("JavaHome")
			_ = subKey.Close()
			if err == nil && home != "" {
				addIfValid(home)
			}
		}
	}

	// 5. Scan PATH
	pathEnv := os.Getenv("PATH")
	for _, p := range strings.Split(pathEnv, string(filepath.ListSeparator)) {
		if p == "" {
			continue
		}
		javaExe := filepath.Join(p, "java.exe")
		if _, err := os.Stat(javaExe); err == nil {
			parent := filepath.Dir(p)
			addIfValid(parent)
		}
	}

	return results, nil
}
