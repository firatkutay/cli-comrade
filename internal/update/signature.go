package update

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
)

// embeddedCosignPub is the cosign public key baked into this binary at
// build time (see cosign.pub's own doc comment) — the anchor
// verifyChecksumsSignatureWith checks every downloaded checksums.txt
// against before Updater.Apply trusts anything it says. Until the
// project embeds a real PEM-encoded key here, this holds the placeholder
// comment cosign.pub ships with, which never parses as a PEM block —
// see ErrSigningNotConfigured.
//
//go:embed cosign.pub
var embeddedCosignPub []byte

// ChecksumsSigFileName is the name of the detached signature asset
// (cosign's own naming convention: "<file>.sig") this package expects a
// release to publish alongside ChecksumsFileName once release signing is
// configured — the release-side counterpart of embeddedCosignPub.
const ChecksumsSigFileName = ChecksumsFileName + ".sig"

// ErrSigningNotConfigured is returned when the embedded public key is
// still cosign.pub's build-time placeholder rather than a real
// PEM-encoded key — signature verification is impossible in that state,
// so Updater.Apply treats it as "not yet configured" and falls back to
// checksum-only verification (with a warning) instead of hard-failing.
// This is what lets a release built before a real key is embedded keep
// working.
var ErrSigningNotConfigured = errors.New("update: release signature verification is not configured (embedded cosign.pub is still the placeholder)")

// ErrSignatureInvalid is returned when a real embedded public key IS
// present but the supplied checksums.txt signature does not verify
// against it — unlike ErrSigningNotConfigured, this is always a hard
// failure: Updater.Apply never falls back to checksum-only verification
// once a real key is configured.
var ErrSignatureInvalid = errors.New("update: checksums.txt signature verification failed")

// ErrMissingSignatureAsset is returned when a real (non-placeholder)
// signing key IS configured but the release being installed does not
// publish a ChecksumsSigFileName asset. Once signing is configured,
// Updater.Apply refuses to install unsigned material rather than
// silently falling back to checksum-only verification — this sentinel
// lets internal/cli's upgrade.go render that refusal as a clean,
// translated message instead of a raw internal error string.
var ErrMissingSignatureAsset = errors.New("update: no signature asset found for a signed release")

// signingConfigured reports whether pubPEM is a real embedded public key
// (a decodable "PUBLIC KEY" PEM block) rather than cosign.pub's
// placeholder text — the same gate verifyChecksumsSignatureWith applies,
// factored out so Updater.Apply can decide up front (before it even
// looks for a .sig release asset) whether a signature is required at
// all.
func signingConfigured(pubPEM []byte) bool {
	block, _ := pem.Decode(pubPEM)
	return block != nil && block.Type == "PUBLIC KEY"
}

// verifyChecksumsSignatureWith is VerifyChecksumsSignature's testable
// inner implementation: pubPEM, checksums, and sigB64 are all explicit
// parameters (rather than closing over the embedded key) specifically so
// tests can exercise every branch — placeholder key, mismatched key,
// tampered checksums, malformed signature — against an ephemeral,
// in-test-generated ECDSA key pair instead of the one real key actually
// embedded in the binary.
//
// sigB64 is the base64 encoding (cosign's checksums.txt.sig format) of
// an ASN.1 DER-encoded ECDSA signature over checksums' SHA-256 digest.
func verifyChecksumsSignatureWith(pubPEM, checksums, sigB64 []byte) error {
	block, _ := pem.Decode(pubPEM)
	if block == nil || block.Type != "PUBLIC KEY" {
		return ErrSigningNotConfigured
	}

	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("update: parse embedded public key: %w", err)
	}
	pub, ok := pubAny.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("update: embedded public key is not an ECDSA key (got %T)", pubAny)
	}

	digest := sha256.Sum256(checksums)

	sigDER, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil {
		return fmt.Errorf("update: decode signature base64: %w", err)
	}

	if !ecdsa.VerifyASN1(pub, digest[:], sigDER) {
		return ErrSignatureInvalid
	}
	return nil
}

// VerifyChecksumsSignature verifies sigB64 (checksums.txt.sig's
// contents) against checksums (checksums.txt's own contents) using the
// public key embedded in this binary at build time (cosign.pub). See
// verifyChecksumsSignatureWith for the actual verification steps.
func VerifyChecksumsSignature(checksums, sigB64 []byte) error {
	return verifyChecksumsSignatureWith(embeddedCosignPub, checksums, sigB64)
}
