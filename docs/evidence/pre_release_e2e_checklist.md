# Pre-Release E2E Verification Protocol (M3/M4 & v0.1.1 Production Gate)

This document formalizes the two-tier verification protocol required prior to tagging production releases (`v*.*.*`). Automated protocol, parser, and IPC testing are verified via `cmd/e2e/main.go` producing cryptographic evidence in `docs/evidence/e2e_m3_m4_trace.txt`, while live account login and OAuth loopback browser redirects undergo interactive human verification.

---

## 1. Tier 1: Automated E2E Test Suite (`cmd/e2e/main.go`)

Every build in CI (`.github/workflows/ci.yml`) executes the standalone runner verifying all core subsystems without mock shortcuts:

| Step | Subsystem Verified | Validation Criteria | Automated Status |
|---|---|---|---|
| **Step 1** | Keyring Persistence | Cross-instance token persistence via OS Credential Manager (wincred / SecretService) | ✅ **PASS** (Trace: `e2e_m3_m4_trace.txt`) |
| **Step 2** | Mojang UUID v3 Parity | Exact match for `"Steve"` (`5627dd98...`) and `"Alex"` (`36532b5e...`) | ✅ **PASS** (Trace: `e2e_m3_m4_trace.txt`) |
| **Step 3** | Java Version Matrix & Detection | Epoch mapping (8, 17, 21) and Adoptium release file parsing | ✅ **PASS** (Trace: `e2e_m3_m4_trace.txt`) |
| **Step 4** | Content & Parallel Downloader | `.mrpack` index parsing, overrides extraction, and HTTP 206 Range download | ✅ **PASS** (Trace: `e2e_m3_m4_trace.txt`) |
| **Step 5** | Process Manager & Crash Classifier | OS process supervision, Win32 Job Objects, and Log4j OutOfMemoryError diagnostics | ✅ **PASS** (Trace: `e2e_m3_m4_trace.txt`) |
| **Step 6** | Product Launch Lifecycle | `InstanceService.Launch()` real launch with positive system PID, Idle on 0, Crashed on 1 | ✅ **PASS** (Trace: `e2e_m3_m4_trace.txt`) |

*Full automated execution trace log: [docs/evidence/e2e_m3_m4_trace.txt](docs/evidence/e2e_m3_m4_trace.txt).*

---

## 2. Tier 2: Interactive Staging & Production Verification Protocol

### 2.1. Live Microsoft OAuth2 Interactive Login
- **Objective**: Verify live browser redirection via `WailsAdapter.LoginMicrosoft`, loopback callback on `127.0.0.1:port`, and token acquisition.
- **Steps**:
  1. Launch `NordLauncher.exe`.
  2. Navigate to Account Manager.
  3. Click "Add Microsoft Account".
  4. Complete login in default system browser with valid Mojang/Microsoft credentials.
  5. Confirm launcher captures OAuth code, exchanges for XSTS token, queries Mojang profile API, and stores refresh token in Keyring.
  6. Restart launcher; verify profile remains active without re-login prompt.
- **Status**: Manual protocol defined; ready for live credentials verification.

### 2.2. Real Vanilla Minecraft Download & Launch
- **Objective**: Full chain download of Mojang version JSON, client JAR, asset indexes, and game launch via `InstanceService.Launch()`.
- **Steps**:
  1. Create a new instance: Version `1.21.1` (Vanilla).
  2. Click "Play" (`LaunchButton`).
  3. Verify parallel download of `client.jar` and assets (`~500 MB`) with range requests.
  4. Verify Java 21 detection/provisioning.
  5. Verify game window opens; real process PID (> 0) tracked in launcher.
  6. Exit game; verify process unregisters cleanly and launcher transitions to `Idle`.
- **Status**: Pipeline implemented in `internal/core/game`; automated in Step 6; ready for live Mojang download.

### 2.3. Real .mrpack Import & Modded Launch (Fabric)
- **Objective**: Verify third-party modpack installation from Modrinth.
- **Steps**:
  1. Download `.mrpack` test modpack (e.g. Fabulously Optimized for 1.21.1).
  2. Drag-and-drop or select file via "Import Modpack" dialog.
  3. Verify loader resolution (Fabric loader version resolved).
  4. Verify all referenced mod files downloaded and validated via SHA-1/SHA-512 hashes.
  5. Launch instance; verify Fabric mod loading screen and main menu.
- **Status**: Automated in Step 4; ready for user modpack import.

### 2.4. Crash Diagnostics Verification
- **Objective**: Force a JVM crash and verify Log4j error classification.
- **Steps**:
  1. Configure instance JVM arguments with deliberately insufficient RAM (`-Xmx64M`).
  2. Launch game; wait for JVM crash.
  3. Verify launcher catches non-zero exit code and displays `CrashModal`.
  4. Verify diagnosis correctly flags `Out of Memory` with remedy recommendation.
- **Status**: Verified in automated Step 5 and Step 6.
