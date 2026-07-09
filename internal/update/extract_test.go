package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestExtractBinaryFromTarGz(t *testing.T) {
	archive := buildTarGz(t, map[string]string{
		"comrade":   "binary-content",
		"README.md": "docs",
	})

	got, err := ExtractBinary(archive, "comrade_0.2.0_linux_amd64.tar.gz", "comrade")
	require.NoError(t, err)
	assert.Equal(t, "binary-content", string(got))
}

func TestExtractBinaryFromZip(t *testing.T) {
	archive := buildZip(t, map[string]string{
		"comrade.exe": "windows-binary-content",
		"README.md":   "docs",
	})

	got, err := ExtractBinary(archive, "comrade_0.2.0_windows_amd64.zip", "comrade.exe")
	require.NoError(t, err)
	assert.Equal(t, "windows-binary-content", string(got))
}

func TestExtractBinaryMissingFromTarGz(t *testing.T) {
	archive := buildTarGz(t, map[string]string{"README.md": "docs"})

	_, err := ExtractBinary(archive, "comrade_0.2.0_linux_amd64.tar.gz", "comrade")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found in tar archive")
}

func TestExtractBinaryMissingFromZip(t *testing.T) {
	archive := buildZip(t, map[string]string{"README.md": "docs"})

	_, err := ExtractBinary(archive, "comrade_0.2.0_windows_amd64.zip", "comrade.exe")
	require.Error(t, err)
	assert.ErrorContains(t, err, "not found in zip archive")
}

func TestExtractBinaryUnrecognizedFormat(t *testing.T) {
	_, err := ExtractBinary([]byte("data"), "comrade_0.2.0_linux_amd64.tar.xz", "comrade")
	require.Error(t, err)
	assert.ErrorContains(t, err, "unrecognized archive format")
}
