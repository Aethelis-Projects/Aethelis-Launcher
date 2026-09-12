package main

import (
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nord-launcher/launcher/frontend"
	"github.com/nord-launcher/launcher/internal/adapters/fs"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/adapters/wails"
	"github.com/nord-launcher/launcher/internal/core/auth"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/content/modrinth"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/storage"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--idle-test" {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fmt.Printf("IDLE_HEAP_ALLOC_MB: %.2f\n", float64(m.Alloc)/(1024*1024))
		fmt.Printf("IDLE_SYS_MB: %.2f\n", float64(m.Sys)/(1024*1024))
		return
	}

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
	var accRepo *storage.AccountRepository
	if db != nil {
		instRepo = storage.NewInstanceRepository(db)
		accRepo = storage.NewAccountRepository(db)
	}

	// 3. Initialize Core Domain Services
	instanceSvc := launch.NewInstanceService(instRepo, fileSys, procMgr, keyRing, sysClock)
	authSvc := auth.NewAuthService("", nil, accRepo, keyRing)
	mrClient := modrinth.NewClient("", nil)
	cfClient := curseforge.NewClient("", "", nil)

	// 4. Initialize Wails IPC Adapter
	adapter := wails.NewWailsAdapter(instanceSvc)
	adapter.SetAuth(authSvc, accRepo)
	adapter.SetContent(mrClient, cfClient)
	adapter.SetFileSystem(fileSys, filepath.Join(dbDir, "instances"))

	if len(os.Args) > 1 && os.Args[1] == "--headless" {
		fmt.Printf("Nord Launcher core initialized. Database: %s. Instances registered: %d\n", dbPath, len(adapter.ListInstances()))
		return
	}

	// 5. Initialize Embedded Assets & Wails v3 Desktop Window
	assetsSub, err := iofs.Sub(frontend.Dist, "dist")
	if err != nil {
		panic(fmt.Sprintf("failed to load embedded frontend: %v", err))
	}

	app := application.New(application.Options{
		Name:        "Nord Launcher",
		Description: "High-performance anti-AI-slop Minecraft launcher",
		Services: []application.Service{
			application.NewService(adapter),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assetsSub),
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Nord Launcher",
		Width:            1120,
		Height:           720,
		MinWidth:         860,
		MinHeight:        580,
		BackgroundColour: application.NewRGB(9, 9, 11), // #09090b
	})

	if err := app.Run(); err != nil {
		fmt.Printf("Fatal: failed to run Wails application: %v\n", err)
	}
}