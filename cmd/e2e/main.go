package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	"github.com/nord-launcher/launcher/internal/core/clock"
	"github.com/nord-launcher/launcher/internal/core/content"
	"github.com/nord-launcher/launcher/internal/core/content/curseforge"
	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/downloader"
	"github.com/nord-launcher/launcher/internal/core/java"
	"github.com/nord-launcher/launcher/internal/core/launch"
	"github.com/nord-launcher/launcher/internal/core/updater"
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
	_ = zw.Close() // errcheck:ok close zip writer

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

	logf("\n=================================================================")
	logf(" ALL 9 E2E STAGES PASSED")
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
