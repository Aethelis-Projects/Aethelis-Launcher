package java

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nord-launcher/launcher/internal/core/ports"
)

// ParseJavaMajor parses "21.0.2", "17.0.10", or "1.8.0_391" into an integer major version.
func ParseJavaMajor(fullVersion string) int {
	clean := strings.Trim(fullVersion, `"'`)
	if strings.HasPrefix(clean, "1.") {
		parts := strings.Split(clean, ".")
		if len(parts) >= 2 {
			m, _ := strconv.Atoi(parts[1])
			return m
		}
	}
	parts := strings.Split(clean, ".")
	if len(parts) >= 1 {
		m, _ := strconv.Atoi(parts[0])
		return m
	}
	return 0
}

// ParseReleaseFile reads a JDK 'release' file and constructs a JavaInstallation if valid.
func ParseReleaseFile(homeDir string) (*ports.JavaInstallation, error) {
	releasePath := filepath.Join(homeDir, "release")
	f, err := os.Open(releasePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var javaVersion string
	var vendor string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "JAVA_VERSION=") {
			javaVersion = strings.Trim(strings.TrimPrefix(line, "JAVA_VERSION="), `"'`)
		} else if strings.HasPrefix(line, "IMPLEMENTOR=") {
			vendor = strings.Trim(strings.TrimPrefix(line, "IMPLEMENTOR="), `"'`)
		}
	}

	if javaVersion == "" {
		return nil, os.ErrNotExist
	}

	major := ParseJavaMajor(javaVersion)
	if major == 0 {
		return nil, os.ErrNotExist
	}

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}
	binPath := filepath.Join(homeDir, "bin", javaExe)

	return &ports.JavaInstallation{
		Path:         binPath,
		HomeDir:      homeDir,
		MajorVersion: major,
		FullVersion:  javaVersion,
		Vendor:       vendor,
	}, nil
}
