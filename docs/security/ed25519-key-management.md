# Ed25519 Update Signing & Key Management Specification

This specification documents the cryptographic design, key lifecycle, verification guarantees, and rotation protocols governing the Nord Launcher auto-updater (`internal/core/updater`).

---

## 1. Cryptographic Primitive & Architecture

- **Algorithm**: Pure-Go Ed25519 (RFC 8032 via Go standard library `crypto/ed25519`).
- **Hash Function**: SHA-256 (`crypto/sha256`).
- **Signature Payload**: Binary payload is hashed via SHA-256, and the resulting hex hash is cryptographically signed by the release authority:
  $$\sigma = \text{Sign}_{\text{Ed25519}}(\text{PrivateKey}, \text{SHA256}(\text{BinaryPayload}))$$
- **Manifest Transport**: Update manifests are distributed over HTTPS via GitHub Releases (`manifest-stable.json` and `manifest-beta.json`).

---

## 2. Key Architecture

| Key | Format | Location | Purpose |
|---|---|---|---|
| **Public Key** | 32-byte Ed25519 raw public key (`hex`) | Hardcoded in `internal/core/updater/updater.go` (`DefaultPublicKeyHex`) | Embedded in every client binary to verify authenticity of downloaded update manifests. |
| **Private Key (Seed)** | 32-byte Ed25519 private seed (`hex`) | GitHub Actions Secret: `ED25519_PRIVATE_KEY` | Never committed to source control; accessible only to automated CI release workflows. |

### Current Public Key
```
a7dd59ba003395467e78b7bc21c3d329bc3e1bab778180624943805b0111f48a
```

---

## 3. Two-Stage Client Verification Flow

When the launcher evaluates a candidate update:

1. **Manifest Authenticity Gate**:
   - Client fetches `manifest-{channel}.json` from GitHub Releases.
   - For the client's current platform (`runtime.GOOS-runtime.GOARCH`), the client extracts `asset.Signature` (Base64) and `asset.SHA256`.
   - The signature is verified against the embedded public key:
     ```go
     ed25519.Verify(publicKey, []byte(asset.SHA256), signatureBytes)
     ```
   - If signature verification fails, update processing aborts immediately with `ErrSignatureInvalid`.

2. **Payload Integrity Gate**:
   - The installer or archive is downloaded to a temporary location.
   - The SHA-256 hash of the downloaded file is computed and compared against `asset.SHA256`.
   - If mismatched, the file is deleted and `ErrChecksumMismatch` is returned.

---

## 4. Key Rotation & Revocation Protocol

In the event of key compromise or routine rotation:

1. **Dual-Key Migration Release**:
   - A new keypair $(K_{\text{new}}^{\text{pub}}, K_{\text{new}}^{\text{priv}})$ is generated.
   - An intermediate launcher version is built supporting an array of trusted public keys:
     ```go
     trustedKeys := []ed25519.PublicKey{newKey, oldKey}
     ```
   - Manifests are signed with $K_{\text{old}}^{\text{priv}}$ to allow legacy clients to upgrade to the dual-key release.
2. **Transition**:
   - Once the fleet migrates to the dual-key client, GitHub Secrets are updated with $K_{\text{new}}^{\text{priv}}$.
   - Subsequent releases discontinue the old key.
3. **Emergency Revocation**:
   - If an immediate revocation is required, DNS/Cloudflare edge proxy can block manifest distribution while an expedited hotfix is issued.
