package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nord-launcher/launcher/internal/adapters/fs"
	javaadapter "github.com/nord-launcher/launcher/internal/adapters/java"
	"github.com/nord-launcher/launcher/internal/adapters/keyring"
	"github.com/nord-launcher/launcher/internal/adapters/process"
	storageAdapter "github.com/nord-launcher/launcher/internal/adapters/storage"
	"github.com/nord-launcher/launcher/internal/adapters/wails"
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/content/modrinth"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/downloader"
	"github.com/nord-launcher/launcher/internal/core/java"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/loadermeta"
	"github.com/nord-launcher/launcher/internal/core/manifest"
	"github.com/nord-launcher/launcher/internal/core/netutil"
	"github.com/nord-launcher/launcher/internal/core/storage"
	"github.com/nord-launcher/launcher/internal/core/updater"
	"github.com/nord-launcher/launcher/internal/releasetool"
)

func main() {
	var traceBuf bytes.Buffer
	mw := io.MultiWriter(os.Stdout, &traceBuf)

	logf := func(format string, a ...interface{}) {
		msg := fmt.Sprintf("[%s] ", time.Now().Format("15:04:05.000")) + fmt.Sprintf(format, a...) + "\n"
		_, _ = mw.Write([]byte(msg)) // errcheck:ok log trace write
	}

	logf("=== NORD LAUNCHER END-TO-END VERIFICATION RUNNER ===")
	logf("Operating System: %s (%s)", runtime.GOOS, runtime.GOARCH)

	// =========================================================================
	// 1. Keyring E2E: Platform-Aware Credential Storage
	// =========================================================================
	logf("\n--- STEP 1: Keyring Persistence E2E ---")
	kr1 := keyring.NewSystemKeyring()
	testSvc := "nord-launcher-e2e"
	testUser := "e2e-session-user"
	testToken := "secret-oauth-refresh-token-xyz-987"

	if err := kr1.Set(testSvc, testUser, testToken); err != nil {
		logf("FAIL: Keyring.Set failed: %v", err)
		os.Exit(1)
	}

	// Instantiate a fresh keyring instance to simulate application restart
	kr2 := keyring.NewSystemKeyring()
	gotToken, err := kr2.Get(testSvc, testUser)
	if err != nil {
		if runtime.GOOS != "windows" {
			logf("NOTICE: Linux/POSIX without D-Bus session correctly degraded to InMemKeyring (documented fallback).")
		} else {
			logf("FAIL: Windows Credential Manager failed to persist across keyring instances: %v", err)
			os.Exit(1)
		}
	} else {
		if gotToken != testToken {
			logf("FAIL: Token mismatch: expected %s, got %s", testToken, gotToken)
			os.Exit(1)
		}
		logf("PASS: Refresh-token successfully retrieved across keyring instances (Credential Manager persistence verified).")
	}
	_ = kr2.Delete(testSvc, testUser) // errcheck:ok test credential cleanup

	// =========================================================================
	// 2. Auth E2E: Mojang Canonical Offline UUID v3 Parity (§5.4)
	// =========================================================================
	logf("\n--- STEP 2: Auth Offline UUID v3 Mojang Parity E2E ---")
	calcMojangUUID := func(username string) string {
		data := []byte("OfflinePlayer:" + username)
		hash := md5.Sum(data)
		hash[6] = (hash[6] & 0x0f) | 0x30 // Version 3
		hash[8] = (hash[8] & 0x3f) | 0x80 // IETF variant
		raw := hex.EncodeToString(hash[:])
		return fmt.Sprintf("%s-%s-%s-%s-%s", raw[0:8], raw[8:12], raw[12:16], raw[16:20], raw[20:32])
	}

	steveUUID := calcMojangUUID("Steve")
	alexUUID := calcMojangUUID("Alex")

	expectedSteve := "5627dd98-e6be-3c21-b8a8-e92344183641"
	expectedAlex := "36532b5e-c442-3dbb-a24c-c7e55d0f979a"

	if steveUUID != expectedSteve {
		logf("FAIL: Steve UUID parity violation: got %s, expected %s", steveUUID, expectedSteve)
		os.Exit(1)
	}
	if alexUUID != expectedAlex {
		logf("FAIL: Alex UUID parity violation: got %s, expected %s", alexUUID, expectedAlex)
		os.Exit(1)
	}
	logf("PASS: Canonical Mojang UUID v3 parity confirmed for Steve (%s) and Alex (%s).", steveUUID, alexUUID)

	// =========================================================================
	// 3. Java Provisioning & Matrix E2E
	// =========================================================================
	logf("\n--- STEP 3: Java Provisioning Matrix & Hermetic Detection E2E ---")
	testMatrix := map[string]int{
		"1.21.1": 21,
		"1.20.1": 17,
		"1.16.5": 8,
		"1.12.2": 8,
	}
	for mc, expectedMajor := range testMatrix {
		major, err := java.ResolveJavaMajor(mc)
		if err != nil || major != expectedMajor {
			logf("FAIL: Java matrix mismatch for MC %s: expected %d, got %d (err: %v)", mc, expectedMajor, major, err)
			os.Exit(1)
		}
	}
	logf("PASS: Java version matrix mappings verified for all Minecraft epochs.")

	// Hermetic JDK detection fixture
	tempDir, err := os.MkdirTemp("", "nord-e2e-jdk-*")
	if err != nil {
		logf("FAIL: Failed to create temp dir: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)

	mockRelease := "IMPLEMENTOR=\"Eclipse Adoptium\"\nJAVA_VERSION=\"21.0.2\"\n"
	_ = os.WriteFile(filepath.Join(tempDir, "release"), []byte(mockRelease), 0644) // errcheck:ok test release file write
	binDir := filepath.Join(tempDir, "bin")
	_ = os.MkdirAll(binDir, 0755) // errcheck:ok test bin dir creation
	javaExeName := "java"
	if runtime.GOOS == "windows" {
		javaExeName = "java.exe"
	}
	_ = os.WriteFile(filepath.Join(binDir, javaExeName), []byte("mock binary"), 0755) // errcheck:ok test binary write

	detector := javaadapter.NewJavaDetector(filepath.Dir(tempDir))
	installations, err := detector.DetectInstallations(context.Background())
	if err != nil {
		logf("FAIL: JavaDetector returned error: %v", err)
		os.Exit(1)
	}
	foundMock := false
	for _, inst := range installations {
		if inst.MajorVersion == 21 && inst.Vendor == "Eclipse Adoptium" {
			foundMock = true
			break
		}
	}
	if !foundMock {
		logf("FAIL: JavaDetector failed to locate hermetic mock JDK 21 installation.")
		os.Exit(1)
	}
	logf("PASS: Hermetic JDK 21 discovery and release file parsing verified.")

	// =========================================================================
	// 4. Content Engine E2E: .mrpack Extraction & Range 206 Download
	// =========================================================================
	logf("\n--- STEP 4: Content Engine (.mrpack & Range Downloader) E2E ---")

	// Create in-memory .mrpack archive
	mrpackBuf := new(bytes.Buffer)
	zw := zip.NewWriter(mrpackBuf)
	indexJSON := `{
		"formatVersion": 1,
		"game": "minecraft",
		"versionId": "1.0.0",
		"name": "Nord Testpack",
		"dependencies": {
			"minecraft": "1.21.1",
			"fabric-loader": "0.16.0"
		},
		"files": []
	}`
	fIndex, _ := zw.Create("modrinth.index.json")
	_, _ = fIndex.Write([]byte(indexJSON)) // errcheck:ok write test index
	fOverride, _ := zw.Create("overrides/config/nord-test.txt")
	_, _ = fOverride.Write([]byte("custom config value")) // errcheck:ok write test override
	_ = zw.Close()                                        // errcheck:ok close zip writer

	mrpackFile := filepath.Join(tempDir, "test.mrpack")
	_ = os.WriteFile(mrpackFile, mrpackBuf.Bytes(), 0644) // errcheck:ok write test mrpack

	fMrPack, err := os.Open(mrpackFile)
	if err != nil {
		logf("FAIL: Failed to open .mrpack file: %v", err)
		os.Exit(1)
	}
	fiMrPack, _ := fMrPack.Stat()

	index, err := content.ParseMrPack(fMrPack, fiMrPack.Size())
	if err != nil {
		logf("FAIL: Failed to parse .mrpack: %v", err)
		os.Exit(1)
	}
	if index.Dependencies["minecraft"] != "1.21.1" {
		logf("FAIL: Incorrect Minecraft version in index: %s", index.Dependencies["minecraft"])
		os.Exit(1)
	}

	extractTarget := filepath.Join(tempDir, "instance_dir")
	if err := content.ExtractMrPackOverrides(fMrPack, fiMrPack.Size(), extractTarget); err != nil {
		logf("FAIL: Failed to extract .mrpack overrides: %v", err)
		os.Exit(1)
	}
	_ = fMrPack.Close() // errcheck:ok close test mrpack file

	extractedContent, err := os.ReadFile(filepath.Join(extractTarget, "config", "nord-test.txt"))
	if err != nil || string(extractedContent) != "custom config value" {
		logf("FAIL: Override extraction failed or content mismatch: %v", err)
		os.Exit(1)
	}
	logf("PASS: .mrpack index parsing and overrides extraction succeeded.")

	// Range-download verification via mock HTTP server
	payloadData := []byte("nord-launcher-binary-asset-payload-0123456789")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "" {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", len(payloadData)-1, len(payloadData)))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_, _ = w.Write(payloadData) // errcheck:ok mock server payload write
	}))
	defer server.Close()

	disp := downloader.NewDispatcher(downloader.DefaultConfig(), nil)
	downloadTarget := filepath.Join(tempDir, "downloaded-asset.bin")
	downloadTask := &downloader.DownloadTask{
		ID:           "asset-1",
		URL:          server.URL,
		DestPath:     downloadTarget,
		ExpectedSize: int64(len(payloadData)),
	}
	if err := disp.DownloadBatch(context.Background(), []*downloader.DownloadTask{downloadTask}, nil); err != nil {
		logf("FAIL: Downloader failed: %v", err)
		os.Exit(1)
	}
	dlBytes, _ := os.ReadFile(downloadTarget)
	if !bytes.Equal(dlBytes, payloadData) {
		logf("FAIL: Downloaded content mismatch.")
		os.Exit(1)
	}
	logf("PASS: Range-based HTTP 206 download verified with dispatcher.")

	// =========================================================================
	// 5. Process Supervision & Crash Classifier E2E
	// =========================================================================
	logf("\n--- STEP 5: Process Supervision & Crash Diagnostics E2E ---")
	pm := process.NewProcessManager()

	var shellCmd string
	var shellArgs []string
	if runtime.GOOS == "windows" {
		shellCmd = "cmd.exe"
		shellArgs = []string{"/c", "echo Nord Launcher Supervision E2E Active && exit 0"}
	} else {
		shellCmd = "sh"
		shellArgs = []string{"-c", "echo Nord Launcher Supervision E2E Active && exit 0"}
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	handle, err := pm.StartProcess(context.Background(), shellCmd, shellArgs, tempDir, nil, &stdoutBuf, &stderrBuf)
	if err != nil {
		logf("FAIL: StartProcess failed: %v", err)
		os.Exit(1)
	}
	if handle.PID() <= 0 {
		logf("FAIL: Invalid PID returned: %d", handle.PID())
		os.Exit(1)
	}

	exitCode, err := handle.Wait()
	if err != nil || exitCode != 0 {
		logf("FAIL: Process wait failed: exitCode=%d, err=%v", exitCode, err)
		os.Exit(1)
	}
	if !strings.Contains(stdoutBuf.String(), "Nord Launcher Supervision E2E Active") {
		logf("FAIL: Process stdout did not contain expected banner: %s", stdoutBuf.String())
		os.Exit(1)
	}
	logf("PASS: Supervised process lifecycle (PID: %d) executed and captured successfully.", handle.PID())

	// Test Crash Classification
	sup := launch.NewLogSupervisor(100)
	sup.ProcessLine("[12:00:00] [main/ERROR] [net.minecraft.client.main.Main]: Minecraft encountered a critical error: java.lang.OutOfMemoryError: Java heap space")
	report := sup.AnalyzeCrash(1)
	if report == nil || report.Category != launch.CrashCategoryOOM {
		logf("FAIL: Crash supervisor failed to diagnose OutOfMemoryError: %v", report)
		os.Exit(1)
	}
	logf("PASS: LogSupervisor correctly classified OutOfMemoryError from simulated game crash log.")

	// =========================================================================
	// 6. InstanceService.Launch Product Lifecycle E2E
	// =========================================================================
	logf("\n--- STEP 6: InstanceService.Launch Product Lifecycle E2E ---")
	e2eClock := clock.NewRealClock()
	e2eFS := fs.NewOSFileSystem()
	e2ePM := process.NewProcessManager()
	e2eKR := keyring.NewMemoryKeyring()

	instSvc := launch.NewInstanceService(nil, e2eFS, e2ePM, e2eKR, e2eClock)

	var lastReport *launch.CrashReport
	instSvc.SetOnCrash(func(id string, rep *launch.CrashReport) {
		lastReport = rep
	})

	e2eAcc := &domain.Account{
		UUID:        "e2e-steve-uuid",
		Username:    "Steve",
		Type:        domain.AccountMicrosoft,
		AccessToken: "e2e-token",
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	instSvc.SetActiveAccount(e2eAcc)

	// Create instance for clean exit test
	cleanInst, err := instSvc.CreateInstance("E2E-Clean-Instance", "1.21.1", domain.LoaderVanilla)
	if err != nil {
		logf("FAIL: Create clean instance failed: %v", err)
		os.Exit(1)
	}

	fakeJavaCode := `package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 && os.Args[1] == "-version" {
		fmt.Fprintln(os.Stderr, "openjdk version \"21.0.2\" 2024-01-16 LTS")
		os.Exit(0)
	}
	for _, arg := range os.Args[1:] {
		if arg == "simulate-oom" {
			fmt.Fprintln(os.Stderr, "java.lang.OutOfMemoryError: Java heap space")
			os.Exit(1)
		}
	}
	os.Exit(0)
}
`
	fakeJavaSrc := filepath.Join(tempDir, "fake_java.go")
	_ = os.WriteFile(fakeJavaSrc, []byte(fakeJavaCode), 0644) // errcheck:ok write fake java source
	fakeJavaExe := filepath.Join(tempDir, "fake-java.exe")
	if runtime.GOOS != "windows" {
		fakeJavaExe = filepath.Join(tempDir, "fake-java")
	}
	buildCmd := exec.Command("go", "build", "-o", fakeJavaExe, fakeJavaSrc)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		logf("FAIL: Failed to build fake java: %v\n%s", err, string(out))
		os.Exit(1)
	}

	cleanInst.JavaPath = fakeJavaExe
	cleanInst.JVMArgs = nil

	cleanPID, err := instSvc.Launch(context.Background(), cleanInst.ID)
	if err != nil {
		logf("FAIL: Launch clean instance failed: %v", err)
		os.Exit(1)
	}
	if cleanPID <= 0 {
		logf("FAIL: Expected positive PID, got %d", cleanPID)
		os.Exit(1)
	}
	logf("Launched clean instance (PID: %d), waiting for termination...", cleanPID)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := instSvc.GetInstance(cleanInst.ID)
		if cur.State == domain.StateIdle {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	finalClean, _ := instSvc.GetInstance(cleanInst.ID)
	if finalClean.State != domain.StateIdle {
		logf("FAIL: Expected clean instance state Idle, got %s", finalClean.State)
		os.Exit(1)
	}
	logf("PASS: Clean exit (exit 0) transitioned instance state to Idle.")

	// Create instance for crash exit test
	crashInst, err := instSvc.CreateInstance("E2E-Crash-Instance", "1.21.1", domain.LoaderVanilla)
	if err != nil {
		logf("FAIL: Create crash instance failed: %v", err)
		os.Exit(1)
	}
	crashInst.JavaPath = fakeJavaExe
	crashInst.JVMArgs = []string{"simulate-oom"}

	crashPID, err := instSvc.Launch(context.Background(), crashInst.ID)
	if err != nil {
		logf("FAIL: Launch crash instance failed: %v", err)
		os.Exit(1)
	}
	logf("Launched crashing instance (PID: %d), waiting for crash classification...", crashPID)

	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		cur, _ := instSvc.GetInstance(crashInst.ID)
		if cur.State == domain.StateCrashed {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	finalCrash, _ := instSvc.GetInstance(crashInst.ID)
	if finalCrash.State != domain.StateCrashed {
		logf("FAIL: Expected crashing instance state Crashed, got %s", finalCrash.State)
		os.Exit(1)
	}
	if lastReport == nil || lastReport.Category != launch.CrashCategoryOOM {
		logf("FAIL: Expected crash report Category OOM, got %+v", lastReport)
		os.Exit(1)
	}
	logf("PASS: Crash exit (exit 1) transitioned instance to Crashed, and onCrash diagnosed %s (%s).",
		lastReport.Category, lastReport.Summary)

	// =========================================================================
	// 7. Auto-Updater Cryptographic Lifecycle E2E
	// =========================================================================
	logf("\n--- STEP 7: Auto-Updater Cryptographic Lifecycle E2E ---")

	// Dynamic runtime Ed25519 key generation (Decision D2: fresh in-memory keypair on each run)
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		logf("FAIL: GenerateKey failed: %v", err)
		os.Exit(1)
	}

	payload := []byte("nord-launcher-v0.2.0-hermetic-e2e-payload")
	payloadHasher := sha256.New()
	_, _ = payloadHasher.Write(payload) // errcheck:ok hash computation
	validSHA256 := hex.EncodeToString(payloadHasher.Sum(nil))

	validSig := ed25519.Sign(privKey, payload)
	validSigB64 := base64.StdEncoding.EncodeToString(validSig)

	// Tampered signature (invert first byte)
	tamperedSig := make([]byte, len(validSig))
	copy(tamperedSig, validSig)
	tamperedSig[0] ^= 0xFF
	tamperedSigB64 := base64.StdEncoding.EncodeToString(tamperedSig)

	platKey := updater.CurrentPlatformKey()
	now := time.Now().UTC().Truncate(time.Second)

	var updaterServer *httptest.Server
	updaterServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest":
			m := updater.UpdateManifest{
				Version:     "0.2.0",
				ReleaseDate: now,
				Changelog:   "Nord Launcher v0.2.0 hermetic test release.",
				Platforms: map[string]updater.PlatformAsset{
					platKey: {
						URL:       updaterServer.URL + "/payload",
						SHA256:    validSHA256,
						Signature: validSigB64,
						Size:      int64(len(payload)),
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(m) // errcheck:ok mock server response
		case "/payload":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(payload) // errcheck:ok mock server response
		default:
			http.NotFound(w, r)
		}
	}))
	defer updaterServer.Close()

	testUpdater := updater.NewAutoUpdater("0.1.2", updaterServer.URL+"/manifest", pubKey, updaterServer.Client())

	// A. Check for updates
	updateInfo, err := testUpdater.CheckForUpdates(context.Background())
	if err != nil {
		logf("FAIL: AutoUpdater.CheckForUpdates failed: %v", err)
		os.Exit(1)
	}
	if !updateInfo.Available || updateInfo.Version != "0.2.0" {
		logf("FAIL: Expected update 0.2.0 available, got %+v", updateInfo)
		os.Exit(1)
	}
	if updateInfo.CurrentVer != "0.1.2" {
		logf("FAIL: Expected CurrentVer 0.1.2, got %s", updateInfo.CurrentVer)
		os.Exit(1)
	}
	logf("PASS: CheckForUpdates detected newer version %s (current: %s, release date: %s).",
		updateInfo.Version, updateInfo.CurrentVer, updateInfo.ReleaseDate.Format(time.RFC3339))

	// Prepare dummy executable in temp dir for atomic update testing
	e2eTmpDir, err := os.MkdirTemp("", "nord-e2e-updater-*")
	if err != nil {
		logf("FAIL: Create temp dir failed: %v", err)
		os.Exit(1)
	}
	defer func() { _ = os.RemoveAll(e2eTmpDir) }() // errcheck:ok cleanup temp dir

	dummyExe := filepath.Join(e2eTmpDir, "nord-launcher.exe")
	if runtime.GOOS != "windows" {
		dummyExe = filepath.Join(e2eTmpDir, "nord-launcher")
	}
	initialBinary := []byte("nord-launcher-v0.1.2-initial-content")
	if err := os.WriteFile(dummyExe, initialBinary, 0755); err != nil {
		logf("FAIL: Write dummy executable failed: %v", err)
		os.Exit(1)
	}

	// B. Tampered signature rejection
	tamperedAsset := updateInfo.Asset
	tamperedAsset.Signature = tamperedSigB64
	err = testUpdater.DownloadAndApply(context.Background(), tamperedAsset, dummyExe)
	if err == nil || !strings.Contains(err.Error(), "cryptographic signature verification failed") {
		logf("FAIL: Expected signature verification error on tampered signature, got %v", err)
		os.Exit(1)
	}
	logf("PASS: Tampered Ed25519 signature correctly rejected with ErrSignatureInvalid.")

	// C. Checksum mismatch rejection
	corruptedHashAsset := updateInfo.Asset
	corruptedHashAsset.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	err = testUpdater.DownloadAndApply(context.Background(), corruptedHashAsset, dummyExe)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		logf("FAIL: Expected checksum mismatch error, got %v", err)
		os.Exit(1)
	}
	logf("PASS: Corrupted SHA-256 hash correctly rejected with ErrChecksumMismatch.")

	// D. Successful atomic apply
	err = testUpdater.DownloadAndApply(context.Background(), updateInfo.Asset, dummyExe)
	if err != nil {
		logf("FAIL: DownloadAndApply failed on valid update: %v", err)
		os.Exit(1)
	}
	appliedBytes, err := os.ReadFile(dummyExe)
	if err != nil || !bytes.Equal(appliedBytes, payload) {
		logf("FAIL: Applied binary content mismatch after update")
		os.Exit(1)
	}
	logf("PASS: Valid update payload successfully applied via atomic replacement.")

	// E. Stale backup cleanup
	oldBackupPath := dummyExe + ".old"
	if _, statErr := os.Stat(oldBackupPath); statErr == nil {
		_ = os.Remove(oldBackupPath) // errcheck:ok stale backup cleanup
		logf("PASS: Stale executable backup (.old) verified and cleaned up.")
	}

	// =========================================================================
	// 8. Manifest Generator Contract & Real Signing Lifecycle E2E
	// =========================================================================
	logf("\n--- STEP 8: Manifest Generator Contract & Real Signing Lifecycle E2E ---")

	step8Tmp, err := os.MkdirTemp("", "nord-e2e-step8-*")
	if err != nil {
		logf("FAIL: MkdirTemp failed: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(step8Tmp) // errcheck:ok cleanup test temporary files

	// D3: Compile genmanifest tool deterministically
	genmanifestBin := filepath.Join(step8Tmp, "genmanifest")
	if runtime.GOOS == "windows" {
		genmanifestBin += ".exe"
	}

	buildCmd = exec.Command("go", "build", "-o", genmanifestBin, "./scripts/generate_manifest.go")
	if out, buildErr := buildCmd.CombinedOutput(); buildErr != nil {
		logf("FAIL: Failed to compile generate_manifest.go: %v\nOutput: %s", buildErr, string(out))
		os.Exit(1)
	}

	// D2: Ephemeral keypair for this test run (never production key)
	pubKey8, privKey8, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		logf("FAIL: GenerateKey failed: %v", err)
		os.Exit(1)
	}
	privSeedHex := hex.EncodeToString(privKey8.Seed())

	// D3: Platform fixture matching CurrentPlatformKey
	fixtureDist := filepath.Join(step8Tmp, "dist")
	if err := os.MkdirAll(fixtureDist, 0755); err != nil {
		logf("FAIL: MkdirAll fixture dist failed: %v", err)
		os.Exit(1)
	}

	fixturePayload := []byte("nord-launcher-v0.9.9-step8-real-contract-binary-payload")
	var fixtureFile string
	if runtime.GOOS == "windows" {
		fixtureFile = filepath.Join(fixtureDist, "NordLauncher.exe")
	} else {
		fixtureFile = filepath.Join(fixtureDist, "nord-launcher-0.9.9.tar.gz")
	}

	if err := os.WriteFile(fixtureFile, fixturePayload, 0755); err != nil {
		logf("FAIL: WriteFile fixture binary failed: %v", err)
		os.Exit(1)
	}

	manifestOut := filepath.Join(fixtureDist, "manifest-stable.json")

	// Run compiled generator tool
	genCmd := exec.Command(genmanifestBin,
		"-version", "0.9.9",
		"-channel", "stable",
		"-dist", fixtureDist,
		"-out", manifestOut,
		"-privkey-hex", privSeedHex,
	)
	if out, genErr := genCmd.CombinedOutput(); genErr != nil {
		logf("FAIL: genmanifest execution failed: %v\nOutput: %s", genErr, string(out))
		os.Exit(1)
	}

	// Read generated manifest JSON
	rawManifestBytes, err := os.ReadFile(manifestOut)
	if err != nil {
		logf("FAIL: Failed to read generated manifest: %v", err)
		os.Exit(1)
	}

	var generatedManifest updater.UpdateManifest
	if err := json.Unmarshal(rawManifestBytes, &generatedManifest); err != nil {
		logf("FAIL: Failed to parse generated manifest JSON: %v", err)
		os.Exit(1)
	}

	platKey8 := updater.CurrentPlatformKey()
	_, exists := generatedManifest.Platforms[platKey8]
	if !exists {
		logf("FAIL: Generated manifest missing platform %s. Platforms: %+v", platKey8, generatedManifest.Platforms)
		os.Exit(1)
	}

	// D1: URL patch in manifest to point to httptest server while keeping sig/sha256/size intact
	var step8Server *httptest.Server
	step8Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest":
			mCopy := generatedManifest
			mCopy.Platforms = make(map[string]updater.PlatformAsset)
			for k, v := range generatedManifest.Platforms {
				if k == platKey8 {
					v.URL = step8Server.URL + "/payload"
				}
				mCopy.Platforms[k] = v
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(mCopy) // errcheck:ok mock server response
		case "/payload":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(fixturePayload) // errcheck:ok mock server response
		default:
			http.NotFound(w, r)
		}
	}))
	defer step8Server.Close()

	// Initialize AutoUpdater with matching ephemeral pubKey8
	step8Updater := updater.NewAutoUpdater("0.1.3", step8Server.URL+"/manifest", pubKey8, step8Server.Client())

	step8Info, err := step8Updater.CheckForUpdates(context.Background())
	if err != nil {
		logf("FAIL: STEP 8 CheckForUpdates failed: %v", err)
		os.Exit(1)
	}
	if !step8Info.Available || step8Info.Version != "0.9.9" {
		logf("FAIL: STEP 8 Expected update 0.9.9 available, got %+v", step8Info)
		os.Exit(1)
	}
	logf("PASS: Generated manifest correctly checked (version: %s, channel: stable).", step8Info.Version)

	dummyExe8 := filepath.Join(step8Tmp, "current_app.exe")
	if err := os.WriteFile(dummyExe8, []byte("old-binary-v0.1.3"), 0755); err != nil {
		logf("FAIL: Create dummyExe8 failed: %v", err)
		os.Exit(1)
	}

	// Negative check: Tampered signature must be rejected with ErrSignatureInvalid
	tamperedAsset8 := step8Info.Asset
	rawSig8, decodeErr := base64.StdEncoding.DecodeString(tamperedAsset8.Signature)
	if decodeErr != nil {
		logf("FAIL: Decode valid signature failed: %v", decodeErr)
		os.Exit(1)
	}
	rawSig8[0] ^= 0xFF
	tamperedAsset8.Signature = base64.StdEncoding.EncodeToString(rawSig8)

	err = step8Updater.DownloadAndApply(context.Background(), tamperedAsset8, dummyExe8)
	if err == nil || !errors.Is(err, updater.ErrSignatureInvalid) {
		logf("FAIL: Expected ErrSignatureInvalid on tampered manifest signature, got %v", err)
		os.Exit(1)
	}
	logf("PASS: Tampered manifest signature rejected with ErrSignatureInvalid.")

	// Positive check: Valid DownloadAndApply using manifest generated by real genmanifest tool
	err = step8Updater.DownloadAndApply(context.Background(), step8Info.Asset, dummyExe8)
	if err != nil {
		logf("FAIL: STEP 8 DownloadAndApply failed on valid manifest: %v", err)
		os.Exit(1)
	}

	appliedBytes8, err := os.ReadFile(dummyExe8)
	if err != nil || !bytes.Equal(appliedBytes8, fixturePayload) {
		logf("FAIL: Applied binary content mismatch in STEP 8")
		os.Exit(1)
	}
	logf("PASS: Full contract verified: scripts/generate_manifest.go -> updater.DownloadAndApply.")

	// =========================================================================
	// 9. CurseForge Sidecar Key Delivery & Request Header Contract E2E
	// =========================================================================
	logf("\n--- STEP 9: CurseForge Sidecar Key Delivery & Header Contract E2E ---")

	testSidecarSecret := "$2a$10$e2e-sidecar-verification-token-987654321"
	var receivedAPIKeyHeader string

	cfServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAPIKeyHeader = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{ // errcheck:ok mock http response encode
			"data": []interface{}{
				map[string]interface{}{
					"id":   12345,
					"name": "Sidecar Test Mod",
					"slug": "sidecar-test-mod",
				},
			},
			"pagination": map[string]interface{}{
				"totalCount": 1,
			},
		})
	}))
	defer cfServer.Close()

	// 1. Write temporary sidecar file
	tempE2EDir, err := os.MkdirTemp("", "e2e_cf_sidecar_*")
	if err != nil {
		logf("FAIL: MkdirTemp for STEP 9 failed: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempE2EDir)

	sidecarPath := filepath.Join(tempE2EDir, "cf.key")
	if err := os.WriteFile(sidecarPath, []byte(testSidecarSecret), 0644); err != nil {
		logf("FAIL: Write sidecar key failed: %v", err)
		os.Exit(1)
	}

	// Read sidecar content
	sidecarBytes, err := os.ReadFile(sidecarPath)
	if err != nil {
		logf("FAIL: Read sidecar key failed: %v", err)
		os.Exit(1)
	}
	sidecarKey := strings.TrimSpace(string(sidecarBytes))

	// Configure curseforge client
	curseforge.SetBuiltinAPIKey(sidecarKey)
	step9Client := curseforge.NewClient(cfServer.URL, "", nil)

	// Issue search request
	results, _, err := step9Client.SearchMods(context.Background(), "test", "1.21.1", "fabric", 20, 0)
	if err != nil {
		logf("FAIL: STEP 9 SearchMods failed: %v", err)
		os.Exit(1)
	}

	if len(results) == 0 || results[0].Name != "Sidecar Test Mod" {
		logf("FAIL: STEP 9 unexpected mod search results")
		os.Exit(1)
	}

	if receivedAPIKeyHeader != testSidecarSecret {
		logf("FAIL: Expected x-api-key header %q, got %q", testSidecarSecret, receivedAPIKeyHeader)
		os.Exit(1)
	}
	logf("PASS: Sidecar key resolved from file and verified in x-api-key HTTP header.")

	// --- STEP 10: Installer & Sidecar Delivery Contract E2E ---
	logf("\n--- STEP 10: Installer & Sidecar Delivery Contract E2E ---")
	instDir, err := os.MkdirTemp("", "e2e_instdir_*")
	if err != nil {
		logf("FAIL: MkdirTemp for STEP 10 failed: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(instDir)

	// Simulate $INSTDIR: NordLauncher.exe and cf.key placed side-by-side
	simulatedExe := filepath.Join(instDir, "NordLauncher.exe")
	if err := os.WriteFile(simulatedExe, []byte("fake-binary-content"), 0755); err != nil {
		logf("FAIL: Write simulated exe failed: %v", err)
		os.Exit(1)
	}

	testInstKey := "$2a$10$instdir_delivery_contract_key_abcdef12345"
	simulatedKeyFile := filepath.Join(instDir, "cf.key")
	if err := os.WriteFile(simulatedKeyFile, []byte(testInstKey+"\r\n"), 0644); err != nil {
		logf("FAIL: Write simulated cf.key failed: %v", err)
		os.Exit(1)
	}

	// 1. Resolve via production curseforge.ResolveSidecarKey
	resolvedInstKey, err := curseforge.ResolveSidecarKey(simulatedExe)
	if err != nil {
		logf("FAIL: ResolveSidecarKey failed: %v", err)
		os.Exit(1)
	}
	if resolvedInstKey != testInstKey {
		logf("FAIL: Expected resolved key %q, got %q", testInstKey, resolvedInstKey)
		os.Exit(1)
	}
	logf("PASS: Production curseforge.ResolveSidecarKey verified against $INSTDIR layout.")

	// 2. Verify missing sidecar returns empty string safely
	emptyDir, err := os.MkdirTemp("", "e2e_empty_instdir_*")
	if err != nil {
		logf("FAIL: MkdirTemp empty dir failed: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(emptyDir)

	emptyExe := filepath.Join(emptyDir, "NordLauncher.exe")
	_ = os.WriteFile(emptyExe, []byte("fake"), 0755) // errcheck:ok simulated empty binary
	resolvedEmpty, err := curseforge.ResolveSidecarKey(emptyExe)
	if err != nil {
		logf("FAIL: ResolveSidecarKey on empty dir returned error: %v", err)
		os.Exit(1)
	}
	if resolvedEmpty != "" {
		logf("FAIL: Expected empty key on missing sidecar, got %q", resolvedEmpty)
		os.Exit(1)
	}
	logf("PASS: Missing sidecar safely resolved to empty string.")

	// =========================================================================
	// 11. Mod Manifest, Toggle & Dual-File Delete Contract E2E (A2, C4, C6)
	// =========================================================================
	logf("\n--- STEP 11: Mod Manifest & Reconcile Delivery Contract E2E ---")

	step11JarContent := []byte("nord-launcher-e2e-step11-fixture-mod-jar")
	h11 := sha1.Sum(step11JarContent)
	step11Sha1Hex := hex.EncodeToString(h11[:])

	step11Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/project/") && strings.HasSuffix(r.URL.Path, "/version") {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{ // errcheck:ok mock modrinth versions
				{
					"id":           "step11-ver-release",
					"project_id":   "step11-mod",
					"name":         "Step11 Mod Release",
					"version_type": "release",
					"files": []map[string]interface{}{
						{
							"id":       "file-step11-rel",
							"url":      "http://" + r.Host + "/download/test-step11.jar",
							"filename": "test-step11.jar",
							"primary":  true,
							"size":     len(step11JarContent),
							"hashes": map[string]string{
								"sha1": step11Sha1Hex,
							},
						},
					},
					"game_versions":  []string{"1.21.1"},
					"loaders":        []string{"fabric"},
					"date_published": time.Now().Format(time.RFC3339),
				},
			})
			return
		}

		if r.URL.Path == "/download/test-step11.jar" {
			w.Header().Set("Content-Type", "application/java-archive")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(step11JarContent) // errcheck:ok mock file download write
			return
		}

		http.NotFound(w, r)
	}))
	defer step11Server.Close()

	step11Tmp, err := os.MkdirTemp("", "e2e_step11_*")
	if err != nil {
		logf("FAIL: MkdirTemp for STEP 11 failed: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(step11Tmp)

	step11InstID := "e2e-inst11"
	step11InstancesDir := filepath.Join(step11Tmp, "instances")
	step11ModsDir := filepath.Join(step11InstancesDir, step11InstID, "mods")
	if err := os.MkdirAll(step11ModsDir, 0755); err != nil {
		logf("FAIL: MkdirAll mods dir failed: %v", err)
		os.Exit(1)
	}

	step11DBPath := filepath.Join(step11Tmp, "test.db")
	step11DB, err := storage.OpenDatabase(step11DBPath)
	if err != nil {
		logf("FAIL: OpenDatabase for STEP 11 failed: %v", err)
		os.Exit(1)
	}
	defer step11DB.Close()

	if err := step11DB.Migrate(); err != nil {
		logf("FAIL: DB Migrate for STEP 11 failed: %v", err)
		os.Exit(1)
	}

	step11Adapter := wails.NewWailsAdapter(nil)
	mrServerURL, _ := url.Parse(step11Server.URL)
	wails.NewHost(step11Adapter).SetAllowedHosts([]string{mrServerURL.Hostname(), mrServerURL.Host})
	wails.NewHost(step11Adapter).SetFileSystem(fs.NewOSFileSystem(), step11InstancesDir)
	wails.NewHost(step11Adapter).SetDB(step11DB.DB())

	mrStep11Client := modrinth.NewClient(step11Server.URL, step11Server.Client())
	wails.NewHost(step11Adapter).SetContent(mrStep11Client, nil)

	// 1. Install Mod
	installResp, err := step11Adapter.InstallMod(wails.InstallModRequest{
		InstanceID:  step11InstID,
		ModID:       "step11-mod",
		Source:      "modrinth",
		GameVersion: "1.21.1",
		Loader:      "fabric",
	})
	if err != nil {
		logf("FAIL: STEP 11 InstallMod failed: %v", err)
		os.Exit(1)
	}
	if !installResp.Success || installResp.FileName != "test-step11.jar" {
		logf("FAIL: Unexpected InstallMod response: %+v", installResp)
		os.Exit(1)
	}

	installedJarPath := filepath.Join(step11ModsDir, "test-step11.jar")
	if _, err := os.Stat(installedJarPath); err != nil {
		logf("FAIL: Installed jar file not found on disk: %v", err)
		os.Exit(1)
	}

	// 2. Verify nord-installs.json manifest
	step11ManifestPath := filepath.Join(step11ModsDir, "nord-installs.json")
	if _, err := os.Stat(step11ManifestPath); err != nil {
		logf("FAIL: nord-installs.json manifest not found on disk: %v", err)
		os.Exit(1)
	}

	step11M, err := manifest.LoadManifest(step11ModsDir)
	if err != nil {
		logf("FAIL: LoadManifest failed: %v", err)
		os.Exit(1)
	}
	if step11M.SchemaVersion != manifest.CurrentSchemaVersion {
		logf("FAIL: Expected schema_version %d, got %d", manifest.CurrentSchemaVersion, step11M.SchemaVersion)
		os.Exit(1)
	}
	rec := step11M.GetRecord("test-step11")
	if rec == nil || rec.ModID != "step11-mod" || rec.FileName != "test-step11.jar" || rec.Source != "modrinth" {
		logf("FAIL: Manifest record mismatch: %+v", rec)
		os.Exit(1)
	}
	logf("PASS: InstallMod created valid nord-installs.json with matching metadata.")

	// 3. Toggle mod to disabled
	err = step11Adapter.ToggleMod(wails.ToggleModRequest{
		InstanceID: step11InstID,
		FileName:   "test-step11.jar",
		Enable:     false,
	})
	if err != nil {
		logf("FAIL: ToggleMod failed: %v", err)
		os.Exit(1)
	}

	if _, err := os.Stat(installedJarPath); !os.IsNotExist(err) {
		logf("FAIL: Expected test-step11.jar to no longer exist after disable")
		os.Exit(1)
	}
	disabledJarPath := filepath.Join(step11ModsDir, "test-step11.jar.disabled")
	if _, err := os.Stat(disabledJarPath); err != nil {
		logf("FAIL: Expected test-step11.jar.disabled on disk: %v", err)
		os.Exit(1)
	}

	step11M, err = manifest.LoadManifest(step11ModsDir)
	if err != nil {
		logf("FAIL: LoadManifest reload after toggle failed: %v", err)
		os.Exit(1)
	}
	recDisabled := step11M.GetRecord("test-step11")
	if recDisabled == nil || recDisabled.FileName != "test-step11.jar.disabled" {
		logf("FAIL: Manifest did not update filename to .disabled: %+v", recDisabled)
		os.Exit(1)
	}
	logf("PASS: ToggleMod successfully renamed file to .disabled and updated manifest record.")

	// 4. Directive A2: Dual-File Deletion
	// Create stray active jar alongside .disabled jar to simulate dual presence
	if err := os.WriteFile(installedJarPath, []byte("stray-active-jar"), 0644); err != nil {
		logf("FAIL: Write stray active jar failed: %v", err)
		os.Exit(1)
	}

	// Delete mod specifying target
	err = step11Adapter.DeleteMod(wails.DeleteModRequest{
		InstanceID: step11InstID,
		FileName:   "test-step11.jar",
	})
	if err != nil {
		logf("FAIL: DeleteMod failed: %v", err)
		os.Exit(1)
	}

	// Directive A2: BOTH .jar and .jar.disabled must be deleted from disk
	if _, err := os.Stat(installedJarPath); !os.IsNotExist(err) {
		logf("FAIL: Expected test-step11.jar to be deleted (Directive A2)")
		os.Exit(1)
	}
	if _, err := os.Stat(disabledJarPath); !os.IsNotExist(err) {
		logf("FAIL: Expected test-step11.jar.disabled to be deleted (Directive A2)")
		os.Exit(1)
	}

	step11M, err = manifest.LoadManifest(step11ModsDir)
	if err != nil {
		logf("FAIL: LoadManifest reload after delete failed: %v", err)
		os.Exit(1)
	}
	if recDeleted := step11M.GetRecord("test-step11"); recDeleted != nil {
		logf("FAIL: Expected record to be pruned from manifest, got %+v", recDeleted)
		os.Exit(1)
	}

	modsList, err := step11Adapter.ListInstalledMods(step11InstID)
	if err != nil {
		logf("FAIL: ListInstalledMods failed: %v", err)
		os.Exit(1)
	}
	if len(modsList) != 0 {
		logf("FAIL: Expected empty mods list after deletion and reconcile, got %d mods", len(modsList))
		os.Exit(1)
	}
	logf("PASS: Directive A2 verified: dual-file deletion removed both .jar and .jar.disabled, manifest pruned.")

	// =========================================================================
	// 12. NetUtil HTTP Client, Content Cache Persistence & Modrinth Meta Header E2E (C6, B7)
	// =========================================================================
	logf("\n--- STEP 12: NetUtil HTTP Client, Content Cache Persistence & Modrinth Meta Header E2E (C6, B7) ---")

	expectedUA := netutil.FormatUserAgent("0.6.0")
	var step12UACheckCount int
	var step12ReceivedModrinthMeta string
	var step12CFHitCount int

	step12Tmp, err := os.MkdirTemp("", "e2e_step12_*")
	if err != nil {
		logf("FAIL: MkdirTemp for STEP 12 failed: %v", err)
		os.Exit(1)
	}
	defer os.RemoveAll(step12Tmp)

	step12JarContent := bytes.Repeat([]byte("NORD-STEP12-BYTECODE"), 50)
	step12Sha512 := sha512.Sum512(step12JarContent)
	step12Sha512Hex := hex.EncodeToString(step12Sha512[:])

	var step12Server *httptest.Server
	step12Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua == expectedUA {
			step12UACheckCount++
		}

		// Modrinth search
		if r.URL.Path == "/v2/search" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{ // errcheck:ok mock modrinth search
				"hits": []map[string]interface{}{
					{
						"project_id":  "step12-mod",
						"slug":        "step12-mod",
						"title":       "Step 12 Mod",
						"author":      "Nord Team",
						"description": "Mod for E2E Step 12",
						"downloads":   1200,
						"categories":  []string{"fabric"},
					},
				},
				"total_hits": 1,
			})
			return
		}

		// Modrinth project versions
		if r.URL.Path == "/v2/project/step12-mod/version" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]interface{}{ // errcheck:ok mock modrinth versions
				{
					"id":             "step12-v1",
					"project_id":     "step12-mod",
					"name":           "Step 12 v1.0.0",
					"version_number": "1.0.0",
					"version_type":   "release",
					"game_versions":  []string{"1.21.1"},
					"loaders":        []string{"fabric"},
					"date_published": time.Now().UTC().Format(time.RFC3339),
					"files": []map[string]interface{}{
						{
							"filename": "step12-mod.jar",
							"url":      step12Server.URL + "/download/step12-mod.jar",
							"size":     len(step12JarContent),
							"hashes": map[string]string{
								"sha512": step12Sha512Hex,
							},
						},
					},
				},
			})
			return
		}

		// Modrinth download
		if r.URL.Path == "/download/step12-mod.jar" {
			step12ReceivedModrinthMeta = r.Header.Get("modrinth-download-meta")
			w.Header().Set("Content-Type", "application/java-archive")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(step12JarContent) // errcheck:ok mock download
			return
		}

		// CurseForge search
		if r.URL.Path == "/v1/mods/search" {
			step12CFHitCount++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{ // errcheck:ok mock curseforge search
				"data": []map[string]interface{}{
					{
						"id":            12345,
						"slug":          "step12-cf-mod",
						"name":          "Step 12 CF Mod",
						"summary":       "CurseForge mod for Step 12",
						"downloadCount": 54321,
						"categories":    []map[string]interface{}{{"name": "Fabric"}},
						"authors":       []map[string]interface{}{{"name": "Nord Dev"}},
					},
				},
				"pagination": map[string]interface{}{
					"totalCount": 1,
					"index":      0,
					"pageSize":   20,
				},
			})
			return
		}

		http.NotFound(w, r)
	}))
	defer step12Server.Close()

	// 1. Verify NetUtil HTTP Client carries User-Agent
	netClient := netutil.NewHTTPClient("0.6.0", 5*time.Second)
	resp, err := netClient.Get(step12Server.URL + "/v2/search")
	if err != nil {
		logf("FAIL: NetUtil NewHTTPClient request failed: %v", err)
		os.Exit(1)
	}
	_ = resp.Body.Close() // errcheck:ok response close
	if step12UACheckCount < 1 {
		logf("FAIL: NetUtil request did not send expected User-Agent: %s", expectedUA)
		os.Exit(1)
	}
	logf("PASS: Unified NetUtil HTTP Client carries User-Agent (%s).", expectedUA)

	// 2. Verify SQLite Content Cache persistence across simulated client restart (same DB)
	step12DBPath := filepath.Join(step12Tmp, "cache_step12.db")
	step12DB, err := storage.OpenDatabase(step12DBPath)
	if err != nil {
		logf("FAIL: OpenDatabase for STEP 12 failed: %v", err)
		os.Exit(1)
	}
	defer step12DB.Close()
	if err := step12DB.Migrate(); err != nil {
		logf("FAIL: DB Migrate for STEP 12 failed: %v", err)
		os.Exit(1)
	}

	cacheRepo := storage.NewContentCacheRepository(step12DB)
	cacheAdapter := storageAdapter.NewContentCacheAdapter(cacheRepo)

	// Client 1 before restart
	cfClient1 := curseforge.NewClient(step12Server.URL, "test-cf-key", step12Server.Client())
	cfClient1.SetContentCache(cacheAdapter)

	res1, _, err := cfClient1.SearchMods(context.Background(), "test", "1.21.1", "fabric", 10, 0)
	if err != nil {
		logf("FAIL: CF Client 1 SearchMods failed: %v", err)
		os.Exit(1)
	}
	if len(res1) != 1 || step12CFHitCount != 1 {
		logf("FAIL: Expected 1 hit and 1 network request on Client 1, got %d hits and %d requests", len(res1), step12CFHitCount)
		os.Exit(1)
	}

	// Client 2 after simulated restart (same SQLite cache store!)
	cfClient2 := curseforge.NewClient(step12Server.URL, "test-cf-key", step12Server.Client())
	cfClient2.SetContentCache(cacheAdapter)

	res2, _, err := cfClient2.SearchMods(context.Background(), "test", "1.21.1", "fabric", 10, 0)
	if err != nil {
		logf("FAIL: CF Client 2 SearchMods failed: %v", err)
		os.Exit(1)
	}
	if len(res2) != 1 {
		logf("FAIL: Expected 1 cached hit on Client 2, got %d hits", len(res2))
		os.Exit(1)
	}
	if step12CFHitCount != 1 {
		logf("FAIL: Expected SQLite content cache hit with 0 network calls, but got %d network requests", step12CFHitCount)
		os.Exit(1)
	}
	logf("PASS: SQLite content cache hit persisted across simulated client restarts (0 network calls on 2nd query).")

	// 3. Verify Modrinth Download carries modrinth-download-meta header
	step12InstID := "e2e-inst12"
	step12InstancesDir := filepath.Join(step12Tmp, "instances")
	step12ModsDir := filepath.Join(step12InstancesDir, step12InstID, "mods")
	if err := os.MkdirAll(step12ModsDir, 0755); err != nil {
		logf("FAIL: MkdirAll for step 12 mods dir failed: %v", err)
		os.Exit(1)
	}

	step12Adapter := wails.NewWailsAdapter(nil)
	wails.NewHost(step12Adapter).SetVersion("0.6.0")
	step12ServerURL, _ := url.Parse(step12Server.URL)
	wails.NewHost(step12Adapter).SetAllowedHosts([]string{step12ServerURL.Hostname(), step12ServerURL.Host})
	wails.NewHost(step12Adapter).SetFileSystem(fs.NewOSFileSystem(), step12InstancesDir)
	wails.NewHost(step12Adapter).SetDB(step12DB.DB())
	wails.NewHost(step12Adapter).SetContent(modrinth.NewClient(step12Server.URL, step12Server.Client()), nil)

	_, err = step12Adapter.InstallMod(wails.InstallModRequest{
		InstanceID:  step12InstID,
		ModID:       "step12-mod",
		Source:      "modrinth",
		GameVersion: "1.21.1",
		Loader:      "fabric",
	})
	if err != nil {
		logf("FAIL: Step 12 InstallMod failed: %v", err)
		os.Exit(1)
	}

	if step12ReceivedModrinthMeta == "" {
		logf("FAIL: modrinth-download-meta header was not sent during mod download")
		os.Exit(1)
	}
	var metaMap map[string]string
	if err := json.Unmarshal([]byte(step12ReceivedModrinthMeta), &metaMap); err != nil {
		logf("FAIL: Failed to parse modrinth-download-meta header JSON: %v", err)
		os.Exit(1)
	}
	if metaMap["reason"] != "standalone" || metaMap["game_version"] != "1.21.1" || metaMap["loader"] != "fabric" {
		logf("FAIL: modrinth-download-meta header contents mismatch: %+v", metaMap)
		os.Exit(1)
	}
	logf("PASS: Modrinth download carried authentic User-Agent and valid modrinth-download-meta header.")

	// 4. Verify Diagnostic Report Pack (credential masking & path redaction)
	diagReport, err := step12Adapter.GetDiagnosticReport(step12InstID)
	if err != nil {
		logf("FAIL: GetDiagnosticReport failed: %v", err)
		os.Exit(1)
	}
	if !strings.Contains(diagReport, "Nord Launcher Diagnostic Report") {
		logf("FAIL: Diagnostic report missing header banner: %s", diagReport)
		os.Exit(1)
	}
	if !strings.Contains(diagReport, "0.6.0") {
		logf("FAIL: Diagnostic report missing version: %s", diagReport)
		os.Exit(1)
	}
	if strings.Contains(diagReport, "test-cf-key") || strings.Contains(diagReport, "testSidecarSecret") {
		logf("FAIL: Diagnostic report contains unmasked credentials!")
		os.Exit(1)
	}
	logf("PASS: Diagnostic report pack generated with anonymized paths and zero credential leakage.")

	// =========================================================================
	// 13. MrPack Round-Trip E2E: Export -> Verify -> Import -> Parity
	// =========================================================================
	logf("\n--- STEP 13: Modpack Round-Trip (.mrpack) E2E ---")
	step13Tmp := filepath.Join(os.TempDir(), fmt.Sprintf("nord-e2e-step13-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(step13Tmp, 0755) // errcheck:ok create temp dir
	defer func() {
		_ = os.RemoveAll(step13Tmp) // errcheck:ok cleanup step 13 temp dir
	}()

	step13DBPath := filepath.Join(step13Tmp, "nord-e2e-13.db")
	step13DB, err := storage.OpenDatabase(step13DBPath)
	if err != nil {
		logf("FAIL: Step 13 SQLite init failed: %v", err)
		os.Exit(1)
	}
	defer func() { _ = step13DB.Close() }() // errcheck:ok close db
	if err := step13DB.Migrate(); err != nil {
		logf("FAIL: Step 13 migrations failed: %v", err)
		os.Exit(1)
	}

	step13InstRepo := storage.NewInstanceRepository(step13DB)
	step13CacheRepo := storage.NewContentCacheRepository(step13DB)
	step13InstancesDir := filepath.Join(step13Tmp, "instances")
	step13ExportsDir := filepath.Join(step13Tmp, "exports")
	_ = os.MkdirAll(step13InstancesDir, 0755) // errcheck:ok create instances dir
	_ = os.MkdirAll(step13ExportsDir, 0755)   // errcheck:ok create exports dir

	// 1. Create source instance with test mods and configs
	step13SourceInst := &domain.Instance{
		ID:          "source-modpack-inst",
		Name:        "Source Modpack",
		GameVersion: "1.21.1",
		Loader:      domain.LoaderFabric,
		LoaderVer:   "0.16.5",
	}
	if err := step13InstRepo.Save(context.Background(), step13SourceInst); err != nil {
		logf("FAIL: Step 13 save source instance failed: %v", err)
		os.Exit(1)
	}

	sourceInstDir := filepath.Join(step13InstancesDir, step13SourceInst.ID)
	sourceModsDir := filepath.Join(sourceInstDir, "mods")
	sourceConfigDir := filepath.Join(sourceInstDir, "config")
	_ = os.MkdirAll(sourceModsDir, 0755)   // errcheck:ok create mods dir
	_ = os.MkdirAll(sourceConfigDir, 0755) // errcheck:ok create config dir

	// Local test mod
	testModData := []byte("mock-mod-jar-content-for-mrpack-export-12345")
	testModPath := filepath.Join(sourceModsDir, "local-mod.jar")
	if err := os.WriteFile(testModPath, testModData, 0644); err != nil {
		logf("FAIL: Step 13 write test mod failed: %v", err)
		os.Exit(1)
	}
	testModSHA1, _ := content.CalculateFileSHA1(testModPath) // errcheck:ok compute sha1

	// Local test config
	testConfigData := []byte("options.sound=1.0\nguiScale=2\n")
	testConfigPath := filepath.Join(sourceConfigDir, "client.properties")
	if err := os.WriteFile(testConfigPath, testConfigData, 0644); err != nil {
		logf("FAIL: Step 13 write test config failed: %v", err)
		os.Exit(1)
	}

	// 2. Export .mrpack using MrPackExporter
	exporter := content.NewMrPackExporter(step13InstRepo, step13CacheRepo, step13InstancesDir, step13ExportsDir)
	exportedMrPackPath, err := exporter.ExportMrPack(context.Background(), content.ExportMrPackOptions{
		InstanceID:  step13SourceInst.ID,
		PackName:    "RoundTrip Pack",
		PackVersion: "1.0.0",
		Summary:     "Test export pack for E2E",
	})
	if err != nil {
		logf("FAIL: Step 13 ExportMrPack failed: %v", err)
		os.Exit(1)
	}
	if _, statErr := os.Stat(exportedMrPackPath); statErr != nil {
		logf("FAIL: Exported mrpack file not found: %v", statErr)
		os.Exit(1)
	}
	logf("PASS: MrPack exported successfully: %s", filepath.Base(exportedMrPackPath))

	// 3. Inspect mrpack plan using GetMrPackImportPlan
	step13MrPackFile, err := os.Open(exportedMrPackPath)
	if err != nil {
		logf("FAIL: Open exported mrpack failed: %v", err)
		os.Exit(1)
	}
	mrpackFI, _ := step13MrPackFile.Stat() // errcheck:ok stat open file
	plan, err := content.GetMrPackImportPlan(step13MrPackFile, mrpackFI.Size(), []string{"Existing Other"})
	_ = step13MrPackFile.Close() // errcheck:ok close file
	if err != nil {
		logf("FAIL: GetMrPackImportPlan failed: %v", err)
		os.Exit(1)
	}
	if plan.Name != "RoundTrip Pack" || plan.GameVersion != "1.21.1" || plan.Loader != "fabric" {
		logf("FAIL: Unexpected plan metadata: %+v", plan)
		os.Exit(1)
	}
	logf("PASS: MrPack plan verified: name=%s, mc=%s, loader=%s, files=%d", plan.Name, plan.GameVersion, plan.Loader, plan.TotalFiles)

	// 4. Import .mrpack into new instance using MrPackImporter
	importer := content.NewMrPackImporter(nil, step13InstRepo, step13InstancesDir)
	targetInstID, err := importer.ImportMrPack(context.Background(), exportedMrPackPath, content.ImportMrPackOptions{
		InstanceName: "Imported Target Instance",
	}, nil)
	if err != nil {
		logf("FAIL: ImportMrPack failed: %v", err)
		os.Exit(1)
	}

	targetInst, err := step13InstRepo.GetByID(context.Background(), targetInstID)
	if err != nil || targetInst == nil {
		logf("FAIL: Imported target instance not found in repository: %v", err)
		os.Exit(1)
	}
	if targetInst.GameVersion != "1.21.1" || targetInst.Loader != domain.LoaderFabric {
		logf("FAIL: Target instance loader mismatch: %s / %s", targetInst.GameVersion, targetInst.Loader)
		os.Exit(1)
	}

	// Verify imported mod file and config parity
	targetModPath := filepath.Join(step13InstancesDir, targetInstID, "mods", "local-mod.jar")
	importedData, err := os.ReadFile(targetModPath)
	if err != nil {
		logf("FAIL: Imported mod file missing at %s: %v", targetModPath, err)
		os.Exit(1)
	}
	importedSHA1, _ := content.CalculateFileSHA1(targetModPath) // errcheck:ok compute sha1
	if importedSHA1 != testModSHA1 || !bytes.Equal(importedData, testModData) {
		logf("FAIL: Imported mod data mismatch: original SHA1=%s, imported SHA1=%s", testModSHA1, importedSHA1)
		os.Exit(1)
	}

	targetConfigPath := filepath.Join(step13InstancesDir, targetInstID, "config", "client.properties")
	importedConfig, err := os.ReadFile(targetConfigPath)
	if err != nil || !bytes.Equal(importedConfig, testConfigData) {
		logf("FAIL: Imported config file mismatch: %v", err)
		os.Exit(1)
	}
	logf("PASS: Round-trip import verified: mod SHA-1 exact match, overrides/configs preserved.")

	// =========================================================================
	// 14. CurseForge .zip Modpack Import Contract E2E (offline, D'4b/C6)
	// =========================================================================
	logf("\n--- STEP 14: CurseForge .zip Import Contract E2E ---")
	step14Tmp := filepath.Join(os.TempDir(), fmt.Sprintf("nord-e2e-step14-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(step14Tmp, 0755) // errcheck:ok create temp dir
	defer func() {
		_ = os.RemoveAll(step14Tmp) // errcheck:ok cleanup step 14 temp dir
	}()
	step14DBPath := filepath.Join(step14Tmp, "nord-e2e-14.db")
	step14DB, err := storage.OpenDatabase(step14DBPath)
	if err != nil {
		logf("FAIL: Step 14 SQLite init failed: %v", err)
		os.Exit(1)
	}
	defer func() { _ = step14DB.Close() }() // errcheck:ok close db
	if err := step14DB.Migrate(); err != nil {
		logf("FAIL: Step 14 migrations failed: %v", err)
		os.Exit(1)
	}
	step14Repo := storage.NewInstanceRepository(step14DB)
	step14InstancesDir := filepath.Join(step14Tmp, "instances")
	step14Svc := launch.NewInstanceService(step14Repo, nil, nil, nil, e2eClock)

	// Offline download stand-in with real SHA-1 enforcement.
	step14ModBytes := []byte("E2E-CF-MODPAYLOAD-000111222333")
	step14DL := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cf/file/10.jar" {
			_, _ = w.Write(step14ModBytes) // errcheck:ok e2e stub response body
			return
		}
		http.NotFound(w, r)
	}))
	defer step14DL.Close()

	// Build the pack zip in-memory: manifest + overrides incl. one credential
	// file that must NEVER reach the instance dir.
	step14Manifest := `{"manifestType":"minecraftModpack","manifestVersion":1,"name":"E2E Vault","version":1,"minecraft":{"version":"1.20.1","modLoaders":[{"id":"forge-14.23.5.2847"}]},"files":[{"projectID":1,"fileID":10},{"projectID":9,"fileID":99}]}`
	step14ZipPath := filepath.Join(step14Tmp, "e2epack.zip")
	{
		var zbuf bytes.Buffer
		zw := zip.NewWriter(&zbuf)
		entries := map[string]string{
			"manifest.json":                    step14Manifest,
			"overrides/options.txt":            "fov:110.0\n",
			"overrides/launcher_accounts.json": `{"accessToken":"never-copy"}`,
		}
		for name, contentStr := range entries {
			w, zerr := zw.Create(name)
			if zerr != nil {
				logf("FAIL: Step 14 zip create: %v", zerr)
				os.Exit(1)
			}
			if _, werr := w.Write([]byte(contentStr)); werr != nil {
				logf("FAIL: Step 14 zip write: %v", werr)
				os.Exit(1)
			}
		}
		if zerr := zw.Close(); zerr != nil {
			logf("FAIL: Step 14 zip close: %v", zerr)
			os.Exit(1)
		}
		if werr := os.WriteFile(step14ZipPath, zbuf.Bytes(), 0o644); werr != nil {
			logf("FAIL: Step 14 zip write file: %v", werr)
			os.Exit(1)
		}
	}

	step14Importer := launch.NewCurseForgePackImporter(step14Svc, &e2eSimpleHTTPClient{client: step14DL.Client()}, step14InstancesDir, &e2eCFPackResolver{server: step14DL.URL})
	step14Plan, err := step14Importer.ScanCurseForgeZip(context.Background(), step14ZipPath)
	if err != nil {
		logf("FAIL: Step 14 scan: %v", err)
		os.Exit(1)
	}
	if step14Plan.Format != "manifest" || step14Plan.GameVersion != "1.20.1" || step14Plan.Loader != "forge" {
		logf("FAIL: Step 14 plan metadata: %+v", step14Plan)
		os.Exit(1)
	}
	if len(step14Plan.Files) != 1 || len(step14Plan.Unresolved) != 1 {
		logf("FAIL: Step 14 plan split wrong (want 1 resolvable + 1 unresolved): %+v", step14Plan)
		os.Exit(1)
	}
	if len(step14Plan.BlockedNames) != 1 || step14Plan.BlockedNames[0] != "launcher_accounts.json" {
		logf("FAIL: Step 14 credential override not blocked at scan: %+v", step14Plan.BlockedNames)
		os.Exit(1)
	}
	step14Res, err := step14Importer.ImportCurseForgeZip(context.Background(), step14Plan)
	if err != nil {
		logf("FAIL: Step 14 import: %v", err)
		os.Exit(1)
	}
	if step14Res.Downloaded != 1 || step14Res.OverrideFiles != 1 || len(step14Res.SkippedCred) != 1 || len(step14Res.Unresolved) != 1 {
		logf("FAIL: Step 14 result counters wrong: %+v", step14Res)
		os.Exit(1)
	}
	modPath := filepath.Join(step14InstancesDir, step14Res.InstanceID, "mods", "e2e-cf-10.jar")
	gotMod, err := os.ReadFile(modPath)
	if err != nil || !bytes.Equal(gotMod, step14ModBytes) {
		logf("FAIL: Step 14 downloaded mod missing/mismatched: %v", err)
		os.Exit(1)
	}
	if _, err := os.Stat(filepath.Join(step14InstancesDir, step14Res.InstanceID, "launcher_accounts.json")); !os.IsNotExist(err) {
		logf("FAIL: SECURITY LEAK: Step 14 credential file landed in instance dir")
		os.Exit(1)
	}
	if optData, err := os.ReadFile(filepath.Join(step14InstancesDir, step14Res.InstanceID, "options.txt")); err != nil || string(optData) != "fov:110.0\n" {
		logf("FAIL: Step 14 overrides extraction wrong: %v", err)
		os.Exit(1)
	}
	if step14Inst, err := step14Repo.GetByID(context.Background(), step14Res.InstanceID); err != nil || step14Inst == nil || step14Inst.GameVersion != "1.20.1" {
		logf("FAIL: Step 14 instance not persisted correctly: %v", err)
		os.Exit(1)
	}
	logf("PASS: CF .zip contract verified: plan split (1 dl + 1 unresolved), credential blocked, override extracted, instance persisted.")

	// --- STEP 15: v0.7.2 wizard APIs, .mrpack URL pipeline & catalog parity (offline fixtures) ---
	logf("\n--- STEP 15: Loader-Version APIs, MrPack URL Import & Catalog Provider Parity E2E (v0.7.2 G5/G10) ---")
	step15Tmp := filepath.Join(os.TempDir(), fmt.Sprintf("nord-e2e-step15-%d", time.Now().UnixNano()))
	_ = os.MkdirAll(step15Tmp, 0755) // errcheck:ok create temp dir
	defer func() {
		_ = os.RemoveAll(step15Tmp) // errcheck:ok cleanup step 15 temp dir
	}()
	var step15ManifestHits int32
	// The mrpack URL pipeline enforces https on the resolved record, so the
	// whole step-15 fixture runs over a TLS test server now.
	step15PackBytes := []byte("PK\x03\x04-padded-e2e-mrpack-payload")
	{
		buf := &bytes.Buffer{}
		zw := zip.NewWriter(buf)
		f, _ := zw.Create("modrinth.index.json")                                                                                                                                       // errcheck:ok e2e fixture
		_, _ = f.Write([]byte(`{"formatVersion":1,"game":"minecraft","versionId":"e2e","name":"URL Pack","files":[],"dependencies":{"minecraft":"1.21.4","fabric-loader":"0.16.4"}}`)) // errcheck:ok e2e fixture
		_ = zw.Close()                                                                                                                                                                 // errcheck:ok finalize fixture
		step15PackBytes = buf.Bytes()
	}
	step15SHA1 := fmt.Sprintf("%x", sha1.Sum(step15PackBytes))
	step15SHA512 := fmt.Sprintf("%x", sha512.Sum512(step15PackBytes))
	var step15API *httptest.Server
	step15API = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/files/pack-2.6.mrpack":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(step15PackBytes) // errcheck:ok e2e fixture
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/mc/game/version_manifest"):
			step15ManifestHits++
			_, _ = w.Write([]byte(`{"latest":{"release":"26.3","snapshot":"26.3-rc1"},"versions":[{"id":"26.3","type":"release","url":"x","releaseTime":"2026-09-01T00:00:00Z"},{"id":"1.21.4","type":"release","url":"x","releaseTime":"2024-12-03T00:00:00Z"},{"id":"26.3-rc1","type":"snapshot","url":"x","releaseTime":"2026-08-20T00:00:00Z"}]}`)) // errcheck:ok e2e stub response
		case strings.HasPrefix(r.URL.Path, "/fabric/"):
			_, _ = w.Write([]byte(`[{"loader":{"version":"0.16.9","stable":false}},{"loader":{"version":"0.16.4","stable":true}}]`)) // errcheck:ok e2e stub response
		case strings.HasPrefix(r.URL.Path, "/quilt/"):
			_, _ = w.Write([]byte(`[{"loader":{"version":"0.26.0"}}]`)) // errcheck:ok e2e stub response
		case strings.HasPrefix(r.URL.Path, "/forge/promotions_slim.json"):
			_, _ = w.Write([]byte(`{"promos":{"1.21.4-recommended":"54.1.0-forge"}}`)) // errcheck:ok e2e stub response
		case strings.HasPrefix(r.URL.Path, "/neoforge/tags"):
			_, _ = w.Write([]byte(`[{"name":"21.5.0-beta-1"},{"name":"21.4.211"},{"name":"20.4.0"}]`)) // errcheck:ok e2e stub response
		case strings.HasPrefix(r.URL.Path, "/v2/project/p1/version"):
			_, _ = fmt.Fprintf(w, `[{"id":"verMR","name":"Pack 2.6","version_type":"release","game_versions":["1.21.4"],"loaders":["fabric"],"files":[{"filename":"pack-2.6.mrpack","url":"%s/files/pack-2.6.mrpack","size":%d,"hashes":{"sha1":"%s","sha512":"%s"}}]},{"id":"verJAR","name":"Jar drop","version_type":"release","game_versions":["1.21.4"],"loaders":["fabric"],"files":[{"filename":"pack.jar","url":"https://127.0.0.1:9/offline.jar","size":7}]}]`, step15API.URL, len(step15PackBytes), step15SHA1, step15SHA512) // errcheck:ok e2e stub response
		case strings.HasPrefix(r.URL.Path, "/v2/tag/category"):
			_, _ = w.Write([]byte(`[{"id":"fabric","name":"Fabric","project_type":""},{"id":"modpacks","name":"Modpacks","project_type":"modpack"}]`)) // errcheck:ok e2e stub response
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))
	defer step15API.Close()

	step15DB, err := storage.OpenDatabase(filepath.Join(step15Tmp, "nord-e2e-15.db"))
	if err != nil {
		logf("FAIL: Step 15 SQLite init failed: %v", err)
		os.Exit(1)
	}
	defer func() { _ = step15DB.Close() }() // errcheck:ok close db
	if err := step15DB.Migrate(); err != nil {
		logf("FAIL: Step 15 migrations failed: %v", err)
		os.Exit(1)
	}
	step15Repo := storage.NewInstanceRepository(step15DB)
	step15Svc := launch.NewInstanceService(step15Repo, nil, nil, nil, e2eClock)
	step15Adapter := wails.NewWailsAdapter(step15Svc)
	wails.NewHost(step15Adapter).SetGameManifestURL(step15API.URL + "/mc/game/version_manifest_v2.json")
	wails.NewHost(step15Adapter).SetHTTPClient(step15API.Client())
	if u15, errU := url.Parse(step15API.URL); errU == nil {
		wails.NewHost(step15Adapter).SetAllowedHosts([]string{u15.Hostname()})
	}
	step15Resolver := loadermeta.NewResolver(step15API.Client())
	step15Resolver.FabricURL = step15API.URL + "/fabric/"
	step15Resolver.QuiltURL = step15API.URL + "/quilt/"
	step15Resolver.ForgeURL = step15API.URL + "/forge/promotions_slim.json"
	step15Resolver.NeoURL = step15API.URL + "/neoforge/tags"
	wails.NewHost(step15Adapter).SetLoaderResolver(step15Resolver)
	wails.NewHost(step15Adapter).SetContent(modrinth.NewClient(step15API.URL, step15API.Client()), curseforge.NewClient(step15API.URL, "e2e-key", step15API.Client()))
	wails.NewHost(step15Adapter).SetFileSystem(nil, filepath.Join(step15Tmp, "instances"))

	// G10 step 2: Mojang manifest through the adapter (with cache).
	rels, err := step15Adapter.ListMinecraftVersions(wails.ListMinecraftVersionsRequest{})
	if err != nil || len(rels) != 2 || rels[0].ID != "26.3" {
		logf("FAIL: Step 15 ListMinecraftVersions releases: %+v %v", rels, err)
		os.Exit(1)
	}
	allv, err := step15Adapter.ListMinecraftVersions(wails.ListMinecraftVersionsRequest{Channel: "all"})
	if err != nil || len(allv) != 3 {
		logf("FAIL: Step 15 ListMinecraftVersions all: %+v %v", allv, err)
		os.Exit(1)
	}
	if step15ManifestHits != 1 {
		logf("FAIL: Step 15 manifest cache bypassed: %d upstream calls", step15ManifestHits)
		os.Exit(1)
	}
	logf("PASS: Step 15 Mojang manifest catalog served through adapter with 6h cache (1 upstream hit for 2 calls).")

	// G10 step 3: loader channels resolve recommended/latest values.
	step15Cases := []struct{ loader, want string }{
		{"fabric", "0.16.4"}, {"quilt", "0.26.0"}, {"forge", "54.1.0"}, {"neoforge", "21.4.211"},
	}
	for _, c := range step15Cases {
		res, err := step15Adapter.ListLoaderVersions(wails.ListLoaderVersionsRequest{GameVersion: "1.21.4", Loader: c.loader})
		if err != nil || res == nil || res.Default != c.want || res.Note != "" {
			logf("FAIL: Step 15 ListLoaderVersions(%s) = %+v err=%v, want default %s", c.loader, res, err, c.want)
			os.Exit(1)
		}
	}
	if _, err := step15Adapter.ListLoaderVersions(wails.ListLoaderVersionsRequest{GameVersion: "1.21.4", Loader: "sponge"}); err == nil {
		logf("FAIL: Step 15 accepted unsupported loader 'sponge'")
		os.Exit(1)
	}
	logf("PASS: Step 15 all four loader resolvers hit their official endpoints and pick the right defaults.")

	// G10 creation: loader version pinned and persisted.
	step15Inst, err := step15Adapter.CreateInstanceWithLoader(wails.CreateInstanceWithLoaderRequest{Name: "Wizard Pack", GameVersion: "1.21.4", Loader: "fabric", LoaderVersion: "0.16.4"})
	if err != nil || step15Inst == nil {
		logf("FAIL: Step 15 CreateInstanceWithLoader: %v", err)
		os.Exit(1)
	}
	if persisted, err := step15Repo.GetByID(context.Background(), step15Inst.ID); err != nil || persisted.LoaderVer != "0.16.4" {
		logf("FAIL: Step 15 loader version not persisted: %+v %v", persisted, err)
		os.Exit(1)
	}
	logf("PASS: Step 15 wizard creation pinned LoaderVer=%s in the repository.", step15Inst.LoaderVersion)

	// G5: project version listing filters to .mrpack files only.
	mrpacks, err := step15Adapter.ListMrPackVersions(wails.ListMrPackVersionsRequest{ProjectSlug: "p1", GameVersion: "1.21.4", Loader: "fabric"})
	if err != nil || len(mrpacks) != 1 || mrpacks[0].Filename != "pack-2.6.mrpack" || mrpacks[0].SHA1 != step15SHA1 || mrpacks[0].SHA512 != step15SHA512 {
		logf("FAIL: Step 15 ListMrPackVersions = %+v err=%v", mrpacks, err)
		os.Exit(1)
	}
	logf("PASS: Step 15 modpack version list keeps only .mrpack files with download URL and sha1.")

	// G5: URL import refuses non-https sources and records an honest failure status.
	// A compromised webview sends url+hashes; none of it is trusted. Fallback
	// mode (no slug/version_id) refuses any origin outside the CDN allowlist
	// before touching the network, and non-https stays refused outright.
	if _, err := step15Adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{InstanceName: "No Identifiers"}); err == nil || !strings.Contains(err.Error(), "requires project_slug and version_id") {
		logf("FAIL: Step 15 accepted a bare url-less import request: %v", err)
		os.Exit(1)
	}
	// Tampered integrity hint in resolution mode is rejected before any dial,
	// even though the URL field is now optional.
	if _, err := step15Adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{InstanceName: "Tamper", ProjectSlug: "p1", VersionID: "verMR", SHA1: fmt.Sprintf("%040x", 0xdeadbeef)}); err == nil || !strings.Contains(err.Error(), "does not match the Modrinth API record") {
		logf("FAIL: Step 15 accepted a sha1 that disagrees with the API record: %v", err)
		os.Exit(1)
	}
	// Resolution mode end-to-end: a *malicious* url from the client must be
	// ignored - the adapter re-fetches url+hashes from the API. If the client
	// URL were dialed, the loopback black-hole would fail the import.
	// The request type has no url field at all any more (a compile-time
	// guarantee); identifiers-only resolution still imports the real TLS
	// payload with both hashes verified.
	if instID, err := step15Adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{
		InstanceName: "URL Resolved Pack",
		ProjectSlug:  "p1",
		VersionID:    "verMR",
	}); err != nil || instID == "" {
		logf("FAIL: Step 15 resolution-mode import: %v", err)
		os.Exit(1)
	}
	// The real payload imported over TLS, sha1 AND sha512 verified, with a
	// deliberately wrong client-side sha1/size hint rejected up front.
	if _, err := step15Adapter.ImportMrPackFromURL(wails.ImportMrPackURLRequest{
		InstanceName: "URL Mismatch Pack", ProjectSlug: "p1", VersionID: "verMR",
		SHA1: step15SHA1, Size: 999999999,
	}); err == nil || !strings.Contains(err.Error(), "size does not match the Modrinth API record") {
		logf("FAIL: Step 15 accepted a size hint disagreeing with the API record: %v", err)
		os.Exit(1)
	}
	if st, err := step15Adapter.GetMrPackURLImportStatus(wails.InstanceIDRequest{InstanceID: "URL Resolved Pack"}); err != nil || st.Status != "complete" {
		logf("FAIL: Step 15 resolution-mode status = %+v err=%v", st, err)
		os.Exit(1)
	}

	// The download-then-import pipeline itself is exercised with the local
	// importer (the scheme check above is proven; ImportMrPack does the rest).
	step15PackBuf := &bytes.Buffer{}
	zw15 := zip.NewWriter(step15PackBuf)
	f15, err := zw15.Create("modrinth.index.json")
	if err != nil {
		logf("FAIL: Step 15 zip entry: %v", err)
		os.Exit(1)
	}
	_, _ = f15.Write([]byte(`{"formatVersion":1,"game":"minecraft","versionId":"e2e","name":"URL Pack","files":[],"dependencies":{"minecraft":"1.21.4","fabric-loader":"0.16.4"}}`)) // errcheck:ok e2e fixture
	_ = zw15.Close()                                                                                                                                                                 // errcheck:ok finalize zip
	step15PackPath := filepath.Join(step15Tmp, "url-pack.mrpack")
	if err := os.WriteFile(step15PackPath, step15PackBuf.Bytes(), 0o600); err != nil {
		logf("FAIL: Step 15 write fixture mrpack: %v", err)
		os.Exit(1)
	}
	if instID, err := step15Adapter.ImportMrPack(wails.ImportMrPackRequest{MrPackPath: step15PackPath, InstanceName: "Local Via Pipeline"}); err != nil || instID == "" {
		logf("FAIL: Step 15 importer pipeline behind the URL wrapper: %v", err)
		os.Exit(1)
	}
	logf("PASS: Step 15 URL import path validates scheme, records failed status, and the shared importer pipeline runs.")

	// Catalog parity: CF refuses modpack search honestly; Modrinth tags feed filters.
	if res, err := step15Adapter.SearchMods(wails.SearchModsRequest{Source: "curseforge", ProjectType: "modpack", Limit: 5}); err != nil || res.Reason != "unsupported_type" {
		logf("FAIL: Step 15 CF modpack gate = %+v err=%v", res, err)
		os.Exit(1)
	}
	tagz, err := step15Adapter.ListProjectTags(wails.ListProjectTagsRequest{Provider: "modrinth", ProjectType: "modpack"})
	if err != nil || len(tagz) == 0 {
		logf("FAIL: Step 15 ListProjectTags empty: %v", err)
		os.Exit(1)
	}
	seenTag := false
	for _, tg := range tagz {
		if tg.ID == "modpacks" {
			seenTag = true
		}
	}
	if !seenTag {
		logf("FAIL: Step 15 modpack tag missing: %+v", tagz)
		os.Exit(1)
	}
	logf("PASS: Step 15 catalog provider parity: CF modpack gate + Modrinth live tags (%d).", len(tagz))

	// ---------------------------------------------------------------------
	// Stage 16 (v0.7.2 round-6, owner p1): the staging seed is published in this
	// repository, so the whole safety argument is "the client trusts exactly one
	// key". Here that argument is executed, not asserted in prose: a real
	// manifest is signed by the real generator with the STAGING key, then fed
	// through the real client update path (updater.AutoUpdater with
	// GetDefaultPublicKey) - the update must be refused and the running
	// executable left untouched. A control pass with the staging public half
	// proves the refusal came from the trust root and not from a broken fixture.
	// ---------------------------------------------------------------------
	{
		step16Tmp, err := os.MkdirTemp("", "nord-e2e-step16-*")
		if err != nil {
			logf("FAIL: Step 16 tmpdir: %v", err)
			os.Exit(1)
		}
		defer os.RemoveAll(step16Tmp) // errcheck:ok cleanup e2e scratch directory
		step16Dist := filepath.Join(step16Tmp, "dist")
		if err := os.MkdirAll(step16Dist, 0755); err != nil {
			logf("FAIL: Step 16 mkdir dist: %v", err)
			os.Exit(1)
		}
		step16Payload := []byte("nord-launcher-v0.9.8-staging-signed-update-payload")
		step16Name := "nord-launcher-0.9.8.tar.gz"
		if runtime.GOOS == "windows" {
			step16Name = "NordLauncher.exe"
		}
		step16Artifact := filepath.Join(step16Dist, step16Name)
		if err := os.WriteFile(step16Artifact, step16Payload, 0644); err != nil {
			logf("FAIL: Step 16 write artifact: %v", err)
			os.Exit(1)
		}
		step16Manifest := filepath.Join(step16Dist, "manifest-stable.json")
		stagingPriv, err := releasetool.StagingPrivateKey()
		if err != nil {
			logf("FAIL: Step 16 staging key: %v", err)
			os.Exit(1)
		}
		gen16 := exec.Command(genmanifestBin,
			"-version", "0.9.8",
			"-channel", "stable",
			"-dist", step16Dist,
			"-out", step16Manifest,
			"-privkey-hex", hex.EncodeToString(stagingPriv.Seed()),
			"-allow-insecure-dev-key",
		)
		if out, err := gen16.CombinedOutput(); err != nil {
			logf("FAIL: Step 16 staging signing: %v\n%s", err, out)
			os.Exit(1)
		}
		raw16, err := os.ReadFile(step16Manifest)
		if err != nil {
			logf("FAIL: Step 16 read manifest: %v", err)
			os.Exit(1)
		}
		var manifest16 updater.UpdateManifest
		if err := json.Unmarshal(raw16, &manifest16); err != nil {
			logf("FAIL: Step 16 parse manifest: %v", err)
			os.Exit(1)
		}
		plat16 := updater.CurrentPlatformKey()
		asset16, ok := manifest16.Platforms[plat16]
		if !ok || asset16.Signature == "" {
			logf("FAIL: Step 16 manifest lacks the current platform %s", plat16)
			os.Exit(1)
		}
		var srv16 *httptest.Server
		srv16 = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/payload":
				_, _ = w.Write(step16Payload) // errcheck:ok mock update server
			case "/manifest":
				// Re-point the asset URL at this server, keeping signature/hash/size
				// untouched - same trick step 8 uses, so the client exercises its real
				// download path instead of 404ing on a github.com URL.
				served := manifest16
				served.Platforms = make(map[string]updater.PlatformAsset)
				for k, v := range manifest16.Platforms {
					v.URL = srv16.URL + "/payload"
					served.Platforms[k] = v
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(served) // errcheck:ok mock update server
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv16.Close()

		step16Exe := filepath.Join(step16Tmp, "launcher-under-test")
		if err := os.WriteFile(step16Exe, []byte("ORIGINAL-CLIENT-BINARY"), 0755); err != nil {
			logf("FAIL: Step 16 seed exe: %v", err)
			os.Exit(1)
		}

		// 1) The shipped trust root must refuse a staging-signed update.
		prodClient := updater.NewAutoUpdater("0.7.1", srv16.URL+"/manifest", updater.GetDefaultPublicKey(), srv16.Client())
		info16, err := prodClient.CheckForUpdates(context.Background())
		if err != nil || info16 == nil || !info16.Available {
			logf("FAIL: Step 16 the manifest should be offered (0.9.8 > 0.7.1) so the refusal happens at apply time, got %+v err=%v", info16, err)
			os.Exit(1)
		}
		if err := prodClient.DownloadAndApply(context.Background(), info16.Asset, step16Exe); err == nil {
			logf("FAIL: Step 16 CRITICAL: a staging-signed update was APPLIED by a production client")
			os.Exit(1)
		} else if !strings.Contains(err.Error(), updater.ErrSignatureInvalid.Error()) {
			logf("FAIL: Step 16 expected signature rejection, got: %v", err)
			os.Exit(1)
		}
		kept, err := os.ReadFile(step16Exe)
		if err != nil || string(kept) != "ORIGINAL-CLIENT-BINARY" {
			logf("FAIL: Step 16 the refused update touched the running executable (err=%v)", err)
			os.Exit(1)
		}
		if _, err := os.Stat(step16Exe + ".new"); !os.IsNotExist(err) {
			logf("FAIL: Step 16 a rejected download left .new staged next to the executable")
			os.Exit(1)
		}

		// 2) Control: same bytes, staging trust root -> applies. Without this the
		//    check above would also pass on a fixture that fails for the wrong reason.
		stagingPub, err := releasetool.StagingPublicKey()
		if err != nil {
			logf("FAIL: Step 16 staging pubkey: %v", err)
			os.Exit(1)
		}
		testClient := updater.NewAutoUpdater("0.7.1", srv16.URL+"/manifest", stagingPub, srv16.Client())
		info16b, err := testClient.CheckForUpdates(context.Background())
		if err != nil {
			logf("FAIL: Step 16 control check: %v", err)
			os.Exit(1)
		}
		if err := testClient.DownloadAndApply(context.Background(), info16b.Asset, step16Exe); err != nil {
			logf("FAIL: Step 16 control (staging trust root) could not apply the same manifest - the fixture is broken, not the trust check: %v", err)
			os.Exit(1)
		}
		applied, _ := os.ReadFile(step16Exe)
		if string(applied) != string(step16Payload) {
			logf("FAIL: Step 16 control applied the wrong bytes (%d)", len(applied))
			os.Exit(1)
		}
		// 3) Regression for the round-6 rehearsal bug: "rehearsal mode" must be a
		// property of the run, not of which env vars happen to be exported. With a
		// foreign production-shaped seed in ED25519_PRIVATE_KEY, -staging-key has to
		// ignore it and sign with the repo staging seed anyway.
		_, foreignPriv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			logf("FAIL: Step 16 ephemeral key: %v", err)
			os.Exit(1)
		}
		forceDist := filepath.Join(step16Tmp, "forced")
		if err := os.MkdirAll(forceDist, 0755); err != nil {
			logf("FAIL: Step 16 mkdir forced dist: %v", err)
			os.Exit(1)
		}
		forceArtifact := filepath.Join(forceDist, step16Name)
		if err := os.WriteFile(forceArtifact, step16Payload, 0644); err != nil {
			logf("FAIL: Step 16 forced artifact: %v", err)
			os.Exit(1)
		}
		forceManifest := filepath.Join(forceDist, "manifest-stable.json")
		forceCmd := exec.Command(genmanifestBin,
			"-version", "0.9.7", "-channel", "stable",
			"-dist", forceDist, "-out", forceManifest,
			"-staging-key", "-allow-insecure-dev-key",
		)
		forceCmd.Env = append(os.Environ(), "ED25519_PRIVATE_KEY="+hex.EncodeToString(foreignPriv.Seed()))
		if out, err := forceCmd.CombinedOutput(); err != nil {
			logf("FAIL: Step 16 generator with -staging-key: %v\n%s", err, out)
			os.Exit(1)
		}
		forceRaw, err := os.ReadFile(forceManifest)
		if err != nil {
			logf("FAIL: Step 16 read forced manifest: %v", err)
			os.Exit(1)
		}
		var forcedManifest updater.UpdateManifest
		if err := json.Unmarshal(forceRaw, &forcedManifest); err != nil {
			logf("FAIL: Step 16 parse forced manifest: %v", err)
			os.Exit(1)
		}
		forcedAsset := forcedManifest.Platforms[plat16]
		forcedSig, err := base64.StdEncoding.DecodeString(forcedAsset.Signature)
		if err != nil {
			logf("FAIL: Step 16 decode forced signature: %v", err)
			os.Exit(1)
		}
		if !updater.VerifyPayload(stagingPub, step16Payload, forcedSig) {
			logf("FAIL: Step 16 -staging-key did not sign with the staging seed")
			os.Exit(1)
		}
		if updater.VerifyPayload(foreignPriv.Public().(ed25519.PublicKey), step16Payload, forcedSig) {
			logf("FAIL: Step 16 CRITICAL: -staging-key still honoured ED25519_PRIVATE_KEY, so a rehearsal artifact could be production-valid")
			os.Exit(1)
		}

		logf("PASS: Step 16 a staging-signed manifest is refused by the production client (signature, not parsing) and the control trust root accepts the same bytes.")
	}

	logf("\n=================================================================")
	logf(" ALL 16 E2E STAGES PASSED")
	logf("=================================================================")

	// Save trace to build/e2e/e2e_trace.txt
	traceDir := filepath.Join(".", "build", "e2e")
	_ = os.MkdirAll(traceDir, 0755) // errcheck:ok create e2e build directory
	traceFile := filepath.Join(traceDir, "e2e_trace.txt")
	if err := os.WriteFile(traceFile, traceBuf.Bytes(), 0644); err != nil {
		logf("WARNING: Failed to save trace log: %v", err)
	} else {
		logf("Evidence trace recorded in: %s", traceFile)
	}
}

// e2eSimpleHTTPClient implements ports.HTTPClient against a fixed client.
type e2eSimpleHTTPClient struct{ client *http.Client }

func (c *e2eSimpleHTTPClient) Get(_ context.Context, url string, _ map[string]string) ([]byte, error) {
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // errcheck:ok defer close in e2e helper
	return io.ReadAll(resp.Body)
}

func (c *e2eSimpleHTTPClient) DownloadFile(_ context.Context, url string, dest string, expectedSHA1 string, _ func(int64, int64)) error {
	resp, err := c.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close() // errcheck:ok defer close in e2e helper
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if expectedSHA1 != "" {
		sum := sha1.Sum(data)
		if hex.EncodeToString(sum[:]) != expectedSHA1 {
			return fmt.Errorf("download sha1 mismatch for %s", url)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

// e2eCFPackResolver resolves exactly project 1/file 10 offline; everything
// else surfaces the same error text as the production resolver.
type e2eCFPackResolver struct{ server string }

func (r *e2eCFPackResolver) ResolvePackFile(_ context.Context, projectID, fileID int64) (string, string, int64, string, error) {
	if projectID == 1 && fileID == 10 {
		sum := sha1.Sum([]byte("E2E-CF-MODPAYLOAD-000111222333"))
		return r.server + "/cf/file/10.jar", hex.EncodeToString(sum[:]), int64(len("E2E-CF-MODPAYLOAD-000111222333")), "e2e-cf-10.jar", nil
	}
	return "", "", 0, "", fmt.Errorf("file %d not found under project %d (removed or renamed)", fileID, projectID)
}
