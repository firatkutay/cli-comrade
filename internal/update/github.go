package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
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

// apiTimeout bounds how long a single "latest release" API call is
// allowed to take end-to-end — a small JSON response over a metadata
// API call needs nowhere near downloader.go's downloadTimeout, but it
// still needs a finite bound so a stalled or hostile endpoint can never
// hang `comrade upgrade`/`upgrade --check` forever (LOW#7).
const apiTimeout = 30 * time.Second

// maxReleaseJSONBytes bounds how many bytes of the "latest release" API
// response body LatestRelease will decode. GitHub's real response is at
// most a few hundred KB even for a release with many assets; this caps
// memory use against a compromised or misbehaving endpoint serving an
// unbounded response body (LOW#7).
const maxReleaseJSONBytes = 10 << 20 // 10 MiB

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

// ErrReleaseNotFound is LatestRelease's sentinel for a 404 from GitHub's
// "latest release" endpoint — meaning this repository has no published
// release yet (GitHub's API returns 404 for that endpoint specifically
// when a repo exists but has zero releases; it does not mean the
// repository itself is missing). internal/cli's upgrade.go checks this
// via errors.Is and renders a clean, i18n'd message instead of surfacing
// GitHub's own raw English 404 JSON body to the user (QA D3) — see
// LatestRelease's own doc comment for why the body is never included in
// the returned error at all, for ANY status code, not just 404.
var ErrReleaseNotFound = errors.New("update: no published release found")

// ErrFetchFailed is LatestRelease's sentinel for EVERY failure mode of
// the fetch step itself (request build, network Do, any non-200-non-404
// status, response decode) — ErrReleaseNotFound also always wraps this
// (a 404 IS a fetch failure, just a specific/expected one), so
// `errors.Is(err, ErrFetchFailed)` is the one check internal/cli's
// upgrade.go needs to classify "something went wrong reaching/reading
// GitHub" as a single family, distinct from Updater.Apply's OWN,
// separate failure modes further down the pipeline (no matching release
// asset for this platform, a checksum mismatch, a failed binary
// replace) — those keep their own, already-reasonably-specific existing
// messages untouched; only the fetch step's raw HTTP/body detail was
// QA D3's actual bug.
var ErrFetchFailed = errors.New("update: fetch latest release failed")

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
		client = &http.Client{Timeout: apiTimeout}
	}

	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", base, RepoOwner, RepoName)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Release{}, fmt.Errorf("update: build latest-release request: %w: %w", ErrFetchFailed, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return Release{}, fmt.Errorf("update: fetch latest release: %w: %w", ErrFetchFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		// GitHub's "latest release" endpoint 404s specifically when the
		// repository exists but has no published release at all — the
		// one case internal/cli/upgrade.go renders as a clean, expected,
		// i18n'd outcome rather than a failure. errors.Is (not a raw
		// status-code comparison at the call site) is what lets that
		// caller recognize this case; drain+discard the body without
		// including it anywhere — GitHub's 404 JSON body is redundant
		// with the status code here and would only ever end up as
		// English text a non-English user cannot read (QA D3).
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return Release{}, fmt.Errorf("%w: %w", ErrReleaseNotFound, ErrFetchFailed)
	}
	if resp.StatusCode != http.StatusOK {
		// Every OTHER non-200 status (rate limit, auth, 5xx, ...) also
		// never surfaces GitHub's raw response body to the end user
		// (QA D3's "other HTTP errors" case) — only the status code, in
		// this package's own internal (English) error text, which
		// upgrade.go re-renders through a concise i18n'd wrapper rather
		// than passing verbatim; the body is truncated to a small
		// diagnostic snippet here (not the full 4096-byte dump this used
		// to keep) purely so a future COMRADE_DEBUG-gated detail path has
		// something short to show, never as the primary user-facing text.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return Release{}, fmt.Errorf("update: fetch latest release: %w: unexpected status %d: %s", ErrFetchFailed, resp.StatusCode, string(body))
	}

	var rel Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseJSONBytes)).Decode(&rel); err != nil {
		return Release{}, fmt.Errorf("update: decode latest release response: %w: %w", ErrFetchFailed, err)
	}
	return rel, nil
}
