# Changelog

All notable changes to Nord Launcher are documented in this file.
The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

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
