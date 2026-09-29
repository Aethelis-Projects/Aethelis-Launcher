# Changelog

All notable changes to Nord Launcher are documented in this file.
The format is based on Keep a Changelog, and this project adheres to Semantic Versioning.

## [Unreleased]

## [0.7.2] - 2026-09-28

SemVer note: this ships new `core/` APIs and 12 IPC methods, which would make it 0.8.0 under the project's own rules; the release is numbered 0.7.2 by explicit owner decision (same deviation as 0.7.1). Kept as a record, not a rename proposal.

### Added
- Modpacks page (G5): left-nav "Модпаки" storefront over Modrinth modpack search with server-side game-version/loader/category filters, live Modrinth tag feed, per-card .mrpack version picker and one-click import via the HTTPS URL pipeline. CurseForge modpacks are honestly declared unsupported in its search API with a hand-off button to the unified import modal.
- Instance creation wizard (G10): "Создать" opens a three-step modal - name/group, Minecraft version straight from the Mojang manifest (release/snapshot switch, 6h cache) via `ListMinecraftVersions`, and loader choice (vanilla/fabric/quilt/forge/neoforge) with recommended/latest-stable preselected through `ListLoaderVersions` (official Fabric/Quilt meta, Forge promotions_slim, NeoForge GitHub tags) plus manual override; creation pins the chosen loader version via `CreateInstanceWithLoader`.
- Six IPC methods for the flows above (`ListMinecraftVersions`, `ListLoaderVersions`, `CreateInstanceWithLoader`, `ListMrPackVersions`, `ImportMrPackFromURL`, `GetMrPackURLImportStatus`) plus `ListProjectTags` for the tag feed; Wails registry 59 -> 72 (73 after the round-6 `OpenExternal` addition below) with generated TS bindings re-run.
- `OpenExternal` IPC method (73rd in the bound registry): routes a link or a dropped http(s) URL to the system browser and nothing else - the only outbound URL surface reachable from the webview, gated by the same `validateExternalURL` check the auth flow uses (http/https + non-empty host).
- Root `VERSION` file as the single release identity, enforced by `scripts/check_version_consistency.js` (CI on every push, release job against the tag) - see Fixed.
- `AutoUpdater.IsDevVersion`: builds carrying the dev sentinel version (`0.0.0-dev`, the no-ldflags fallback) never contact the update manifest and never offer an update - dev builds would otherwise "upgrade" themselves down to the latest release on every start.

### Changed
- UI consolidation (G1/G2): the home cockpit no longer duplicates the Java runtime editor (path input + "Среда выполнения Java" plate removed; settings remain the single owner), and Instance Settings now carry one "Производительность и Java" tab combining the Java runtime, RAM presets, JVM flags and performance presets (the standalone "Оптимизация" tab with the curated mod catalog is gone). The standalone "Рекомендуемая Java" plate was replaced by an inline "install missing recommended runtime" affordance.
- Discord Rich Presence settings moved out of the global Settings page into the unified instance tab (single toggle); the Discord Application ID is now a built-in binary constant, so `SetDiscordAppID` and the `discord_app_id` setting were removed.
- Java manager usage attribution is honest now: path comparison is normalized (slashes/case) and instances running on auto-detected Java are attributed to the runtime the launcher would actually pick (including the Java 8 -> 11 fallback), so "used by" no longer shows empty for active runtimes.
- The mod catalog left the instance settings: the "Моды" tab is now purely the installed-mods manager, while the catalog (mods / resource packs / shaders / datapacks, provider switch, filters) lives on its own left-nav page; opening it keeps the active instance as the install context (installs still land in instance folders only, per the v0.4.0 rule).
- Provider-honest catalog filters (G4): server-side game version and loader filters on both Modrinth and CurseForge; Modrinth tags come from the live `/v2/tag/category` feed (24h cache, narrowed by project type), while CurseForge uses the mirrored UISP table with documented client-side name filtering and an honest "CurseForge не отдаёт датапаки/modpacks" gate (search endpoint is never queried for those). CF search gained the resource pack (class 12) and shader (class 65536) providers.
- One import entry point (G6): the header keeps a single "Импорт" button opening the unified modal, which now carries a fourth ".mrpack" tab (default); the standalone "Импорт .mrpack" button and its separate modal mount were removed while the full mrpack flow stays intact inside the panel. A fresh modal open always lands on the requested entry tab (Modpacks hand-off opens the CurseForge tab, not the previously-left one). The misleading "Поиск модов и сборок" field on the home header became an honest "Фильтр сборок".
- Wails method registry is enforced bidirectionally: forward `wailsBindings()` parity plus a reverse `reflect` exhaustive check with a zero allowlist - every exported method on the bound receiver must be declared, nothing more (README metrics line and registry size updated to 73, including `OpenExternal`).
- Codegen is now a gofmt fixed point: the generator canonicalizes its Go output through `go/format`, so "run generator" == "gofmt" == the committed artifact (CI parity step and local formatting no longer oscillate between two states).
- `frontend/package-lock.json` (dev-only, created with `--no-package-lock`) is untracked and gitignored; the repository default branch is `master` (no `main` exists - workflow docs no longer refer to pushing a "main").

### Fixed (pre-tag review, owner directive)
- **Host-only wiring left the bound struct.** Wails beta.20 binds every exported method of the receiver (its binding generator has no ignore directive), so the adapter's wiring setters (22 `Set*` + the `RecordCrash` delegate; the never-bound `wails.Host` facade exposes exactly those 23 methods today, and the reverse registry check derives its prohibition list from `reflect` on `Host` so it cannot drift) - `SetGameManifestURL`, `SetLoaderResolver`, `SetHTTPClient`, `SetDB`, `SetUpdater`, `SetFilePicker`, service pointers - were reachable IPC names from the webview; a compromised page (user-authored Modrinth descriptions are an XSS surface) could repoint the Mojang version manifest and get attacker-controlled JARs. They are unexported now and live on the never-bound `wails.Host` facade used by `cmd/launcher`, `cmd/e2e` and tests; the reverse registry check asserts their absence.
- **`.mrpack` URL import trusts Go, not the webview.** With `project_slug` + `version_id` the adapter re-fetches the file record from the Modrinth API itself and uses that URL/hashes/size; client-supplied `sha1`/`size` are treated as hints that must agree (a mismatch aborts before any dial). The url+hash fallback mode remains only for callers without identifiers and is pinned to https on `cdn.modrinth.com`, checked on the initial URL *and* every redirect hop. Downloads stream through the core `downloader.Dispatcher` (resume, atomic rename) with SHA-1 and - where Modrinth advertises it - SHA-512 verification (new `ExpectedSHA512` in the task), a post-download size cross-check, and the temp file staged under a fresh `0700` directory with `O_EXCL 0600` (the predictable per-user temp path was symlink-attackable). `ListMrPackVersions` drops files above the mrpack size budget; the UI sends only slug + version id.
- **Discord Rich Presence on Windows**: the RPC client dials the real Windows transport - named pipes `\\\.\pipe\discord-ipc-{0..9}` through `go-winio` with cancellation-aware connect - instead of pretending AF_UNIX exists there; platform dialers are injected (`platformDialer()`) and the Windows tests drive a live `winio.ListenPipe` listener, so the job cannot go green without exercising named pipes. Linux behavior is unchanged (XDG/CACHE_DIR/HOME candidate order).
- **Version single source**: `VERSION` is the release identity; CI enforces it against `frontend/package.json`, `wails.json` `version`/`info.productVersion` (drifted: 0.7.1 vs 0.6.1) and the CHANGELOG section, and rehearses the release job's exact tag check (positive `v$(cat VERSION)` + negative `v9.9.9` + changelog grep) on every push; release runs assert `tag == VERSION` and builds inject `-X main.version=$(cat VERSION)`; the stale in-code fallback became `0.0.0-dev`. Tags are cut on the merged `master` commit (see docs/RELEASE.md), never on a branch head.
- **Schema over casts**: `modpack` added to the `ProjectType` JSON-schema enum with bindings regenerated, removing the `project_type: "modpack" as never` cast from the Modpacks page.
- README metrics now report numbers measured for this release (bundle 81 KB gzip main chunk, binaries 17.0/17.4 MB stripped, coverage 80.4%, IPC ~109 ns, core init 9 ms); the stale "release v0.1.2" table is gone, GUI cold-start/idle-RAM moved to the hardware acceptance checklist.

### Security

- **Rehearsal signing material is compiled out of the client.** The dev/test Ed25519 seed is published in this repository, so the safety of rehearsal-signed artifacts rests on the shipped client trusting exactly one key. The seed and its parser moved from `internal/core/updater` (client-side) into a new `internal/releasetool` package that no client dependency imports, and the client package now carries no key-derivation or key-override path at all. Three independent checks pin this: `internal/releasetool/isolation_test.go` (staging signatures are rejected by the production trust root; `go list -deps` for both the updater package and `cmd/launcher` must not contain `internal/releasetool`, with a non-empty-graph requirement so the check cannot pass vacuously), `internal/core/updater` source guard (no seed, no `NewKeyFromSeed`, no `ED25519_PRIVATE_KEY`, no public-key setter), and a CI step that greps the built binaries for the seed - including a positive control on the generator binary so a broken grep cannot fake a pass. `cmd/e2e` stage 16 runs the real client update path over a real staging-signed manifest: `DownloadAndApply` must fail with `ErrSignatureInvalid`, the executable must be untouched, no `.new` may be left staged, and a control pass with the staging public half must succeed (otherwise the fixture, not the trust root, would be what "passed").
- **Production signing key is verified before it is used.** `scripts/check_signing_key.go` derives the public half of `ED25519_PRIVATE_KEY` with the very parser the generator uses and compares it to `DefaultPublicKeyHex` pinned in the client, printing only public material. It runs in every release job: `-selftest` always (format contract + staging/production isolation, needs no secret), the secret check whenever the secret exists. An empty secret fails a tag run outright - there is no staging fallback on tags, and dispatch can never acquire production authority because the mode is derived from `github.ref`, not from `inputs.tag`. Manifest verification after signing and before publication uses the production client key on tag runs, and a rehearsal additionally asserts that the production key rejects it.
 `WailsAdapter.SetVersion(string)` was a public setter on the bound adapter since v0.1.6 (introduced in f7f1d4a) and therefore reachable from the webview in every released version up to and including v0.7.1: a compromised renderer could have rewritten the advertised version string (User-Agent formatting, update-semantics inputs). It is now an unexported delegate on the non-bound `wails.Host`. The other historical string setters were not JS-callable because their argument types are not marshallable by the v3 beta codegen (`*http.Client`, `[]string`, function types) — they were still moved to the facade for symmetry. No other released version gains a callable setter; no external report existed (found in our own v0.7.2 audit).
- **Strict CSP on the asset server** (`wails.CSPMiddleware`, wired into `application.Options.Assets.Middleware`): `default-src 'self'`, `script-src 'self'` (no inline, no eval), `object-src 'none'`, `frame-ancestors 'none'`. Combined with the finding that no HTML sink exists in `frontend/src` (zero `innerHTML`/`dangerouslySetInnerHTML`; markdown renders through the `MarkdownLite` text-node component) this closes the *class*: future IPC methods returning attacker-influenced strings can no longer become script execution.
- **mrpack import request type no longer carries a URL at all** — `ImportMrPackURLRequest` is identifiers-only (`instance_name`+`project_slug`+`version_id`, optional `sha1`/`size` cross-checks); the fallback branch that accepted a pasted URL was removed because no UI control feeds it (ModpacksView filters are version+loader only). The webview can neither name nor poison an origin.binaries 17.0/17.4 MB stripped, coverage 80.4%, IPC ~109 ns, core init 9 ms); the stale "release v0.1.2" table is gone, GUI cold-start/idle-RAM moved to the hardware acceptance checklist.

- **Round-4 hardening follow-ups.** CSP `img-src` made explicit (`'self' data: blob:` + cdn.modrinth.com/*.modrinth.com/mediafiles.forgecdn.net/*.forgecdn.net) - a bare `default-src 'self'` would have blanked every remote mod icon; `style-src 'unsafe-inline'` kept for component style attributes. A byte-identical `<meta http-equiv>` twin of the header now ships in the built HTML (unit-tested for drift) so webview backends that drop asset-server headers still enforce the policy. Browser-launch paths (`openBrowserCrossPlatform`) reject any non-http(s) scheme at the choke point (`validateExternalURL`) - the only current caller is the MS-auth flow, but the gate is position-independent for future OpenURL surfaces. CI gained an `actionlint` step (a workflow that fails to parse schedules ZERO jobs - observed live this cycle), release dispatch rehearsals resolve their version from the checked-out `VERSION` (never a branch name), the publish checkout uses `fetch-depth: 0` so the placement gate's `merge-base` sees real history, and Linux CI/release verified building the README-documented default backend (GTK4/WebKitGTK 6.0, no build tags).

## [0.7.1] - 2026-09-28

### Added
- Per-world datapack management: catalog tab pinned to Modrinth, install into selected worlds (or unassigned instance storage), enable/disable by physical move between `datapacks/` and `datapacks-disabled/`, delete, world picker with last-played and pack counts (Feature E).
- Performance preset API: `GetPerformancePreset` (physical-RAM heuristic clamped to 1-4 GB + canonical Aikar G1GC flag set) and `ListOptimizationMods` (curated per-loader optimization catalog resolved against the instance version); Optimization tab in Instance Settings with apply/reset and one-click curated installs (Feature B).
- File integrity check and repair: `CheckInstanceFiles`/`RepairInstanceFiles` verify the Mojang cache (version JSON, client jar, libraries, natives, asset index and objects) against official SHA-1 and re-download only broken or missing entries through the atomic downloader; Files tab in Instance Settings reports checked/problem/repaired counts with a capped findings list (Feature D'2).
- CurseForge modpack `.zip` import (D'4b): scan/commit pipeline over `manifest.json` with CF-API file resolution (exact fileID, SHA-1 verified downloads), `overrides/` extraction behind the shared credential blocklist and zip-slip guards, `modlist.html` reported as manual-install-only, and a third tab in the import modal: honest plan card with per-file unresolved reasons, a result view listing failed downloads, manual-install backlog and blocked credentials, and security audit tests (case-insensitive blocklist, nested `..` rejection, mid-import tamper re-check) plus offline e2e STEP 14 covering the full scan-import-verify contract.
- Discord Rich Presence (D'5): local-IPC-only client with handshake, capped-backoff reconnect and in-flight activity resync; `SetActivity` payload is details/state/timestamps with zero account identifiers; opt-in default off, silent degradation without Discord, graceful CLOSE on toggle-off (<=5s); Settings card with live "what others will see" preview and BYO Application ID field; `MonitorProcess` gained an `onExit` observer so presence clears exactly on game exit; offline fake-Discord lifecycle tests and an adapter no-repo status test.
- Wails method registry expanded to 61 methods (`SetDiscordRpcEnabled`, `GetDiscordRpcStatus`, `GetDiscordRpcPreview`, `SetDiscordAppID` added) (`EnsureInstanceDir`, `GetPerformancePreset`, `ListOptimizationMods`, `CheckInstanceFiles`, `RepairInstanceFiles`, `ScanCurseForgePackZip`, `ImportCurseForgePackZip`, datapack/world management).

### Fixed
- `OpenPath` no longer mutates the filesystem: directory creation moved to explicit `EnsureInstanceDir` with strict allowlist and traversal guards (T1).
- Java 16 wording corrected from "LTS" to "required by 1.17-1.17.1" across matrix, UI and docs (T2).

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
