package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPDownloaderDownloadReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("archive-bytes"))
	}))
	defer srv.Close()

	d := HTTPDownloader{HTTPClient: srv.Client()}
	data, err := d.Download(context.Background(), srv.URL)
	require.NoError(t, err)
	assert.Equal(t, "archive-bytes", string(data))
}

func TestHTTPDownloaderDownloadNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := HTTPDownloader{HTTPClient: srv.Client()}
	_, err := d.Download(context.Background(), srv.URL)
	require.Error(t, err)
	assert.ErrorContains(t, err, "500")
}
