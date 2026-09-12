package ports

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/nord-launcher/launcher/internal/core/domain"
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

// InstanceRepository defines storage operations for instances.
type InstanceRepository interface {
	Save(ctx context.Context, inst *domain.Instance) error
	GetByID(ctx context.Context, id string) (*domain.Instance, error)
	ListAll(ctx context.Context) ([]*domain.Instance, error)
	Delete(ctx context.Context, id string) error
	UpdateState(ctx context.Context, id string, state domain.InstanceState) error
}

// AccountRepository defines storage operations for user accounts.
type AccountRepository interface {
	Save(ctx context.Context, acc *domain.Account) error
	GetByUUID(ctx context.Context, uuid string) (*domain.Account, error)
	GetActive(ctx context.Context) (*domain.Account, error)
	ListAll(ctx context.Context) ([]*domain.Account, error)
	SetActive(ctx context.Context, uuid string) error
	Delete(ctx context.Context, uuid string) error
}

// SettingsRepository defines persistent key-value configuration.
type SettingsRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	GetAll(ctx context.Context) (map[string]string, error)
}

// GameProvisioner resolves, downloads, and validates game files, returning a complete LaunchConfig.
type GameProvisioner interface {
	Provision(ctx context.Context, inst *domain.Instance, acc *domain.Account) (*domain.LaunchConfig, error)
}

