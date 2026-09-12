# Nord Launcher

[![CI Quality Gate](https://github.com/Aethelis-Projects/Aethelis-Launcher/actions/workflows/ci.yml/badge.svg)](https://github.com/Aethelis-Projects/Aethelis-Launcher/actions/workflows/ci.yml)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
[![SolidJS](https://img.shields.io/badge/SolidJS-1.9-2c4f7c.svg)](https://www.solidjs.com/)
[![Wails v3](https://img.shields.io/badge/Wails-v3.0.0--beta.20-df1a22.svg)](https://wails.io/)

**Nord Launcher** is an open-source, high-performance, resource-efficient Minecraft launcher built with Go and SolidJS, running inside a native desktop shell powered by Wails v3.

It is engineered from first principles with a strict hexagonal architecture, contract-first IPC, fine-grained UI reactivity (without virtual DOM overhead), and zero tolerance for AI-generated boilerplate ("Anti-AI-Slop" discipline).

---

## Key Features

- **Hexagonal Core**: Completely decoupled business logic in pure Go (`internal/core`) with testable ports and swappable adapters (`internal/adapters`).
- **Native Desktop Shell**: Integrated Wails v3 (`v3.0.0-beta.20`) with native OS windowing, serving an embedded SolidJS single-page application from memory.
- **Contract-First IPC**: Strongly-typed JSON schema definitions in `ipc/schema` with deterministic codegen (`ipc/codegen`) enforced by CI gates (`git diff --exit-code`).
- **Game Provisioning Pipeline**: Automated resolution of Mojang Version Manifest V2, `version.json`, `client.jar` with SHA-1 validation, OS-rule library filtering, assets downloads, and native extraction.
- **Resilient Content Engine**: Parallel Range-based downloader (`downloader.Dispatcher`) supporting HTTP 206 resume, multi-mirror failover, SHA-1 / SHA-256 verification, and `.mrpack` modpack extraction.
- **Modloader Matrix**: Native metadata resolvers for Vanilla, Fabric, Quilt, and NeoForge.
- **Secure Credential Storage**: Persistent refresh-token management via OS Credential Manager (Windows Credential Manager / Linux Secret Service) with safe in-memory fallback.
- **Robust Process Supervision**: Windows Job Objects (`KILL_ON_JOB_CLOSE`) to eliminate orphaned background Java processes, paired with a Log4j crash classifier for diagnostics.
- **Cryptographic Auto-Updater**: Ed25519-signed update manifests (`manifest-stable.json`, `manifest-beta.json`) guarding binary authenticity before payload execution.

---

## Target Platforms & System Requirements

- **Windows**: Windows 10 (version 1809+, Build 17763 or newer) and Windows 11 (64-bit x64). Legacy Windows (7, 8, 8.1) is explicitly rejected at the installer level.
- **Linux**: Ubuntu 22.04 LTS or newer (64-bit x64) with GTK4 and WebKitGTK 6.0.
- **macOS**: Deferred to M7 (see `docs/backlog.md`).

---

## Architecture Overview

Nord Launcher follows strict Hexagonal Architecture (Ports & Adapters):

```
+-------------------------------------------------------------+
|                     SolidJS Frontend UI                     |
|           (Tailwind CSS, Lucide, Fine-Grained Signals)      |
+------------------------------+------------------------------+
                               | Wails v3 IPC (JSON-RPC)
+------------------------------v------------------------------+
|                   Adapters Layer (`adapters/`)              |
|   - wails:      IPC Dispatcher (145 ns latency)             |
|   - keyring:    OS Credential Manager (WinCred / SecretSvc) |
|   - process:    Win32 Job Objects / POSIX Supervision       |
|   - java:       Registry & Path Scanner                     |
|   - fs & http:  OS Filesystem & Connection-Pooled HTTP      |
+------------------------------+------------------------------+
                               | Ports Interfaces
+------------------------------v------------------------------+
|                     Core Domain (`core/`)                   |
|   - auth:       Microsoft OAuth2 PKCE & Offline UUID v3     |
|   - content:    Modrinth, CurseForge, Loaders, Resolver     |
|   - downloader: Parallel Range 206, Mirror Failover         |
|   - java:       Adoptium Client & Version Matrix            |
|   - launch:     JVM Arguments & Crash Classification        |
|   - storage:    Pure-Go SQLite WAL (modernc.org/sqlite)     |
|   - updater:    Ed25519 Cryptographic Manifest Verifier     |
+-------------------------------------------------------------+
```

---

## Quick Start & Development

### Prerequisites

- **Go**: 1.26 or newer
- **Node.js**: 22 or newer
- **pnpm**: 12 or newer
- **Linux dependencies** (if building on Ubuntu):
  ```bash
  sudo apt-get install -y libgtk-4-dev libwebkitgtk-6.0-dev libsoup-3.0-dev pkg-config
  ```

### Local Setup

1. **Clone the repository**:
   ```bash
   git clone https://github.com/Aethelis-Projects/Aethelis-Launcher.git
   cd Aethelis-Launcher
   ```

2. **Verify IPC contract parity**:
   ```bash
   go run ./ipc/codegen/generate.go
   ```

3. **Install frontend dependencies & build UI**:
   ```bash
   cd frontend
   pnpm install
   pnpm build
   cd ..
   ```

4. **Run core tests**:
   ```bash
   go test -v -race ./internal/...
   ```

5. **Launch in development mode**:
   ```bash
   go run ./cmd/launcher/main.go
   ```

6. **Run headless verification**:
   ```bash
   go run ./cmd/launcher/main.go --headless
   ```

---

## Releases & Downloads

Official releases and artifacts are available on GitHub Releases:
- **Latest Release**: [Aethelis-Launcher Releases](https://github.com/Aethelis-Projects/Aethelis-Launcher/releases)
- **Stable Update Manifest**: [manifest-stable.json](https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/latest/download/manifest-stable.json)

### Building Production Releases Locally

#### Windows (EXE + NSIS Setup)

Run the automated release builder script:
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\build_release.ps1 -Version 0.1.1
```
This produces:
- `dist/windows/NordLauncher.exe` (Production binary with `-H=windowsgui`)
- `dist/windows/NordLauncher-Setup.exe` (Signed modern NSIS installer)
- `dist/windows/SHA256SUMS.txt` (Cryptographic verification checksums)

#### Linux (tar.gz)

```bash
chmod +x ./scripts/build_release.sh
./scripts/build_release.sh 0.1.1
```

---

## Quality Gates & Verification

Every pull request and push to `master` and `main` must pass the automated CI Quality Gate:

- **IPC Parity**: Schema matching Go DTOs and TypeScript models (`ipc/codegen`).
- **Unit & Race Tests**: `go test -v -race ./internal/...`.
- **Coverage Gate**: Statement coverage $\ge 80.0\%$ on `internal/core/...`.
- **Static Analysis & Security**: Native `go vet ./...`, `govulncheck@v1.8.0`, and `go-licenses`.
- **E2E Integration Gate**: Standalone multi-step runner (`cmd/e2e/main.go`) testing Keyring, Auth, Java, Downloader, Update signature, and Instance launch lifecycle.
- **Bundle Budget**: SolidJS frontend $\le 250$ KB gzip.
- **Binary Budget**: Release launcher $\le 40$ MB.
- **Latency SLA**: In-process IPC dispatch p95 $\le 5000$ ns.
- **Anti-AI-Slop Linter**: Zero tolerance for synthetic AI markers, unhandled blank discards (`_ = err`), and untyped escapes across TypeScript and Go.

---

## License

Nord Launcher is licensed under the [GNU General Public License v3.0 (GPLv3)](LICENSE).
