package java_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nord-launcher/launcher/internal/adapters/java"
)

func TestParseReleaseFile_Hermetic(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create a mock release file for Java 21
	releaseContent := `IMPLEMENTOR="Eclipse Adoptium"
IMPLEMENTOR_VERSION="Temurin-21.0.2+13"
JAVA_VERSION="21.0.2"
JAVA_VERSION_DATE="2024-01-16"
OS_NAME="Windows"
`
	if err := os.WriteFile(filepath.Join(tempDir, "release"), []byte(releaseContent), 0644); err != nil {
		t.Fatalf("failed to write mock release file: %v", err)
	}

	// 2. Create mock bin/java or bin/java.exe
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatalf("failed to create bin dir: %v", err)
	}

	javaExe := "java"
	if filepath.Separator == '\\' {
		javaExe = "java.exe"
	}
	if err := os.WriteFile(filepath.Join(binDir, javaExe), []byte("mock binary"), 0755); err != nil {
		t.Fatalf("failed to write mock java binary: %v", err)
	}

	// 3. Test ParseReleaseFile
	inst, err := java.ParseReleaseFile(tempDir)
	if err != nil {
		t.Fatalf("failed to parse release file: %v", err)
	}

	if inst.MajorVersion != 21 {
		t.Errorf("expected major 21, got %d", inst.MajorVersion)
	}
	if inst.FullVersion != "21.0.2" {
		t.Errorf("expected full version 21.0.2, got %s", inst.FullVersion)
	}
	if inst.Vendor != "Eclipse Adoptium" {
		t.Errorf("expected vendor Eclipse Adoptium, got %s", inst.Vendor)
	}

	// 4. Test Java 8 legacy format
	legacyDir := t.TempDir()
	legacyRelease := `JAVA_VERSION="1.8.0_391"
IMPLEMENTOR="Oracle Corporation"
`
	_ = os.WriteFile(filepath.Join(legacyDir, "release"), []byte(legacyRelease), 0644)
	legacyInst, err := java.ParseReleaseFile(legacyDir)
	if err != nil {
		t.Fatalf("failed to parse legacy release: %v", err)
	}
	if legacyInst.MajorVersion != 8 {
		t.Errorf("expected major 8 for 1.8.0_391, got %d", legacyInst.MajorVersion)
	}
}
