# ADR-0005: Dual Update Channels (Stable & Beta) with Minisign
## Status: Superseded by ADR-0013
## Context
Need isolated staging of risky features before public releases.
## Decision
Support stable (default) and beta channels. Updater validates signed manifests (manifest-stable.json and manifest-beta.json).
## Notes
The signing primitive was superseded by ADR-0013 (transition from external minisign dependencies to Go standard library crypto/ed25519). The dual-channel architecture (stable/beta manifests) remains active.
