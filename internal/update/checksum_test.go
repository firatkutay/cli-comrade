package update

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checksumLine(t *testing.T, data []byte, name string) string {
	t.Helper()
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) + "  " + name
}

func TestVerifyChecksumAccepts(t *testing.T) {
	data := []byte("the archive contents")
	txt := checksumLine(t, data, "comrade_0.2.0_linux_amd64.tar.gz") + "\n" +
		checksumLine(t, []byte("other file"), "checksums.txt")

	err := VerifyChecksum(data, []byte(txt), "comrade_0.2.0_linux_amd64.tar.gz")
	require.NoError(t, err)
}

func TestVerifyChecksumRejectsMismatch(t *testing.T) {
	data := []byte("the archive contents")
	tampered := []byte("the archive contents, tampered")
	txt := checksumLine(t, data, "comrade_0.2.0_linux_amd64.tar.gz")

	err := VerifyChecksum(tampered, []byte(txt), "comrade_0.2.0_linux_amd64.tar.gz")
	require.Error(t, err)
	assert.ErrorContains(t, err, "checksum mismatch")
}

func TestVerifyChecksumMissingEntry(t *testing.T) {
	err := VerifyChecksum([]byte("data"), []byte("deadbeef  other-file.tar.gz\n"), "comrade_0.2.0_linux_amd64.tar.gz")
	require.Error(t, err)
	assert.ErrorContains(t, err, "no checksum entry found")
}

func TestVerifyChecksumHandlesBinaryModeAsterisk(t *testing.T) {
	data := []byte("the archive contents")
	sum := sha256.Sum256(data)
	txt := hex.EncodeToString(sum[:]) + " *comrade_0.2.0_linux_amd64.tar.gz\n"

	err := VerifyChecksum(data, []byte(txt), "comrade_0.2.0_linux_amd64.tar.gz")
	require.NoError(t, err)
}
