package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeReleaseFetcher is the injected ReleaseFetcher every Updater test
// uses instead of a real GitHub API call.
type fakeReleaseFetcher struct {
	release Release
	err     error
}

func (f fakeReleaseFetcher) LatestRelease(context.Context) (Release, error) {
	return f.release, f.err
}

// fakeDownloader is the injected AssetDownloader every Updater test
// uses instead of a real network download — it serves canned bytes
// keyed by URL, standing in for archiveAsset/checksumsAsset's
// BrowserDownloadURL.
type fakeDownloader struct {
	byURL map[string][]byte
	err   error
}

func (f fakeDownloader) Download(_ context.Context, url string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.byURL[url]
	if !ok {
		return nil, fmt.Errorf("test: no fake data registered for %s", url)
	}
	return data, nil
}

func buildTestArchiveAndChecksum(t *testing.T, archiveName, binaryName, binaryContent string) (archive, checksums []byte) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: binaryName, Mode: 0o755, Size: int64(len(binaryContent))}))
	_, err := tw.Write([]byte(binaryContent))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	archive = buf.Bytes()

	sum := sha256.Sum256(archive)
	checksums = []byte(hex.EncodeToString(sum[:]) + "  " + archiveName + "\n")
	return archive, checksums
}

func TestUpdaterCheckReportsNewerVersion(t *testing.T) {
	u := &Updater{
		Fetcher: fakeReleaseFetcher{release: Release{
			TagName: "v0.2.0",
			HTMLURL: "https://github.com/firatkutay/cli-comrade/releases/tag/v0.2.0",
		}},
		GOOS: "linux", GOARCH: "amd64",
	}

	result, err := u.Check(context.Background(), "v0.1.0")
	require.NoError(t, err)
	assert.True(t, result.UpdateAvailable)
	assert.Equal(t, "v0.2.0", result.LatestVersion)
	assert.Equal(t, "v0.1.0", result.CurrentVersion)
	assert.Equal(t, "https://github.com/firatkutay/cli-comrade/releases/tag/v0.2.0", result.ReleaseURL)
}

func TestUpdaterCheckReportsUpToDate(t *testing.T) {
	u := &Updater{
		Fetcher: fakeReleaseFetcher{release: Release{TagName: "v0.1.0"}},
		GOOS:    "linux", GOARCH: "amd64",
	}

	result, err := u.Check(context.Background(), "v0.1.0")
	require.NoError(t, err)
	assert.False(t, result.UpdateAvailable)
}

func TestUpdaterCheckPropagatesFetchError(t *testing.T) {
	u := &Updater{Fetcher: fakeReleaseFetcher{err: errors.New("network down")}}
	_, err := u.Check(context.Background(), "v0.1.0")
	require.Error(t, err)
}

func TestUpdaterApplyDownloadsVerifiesAndExtracts(t *testing.T) {
	archiveName := "comrade_0.2.0_linux_amd64.tar.gz"
	archive, checksums := buildTestArchiveAndChecksum(t, archiveName, "comrade", "new-binary-content")

	u := &Updater{
		Fetcher: fakeReleaseFetcher{release: Release{
			TagName: "v0.2.0",
			HTMLURL: "https://example.com/releases/v0.2.0",
			Assets: []Asset{
				{Name: archiveName, BrowserDownloadURL: "https://example.com/" + archiveName},
				{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
			},
		}},
		Downloader: fakeDownloader{byURL: map[string][]byte{
			"https://example.com/" + archiveName: archive,
			"https://example.com/checksums.txt":  checksums,
		}},
		GOOS: "linux", GOARCH: "amd64",
		// This test only exercises checksum verification, not the
		// signature gate — CosignPub is explicitly set to the
		// placeholder via the test seam so it never depends on whatever
		// key is actually embedded in cosign.pub.
		CosignPub: placeholderPub,
	}

	result, binary, err := u.Apply(context.Background(), "v0.1.0")
	require.NoError(t, err)
	assert.True(t, result.UpdateAvailable)
	assert.Equal(t, "new-binary-content", string(binary))
}

func TestUpdaterApplyNoOpWhenAlreadyUpToDate(t *testing.T) {
	u := &Updater{
		Fetcher: fakeReleaseFetcher{release: Release{TagName: "v0.1.0"}},
		GOOS:    "linux", GOARCH: "amd64",
	}

	result, binary, err := u.Apply(context.Background(), "v0.1.0")
	require.NoError(t, err)
	assert.False(t, result.UpdateAvailable)
	assert.Nil(t, binary)
}

func TestUpdaterApplyMissingPlatformAssetErrors(t *testing.T) {
	u := &Updater{
		Fetcher: fakeReleaseFetcher{release: Release{
			TagName: "v0.2.0",
			Assets:  []Asset{{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"}},
		}},
		GOOS: "linux", GOARCH: "amd64",
	}

	_, _, err := u.Apply(context.Background(), "v0.1.0")
	require.Error(t, err)
	assert.ErrorContains(t, err, "no asset named")
}

func TestUpdaterApplyMissingChecksumsAssetErrors(t *testing.T) {
	archiveName := "comrade_0.2.0_linux_amd64.tar.gz"
	u := &Updater{
		Fetcher: fakeReleaseFetcher{release: Release{
			TagName: "v0.2.0",
			Assets:  []Asset{{Name: archiveName, BrowserDownloadURL: "https://example.com/" + archiveName}},
		}},
		GOOS: "linux", GOARCH: "amd64",
	}

	_, _, err := u.Apply(context.Background(), "v0.1.0")
	require.Error(t, err)
	assert.ErrorContains(t, err, "checksums.txt")
}

func TestUpdaterApplyChecksumMismatchErrors(t *testing.T) {
	archiveName := "comrade_0.2.0_linux_amd64.tar.gz"
	archive, _ := buildTestArchiveAndChecksum(t, archiveName, "comrade", "new-binary-content")
	wrongChecksums := []byte("0000000000000000000000000000000000000000000000000000000000000000  " + archiveName + "\n")

	u := &Updater{
		Fetcher: fakeReleaseFetcher{release: Release{
			TagName: "v0.2.0",
			Assets: []Asset{
				{Name: archiveName, BrowserDownloadURL: "https://example.com/" + archiveName},
				{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
			},
		}},
		Downloader: fakeDownloader{byURL: map[string][]byte{
			"https://example.com/" + archiveName: archive,
			"https://example.com/checksums.txt":  wrongChecksums,
		}},
		GOOS: "linux", GOARCH: "amd64",
		// This test only exercises checksum verification, not the
		// signature gate — CosignPub is explicitly set to the
		// placeholder via the test seam so it never depends on whatever
		// key is actually embedded in cosign.pub.
		CosignPub: placeholderPub,
	}

	_, _, err := u.Apply(context.Background(), "v0.1.0")
	require.Error(t, err)
	assert.ErrorContains(t, err, "checksum mismatch")
}

// --- MEDIUM#4: Apply's signature-verification gate ---------------------

// buildApplyFixture assembles the release + fake downloader plumbing
// every signature-gate orchestration test below shares: a v0.2.0 release
// newer than v0.1.0, whose archive matches its own checksums.txt (so the
// pre-existing VerifyChecksum step downstream of the signature gate
// never itself explains a test failure). sigAssetName is deliberately a
// parameter, not baked in, so callers can omit the .sig asset entirely
// (the "missing signature" scenario). The registered .sig content always
// starts out nil here — callers that need real signature bytes (the
// "bad signature" / "good signature" scenarios) overwrite
// downloader.byURL[...] themselves after this returns, once they have
// generated the actual signature.
func buildApplyFixture(t *testing.T, sigAssetName string) (Release, fakeDownloader, []byte) {
	t.Helper()
	archiveName := "comrade_0.2.0_linux_amd64.tar.gz"
	archive, checksums := buildTestArchiveAndChecksum(t, archiveName, "comrade", "new-binary-content")

	assets := []Asset{
		{Name: archiveName, BrowserDownloadURL: "https://example.com/" + archiveName},
		{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
	}
	byURL := map[string][]byte{
		"https://example.com/" + archiveName: archive,
		"https://example.com/checksums.txt":  checksums,
	}
	if sigAssetName != "" {
		assets = append(assets, Asset{Name: sigAssetName, BrowserDownloadURL: "https://example.com/" + sigAssetName})
		byURL["https://example.com/"+sigAssetName] = nil
	}

	rel := Release{
		TagName: "v0.2.0",
		HTMLURL: "https://example.com/releases/v0.2.0",
		Assets:  assets,
	}
	return rel, fakeDownloader{byURL: byURL}, checksums
}

func TestUpdaterApplyNotConfiguredReportsStatusAndProceeds(t *testing.T) {
	rel, downloader, _ := buildApplyFixture(t, "") // no .sig asset published at all

	u := &Updater{
		Fetcher:    fakeReleaseFetcher{release: rel},
		Downloader: downloader,
		GOOS:       "linux", GOARCH: "amd64",
		// CosignPub set to an EXPLICIT placeholder (non-PEM) key via the
		// test seam, independent of whatever key is actually embedded in
		// cosign.pub, so this test always drives the genuinely
		// "not configured" branch.
		CosignPub: placeholderPub,
	}

	result, binary, err := u.Apply(context.Background(), "v0.1.0")
	require.NoError(t, err)
	assert.True(t, result.UpdateAvailable)
	assert.Equal(t, "new-binary-content", string(binary))
	assert.Equal(t, SignatureStatusNotConfigured, result.SignatureStatus, "Apply must never print anything itself — it reports the status for internal/cli to render")
}

func TestUpdaterApplyConfiguredMissingSignatureHardFails(t *testing.T) {
	_, testPubPEM := generateTestKeyPair(t)
	rel, downloader, _ := buildApplyFixture(t, "") // still no .sig asset

	u := &Updater{
		Fetcher:    fakeReleaseFetcher{release: rel},
		Downloader: downloader,
		GOOS:       "linux", GOARCH: "amd64",
		CosignPub: testPubPEM, // a REAL key is configured
	}

	result, binary, err := u.Apply(context.Background(), "v0.1.0")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingSignatureAsset)
	assert.ErrorContains(t, err, "checksums.txt.sig")
	assert.Nil(t, binary)
	// result is still the already-computed result on this failure path
	// — mirrors the two pre-existing "missing asset" tests above
	// (archive/checksums.txt), which also propagate the enclosing
	// `result` value up through the same early-return shape.
	assert.Equal(t, "v0.2.0", result.LatestVersion)
}

// TestUpdaterApplyEmptyCosignPubFallsBackToEmbeddedKey is a fail-safe
// regression guard: an empty, non-nil CosignPub ([]byte{}) — e.g. a
// misconfigured test seam or a future wiring bug that sets it to a
// zero-length slice instead of leaving it nil — must fall back to the
// real embedded cosign.pub key exactly like the nil zero value does,
// NOT be treated as an explicit (if degenerate) override that skips
// verification. Before the len()==0 fix, verifyChecksumsSignature only
// checked `u.CosignPub == nil`, so []byte{} stayed as pubPEM,
// signingConfigured([]byte{}) reported false (pem.Decode of an empty
// slice never yields a block), and Apply silently proceeded with
// checksum-only verification instead of enforcing the real key.
func TestUpdaterApplyEmptyCosignPubFallsBackToEmbeddedKey(t *testing.T) {
	rel, downloader, _ := buildApplyFixture(t, "") // no .sig asset published

	u := &Updater{
		Fetcher:    fakeReleaseFetcher{release: rel},
		Downloader: downloader,
		GOOS:       "linux", GOARCH: "amd64",
		CosignPub: []byte{}, // empty, non-nil — must behave like nil
	}

	_, binary, err := u.Apply(context.Background(), "v0.1.0")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrMissingSignatureAsset, "empty CosignPub must fall back to the real embedded key and enforce signing, not skip verification")
	assert.Nil(t, binary)
}

func TestUpdaterApplyConfiguredBadSignatureHardFails(t *testing.T) {
	_, testPubPEM := generateTestKeyPair(t)
	// Signed by a DIFFERENT key than testPubPEM — must fail verification.
	otherPriv, _ := generateTestKeyPair(t)
	rel, downloader, checksums := buildApplyFixture(t, ChecksumsSigFileName)
	badSig := signTestChecksums(t, otherPriv, checksums)
	downloader.byURL["https://example.com/"+ChecksumsSigFileName] = badSig

	u := &Updater{
		Fetcher:    fakeReleaseFetcher{release: rel},
		Downloader: downloader,
		GOOS:       "linux", GOARCH: "amd64",
		CosignPub: testPubPEM,
	}

	_, binary, err := u.Apply(context.Background(), "v0.1.0")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSignatureInvalid)
	assert.ErrorContains(t, err, "signature verification failed")
	assert.Nil(t, binary)
}

func TestUpdaterApplyConfiguredGoodSignatureProceeds(t *testing.T) {
	priv, testPubPEM := generateTestKeyPair(t)
	rel, downloader, checksums := buildApplyFixture(t, ChecksumsSigFileName)
	goodSig := signTestChecksums(t, priv, checksums)
	downloader.byURL["https://example.com/"+ChecksumsSigFileName] = goodSig

	u := &Updater{
		Fetcher:    fakeReleaseFetcher{release: rel},
		Downloader: downloader,
		GOOS:       "linux", GOARCH: "amd64",
		CosignPub: testPubPEM,
	}

	result, binary, err := u.Apply(context.Background(), "v0.1.0")
	require.NoError(t, err)
	assert.True(t, result.UpdateAvailable)
	assert.Equal(t, "new-binary-content", string(binary))
	assert.Equal(t, SignatureStatusVerified, result.SignatureStatus)
}
