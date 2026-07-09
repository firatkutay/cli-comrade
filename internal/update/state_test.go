package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatePathForWindows(t *testing.T) {
	getenv := func(k string) string {
		if k == "LOCALAPPDATA" {
			return `C:\Users\alice\AppData\Local`
		}
		return ""
	}
	got, err := StatePathFor("windows", getenv)
	require.NoError(t, err)
	assert.Equal(t, `C:\Users\alice\AppData\Local\cli-comrade\update_check.json`, got)
}

func TestStatePathForWindowsMissingLocalAppData(t *testing.T) {
	_, err := StatePathFor("windows", func(string) string { return "" })
	require.Error(t, err)
}

func TestStatePathForUnixHonorsXDGStateHome(t *testing.T) {
	getenv := func(k string) string {
		if k == "XDG_STATE_HOME" {
			return "/home/alice/.state-custom"
		}
		return ""
	}
	got, err := StatePathFor("linux", getenv)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/home/alice/.state-custom", "cli-comrade", "update_check.json"), got)
}

func TestStatePathForUnixFallsBackToHome(t *testing.T) {
	getenv := func(k string) string {
		if k == "HOME" {
			return "/home/alice"
		}
		return ""
	}
	got, err := StatePathFor("linux", getenv)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("/home/alice", ".local", "state", "cli-comrade", "update_check.json"), got)
}

func TestStatePathForUnixMissingHome(t *testing.T) {
	_, err := StatePathFor("linux", func(string) string { return "" })
	require.Error(t, err)
}

func TestReadStateMissingFileReturnsZeroValue(t *testing.T) {
	got := ReadState(filepath.Join(t.TempDir(), "does-not-exist.json"))
	assert.True(t, got.LastCheckedAt.IsZero())
	assert.Equal(t, "", got.LatestKnownVersion)
}

func TestReadStateEmptyPathReturnsZeroValue(t *testing.T) {
	got := ReadState("")
	assert.True(t, got.LastCheckedAt.IsZero())
}

func TestReadStateCorruptJSONReturnsZeroValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "update_check.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o644))
	got := ReadState(path)
	assert.True(t, got.LastCheckedAt.IsZero())
}

func TestWriteStateThenReadStateRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "update_check.json")
	want := CheckState{
		LastCheckedAt:      time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC),
		LatestKnownVersion: "v0.2.0",
	}
	require.NoError(t, WriteState(path, want))

	got := ReadState(path)
	assert.True(t, want.LastCheckedAt.Equal(got.LastCheckedAt))
	assert.Equal(t, want.LatestKnownVersion, got.LatestKnownVersion)
}

func TestShouldCheck(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)

	assert.True(t, ShouldCheck(now, CheckState{}), "zero-value LastCheckedAt is always due")
	assert.False(t, ShouldCheck(now, CheckState{LastCheckedAt: now.Add(-24 * time.Hour)}), "1 day ago is not due yet")
	assert.False(t, ShouldCheck(now, CheckState{LastCheckedAt: now.Add(-6 * 24 * time.Hour)}), "6 days ago is not due yet")
	assert.True(t, ShouldCheck(now, CheckState{LastCheckedAt: now.Add(-7 * 24 * time.Hour)}), "exactly 7 days ago is due")
	assert.True(t, ShouldCheck(now, CheckState{LastCheckedAt: now.Add(-30 * 24 * time.Hour)}), "30 days ago is due")
}
