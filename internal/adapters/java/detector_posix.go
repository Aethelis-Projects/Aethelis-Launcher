//go:build !windows

package java

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/nord-launcher/launcher/internal/core/ports"
)

type PosixJavaDetector struct {
	customDirs []string
}

func NewJavaDetector(customDirs ...string) ports.JavaDetector {
	return &PosixJavaDetector{
		customDirs: customDirs,
	}
}

func (d *PosixJavaDetector) DetectInstallations(ctx context.Context) ([]ports.JavaInstallation, error) {
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

	// 1. Custom runtime dirs
	for _, dir := range d.customDirs {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					addIfValid(filepath.Join(dir, entry.Name()))
				}
			}
		}
	}

	// 2. ~/.nord-launcher/runtimes
	if home, err := os.UserHomeDir(); err == nil {
		runtimeDir := filepath.Join(home, ".nord-launcher", "runtimes")
		if entries, err := os.ReadDir(runtimeDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					addIfValid(filepath.Join(runtimeDir, entry.Name()))
				}
			}
		}
	}

	// 3. JAVA_HOME
	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		addIfValid(jh)
	}

	// 4. Common POSIX JVM paths (/usr/lib/jvm)
	commonDirs := []string{
		"/usr/lib/jvm",
		"/usr/java",
		"/opt/jdk",
		"/Library/Java/JavaVirtualMachines",
	}
	for _, base := range commonDirs {
		if entries, err := os.ReadDir(base); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					addIfValid(filepath.Join(base, entry.Name()))
				}
			}
		}
	}

	// 5. Scan PATH
	pathEnv := os.Getenv("PATH")
	for _, p := range strings.Split(pathEnv, string(filepath.ListSeparator)) {
		if p == "" {
			continue
		}
		javaBin := filepath.Join(p, "java")
		if _, err := os.Stat(javaBin); err == nil {
			parent := filepath.Dir(p)
			addIfValid(parent)
		}
	}

	return results, nil
}
