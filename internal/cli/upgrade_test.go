package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/update"
)

// fakeReleaseFetcher and fakeDownloader are this package's own tiny test
// doubles for update.ReleaseFetcher/update.AssetDownloader — never a real
// network call, exactly like internal/update's own tests.
type fakeReleaseFetcher struct {
	release update.Release
	err     error
}

func (f fakeReleaseFetcher) LatestRelease(context.Context) (update.Release, error) {
	return f.release, f.err
}

type fakeDownloader struct {
	byURL map[string][]byte
}

func (f fakeDownloader) Download(_ context.Context, url string) ([]byte, error) {
	data, ok := f.byURL[url]
	if !ok {
		return nil, errors.New("test: no fake data registered for " + url)
	}
	return data, nil
}

// buildFakeArchive builds a minimal tar.gz archive containing a single
// "comrade" binary entry, plus a matching checksums.txt line — the same
// shape internal/update's own extract/checksum tests build.
func buildFakeArchive(t *testing.T, archiveName, binaryContent string) (archive, checksums []byte) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "comrade", Mode: 0o755, Size: int64(len(binaryContent))}))
	_, err := tw.Write([]byte(binaryContent))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	archive = buf.Bytes()

	sum := sha256.Sum256(archive)
	checksums = []byte(hex.EncodeToString(sum[:]) + "  " + archiveName + "\n")
	return archive, checksums
}

// testUpgradeDeps builds upgradeDeps wired entirely to fakes — no test
// in this file ever reaches the real network or replaces a real running
// executable.
func testUpgradeDeps(version string, fetcher update.ReleaseFetcher, downloader update.AssetDownloader, replace func(string, []byte, string) error) upgradeDeps {
	return upgradeDeps{
		version:    version,
		goos:       "linux",
		goarch:     "amd64",
		fetcher:    fetcher,
		downloader: downloader,
		executable: func() (string, error) { return "/fake/path/comrade", nil },
		replace:    replace,
	}
}

func execUpgradeCmd(t *testing.T, deps upgradeDeps, args ...string) (string, error) {
	t.Helper()
	withIsolatedConfigDir(t)
	cmd := newUpgradeCmd(newTestLoaderFactory(), deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err := cmd.Execute()
	return buf.String(), err
}

func TestUpgradeRefusesDevBuild(t *testing.T) {
	deps := testUpgradeDeps("dev", fakeReleaseFetcher{}, fakeDownloader{}, nil)

	_, err := execUpgradeCmd(t, deps)
	require.Error(t, err)
	assert.ErrorContains(t, err, "dev build")
}

func TestUpgradeCheckRefusesDevBuild(t *testing.T) {
	deps := testUpgradeDeps("dev", fakeReleaseFetcher{}, fakeDownloader{}, nil)

	_, err := execUpgradeCmd(t, deps, "--check")
	require.Error(t, err)
	assert.ErrorContains(t, err, "dev build")
}

func TestUpgradeCheckReportsUpToDate(t *testing.T) {
	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{release: update.Release{TagName: "v0.1.0"}}, fakeDownloader{}, nil)

	out, err := execUpgradeCmd(t, deps, "--check")
	require.NoError(t, err)
	assert.Contains(t, out, "already on the latest version")
	assert.Contains(t, out, "0.1.0")
}

func TestUpgradeCheckReportsNewerAvailable(t *testing.T) {
	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{release: update.Release{
		TagName: "v0.2.0",
		HTMLURL: "https://github.com/firatkutay/cli-comrade/releases/tag/v0.2.0",
	}}, fakeDownloader{}, nil)

	out, err := execUpgradeCmd(t, deps, "--check")
	require.NoError(t, err)
	assert.Contains(t, out, "v0.2.0")
	assert.Contains(t, out, "v0.1.0")
	assert.Contains(t, out, "https://github.com/firatkutay/cli-comrade/releases/tag/v0.2.0")
}

func TestUpgradeCheckPropagatesFetchError(t *testing.T) {
	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{err: errors.New("network down")}, fakeDownloader{}, nil)

	_, err := execUpgradeCmd(t, deps, "--check")
	require.Error(t, err)
	assert.ErrorContains(t, err, "network down")
}

func TestUpgradeApplyNoOpWhenUpToDate(t *testing.T) {
	replaceCalled := false
	replace := func(string, []byte, string) error { replaceCalled = true; return nil }
	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{release: update.Release{TagName: "v0.1.0"}}, fakeDownloader{}, replace)

	out, err := execUpgradeCmd(t, deps)
	require.NoError(t, err)
	assert.Contains(t, out, "already on the latest version")
	assert.False(t, replaceCalled, "replace must never run when nothing is newer")
}

func TestUpgradeApplyDownloadsVerifiesAndReplaces(t *testing.T) {
	archiveName := "comrade_0.2.0_linux_amd64.tar.gz"
	archive, checksums := buildFakeArchive(t, archiveName, "new-binary-content")

	fetcher := fakeReleaseFetcher{release: update.Release{
		TagName: "v0.2.0",
		Assets: []update.Asset{
			{Name: archiveName, BrowserDownloadURL: "https://example.com/" + archiveName},
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
		},
	}}
	downloader := fakeDownloader{byURL: map[string][]byte{
		"https://example.com/" + archiveName: archive,
		"https://example.com/checksums.txt":  checksums,
	}}

	var replacedPath, replacedGOOS string
	var replacedContent []byte
	replace := func(targetPath string, content []byte, goos string) error {
		replacedPath, replacedContent, replacedGOOS = targetPath, content, goos
		return nil
	}

	deps := testUpgradeDeps("v0.1.0", fetcher, downloader, replace)
	out, err := execUpgradeCmd(t, deps)
	require.NoError(t, err)

	assert.Contains(t, out, "downloading comrade v0.2.0")
	assert.Contains(t, out, "updated to v0.2.0")
	assert.Equal(t, "/fake/path/comrade", replacedPath)
	assert.Equal(t, "new-binary-content", string(replacedContent))
	assert.Equal(t, "linux", replacedGOOS)
}

func TestUpgradeApplyChecksumMismatchNeverCallsReplace(t *testing.T) {
	archiveName := "comrade_0.2.0_linux_amd64.tar.gz"
	archive, _ := buildFakeArchive(t, archiveName, "new-binary-content")
	wrongChecksums := []byte("0000000000000000000000000000000000000000000000000000000000000000  " + archiveName + "\n")

	fetcher := fakeReleaseFetcher{release: update.Release{
		TagName: "v0.2.0",
		Assets: []update.Asset{
			{Name: archiveName, BrowserDownloadURL: "https://example.com/" + archiveName},
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
		},
	}}
	downloader := fakeDownloader{byURL: map[string][]byte{
		"https://example.com/" + archiveName: archive,
		"https://example.com/checksums.txt":  wrongChecksums,
	}}

	replaceCalled := false
	replace := func(string, []byte, string) error { replaceCalled = true; return nil }

	deps := testUpgradeDeps("v0.1.0", fetcher, downloader, replace)
	_, err := execUpgradeCmd(t, deps)
	require.Error(t, err)
	assert.ErrorContains(t, err, "checksum mismatch")
	assert.False(t, replaceCalled, "a checksum mismatch must never reach ReplaceBinary")
}

func TestUpgradeHelpDescribesCheckFlag(t *testing.T) {
	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{}, fakeDownloader{}, nil)
	out, err := execUpgradeCmd(t, deps, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "--check")
}
