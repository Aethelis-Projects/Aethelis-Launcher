//go:build ignore

// check_signing_key.go is the release preflight the owner asked for in v0.7.2
// round 6: a dispatch rehearsal signs with the repo staging key, so nothing in
// CI ever proves that ED25519_PRIVATE_KEY (a) parses and (b) derives to the
// public key pinned inside the shipped client. A mismatch is unrecoverable
// after the tag exists - every installed client would reject the update - so it
// must fail before signing, not after publishing.
//
//	# selftest: exercise the parser + the isolation claim, needs no secret
//	go run ./scripts/check_signing_key.go -selftest
//
//	# preflight: read the secret from the environment, print only public halves
//	go run ./scripts/check_signing_key.go -env ED25519_PRIVATE_KEY
//
// Exit codes: 0 ok, 1 bad format or key mismatch, 3 secret not configured.
// The private material is never printed, not even partially: parse errors are
// replaced with the format contract (see releasetool.PrivateKeyFromHex).
package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nord-launcher/launcher/internal/core/updater"
	"github.com/nord-launcher/launcher/internal/releasetool"
)

func main() {
	var (
		envName = flag.String("env", "ED25519_PRIVATE_KEY", "environment variable holding the production signing key material")
		selfTag = flag.Bool("selftest", false, "verify the parser and the staging/production split without any secret")
	)
	flag.Parse()

	prodPubHex := updater.DefaultPublicKeyHex

	if *selfTag {
		// Same parser the generator uses, applied to the staging constant. This
		// runs on every dry-run, so a format regression (the round-5 panic was
		// exactly "seed where a full key was assumed") is caught without a secret.
		pub, err := releasetool.StagingPublicKey()
		if err != nil {
			fmt.Printf("PREFLIGHT FAILED: staging constant unusable: %v\n", err)
			os.Exit(1)
		}
		stagingHex := hex.EncodeToString(pub)
		if stagingHex == prodPubHex {
			fmt.Println("PREFLIGHT FAILED: staging key equals the production trust root - rehearsal artifacts would be production-valid")
			os.Exit(1)
		}
		priv, err := releasetool.StagingPrivateKey()
		if err != nil || !updater.VerifyPayload(pub, []byte("preflight"), updater.SignPayload(priv, []byte("preflight"))) {
			fmt.Printf("PREFLIGHT FAILED: staging sign/verify circle broken (err=%v)\n", err)
			os.Exit(1)
		}
		fmt.Printf("PASS preflight -selftest: key parser OK, staging pubkey %s... is isolated from the client trust root %s...\n", stagingHex[:12], prodPubHex[:12])
		return
	}

	raw := strings.TrimSpace(os.Getenv(*envName))
	if raw == "" {
		fmt.Printf("%s is not set: production signing key is unavailable.\n", *envName)
		fmt.Printf("A tag release MUST fail on this; a dispatch rehearsal may continue with the repo staging key.\n")
		fmt.Printf("Format contract: %v\n", releasetool.ErrKeyFormat)
		os.Exit(3)
	}
	derived, err := releasetool.PublicKeyHexFromKeyMaterial(raw)
	if err != nil {
		fmt.Printf("PREFLIGHT FAILED: %v\n", err)
		fmt.Printf("Refusing to sign: the release would be published with a manifest no installed client can verify.\n")
		os.Exit(1)
	}
	// The secret itself is out of scope here; both values below are public.
	fmt.Printf("production public key derived from %s: %s\n", *envName, derived)
	fmt.Printf("production public key pinned in the client:   %s\n", prodPubHex)
	if !strings.EqualFold(derived, prodPubHex) {
		fmt.Println("PREFLIGHT FAILED: ED25519_PRIVATE_KEY does not correspond to the public key embedded in internal/core/updater.")
		fmt.Println("Either the secret belongs to another keypair, or DefaultPublicKeyHex was changed without reissuing the secret.")
		os.Exit(1)
	}
	fmt.Println("PASS preflight: the production secret signs exactly what the shipped client will trust.")
}
