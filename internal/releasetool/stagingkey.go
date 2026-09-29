// Package releasetool holds the signing material used by the release tooling
// (scripts/generate_manifest.go, scripts/check_signing_key.go,
// scripts/verify_manifest_sig.go) and by tests.
//
// ISOLATION RULE (v0.7.2 round-6, owner p1): the staging seed below is a
// PRIVATE key that ships in a PUBLIC repository, so it must never be linked
// into the shipped client. Safety rests entirely on the client trusting
// updater.GetDefaultPublicKey() and nothing else - a client that could be told
// "also trust this other key" would turn the published seed into a remote code
// execution path for every installed launcher.
//
// Therefore: internal/core/updater (client-side) must not import this package,
// and no package in `go list -deps ./cmd/launcher` may import it. Both facts are
// enforced by isolation_test.go here, and CI greps the built client binary for
// the seed string (with a positive control against the generator binary, so a
// silently-broken grep cannot pass).
package releasetool

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// StagingPrivateKeyHex is the dev/test-only ed25519 seed (32 bytes hex) that
// rehearsal signing falls back to when ED25519_PRIVATE_KEY is unset.
// Generated for this repository, never used to sign a published release.
const StagingPrivateKeyHex = "88ec59f652844aded5ef72635fd0621042ffff0b75ec7c0e20185255b374f9af"

// ErrKeyFormat names the accepted encodings; kept as a value so the CLI tools
// can print the same contract the parser enforces.
var ErrKeyFormat = errors.New(
	"ed25519 key material must be a hex string of 64 chars (32-byte seed) or 128 chars (64-byte full private key); base64 and PEM are not accepted here",
)

// PrivateKeyFromHex is the ONE parser for signing key material, shared by the
// manifest generator, the preflight check and the tests. It exists because the
// first rehearsal attempt panicked on exactly this ambiguity: the repo constant
// is a 32-byte SEED while ed25519.PrivateKey is 64 bytes, so any consumer that
// assumed the latter broke. Accepting both (and saying so) keeps the failure
// mode loud and local.
func PrivateKeyFromHex(raw string) (ed25519.PrivateKey, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty key material: %w", ErrKeyFormat)
	}
	if len(trimmed)%2 != 0 {
		return nil, fmt.Errorf("odd-length hex key material (%d chars): %w", len(trimmed), ErrKeyFormat)
	}
	decoded, err := hex.DecodeString(trimmed)
	if err != nil {
		// Deliberately does not wrap err: hex.Error includes the offending byte,
		// and key material must not be echoed into CI logs.
		return nil, fmt.Errorf("key material is not hex-encoded: %w", ErrKeyFormat)
	}
	switch len(decoded) {
	case ed25519.SeedSize:
		return ed25519.NewKeyFromSeed(decoded), nil
	case ed25519.PrivateKeySize:
		return ed25519.PrivateKey(decoded), nil
	default:
		return nil, fmt.Errorf("decoded key material is %d bytes: %w", len(decoded), ErrKeyFormat)
	}
}

// PublicKeyHexFromKeyMaterial derives the public half of hex key material,
// without ever printing the private part. Used by the release preflight.
func PublicKeyHexFromKeyMaterial(raw string) (string, error) {
	priv, err := PrivateKeyFromHex(raw)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(priv.Public().(ed25519.PublicKey)), nil
}

// StagingPublicKey is the public half of StagingPrivateKeyHex, used only to
// verify rehearsal artifacts. A release client must reject it.
func StagingPublicKey() (ed25519.PublicKey, error) {
	priv, err := PrivateKeyFromHex(StagingPrivateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("staging key constant is unusable: %w", err)
	}
	return priv.Public().(ed25519.PublicKey), nil
}

// StagingPrivateKey returns the rehearsal signing key for tooling and tests.
func StagingPrivateKey() (ed25519.PrivateKey, error) {
	return PrivateKeyFromHex(StagingPrivateKeyHex)
}
