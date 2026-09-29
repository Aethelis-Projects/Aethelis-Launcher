//go:build ignore

// verify_manifest_sig.go closes the loop the owner asked for in v0.7.2 round 5:
// the SAME verification code path the shipped client uses
// (internal/core/updater.VerifyPayload + the package's public keys) validates
// a freshly generated update manifest and every artifact it lists. Run it in
// CI on both tag releases and dispatch rehearsals, so a wrong key, a wrong
// hash or a broken manifest format can never first appear in the wild.
//
//	go run ./scripts/verify_manifest_sig.go --dist release-assets --channel stable [--staging-pubkey]
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nord-launcher/launcher/internal/core/updater"
)

func main() {
	dist := flag.String("dist", "dist", "directory holding manifest-<channel>.json and the artifacts")
	channel := flag.String("channel", "stable", "release channel")
	staging := flag.Bool("staging-pubkey", false, "verify against the repo staging key's public half (rehearsal signing)")
	privPEM := flag.String("privkey-pem", "", "optional: derive the expected public key from this private key PEM instead")
	flag.Parse()

	var pub ed25519.PublicKey
	switch {
	case *privPEM != "":
		raw, err := os.ReadFile(*privPEM)
		if err != nil {
			fail("read -privkey-pem: %v", err)
		}
		blk, _ := pem.Decode(raw)
		if blk == nil {
			fail("%s is not a PEM file", *privPEM)
		}
		key, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
		if err != nil {
			fail("parse pkcs8: %v", err)
		}
		pk, ok := key.(ed25519.PrivateKey)
		if !ok {
			fail("PEM is not an ed25519 private key")
		}
		pub = pk.Public().(ed25519.PublicKey)
	case *staging:
		pub = updater.StagingPublicKey()
	default:
		pub = updater.GetDefaultPublicKey()
	}

	mpath := filepath.Join(*dist, fmt.Sprintf("manifest-%s.json", *channel))
	data, err := os.ReadFile(mpath)
	if err != nil {
		fail("read manifest: %v", err)
	}
	var m updater.UpdateManifest
	if err := json.Unmarshal(data, &m); err != nil {
		fail("decode manifest %s: %v", mpath, err)
	}
	if m.Version == "" {
		fail("manifest has no version")
	}
	if len(m.Platforms) == 0 {
		fail("manifest lists no platforms")
	}
	checked := 0
	for platform, asset := range m.Platforms {
		name := filepath.Base(asset.URL)
		path := filepath.Join(*dist, name)
		payload, err := os.ReadFile(path)
		if err != nil {
			fail("%s: artifact %q missing from %s: %v", platform, name, *dist, err)
		}
		sum := sha256.Sum256(payload)
		if !equalFoldHex(hex.EncodeToString(sum[:]), asset.SHA256) {
			fail("%s: sha256 mismatch for %s (manifest %s, actual %s)", platform, name, asset.SHA256, hex.EncodeToString(sum[:]))
		}
		if asset.Signature == "" {
			fail("%s: %s carries no signature", platform, name)
		}
		sig, err := base64.StdEncoding.DecodeString(asset.Signature)
		if err != nil {
			fail("%s: decode signature: %v", platform, err)
		}
		if !updater.VerifyPayload(pub, payload, sig) {
			fail("%s: ED25519 SIGNATURE REJECTED by updater.VerifyPayload (wrong key or tampered bytes)", platform)
		}
		fmt.Printf("OK %s: %s sha256=%s... signature valid against the client public key\n", platform, name, asset.SHA256[:12])
		checked++
	}
	if asset, ok := m.Platforms["windows-amd64"]; ok && asset.Size > 0 {
		payload, err := os.ReadFile(filepath.Join(*dist, filepath.Base(asset.URL)))
		if err == nil && int64(len(payload)) != asset.Size {
			fail("windows-amd64: manifest size %d != actual %d", asset.Size, len(payload))
		}
	}
	fmt.Printf("PASS: manifest %s verified (%d platform assets signed+hashed with the shipped updater's code path)\n", filepath.Base(mpath), checked)
}

func equalFoldHex(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	hex := func(c byte) byte {
		switch {
		case c >= '0' && c <= '9':
			return c
		case c >= 'a' && c <= 'z':
			return c - 32
		}
		return c
	}
	for i := range a {
		if hex(a[i]) != hex(b[i]) {
			return false
		}
	}
	return true
}

func fail(f string, args ...any) {
	fmt.Fprintf(os.Stderr, "VERIFY FAILED: "+f+"\n", args...)
	os.Exit(1)
}
