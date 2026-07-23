package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// downloadTimeout bounds how long a single asset download (the platform
// archive, checksums.txt, or checksums.txt.sig, each fetched over the
// real network in production) is allowed to take end-to-end. Generous —
// release archives can be tens of MB on a slow connection — but finite,
// so a stalled or hostile server can never hang `comrade upgrade`
// forever (LOW#7).
const downloadTimeout = 5 * time.Minute

// maxAssetBytes bounds how many bytes a single downloaded asset's body
// may contain. Release archives are a few MB in practice; 200 MiB is
// generous headroom while still capping memory use against a
// compromised or misbehaving server serving an unbounded response body
// (LOW#7).
const maxAssetBytes = 200 << 20 // 200 MiB

// AssetDownloader fetches the raw bytes of a release asset from its
// BrowserDownloadURL. Production code uses HTTPDownloader; tests inject
// a fake returning canned archive/checksums.txt bytes.
type AssetDownloader interface {
	Download(ctx context.Context, url string) ([]byte, error)
}

// HTTPDownloader is the production AssetDownloader: a plain net/http GET
// against url. HTTPClient is optional; a nil value resolves to a plain
// *http.Client{Timeout: downloadTimeout} — TLS verification stays on
// (the zero-value http.Transport this default client uses never disables
// it) and redirects (GitHub always serves assets via a redirect to its
// backing object storage) are still followed, both http.Client defaults.
type HTTPDownloader struct {
	HTTPClient *http.Client
}

// Download implements AssetDownloader.
func (d HTTPDownloader) Download(ctx context.Context, url string) ([]byte, error) {
	client := d.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: downloadTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("update: build download request for %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: download %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: download %s: unexpected status %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes))
	if err != nil {
		return nil, fmt.Errorf("update: read download body for %s: %w", url, err)
	}
	return data, nil
}
