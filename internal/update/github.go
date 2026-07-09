package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// RepoOwner and RepoName identify cli-comrade's GitHub repository — the
// single source of truth every release-fetching/asset-naming call site
// in this package derives from, so a repository rename only ever needs
// to change these two constants.
const (
	RepoOwner = "firatkutay"
	RepoName  = "cli-comrade"
)

// defaultAPIBaseURL is GitHub's REST API root. GitHubClient.APIBaseURL
// overrides it — tests point this at an httptest.Server so no test ever
// makes a real network call.
const defaultAPIBaseURL = "https://api.github.com"

// Asset is one downloadable file attached to a GitHub release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Release is the subset of GitHub's release JSON this package needs.
type Release struct {
	TagName string  `json:"tag_name"`
	HTMLURL string  `json:"html_url"`
	Assets  []Asset `json:"assets"`
}

// AssetByName returns the Asset in r.Assets whose Name matches name, if
// any — used to locate both the platform archive and checksums.txt.
func (r Release) AssetByName(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return Asset{}, false
}

// ReleaseFetcher resolves the latest published GitHub release for
// cli-comrade. Production code uses GitHubClient; tests inject a fake
// implementation returning a canned Release so the self-update flow
// never has to reach the real network to be exercised.
type ReleaseFetcher interface {
	LatestRelease(ctx context.Context) (Release, error)
}

// GitHubClient is the production ReleaseFetcher: it calls the real
// GitHub REST API's "latest release" endpoint. Both fields are optional;
// zero values resolve to sane production defaults (the real API, a
// plain *http.Client) — set APIBaseURL in a test to point at an
// httptest.Server instead.
type GitHubClient struct {
	APIBaseURL string
	HTTPClient *http.Client
}

// LatestRelease implements ReleaseFetcher.
func (c *GitHubClient) LatestRelease(ctx context.Context) (Release, error) {
	base := c.APIBaseURL
	if base == "" {
		base = defaultAPIBaseURL
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{}
	}

	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", base, RepoOwner, RepoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("update: build latest-release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("update: fetch latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Release{}, fmt.Errorf("update: fetch latest release: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("update: decode latest release response: %w", err)
	}
	return rel, nil
}
