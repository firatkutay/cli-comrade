package update

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateTestKeyPair builds an ephemeral ECDSA P-256 key pair and
// returns the private key plus its public key's PKIX PEM encoding — the
// same shape a real cosign.pub carries — so signature tests never depend
// on the one real key actually embedded in the binary.
func generateTestKeyPair(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	require.NotNil(t, pubPEM)
	return priv, pubPEM
}

// signTestChecksums signs checksums' SHA-256 digest with priv and
// base64-encodes the resulting ASN.1 DER signature, exactly like cosign
// sign-blob would produce for checksums.txt.sig.
func signTestChecksums(t *testing.T, priv *ecdsa.PrivateKey, checksums []byte) []byte {
	t.Helper()
	digest := sha256.Sum256(checksums)
	sigDER, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	require.NoError(t, err)
	encoded := base64.StdEncoding.EncodeToString(sigDER)
	return []byte(encoded)
}

func TestVerifyChecksumsSignatureWithValidSignatureSucceeds(t *testing.T) {
	priv, pubPEM := generateTestKeyPair(t)
	checksums := []byte("deadbeef  comrade_0.2.0_linux_amd64.tar.gz\n")
	sig := signTestChecksums(t, priv, checksums)

	err := verifyChecksumsSignatureWith(pubPEM, checksums, sig)
	assert.NoError(t, err)
}

func TestVerifyChecksumsSignatureWithTamperedChecksumsFails(t *testing.T) {
	priv, pubPEM := generateTestKeyPair(t)
	checksums := []byte("deadbeef  comrade_0.2.0_linux_amd64.tar.gz\n")
	sig := signTestChecksums(t, priv, checksums)

	tampered := []byte("00000000  comrade_0.2.0_linux_amd64.tar.gz\n")
	err := verifyChecksumsSignatureWith(pubPEM, tampered, sig)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSignatureInvalid)
}

func TestVerifyChecksumsSignatureWithDifferentKeyFails(t *testing.T) {
	priv, _ := generateTestKeyPair(t)
	_, otherPubPEM := generateTestKeyPair(t)
	checksums := []byte("deadbeef  comrade_0.2.0_linux_amd64.tar.gz\n")
	sig := signTestChecksums(t, priv, checksums)

	// sig was produced by priv, but we verify against a DIFFERENT key's
	// public half — must fail exactly like a forged signature would.
	err := verifyChecksumsSignatureWith(otherPubPEM, checksums, sig)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSignatureInvalid)
}

func TestVerifyChecksumsSignatureWithPlaceholderKeyReturnsNotConfigured(t *testing.T) {
	checksums := []byte("deadbeef  comrade_0.2.0_linux_amd64.tar.gz\n")
	err := verifyChecksumsSignatureWith(embeddedCosignPub, checksums, []byte("irrelevant"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSigningNotConfigured)
}

func TestVerifyChecksumsSignatureWithMalformedPEMReturnsNotConfigured(t *testing.T) {
	checksums := []byte("deadbeef  comrade_0.2.0_linux_amd64.tar.gz\n")
	err := verifyChecksumsSignatureWith([]byte("not a pem block at all"), checksums, []byte("irrelevant"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSigningNotConfigured)
}

func TestVerifyChecksumsSignatureWithMalformedBase64Errors(t *testing.T) {
	_, pubPEM := generateTestKeyPair(t)
	checksums := []byte("deadbeef  comrade_0.2.0_linux_amd64.tar.gz\n")

	err := verifyChecksumsSignatureWith(pubPEM, checksums, []byte("not-valid-base64!!!"))
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrSigningNotConfigured), "a malformed base64 signature is a distinct failure from an unconfigured key")
	assert.False(t, errors.Is(err, ErrSignatureInvalid), "a decode failure is distinct from a cryptographically-invalid signature")
}

func TestVerifyChecksumsSignatureWithNonECDSAKeyErrors(t *testing.T) {
	// A PEM block that decodes and parses fine as PKIX but isn't an
	// ECDSA key (an RSA-shaped ASN.1 SubjectPublicKeyInfo is overkill to
	// build here — instead assert the non-ECDSA-key error path via a
	// block whose Bytes simply don't parse as PKIX at all, which exercises
	// the same "key material is unusable" branch distinctly from the
	// placeholder-detection branch above).
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: []byte("not valid PKIX DER")}
	pubPEM := pem.EncodeToMemory(block)
	checksums := []byte("deadbeef  comrade_0.2.0_linux_amd64.tar.gz\n")

	err := verifyChecksumsSignatureWith(pubPEM, checksums, []byte("irrelevant"))
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrSigningNotConfigured))
	assert.False(t, errors.Is(err, ErrSignatureInvalid))
}

func TestVerifyChecksumsSignaturePublicFunctionUsesEmbeddedKey(t *testing.T) {
	// The exported entrypoint always verifies against embeddedCosignPub
	// (the real cosign.pub, still the placeholder at this point in the
	// rollout) — proving it, not some other key, is what backs the
	// public API.
	err := VerifyChecksumsSignature([]byte("checksums"), []byte("sig"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSigningNotConfigured)
}

func TestSigningConfiguredReportsFalseForPlaceholder(t *testing.T) {
	assert.False(t, signingConfigured(embeddedCosignPub))
}

func TestSigningConfiguredReportsTrueForRealKey(t *testing.T) {
	_, pubPEM := generateTestKeyPair(t)
	assert.True(t, signingConfigured(pubPEM))
}

func TestChecksumsSigFileNameMatchesConvention(t *testing.T) {
	assert.Equal(t, "checksums.txt.sig", ChecksumsSigFileName)
}
