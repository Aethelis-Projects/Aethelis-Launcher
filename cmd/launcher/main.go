package main

import (
	"fmt"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/adapters/wails"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/launch"
)

func main() {
	// 1. Initialize Hexagonal Core Ports & Adapters
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	keyRing := keyring.NewMemoryKeyring()
	sysClock := clock.NewRealClock()

	// 2. Initialize Pure Core Domain Services (Zero GUI / Zero Wails dependencies)
	instanceSvc := launch.NewInstanceService(fileSys, procMgr, keyRing, sysClock)

	// 3. Initialize Wails IPC Adapter
	adapter := wails.NewWailsAdapter(instanceSvc)

	fmt.Printf("Nord Launcher core initialized. Instances registered: %d\n", len(adapter.ListInstances()))
}
