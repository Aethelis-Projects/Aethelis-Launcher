# ADR-0005: Dual Update Channels (Stable & Beta) with Minisign
## Status: Accepted
## Context
Need isolated staging of risky features before public releases.
## Decision
Support stable (default) and eta channels. Updater validates signed manifests (manifest-stable.json and manifest-beta.json) using embedded Minisign public keys. Unsigned updates are strictly rejected.
