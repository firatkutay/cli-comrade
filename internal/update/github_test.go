package update

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestGitHubClientLatestReleaseNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message": "Not Found"}`))
	}))
	defer srv.Close()

	client := &GitHubClient{APIBaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := client.LatestRelease(context.Background())
	require.Error(t, err)
	assert.ErrorContains(t, err, "404")
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
