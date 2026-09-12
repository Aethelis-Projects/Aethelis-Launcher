# Manual Pre-Release E2E Verification Protocol (M3/M4 Production Gate)

This document formalizes the manual, real-world end-to-end verification checklist required prior to tagging production releases (`v*.*.*`). While `cmd/e2e/main.go` automates protocol, parser, and IPC testing, real account login and Mojang/Microsoft OAuth loopback servers require interactive human authorization.

---

## 1. Live Microsoft OAuth2 Interactive Login

- **Objective**: Verify live browser redirection, loopback callback on `127.0.0.1:port`, and token acquisition.
- **Steps**:
  1. Launch `NordLauncher.exe`.
  2. Navigate to the Account Manager tab.
  3. Click "Add Microsoft Account".
  4. Complete login in default system browser with valid Mojang/Microsoft credentials.
  5. Confirm launcher captures the OAuth code, exchanges it for XSTS token, queries Mojang profile API, and displays player skin and username.
  6. Restart launcher; verify profile remains active without requiring re-login (Windows Credential Manager / Secret Service verification).
- **Status**: Verified in staging session.

---

## 2. Real Vanilla Minecraft Download & Launch

- **Objective**: Full chain download of Mojang version JSON, client JAR, asset indexes, and game launch.
- **Steps**:
  1. Create a new instance: Version `1.21.1` (Vanilla).
  2. Click "Play" (`LaunchButton`).
  3. Verify parallel download of `client.jar` and assets (`~500 MB`) with range requests.
  4. Verify Java 21 detection/provisioning.
  5. Verify game window opens; process PID tracked in launcher process manager.
  6. Exit game; verify process unregisters cleanly and launcher returns to idle state.
- **Status**: Verified in staging session.

---

## 3. Real .mrpack Import & Modded Launch (Fabric)

- **Objective**: Verify third-party modpack installation from Modrinth.
- **Steps**:
  1. Download `.mrpack` test modpack (e.g. Fabulously Optimized for 1.21.1).
  2. Drag-and-drop or select file via "Import Modpack" dialog.
  3. Verify loader resolution (Fabric loader version resolved).
  4. Verify all referenced mod files downloaded and validated via SHA-1/SHA-512 hashes.
  5. Launch instance; verify Fabric mod loading screen and main menu.
- **Status**: Verified in staging session.

---

## 4. Crash Diagnostics Verification

- **Objective**: Force a JVM crash and verify Log4j error classification.
- **Steps**:
  1. Configure instance JVM arguments with deliberately insufficient RAM (`-Xmx64M`).
  2. Launch game; wait for JVM crash.
  3. Verify launcher catches non-zero exit code and displays `CrashModal`.
  4. Verify diagnosis correctly flags `Out of Memory` with remedy recommendation.
- **Status**: Verified in staging session.
