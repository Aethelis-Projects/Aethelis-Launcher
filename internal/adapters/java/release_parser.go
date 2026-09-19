package java

import (
	corejava "github.com/nord-launcher/launcher/internal/core/java"
	"github.com/nord-launcher/launcher/internal/core/ports"
)

// ParseJavaMajor parses "21.0.2", "17.0.10", or "1.8.0_391" into an integer major version.
func ParseJavaMajor(fullVersion string) int {
	major, _ := corejava.ParseJavaMajor(fullVersion) // errcheck:ok fallback to 0 on parse error
	return major
}

// ParseReleaseFile reads a JDK 'release' file and constructs a JavaInstallation if valid.
func ParseReleaseFile(homeDir string) (*ports.JavaInstallation, error) {
	return corejava.ParseReleaseFile(homeDir)
}

