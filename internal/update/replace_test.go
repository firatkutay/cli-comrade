package update

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceBinaryUnix(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "comrade")
	require.NoError(t, os.WriteFile(target, []byte("old-content"), 0o755))

	require.NoError(t, ReplaceBinary(target, []byte("new-content"), "linux"))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new-content", string(got))

	info, err := os.Stat(target)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	}
}

func TestReplaceBinaryWindowsDance(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "comrade.exe")
	require.NoError(t, os.WriteFile(target, []byte("old-content"), 0o755))

	require.NoError(t, ReplaceBinary(target, []byte("new-content"), "windows"))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new-content", string(got))

	oldContent, err := os.ReadFile(target + ".old")
	require.NoError(t, err, "the renamed original should be left behind as target+.old")
	assert.Equal(t, "old-content", string(oldContent))
}

func TestReplaceBinaryWindowsDanceWithPreexistingOldFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "comrade.exe")
	require.NoError(t, os.WriteFile(target, []byte("old-content"), 0o755))
	require.NoError(t, os.WriteFile(target+".old", []byte("stale-leftover"), 0o755))

	require.NoError(t, ReplaceBinary(target, []byte("new-content"), "windows"))

	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "new-content", string(got))
}

func TestCleanupOldBinaryRemovesLeftover(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "comrade.exe")
	require.NoError(t, os.WriteFile(target+".old", []byte("stale"), 0o755))

	CleanupOldBinary(target)

	_, err := os.Stat(target + ".old")
	assert.True(t, os.IsNotExist(err), "the .old leftover should be removed")
}

func TestCleanupOldBinaryNoLeftoverIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "comrade.exe")

	assert.NotPanics(t, func() { CleanupOldBinary(target) })
}
