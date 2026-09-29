# Release runbook

## Before tagging

1. All CI runs green on the final commit of the feature branch; PR reviewed and accepted.
2. `VERSION`, `frontend/package.json`, `wails.json` (`version` + `info.productVersion`) and the `CHANGELOG.md` section are consistent — enforced by CI (`node scripts/check_version_consistency.js`), and rehearsed against the release-only tag check on every push. To see it locally: `node scripts/check_version_consistency.js v$(cat VERSION)`.
3. The release-gate path (`tag == VERSION`, CHANGELOG section grep) never fires for the first time on a release: CI runs the same commands with the matching tag plus a deliberate mismatch, so both the accept and the reject branch are exercised pre-tag.

## Tagging

4. Merge the PR, then **tag the merged `master` commit** — never the branch head. With squash (or rebase) merges the merge commit has a different SHA than the branch tip; tagging the branch head would point the release at a commit that is not on `master`, and the release job builds from the tag.
5. `git fetch origin && git checkout master && git pull --ff-only && git tag -s vX.Y.Z -m "vX.Y.Z" && git push origin master vX.Y.Z` — pushing `master` together with the tag keeps the default branch synchronized (the repository has no `main`; `master` is the default branch).
6. The release workflow runs from the tag: builds with `-X main.version=$GITHUB_REF_NAME`, verifies the CHANGELOG entry, asserts `tag == VERSION`, signs the update manifest (Ed25519) and publishes the GitHub Release with assets.

## Live acceptance (per release directive)

Headless CI cannot cover: GUI rendering under WebKitGTK (Linux) and WebView2 (Windows), native dialogs, a real Discord client, and real game launches — Fabric, Quilt, Forge and NeoForge (if Forge does not start on a MC version, mark that loader experimental in the wizard instead of shipping a silent failure), an `.mrpack` import from real Modrinth including network-abort mid-download (temp dir must be cleaned up), and datapack toggling on a real world.

Checklist: `docs/v0.7.2-ACCEPTANCE-TESTS.md`.
