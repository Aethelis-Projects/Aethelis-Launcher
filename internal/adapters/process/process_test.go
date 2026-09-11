package process_test

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/process"
)

func TestProcessManager_Lifecycle(t *testing.T) {
	mgr := process.NewProcessManager()

	var execName string
	var args []string

	if runtime.GOOS == "windows" {
		execName = "cmd.exe"
		args = []string{"/c", "echo NordLauncherProcessSupervision"}
	} else {
		execName = "echo"
		args = []string{"NordLauncherProcessSupervision"}
	}

	var stdout bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handle, err := mgr.StartProcess(ctx, execName, args, "", nil, &stdout, nil)
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	if handle.PID() <= 0 {
		t.Fatalf("expected valid PID, got %d", handle.PID())
	}

	exitCode, err := handle.Wait()
	if err != nil {
		t.Fatalf("process wait error: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	outStr := strings.TrimSpace(stdout.String())
	if !strings.Contains(outStr, "NordLauncherProcessSupervision") {
		t.Fatalf("unexpected stdout: %s", outStr)
	}
}

func TestProcessManager_Kill(t *testing.T) {
	mgr := process.NewProcessManager()

	var execName string
	var args []string

	if runtime.GOOS == "windows" {
		execName = "cmd.exe"
		args = []string{"/c", "ping -n 10 127.0.0.1 > nul"}
	} else {
		execName = "sleep"
		args = []string{"10"}
	}

	ctx := context.Background()
	handle, err := mgr.StartProcess(ctx, execName, args, "", nil, nil, nil)
	if err != nil {
		t.Fatalf("failed to start long running process: %v", err)
	}

	if handle.PID() <= 0 {
		t.Fatalf("expected PID > 0, got %d", handle.PID())
	}

	// Kill immediately
	if err := handle.Kill(); err != nil {
		t.Fatalf("failed to kill process: %v", err)
	}

	exitCode, err := handle.Wait()
	// Process was killed, non-zero or error
	if exitCode == 0 && err == nil {
		t.Fatal("expected killed process to not exit with 0")
	}
}