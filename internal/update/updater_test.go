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
	}

	_, _, err := u.Apply(context.Background(), "v0.1.0")
	require.Error(t, err)
	assert.ErrorContains(t, err, "checksum mismatch")
}
