package wails_test

import (
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/adapters/wails"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/launch"
)

func TestWailsAdapter_IPCBridge(t *testing.T) {
	clk := clock.NewMockClock(time.Now())
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	kr := keyring.NewMemoryKeyring()

	svc := launch.NewInstanceService(fileSys, procMgr, kr, clk)
	adapter := wails.NewWailsAdapter(svc)

	// Test create via IPC DTO
	req := wails.CreateInstanceRequest{
		Name:        "Nord-Test",
		GameVersion: "1.21.1",
		Loader:      "fabric",
	}

	dto, err := adapter.CreateInstance(req)
	if err != nil {
		t.Fatalf("unexpected error creating instance: %v", err)
	}
	if dto.Name != "Nord-Test" || dto.State != "idle" {
		t.Fatalf("unexpected DTO: %+v", dto)
	}

	// Test list via IPC
	list := adapter.ListInstances()
	if len(list) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(list))
	}

	// Test launch via IPC
	res, err := adapter.LaunchInstance(dto.ID)
	if err != nil {
		t.Fatalf("launch error: %v", err)
	}
	if !res.Success || res.PID <= 0 {
		t.Fatalf("launch failed: %+v", res)
	}
}
