package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

// fakeArchiveName and fakeBinaryContent are the release asset name and
// binary payload every buildFakeArchive caller in this file uses,
// matching the fake release fixtures' own asset name and the
// replacedContent assertions downstream.
const (
	fakeArchiveName   = "comrade_0.2.0_linux_amd64.tar.gz"
	fakeBinaryContent = "new-binary-content"
)

// buildFakeArchive builds a minimal tar.gz archive containing a single
// "comrade" binary entry, plus a matching checksums.txt line — the same
// shape internal/update's own extract/checksum tests build.
func buildFakeArchive(t *testing.T) (archive, checksums []byte) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "comrade", Mode: 0o755, Size: int64(len(fakeBinaryContent))}))
	_, err := tw.Write([]byte(fakeBinaryContent))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	archive = buf.Bytes()

	sum := sha256.Sum256(archive)
	checksums = []byte(hex.EncodeToString(sum[:]) + "  " + fakeArchiveName + "\n")
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

// TestUpgradeCheckReleaseNotFoundRendersCleanMessage is QA D3's core
// regression guard: update.ErrReleaseNotFound (GitHub's actual 404 for a
// repo with no published release) must render as the clean, translated
// MsgUpgradeNoReleaseFound message — an EXACT match, not a substring
// check, so nothing else (a status code, a body fragment) can ever sneak
// back into this text unnoticed.
func TestUpgradeCheckReleaseNotFoundRendersCleanMessage(t *testing.T) {
	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{err: fmt.Errorf("%w", update.ErrReleaseNotFound)}, fakeDownloader{}, nil)

	_, err := execUpgradeCmd(t, deps, "--check")
	require.Error(t, err)
	assert.Equal(t, "no published release of comrade is available yet — check back later", err.Error())
}

// TestUpgradeCheckReleaseNotFoundRendersInTurkish is the same case under
// COMRADE_LANG=tr, proving the message is genuinely translated (this
// project's established TR-smoke-test convention), not just routed
// through a Translator that happens to fall back to English.
func TestUpgradeCheckReleaseNotFoundRendersInTurkish(t *testing.T) {
	t.Setenv("COMRADE_LANG", "tr")
	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{err: fmt.Errorf("%w", update.ErrReleaseNotFound)}, fakeDownloader{}, nil)

	_, err := execUpgradeCmd(t, deps, "--check")
	require.Error(t, err)
	assert.Equal(t, "henüz yayımlanmış bir comrade sürümü yok — daha sonra tekrar kontrol edin", err.Error())
}

// TestUpgradeCheckFetchFailedRendersCleanMessageWithoutRawDetail proves
// an update.ErrFetchFailed-classified error (any OTHER GitHub HTTP
// failure — rate limit, 5xx, a malformed body) renders the SAME concise,
// generic message regardless of what raw detail the underlying error
// carried, and that raw detail (here, a fake "unexpected status 500:
// <internal server meltdown JSON>" — standing in for what
// update/github.go's own truncated-body detail would look like) never
// leaks into what the user sees.
func TestUpgradeCheckFetchFailedRendersCleanMessageWithoutRawDetail(t *testing.T) {
	rawDetail := fmt.Errorf("update: fetch latest release: %w: unexpected status 500: {\"message\":\"internal server meltdown\"}", update.ErrFetchFailed)
	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{err: rawDetail}, fakeDownloader{}, nil)

	_, err := execUpgradeCmd(t, deps, "--check")
	require.Error(t, err)
	assert.Equal(t, "could not reach GitHub to check for a newer version — try again later", err.Error())
	assert.NotContains(t, err.Error(), "500")
	assert.NotContains(t, err.Error(), "meltdown")
}

// TestUpgradeCheckFetchFailedDebugDetailOnlyWhenComradeDebugSet proves
// the raw underlying detail is reachable ONLY behind COMRADE_DEBUG (this
// tree's established debug-gated-detail convention, see hook.go), never
// by default.
func TestUpgradeCheckFetchFailedDebugDetailOnlyWhenComradeDebugSet(t *testing.T) {
	rawDetail := fmt.Errorf("update: fetch latest release: %w: unexpected status 500: {\"message\":\"internal server meltdown\"}", update.ErrFetchFailed)

	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{err: rawDetail}, fakeDownloader{}, nil)
	outWithoutDebug, _ := execUpgradeCmd(t, deps, "--check")
	assert.NotContains(t, outWithoutDebug, "meltdown", "no COMRADE_DEBUG: raw detail must not appear anywhere, including stderr")

	t.Setenv("COMRADE_DEBUG", "1")
	outWithDebug, _ := execUpgradeCmd(t, deps, "--check")
	assert.Contains(t, outWithDebug, "meltdown", "COMRADE_DEBUG=1: raw detail must be reachable for diagnosis")
}

// TestUpgradeCheckAgainstRealGitHubClient404NeverLeaksRawJSONBody is the
// most faithful reproduction of the actual QA-reported bug: the REAL
// update.GitHubClient (not a fake ReleaseFetcher) pointed at an
// httptest.Server that returns GitHub's OWN actual 404 response shape,
// driven all the way through `comrade upgrade --check`'s real RunE.
func TestUpgradeCheckAgainstRealGitHubClient404NeverLeaksRawJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found","documentation_url":"https://docs.github.com/rest/releases/releases#get-the-latest-release","status":"404"}`))
	}))
	defer srv.Close()

	fetcher := &update.GitHubClient{APIBaseURL: srv.URL, HTTPClient: srv.Client()}
	deps := testUpgradeDeps("v0.1.0", fetcher, fakeDownloader{}, nil)

	_, err := execUpgradeCmd(t, deps, "--check")
	require.Error(t, err)
	assert.Equal(t, "no published release of comrade is available yet — check back later", err.Error())
	assert.NotContains(t, err.Error(), "Not Found")
	assert.NotContains(t, err.Error(), "documentation_url")
	assert.NotContains(t, err.Error(), "404")
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
	archive, checksums := buildFakeArchive(t)

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
	archive, _ := buildFakeArchive(t)
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

// TestUpgradeApplyResolvesSymlinkedExecutableBeforeReplace is LOW#8's
// regression guard: when os.Executable() resolves to a symlink (as it
// does for a Homebrew-style install, where the PATH entry is a symlink
// into a versioned cellar directory), `comrade upgrade` must call
// deps.replace with the symlink's REAL target, not the symlink path
// itself — otherwise ReplaceBinary would clobber the symlink rather than
// the actual binary it points at.
func TestUpgradeApplyResolvesSymlinkedExecutableBeforeReplace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require elevated privileges on Windows CI runners")
	}

	dir := t.TempDir()
	realBinary := filepath.Join(dir, "comrade-real")
	require.NoError(t, os.WriteFile(realBinary, []byte("old-content"), 0o755))
	symlinkPath := filepath.Join(dir, "comrade")
	require.NoError(t, os.Symlink(realBinary, symlinkPath))

	archiveName := "comrade_0.2.0_linux_amd64.tar.gz"
	archive, checksums := buildFakeArchive(t)
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

	var replacedPath string
	replace := func(targetPath string, _ []byte, _ string) error {
		replacedPath = targetPath
		return nil
	}

	deps := upgradeDeps{
		version:    "v0.1.0",
		goos:       "linux",
		goarch:     "amd64",
		fetcher:    fetcher,
		downloader: downloader,
		executable: func() (string, error) { return symlinkPath, nil },
		replace:    replace,
	}

	_, err := execUpgradeCmd(t, deps)
	require.NoError(t, err)

	wantReal, err := filepath.EvalSymlinks(realBinary)
	require.NoError(t, err)
	assert.Equal(t, wantReal, replacedPath, "replace must target the symlink's real backing file, not the symlink itself")
	assert.NotEqual(t, symlinkPath, replacedPath)
}

// TestUpgradeApplyFallsBackWhenSymlinkResolutionFails proves the
// non-fatal fallback half of LOW#8: when the resolved executable path
// doesn't actually exist on disk (EvalSymlinks fails), the upgrade still
// proceeds using the original, unresolved path rather than aborting.
func TestUpgradeApplyFallsBackWhenSymlinkResolutionFails(t *testing.T) {
	archiveName := "comrade_0.2.0_linux_amd64.tar.gz"
	archive, checksums := buildFakeArchive(t)
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

	var replacedPath string
	replace := func(targetPath string, _ []byte, _ string) error {
		replacedPath = targetPath
		return nil
	}

	deps := testUpgradeDeps("v0.1.0", fetcher, downloader, replace)
	out, err := execUpgradeCmd(t, deps)
	require.NoError(t, err)
	assert.Equal(t, "/fake/path/comrade", replacedPath, "a nonexistent path must fall back to the original, unresolved path")
	assert.Contains(t, out, "could not resolve symlinks")
}

func TestUpgradeHelpDescribesCheckFlag(t *testing.T) {
	deps := testUpgradeDeps("v0.1.0", fakeReleaseFetcher{}, fakeDownloader{}, nil)
	out, err := execUpgradeCmd(t, deps, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "--check")
}
