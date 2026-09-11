# ADR-0013: Transition from Minisign to Go Standard Library Ed25519 for Update Signatures

## Status: Accepted (Supersedes signature format in ADR-0005)

## Context
ADR-0005 originally proposed using Minisign for signing update manifests. However, Minisign in Go either requires third-party packages (e.g., `aead/minisign`) or external binary toolchains. For an auto-updater executing remote code, minimizing third-party supply-chain dependencies is a top-tier security imperative.

## Decision
Adopt Go's standard library `crypto/ed25519` directly for update verification:
1. **Zero Supply-Chain Dependencies**: `crypto/ed25519` is part of the vetted Go standard library, eliminating third-party security vulnerabilities in the update verification path.
2. **Cryptographic Equivalence**: Minisign itself uses Ed25519 (Ed25519-SHA-512) under the hood. Using Ed25519 raw signatures encoded in base64 in the JSON update manifest retains equivalent cryptographic strength.
3. **Binary Footprint & Performance**: Zero additional binary weight and constant-time signature verification.
4. **Strict Rejection**: Updates with invalid signatures (`ErrSignatureInvalid`) or mismatched SHA-256 hashes (`ErrChecksumMismatch`) are strictly aborted without executing filesystem swaps.
