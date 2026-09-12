package downloader

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicReplace(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "dst.txt")

	_ = os.WriteFile(src, []byte("hello"), 0644)
	if err := atomicReplace(src, dst); err != nil {
		t.Fatalf("atomicReplace failed on new dst: %v", err)
	}

	content, _ := os.ReadFile(dst)
	if string(content) != "hello" {
		t.Fatalf("expected hello, got %s", string(content))
	}

	// Now replace existing dst
	src2 := filepath.Join(tmpDir, "src2.txt")
	_ = os.WriteFile(src2, []byte("world"), 0644)
	if err := atomicReplace(src2, dst); err != nil {
		t.Fatalf("atomicReplace failed on existing dst: %v", err)
	}

	content2, _ := os.ReadFile(dst)
	if string(content2) != "world" {
		t.Fatalf("expected world, got %s", string(content2))
	}
}

func TestChecksumMismatchError(t *testing.T) {
	err := &ChecksumMismatchError{
		Algorithm: "SHA256",
		Expected:  "abc",
		Actual:    "def",
		FilePath:  "file.jar",
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error string")
	}
}
