package ports

import (
	"context"
	"io"
	"os"
	"time"
)

// FileSystem defines file operations required by the core.
type FileSystem interface {
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	Exists(path string) bool
	Remove(path string) error
	RemoveAll(path string) error
	Stat(path string) (os.FileInfo, error)
	Open(path string) (io.ReadCloser, error)
	Create(path string) (io.WriteCloser, error)
	Rename(oldPath, newPath string) error
}

// HTTPClient defines network operations for metadata and asset downloads.
type HTTPClient interface {
	Get(ctx context.Context, url string, headers map[string]string) ([]byte, error)
	DownloadFile(ctx context.Context, url string, destPath string, expectedSHA1 string, onProgress func(bytesRead, totalBytes int64)) error
}

// ProcessManager defines game and Java process execution.
type ProcessManager interface {
	StartProcess(ctx context.Context, executable string, args []string, dir string, env []string, stdout, stderr io.Writer) (ProcessHandle, error)
}

// ProcessHandle represents an active supervised operating system process.
type ProcessHandle interface {
	PID() int
	Wait() (int, error)
	Kill() error
}

// Keyring defines secure credential storage for session and refresh tokens.
type Keyring interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

// Clock defines time abstraction for deterministic testing.
type Clock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
	Sleep(d time.Duration)
}
