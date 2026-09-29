package releasetool_test

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nord-launcher/launcher/internal/core/updater"
	"github.com/nord-launcher/launcher/internal/releasetool"
)

// v0.7.2 round-6 (owner p1): the staging seed is published in this repository,
// so the entire security argument is "the client trusts one key and nothing
// else". This test pins the argument in both directions: the circle closes for
// the staging key, and a production client rejects it.
func TestStagingKeyIsolatedFromProduction(t *testing.T) {
	priv, err := releasetool.StagingPrivateKey()
	if err != nil {
		t.Fatalf("staging key constant unusable: %v", err)
	}
	stagingPub, err := releasetool.StagingPublicKey()
	if err != nil {
		t.Fatalf("staging public key: %v", err)
	}
	if want := priv.Public().(ed25519.PublicKey); !stagingPub.Equal(want) {
		t.Fatal("StagingPublicKey() does not match the seed's own public half")
	}
	prod := updater.GetDefaultPublicKey()
	if stagingPub.Equal(ed25519.PublicKey(prod)) {
		t.Fatal("staging key must never equal the production key - a rehearsal artifact would become production-valid")
	}

	payload := []byte("nord-launcher-rehearsal-payload")
	sig := updater.SignPayload(priv, payload)
	if !updater.VerifyPayload(stagingPub, payload, sig) {
		t.Fatal("signer and verifier disagree on the staging key (the sign->verify circle is broken)")
	}
	if updater.VerifyPayload(prod, payload, sig) {
		t.Fatal("a staging signature was accepted by the production trust root")
	}
	if hex.EncodeToString(prod) == updater.DefaultPublicKeyHex {
		t.Log("production trust root unchanged")
	} else {
		t.Fatal("DefaultPublicKeyHex does not decode to GetDefaultPublicKey()")
	}
}

// Base64/PEM key material silently produced the round-5 panic (the generator
// only accepted a 32-byte hex seed). The parser must say exactly that, and must
// never echo the secret back - CI logs are readable by anyone with repo access.
func TestKeyMaterialParserContract(t *testing.T) {
	seed, err := hex.DecodeString(releasetool.StagingPrivateKeyHex)
	if err != nil || len(seed) != ed25519.SeedSize {
		t.Fatalf("staging seed constant is broken: len=%d err=%v", len(seed), err)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	fullHex := hex.EncodeToString(priv)

	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"32-byte seed hex", releasetool.StagingPrivateKeyHex, false},
		{"64-byte full key hex", fullHex, false},
		{"surrounding whitespace tolerated", "  " + releasetool.StagingPrivateKeyHex + "\n", false},
		{"empty", "", true},
		{"odd length", releasetool.StagingPrivateKeyHex + "a", true},
		{"base64 of the same 32 bytes", base64.StdEncoding.EncodeToString(seed), true},
		{"short hex", "aabbcc", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := releasetool.PrivateKeyFromHex(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("accepted %s, want rejection", tc.name)
				}
				if !strings.Contains(err.Error(), "hex") && !strings.Contains(err.Error(), "bytes") {
					t.Errorf("error must name the accepted formats, got: %v", err)
				}
				probe := strings.TrimSpace(tc.in)
				if len(probe) > 16 {
					probe = probe[:16]
				}
				if probe != "" && strings.Contains(err.Error(), probe) {
					t.Error("error message leaked key material")
				}
				return
			}
			if err != nil {
				t.Fatalf("rejected valid %s: %v", tc.name, err)
			}
			if len(got) != ed25519.PrivateKeySize {
				t.Fatalf("parsed key length %d", len(got))
			}
		})
	}
}

// The seed must not be linked into the client. `go list -deps ./cmd/launcher` is
// owner-requested; the updater-package graph is the always-runnable fallback
// (cmd/launcher needs CGO/webkit on some hosts, and a skip there must not hide a
// regression in the client's own trust root).
func TestClientDependencyGraphExcludesReleasetool(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatal("go toolchain not available - this check must not be silently skipped")
	}
	for _, target := range []string{
		// Import paths, not ./-relative ones: `go list` from a package test dir
		// resolves ./... against that dir and would "succeed" on the wrong graph.
		"github.com/nord-launcher/launcher/internal/core/updater",
		"github.com/nord-launcher/launcher/cmd/launcher",
	} {
		// -e, and no skip-on-error: a package that fails to load on a bare runner
		// used to downgrade this to a log line, and a skipped isolation check is
		// how the seed ends up in the client. The graph is therefore required to be
		// non-trivial, so a bogus/empty listing fails loudly instead.
		out, err := exec.Command("go", "list", "-e", "-deps", target).CombinedOutput()
		deps := string(out)
		if strings.Contains(deps, "no required module provides") || strings.Contains(deps, "cannot find module") {
			t.Fatalf("%s: dependency graph could not be resolved (fix the module cache, do not delete this check):\n%s", target, firstLine(deps))
		}
		if strings.Contains(deps, "internal/releasetool") {
			t.Fatalf("%s transitively imports internal/releasetool - the staging seed would be compiled into the shipped client", target)
		}
		for _, must := range []string{"crypto/ed25519", "github.com/nord-launcher/launcher/internal/core/updater"} {
			if !strings.Contains(deps, must) {
				t.Fatalf("%s graph lacks %s (go list err=%v) - the query returned nothing useful, so the check would be vacuous", target, must, err)
			}
		}
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Round-trip of the isolation claim against the actual release generator: a
// manifest signed with the staging key must be rejected by the real client code
// path (updater.DownloadAndApply), and the rejection must be about the trust
// root, not about a broken fixture - so the same manifest must apply cleanly
// when the client is pointed at the staging public half. cmd/e2e step 16 runs
// the identical scenario end to end; this keeps it honest in `go test`.
func TestSigningMaterialIsNotInClientPackageSources(t *testing.T) {
	// this test's package dir is internal/releasetool; the client-side packages
	// are siblings, resolved relative to the repo root.
	root := filepath.Join("..", "..")
	targets := []string{
		filepath.Join(root, "internal", "core", "updater"),
		filepath.Join(root, "internal", "adapters", "wails"),
		filepath.Join(root, "cmd", "launcher"),
	}
	for _, dir := range targets {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("cannot read %s (repo layout changed?): %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", e.Name(), err)
			}
			text := string(data)
			if strings.Contains(text, releasetool.StagingPrivateKeyHex) {
				t.Errorf("%s/%s embeds the staging seed - move it back into internal/releasetool", dir, e.Name())
			}
			if strings.Contains(text, "StagingPrivateKeyHex") {
				t.Errorf("%s/%s references staging key material at all", dir, e.Name())
			}
			if strings.Contains(text, "internal/releasetool") {
				t.Errorf("%s/%s imports internal/releasetool (client must not)", dir, e.Name())
			}
		}
	}
}
