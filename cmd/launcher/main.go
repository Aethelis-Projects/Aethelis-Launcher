package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/adapters/wails"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/storage"
)

func main() {
	// 1. Initialize Hexagonal Core Ports & Adapters
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	keyRing := keyring.NewMemoryKeyring()
	sysClock := clock.NewRealClock()

	// 2. Initialize Database & Migrations
	appData, err := os.UserConfigDir()
	if err != nil {
		appData = "."
	}
	dbDir := filepath.Join(appData, "nord-launcher")
	_ = os.MkdirAll(dbDir, 0755)
	dbPath := filepath.Join(dbDir, "nord.db")

	db, err := storage.OpenDatabase(dbPath)
	if err != nil {
		fmt.Printf("Warning: Failed to open SQLite database: %v. Running in in-memory mode.\n", err)
	} else {
		defer db.Close()
		if err := db.Migrate(); err != nil {
			fmt.Printf("Warning: Failed to run migrations: %v\n", err)
		}
	}

	var instRepo *storage.InstanceRepository
	if db != nil {
		instRepo = storage.NewInstanceRepository(db)
	}

	// 3. Initialize Pure Core Domain Services (Zero GUI / Zero Wails dependencies)
	instanceSvc := launch.NewInstanceService(instRepo, fileSys, procMgr, keyRing, sysClock)

	// 4. Initialize Wails IPC Adapter
	adapter := wails.NewWailsAdapter(instanceSvc)

	fmt.Printf("Nord Launcher core initialized. Database: %s. Instances registered: %d\n", dbPath, len(adapter.ListInstances()))
}