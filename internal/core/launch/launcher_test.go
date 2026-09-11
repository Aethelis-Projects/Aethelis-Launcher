package launch_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/launch"
)

func TestInstanceService_HeadlessLifecycle(t *testing.T) {
	mockTime := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	clk := clock.NewMockClock(mockTime)
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(fileSys, procMgr, kr, clk)

	// Test validation: empty name
	_, err := svc.CreateInstance("", "1.21.1", domain.LoaderFabric)
	if !errors.Is(err, domain.ErrInvalidConfig) {
		t.Fatalf("expected ErrInvalidConfig, got %v", err)
	}

	// Test valid creation
	inst, err := svc.CreateInstance("Nord-Pack", "1.21.1", domain.LoaderFabric)
	if err != nil {
		t.Fatalf("failed to create instance: %v", err)
	}
	if inst.ID == "" || inst.Name != "Nord-Pack" {
		t.Fatalf("unexpected instance data: %+v", inst)
	}
	if inst.State != domain.StateIdle {
		t.Fatalf("expected StateIdle, got %s", inst.State)
	}

	// Test listing
	list := svc.ListInstances()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance in list, got %d", len(list))
	}

	// Test launch
	pid, err := svc.Launch(context.Background(), inst.ID)
	if err != nil {
		t.Fatalf("launch failed: %v", err)
	}
	if pid <= 0 {
		t.Fatalf("invalid pid returned: %d", pid)
	}

	updated, err := svc.GetInstance(inst.ID)
	if err != nil {
		t.Fatalf("failed to get instance: %v", err)
	}
	if updated.State != domain.StateRunning {
		t.Fatalf("expected StateRunning, got %s", updated.State)
	}
	if updated.LastPlayedAt == nil {
		t.Fatal("expected LastPlayedAt to be recorded")
	}
}
