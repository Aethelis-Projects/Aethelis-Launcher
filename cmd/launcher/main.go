package main

import (
	"fmt"
	iofs "io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nord-launcher/launcher/frontend"
	"github.com/nord-launcher/launcher/internal/adapters/fs"
	httpadapter "github.com/nord-launcher/launcher/internal/adapters/http"
	javaadapter "github.com/nord-launcher/launcher/internal/adapters/java"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	"github.com/nord-launcher/launcher/internal/adapters/wails"
	"github.com/nord-launcher/launcher/internal/core/auth"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/content/modrinth"
	"github.com/nord-launcher/launcher/internal/core/game"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/storage"
	"github.com/nord-launcher/launcher/internal/core/updater"
)

var (
	version           = "0.1.4"
	CurseForgeKey     = ""
	MicrosoftClientID = auth.DefaultClientID
	UpdateChannel     = "stable"
)

func init() {
	version = strings.TrimPrefix(version, "v")
}

func main() {
	startInit := time.Now()

	// 0. Clean up stale backup executable from previous update
	updater.CleanupStaleBackup()

	// 1. Initialize Hexagonal Core Ports & Adapters
	fileSys := fs.NewOSFileSystem()
	procMgr := process.NewProcessManager()
	keyRing := keyring.NewSystemKeyring()
	sysClock := clock.NewRealClock()

	// 2. Tuned Shared HTTP Client with Connection Pooling
	sharedHTTPClient := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        50,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
		Timeout: 30 * time.Second,
	}

	// 3. Initialize Database & Migrations
	appData, err := os.UserConfigDir()
	if err != nil {
		appData = "."
	}
	dbDir := filepath.Join(appData, "nord-launcher")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		fmt.Printf("Warning: Failed to create database directory %s: %v\n", dbDir, err)
	}
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

	// 4. Initialize Core Domain Services with Injected Configuration
	cfKey := CurseForgeKey
	if cfKey == "" {
		cfKey = os.Getenv("CURSEFORGE_API_KEY")
	}

	msClientID := MicrosoftClientID
	if envClientID := os.Getenv("MICROSOFT_CLIENT_ID"); envClientID != "" {
		msClientID = envClientID
	}

	instanceSvc := launch.NewInstanceService(instRepo, fileSys, procMgr, keyRing, sysClock)
	authAPIClient := auth.NewAPIClient(sharedHTTPClient, auth.DefaultEndpoints())
	authSvc := auth.NewAuthService(msClientID, authAPIClient, accRepo, keyRing)
	mrClient := modrinth.NewClient(modrinth.DefaultBaseURL, sharedHTTPClient)
	cfClient := curseforge.NewClient(curseforge.DefaultBaseURL, cfKey, sharedHTTPClient)

	// Java detector with local instance and runtime directory scanning
	javaDetector := javaadapter.NewJavaDetector(filepath.Join(dbDir, "runtimes"))
	instanceSvc.SetJavaDetector(javaDetector)
	instanceSvc.SetAccountRepository(accRepo)
	instanceSvc.SetSessionRefresher(authSvc)

	// Game provisioner
	httpAdapter := httpadapter.NewHTTPClient(30 * time.Second)
	gameProvisioner := game.NewGameService(httpAdapter, fileSys, dbDir)
	instanceSvc.SetProvisioner(gameProvisioner)

	// Auto-updater wired with configured channel and embedded Ed25519 public key
	manifestURL := fmt.Sprintf("https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/latest/download/manifest-%s.json", UpdateChannel)
	autoUpdater := updater.NewAutoUpdater(version, manifestURL, updater.GetDefaultPublicKey(), sharedHTTPClient)

	// 5. Initialize Wails IPC Adapter
	adapter := wails.NewWailsAdapter(instanceSvc)
	adapter.SetAuth(authSvc, accRepo)
	adapter.SetContent(mrClient, cfClient)
	adapter.SetFileSystem(fileSys, filepath.Join(dbDir, "instances"))
	adapter.SetUpdater(autoUpdater)
	adapter.SetJavaDetector(javaDetector)

	coreInitDuration := time.Since(startInit)

	if len(os.Args) > 1 && os.Args[1] == "--idle-test" {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fmt.Printf("CORE_INIT_TIME_MS: %d\n", coreInitDuration.Milliseconds())
		fmt.Printf("IDLE_HEAP_ALLOC_MB: %.2f\n", float64(m.Alloc)/(1024*1024))
		fmt.Printf("IDLE_SYS_MB: %.2f\n", float64(m.Sys)/(1024*1024))
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "--headless" {
		fmt.Printf("Nord Launcher core initialized in %d ms. Database: %s. Instances registered: %d\n",
			coreInitDuration.Milliseconds(), dbPath, len(adapter.ListInstances()))
		return
	}

	// 6. Initialize Embedded Assets & Wails v3 Desktop Window
	assetsSub, err := iofs.Sub(frontend.Dist, "dist")
	if err != nil {
		panic(fmt.Sprintf("failed to load embedded frontend: %v", err))
	}

	app := application.New(application.Options{
		Name:        "Nord Launcher",
		Description: "High-performance, resource-efficient Minecraft launcher",
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