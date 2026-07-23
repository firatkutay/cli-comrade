package update

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubClientLatestReleaseParsesResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/firatkutay/cli-comrade/releases/latest", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.2.0",
			"html_url": "https://github.com/firatkutay/cli-comrade/releases/tag/v0.2.0",
			"assets": [
				{"name": "comrade_0.2.0_linux_amd64.tar.gz", "browser_download_url": "https://example.com/a.tar.gz"},
				{"name": "checksums.txt", "browser_download_url": "https://example.com/checksums.txt"}
			]
		}`))
	}))
	defer srv.Close()

	client := &GitHubClient{APIBaseURL: srv.URL, HTTPClient: srv.Client()}
	rel, err := client.LatestRelease(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "v0.2.0", rel.TagName)
	assert.Equal(t, "https://github.com/firatkutay/cli-comrade/releases/tag/v0.2.0", rel.HTMLURL)
	require.Len(t, rel.Assets, 2)
	assert.Equal(t, "comrade_0.2.0_linux_amd64.tar.gz", rel.Assets[0].Name)
}

// TestGitHubClientLatestRelease404IsErrReleaseNotFound is QA D3's
// regression guard at LatestRelease's own layer: a 404 (GitHub's actual
// response for a repo with zero published releases) must classify as
// ErrReleaseNotFound (errors.Is), NOT a generic "unexpected status"
// error — and GitHub's raw response body must never appear anywhere in
// the returned error's text.
func TestGitHubClientLatestRelease404IsErrReleaseNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Not Found", "documentation_url": "https://docs.github.com/rest/releases/releases#get-the-latest-release"}`))
	}))
	defer srv.Close()

	client := &GitHubClient{APIBaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := client.LatestRelease(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReleaseNotFound)
	assert.ErrorIs(t, err, ErrFetchFailed, "a 404 is also a fetch failure, for callers that only care about that broader family")
	assert.NotContains(t, err.Error(), "Not Found")
	assert.NotContains(t, err.Error(), "documentation_url")
}

// TestGitHubClientLatestReleaseOtherNonOKStatusNeverIncludesFullRawBody
// proves a non-404 HTTP failure (rate limit, auth, 5xx, ...) still
// distinguishes itself from ErrReleaseNotFound (errors.Is must be
// false) AND never leaks GitHub's full raw response body into the
// returned error — only a short (<=200-byte) diagnostic snippet, never
// the multi-KB body this endpoint could return.
func TestGitHubClientLatestReleaseOtherNonOKStatusNeverIncludesFullRawBody(t *testing.T) {
	longBody := strings.Repeat("x", 5000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(longBody))
	}))
	defer srv.Close()

	client := &GitHubClient{APIBaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := client.LatestRelease(context.Background())
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrReleaseNotFound), "a 500 must not classify as ErrReleaseNotFound")
	assert.ErrorIs(t, err, ErrFetchFailed)
	assert.ErrorContains(t, err, "500")
	assert.Less(t, len(err.Error()), 300, "the full 5000-byte body must never be included verbatim")
}

// TestGitHubClientLatestReleaseOversizedBodyFailsToDecode is LOW#7's
// guard on LatestRelease's own response-body cap: a body larger than
// maxReleaseJSONBytes must never be decoded in full — the truncated
// (necessarily invalid, mid-array) JSON fails to decode, which
// LatestRelease already reports as ErrFetchFailed, exactly like any
// other malformed response body.
func TestGitHubClientLatestReleaseOversizedBodyFailsToDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name": "v0.2.0", "assets": [`))
		padding := make([]byte, maxReleaseJSONBytes+1024)
		for i := range padding {
			padding[i] = ' '
		}
		_, _ = w.Write(padding)
	}))
	defer srv.Close()

	client := &GitHubClient{APIBaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := client.LatestRelease(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrFetchFailed)
}

func TestReleaseAssetByName(t *testing.T) {
	rel := Release{Assets: []Asset{
		{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
		{Name: "comrade_0.2.0_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/a.tar.gz"},
	}}

	asset, ok := rel.AssetByName("checksums.txt")
	require.True(t, ok)
	assert.Equal(t, "https://example.com/checksums.txt", asset.BrowserDownloadURL)

	_, ok = rel.AssetByName("does-not-exist")
	assert.False(t, ok)
}
