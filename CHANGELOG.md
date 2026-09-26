# Changelog

All notable changes to Nord Launcher are documented in this file.
The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

## [0.7.0] - 2026-09-26

### Added
- Modrinth resource pack and shader pack catalog browsing and 1-click installation to instance folders with .zip and .jar detection (Feature A).
- Instance screenshot gallery with local thumbnail rendering, native clipboard copy via ClipboardItem with PNG blob, and folder opener fallback (Feature C).
- Real-time game console modal featuring non-blocking 5000-line ring buffer streaming, ANSI color support, log level filtering, search, and save-to-file (Feature D'1).
- Instance groups and favorites: star toggle pinning favorites to top, group filter bar, and persistent SQLite schema migration 00007 (Feature D'3).
- 1-click instance migration scanner and importer from official .minecraft and Prism/MultiMC directories with strict credential stripping (Feature D'4a).
- 8 curated procedural SVG avatar presets with random dice picker for local offline profiles (UX1).
- Standalone Windows portable distribution package (nord-launcher-v0.7.0-windows-x64-portable.zip) including adjacent cf.key sidecar.
- Documentation covering security, legal integrity, official APIs, and Wails binding reflection policies in README.md.

### Fixed
- Portable sidecar key resolution honoring executable-adjacent ./cf.key and user config directories.
- Dynamic manifest Java version resolution supporting Java 16 (non-LTS, required by 1.17–1.17.1).
- Windows explorer file revealing with space handling in instance directory paths via SysProcAttr.CmdLine.
- Screenshot gallery folder opener targeting the instance screenshots folder with auto-creation on demand.

## [0.6.1] - 2026-09-25

### Added
- Manifest-driven Java recommendation chip in Instance Settings with 1-click install/select action (F2, H3).
- Expanded LTS Java matrix supporting Java 25 for Minecraft 26.1+, Adoptium 25 download provisioner, and Java Manager offer list {25, 21, 17, 11, 8}.
- Platform native folder opener (OpenPath) integrating native file managers (explorer on Windows, open on macOS, xdg-open on Linux) across MrPackExportModal, InstanceSettingsModal, and InstanceCard (F3).
- Prominent manual update check button, last checked timestamp tracking, and clear status indicators in UpdatePanel (F4, H4).

### Fixed
- Atomic mod update workflow with immediate target file replacement and reliable deletion of older .jar versions (F1, H1).
- Self-healing duplicate active mod .jar files during instance disk reconciliation by disabling stale duplicates (F1, H1).
- Fixed phantom update loop in CheckModUpdates by ensuring target version comparison ignores identical versions (F1, H2).

### Changed
- Bumped application version to v0.6.1 across desktop runtime, IPC bindings, netutil user-agent, and frontend (H5).

## [0.6.0] - 2026-09-24

### Added
- Modpack round-trip support for Modrinth (.mrpack) format.
- Modpack import planner with SHA-1/SHA-512 checksum validation and idempotent download resuming.
- Modpack export pipeline with instance bundling, config packaging, and override manifest creation.
- Mod version history browsing with full markdown changelog viewer in ModCatalog.
- Adoptium Temurin Java runtime update detection and one-click upgrade workflow.
- Java runtime hygiene with unassigned runtime detection and bulk cleanup.
- Safeguards preventing Java runtime deletion while associated instances are active.
- Frontend modals for .mrpack import (plan inspection, progress, error banner) and export.

### Changed
- Modrinth API client enhanced to parse changelogs, dependencies, and file metadata.
- Wails IPC bindings extended with 7 new methods for modpacks and Java lifecycle management.

### Security
- Hardened zip archive extractor against path traversal attacks (zip-slip vulnerability).

## [0.5.0] - 2026-09-20

### Added
- Unified netutil HTTP client with authentic User-Agent and JSON headers.
- Token-bucket rate limiter and Retry-After handling for CurseForge requests.
- Persistent SQLite content cache for mod search and file metadata.
- Reactive mod sorting by catalog source (Modrinth and CurseForge).
- One-click fallback search button between CurseForge and Modrinth.
- In-repo changelog extraction for release update manifests.
- Safe markdown-lite rendering and Ed25519 signature verification badge in update panel.
- Startup update notification modal with 24-hour snooze option.
- Honest 7-state update badge machine with offline safety.
- Mod updates checker and diagnostic reporting pack.

### Changed
- Improved error feedback on rate limits, invalid API keys, and connection errors.
- Modernized update check pacing to prevent server saturation.

### Fixed
- Prevented silent fallback to downloads sorting on CurseForge.
- Fixed unhandled HTTP 401/429 response handling in content clients.

## [0.4.0] - 2026-09-20

### Added
- Mod catalog integration for Modrinth and CurseForge.
- Multi-version mod selector with release type filters.
- Automatic dependency resolution and download manager.
- Mod toggle (.disabled) and deletion workflows.

## [0.3.0] - 2026-09-20

### Added
- Java runtime automatic provisioner with Adoptium Temurin API.
- Game crash log parsing and diagnostic modal.
- Native process launch isolation.

## [0.2.2] - 2026-09-12

### Added
- Ed25519 cryptographic payload signing and signature verification.
- Auto-updater client with atomic binary replacement.

## [0.1.0] - 2026-09-01

### Added
- Initial release of Nord Launcher core.
- Minecraft instance creation, launch pipeline, and account management.
