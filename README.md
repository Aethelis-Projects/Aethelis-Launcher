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

Most Minecraft launchers are Electron appliances: megabytes of overhead, background noise, and opaque update pipelines. Nord Launcher is built the other way around — a pure Go core, an ~81 KB gzipped embedded UI, and a small native shell. Every update is cryptographically signed, every secret lives in the OS credential manager, and nothing is phoned home.

**Measured on v0.7.2 (2026-09-28; CI + headless reruns, Linux amd64 / cross-Windows):**

| Metric | Result | Budget |
|---|---|---|
| Core init (`--idle-test`, headless) | 9 ms | < 100 ms |
| IPC dispatch | ~109 ns/op (`BenchmarkWailsAdapter_IPCDispatch`, 2-core CI runner) | p95 ≤ 5,000 ns |
| Frontend bundle (gzip) | 81 KB main chunk / 88 KB total (`frontend/dist`) | ≤ 250 KB |
| Binary (stripped, `-s -w`, frontend not embedded) | 17.0 MB linux / 17.4 MB windows | < 40 MB |
| Core test coverage | 80.4% (`go test -coverprofile ./internal/core/...`) | ≥ 80% |
| Idle RAM (full GUI) | measured on hardware — acceptance checklist item | < 150 MB |
| Cold start (full GUI) | measured on hardware — acceptance checklist item | < 2.0 s |

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

## Java Runtime Matrix

Nord Launcher resolves the required Java version automatically and provisions Temurin JDK runtimes on demand:

| Minecraft Version | Required Java | Compatible / Recommended Runtimes | Notes |
|:---|:---|:---|:---|
| 26.1+ | Java 25 | Java 25 LTS | Modern Mojang annual release scheme |
| 1.20.5 – 26.0 | Java 21 | Java 21 LTS | Modern default runtime |
| 1.18 – 1.20.4 | Java 17 | Java 17 LTS | Caves & Cliffs Part II through 1.20.4 |
| 1.17 – 1.17.1 | Java 16 | Java 16 | Non-LTS release specifically required for 1.17 |
| ≤ 1.16.5 | Java 8 | Java 8 LTS, Java 11 LTS | Java 11 is supported for modern 1.12.2/1.16.5 modpacks |

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
- **Wails binding registry** — registry of all 72 webview-facing IPC methods enforced bidirectionally (forward registration parity + reverse `reflect` exhaustive check; host-only `Set*` wiring allowlisted, since Wails reflects all public receiver methods by design).
- **Discord Rich Presence (opt-in)** — `SetDiscordRpcEnabled`/`GetDiscordRpcStatus`/`GetDiscordRpcPreview` drive a local-only IPC client (AF_UNIX sockets on Linux, named pipes on Windows — never a network socket) with zero account identifiers in the payload, the built-in Application ID (no configuration needed), a live "what others will see" preview, silent degradation when Discord is closed, and auto-resync when Discord restarts.
- **CurseForge modpack import (`.zip`)** — `ScanCurseForgePackZip`/`ImportCurseForgePackZip` read `manifest.json` (fallback: official-launcher `modlist.html`, which is honestly reported as manual-install-only), resolve each `projectID/fileID` through the CurseForge API (exact-file lookup, never a substitute build; stale files surface as errors), download with SHA-1 verification, and extract `overrides/` through the shared credential blocklist with zip-slip containment. Unresolvable files abort required downloads loudly and are listed for manual install — never silently skipped.
- **File integrity** — `Check/Repair Instance Files` verifies the Mojang cache (version JSON, client jar, libraries, natives, asset index, asset objects) against official SHA-1 and re-downloads only the broken entries via the atomic downloader; loader-side libraries and mods keep their own self-heal and are outside this scope.
- **Optimization set** — curated per-loader list (Fabric: sodium/lithium/ferrite-core/modernfix; Quilt: sodium/ferrite-core; Forge/NeoForge: oculus/ferrite-core/modernfix). Entries are resolved live against the instance game version; anything unvetted (e.g. Forge sodium ports, which are not on Modrinth) is intentionally omitted rather than guessed. Aikar flags follow the canonical G1GC set (aikar.co/mcflags); the preset never mixes with ZGC and never overrides -Xms/-Xmx (instance RAM fields own those).

---

## Roadmap

- **v0.1.3** — in-app update panel (check / apply / restart from the UI; IPC is already in place).
- **v0.2.0** — Fabric, Quilt, and NeoForge provisioning and launch.
- **v0.3.0** — CurseForge & Modrinth integration, sidecar key resolution.
- **v0.4.0** — Contextual mod management, deterministic version selection, sidecar manifest & reconcile.
- **v0.5.0** — CurseForge resilience, SQLite content cache, startup update modal & honest 7-state badge.
- **v0.6.0** — Modpack Round-trip (.mrpack), Mod Version Histories & Changelogs, Temurin update detection & runtime hygiene.
- **v0.7.0** — Content around instance: Modrinth resource packs & shaders, screenshots gallery with clipboard integration, real-time log streaming console, 1-click import (.minecraft & Prism/MultiMC), instance groups & favorites, avatar presets, portable Windows zip.
- **Later** — beta update channel, opt-in telemetry, macOS.

---

## Why Not: Security, Legal Integrity & Official APIs

Nord Launcher intentionally omits third-party authentication services (such as Ely.by or custom authlib-injector servers) and unlicensed account bypasses. This architectural and product decision is guided by three principles:

1. **Session & Credential Security**: Third-party authlib-injector endpoints intercept Minecraft authentication handshakes and session tokens. Pointing authentication traffic to unverified external servers introduces risk of session hijacking, man-in-the-middle exploits, and credential theft. Nord Launcher enforces direct, official Microsoft OAuth2 with PKCE, storing refresh tokens exclusively in the native OS Credential Manager (Windows Credential Manager / Linux Secret Service) with zero telemetry and zero custodial servers.
2. **Legal Integrity & EULA Compliance**: Nord Launcher complies with the Minecraft End User License Agreement (EULA), Terms of Service, and Microsoft Commercial Usage Guidelines. Maintaining strict separation from unofficial authentication bypasses ensures sustainable distribution, clear copyright compliance, and trust from mod authors and platform maintainers.
3. **Local-Only Side Channels**: Auxiliary integrations never leave the machine. Discord Rich Presence talks to the locally running Discord client over its named pipe / AF_UNIX socket only (no network sockets, no proxying, no identifiers beyond the two display strings you can preview in Settings), and world datapack toggling uses physical moves into a sibling `datapacks-disabled/` directory instead of in-place renames, so vanilla game data files are never mutated.
4. **Data Integrity & Non-Custodial Storage**: All configuration, instance states, and credentials remain 100% local on the user's machine. Nord Launcher never operates intermediary proxies or user databases.

---

## License

Nord Launcher is licensed under the [GNU General Public License v3.0 (GPLv3)](LICENSE).

Minecraft is a trademark of Mojang Studios. Nord Launcher is an independent project, not affiliated with or endorsed by Mojang or Microsoft.
