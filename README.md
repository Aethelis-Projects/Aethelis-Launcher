<p align="center">
  <img src="assets/banner.png" alt="Nord Launcher" width="100%">
</p>

<p align="center">
  <a href="https://github.com/Aethelis-Projects/Aethelis-Launcher/actions/workflows/ci.yml">
    <img src="https://github.com/Aethelis-Projects/Aethelis-Launcher/actions/workflows/ci.yml/badge.svg?branch=master" alt="CI Quality Gate">
  </a>
  <a href="https://github.com/Aethelis-Projects/Aethelis-Launcher/releases">
    <img src="https://img.shields.io/github/v/release/Aethelis-Projects/Aethelis-Launcher?include_prereleases&sort=semver" alt="Latest release">
  </a>
  <a href="LICENSE">
    <img src="https://img.shields.io/badge/license-GPLv3-88C0D0.svg" alt="License: GPL v3">
  </a>
  <a href="https://go.dev/">
    <img src="https://img.shields.io/badge/Go-1.26-00ADD8.svg" alt="Go 1.26">
  </a>
  <a href="https://www.solidjs.com/">
    <img src="https://img.shields.io/badge/SolidJS-1.9-2c4f7c.svg" alt="SolidJS">
  </a>
  <a href="https://wails.io/">
    <img src="https://img.shields.io/badge/Wails-v3.0.0--beta.20-df1a22.svg" alt="Wails v3">
  </a>
  <img src="https://img.shields.io/badge/platforms-Windows%2010%2F11%20%C2%B7%20Linux-A3BE8C.svg" alt="Platforms: Windows 10/11, Linux">
</p>

<p align="center">
  <b>A fast, light, and precise Minecraft launcher.</b><br>
  Go core &middot; SolidJS UI &middot; native Wails v3 shell &middot; signed auto-updates &middot; no telemetry
</p>

---

## Why Nord Launcher

Most Minecraft launchers are Electron appliances: megabytes of overhead, background noise, and opaque update pipelines. Nord Launcher is built the other way around — a pure Go core, a ~28 KB embedded UI, and a small native shell. Every update is cryptographically signed, every secret lives in the OS credential manager, and nothing is phoned home.

**Measured in CI (release v0.1.2):**

| Metric | Result | Budget |
|---|---|---|
| Cold start | ~1.03 s | < 2.0 s |
| Idle RAM | ~82 MB | < 150 MB |
| IPC dispatch (p95) | 297 ns | ≤ 5,000 ns |
| Frontend bundle (gzip) | 27.7 KB | ≤ 250 KB |
| Windows binary | 17.5 MB | < 40 MB |
| Core test coverage | 80.9% | ≥ 80% |

---

## Downloads

| Platform | Artifact |
|---|---|
| Windows 10/11 (x64) — installer | [NordLauncher-Setup.exe](https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/latest/download/NordLauncher-Setup.exe) |
| Windows 10/11 (x64) — portable | [NordLauncher.exe](https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/latest/download/NordLauncher.exe) |
| Linux x64 (Ubuntu 22.04+) | [Latest release page](https://github.com/Aethelis-Projects/Aethelis-Launcher/releases/latest) — `nord-launcher-vX.Y.Z-linux-amd64.tar.gz` |

Every release ships `SHA256SUMS.txt` and an Ed25519-signed `manifest-stable.json`. Verify before you run:

```bash
sha256sum -c SHA256SUMS.txt
```

---

## Features

- **Hexagonal core** — pure Go business logic in `internal/core` behind ports; adapters are swappable and individually testable.
- **Native shell** — Wails v3 with an embedded SolidJS SPA served from memory. No Electron.
- **Contract-first IPC** — JSON schemas generate typed Go DTOs and TypeScript models; parity is enforced by CI on every push.
- **Full provisioning pipeline** — Mojang Version Manifest V2, `client.jar` with SHA-1 validation, OS-aware library filtering, native extraction (modern and legacy Maven), assets.
- **Resilient downloads** — parallel HTTP 206 range fetches with resume, mirror failover, and checksum verification.
- **Modloader matrix** — metadata resolvers for Vanilla, Fabric, Quilt, and NeoForge.
- **Secure sessions** — Microsoft OAuth2 with PKCE; refresh tokens stored in the Windows Credential Manager or Secret Service, never in plain files.
- **Process supervision** — Windows Job Objects (`KILL_ON_JOB_CLOSE`) eliminate orphaned Java processes; Log4j crash classification for diagnostics.
- **Signed auto-updater** — Ed25519 manifest verification before any payload is executed, with a clean restart flow.
- **Contextual mod management** — integrated CurseForge & Modrinth catalog within instance settings, deterministic 3-level fallback version selection, manual version picker drawer, and automatic dependency resolution.
- **Manifest & reconcile** — sidecar `mods/nord-installs.json` tracking installation provenance, filesystem source-of-truth reconcile, and dual-file deletion.

---

## Architecture

Strict Hexagonal Architecture (Ports & Adapters):

```
+-------------------------------------------------------------+
|                     SolidJS Frontend UI                     |
|           (Tailwind CSS, Lucide, Fine-Grained Signals)      |
+------------------------------+------------------------------+
                               | Wails v3 IPC (JSON-RPC)
+------------------------------v------------------------------+
|                   Adapters Layer (`adapters/`)              |
|   - wails:      IPC Dispatcher (297 ns p95, CI)             |
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

## Requirements

- **Windows**: Windows 10 (1809+ / Build 17763 or newer) and Windows 11, 64-bit x64. Older Windows is explicitly rejected at the installer level.
- **Linux**: Ubuntu 22.04 LTS or newer (x64) with GTK4 and WebKitGTK 6.0.
- **macOS**: planned for a future milestone.

## Building from Source

Prerequisites: Go 1.26+, Node.js 22+, pnpm 12+ (plus the Linux GTK4/WebKitGTK packages above when building on Linux).

```bash
git clone https://github.com/Aethelis-Projects/Aethelis-Launcher.git
cd Aethelis-Launcher

go run ./ipc/codegen/generate.go      # verify IPC contract parity

cd frontend && pnpm install && pnpm build && cd ..

go test -race ./internal/...          # core tests
go run ./cmd/launcher                 # development mode
```

---

## Quality Gates

Every push and pull request must pass the CI Quality Gate:

- **IPC parity** — generated Go DTOs / TS models match the schema exactly.
- **Unit & race tests** — `go test -race ./internal/...`.
- **Coverage gate** — ≥ 80% statements on `internal/core/...`.
- **Static analysis & security** — `go vet`, `govulncheck`, `go-licenses`.
- **E2E integration runner** — Keyring, Auth, Java, Downloader, update signature, instance launch lifecycle.
- **Code hygiene linter** — zero unhandled blank discards, empty catches, or `as any` bypasses across TypeScript and Go.
- **Bundle budget** — frontend ≤ 250 KB gzip.
- **Binary budget** — release binary ≤ 40 MB.
- **Latency SLA** — in-process IPC dispatch p95 ≤ 5,000 ns.

---

## Roadmap

- **v0.1.3** — in-app update panel (check / apply / restart from the UI; IPC is already in place).
- **v0.2.0** — Fabric, Quilt, and NeoForge provisioning and launch.
- **v0.3.0** — CurseForge & Modrinth integration, sidecar key resolution.
- **v0.4.0** — Contextual mod management, deterministic version selection, sidecar manifest & reconcile.
- **v0.5.0** — CurseForge resilience, SQLite content cache, startup update modal & honest 7-state badge.
- **v0.6.0** — Modpack Round-trip (.mrpack), Mod Version Histories & Changelogs, Temurin update detection & runtime hygiene.
- **Later** — beta update channel, opt-in telemetry, macOS.

---

## License

Nord Launcher is licensed under the [GNU General Public License v3.0 (GPLv3)](LICENSE).

Minecraft is a trademark of Mojang Studios. Nord Launcher is an independent project, not affiliated with or endorsed by Mojang or Microsoft.
