package ports

import (
	"context"
)

// JavaInstallation represents an installed Java Runtime Environment or Development Kit on the host.
type JavaInstallation struct {
	Path         string `json:"path"`          // Path to the java binary executable
	HomeDir      string `json:"home_dir"`      // Root directory of the JVM
	MajorVersion int    `json:"major_version"` // e.g., 8, 17, 21
	FullVersion  string `json:"full_version"`  // e.g., "21.0.2", "17.0.10"
	Vendor       string `json:"vendor"`        // e.g., "Eclipse Adoptium", "Oracle", "Temurin"
}

// JavaDetector defines the port for discovering Java installations on the operating system.
type JavaDetector interface {
	DetectInstallations(ctx context.Context) ([]JavaInstallation, error)
}
