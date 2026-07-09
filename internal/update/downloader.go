package update

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// AssetDownloader fetches the raw bytes of a release asset from its
// BrowserDownloadURL. Production code uses HTTPDownloader; tests inject
// a fake returning canned archive/checksums.txt bytes.
type AssetDownloader interface {
	Download(ctx context.Context, url string) ([]byte, error)
}

// HTTPDownloader is the production AssetDownloader: a plain net/http GET
// against url. HTTPClient is optional; a nil value resolves to a plain
// *http.Client{}.
type HTTPDownloader struct {
	HTTPClient *http.Client
}

// Download implements AssetDownloader.
func (d HTTPDownloader) Download(ctx context.Context, url string) ([]byte, error) {
	client := d.HTTPClient
	if client == nil {
		client = &http.Client{}
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

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("update: read download body for %s: %w", url, err)
	}
	return data, nil
}
