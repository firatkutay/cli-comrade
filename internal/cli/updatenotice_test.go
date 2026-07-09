package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/update"
)

// execRootWithFetcher runs newRootCmd directly (not the public
// NewRootCmd) so the passive version-notification hook is wired to
// fetcher instead of a real update.GitHubClient — no test in this file
// ever reaches the real network.
func execRootWithFetcher(t *testing.T, version string, fetcher update.ReleaseFetcher, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := newRootCmd(version, fetcher)
	var outBuf, errBuf strings.Builder
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func updateCheckStatePath(t *testing.T, dir string) string {
	t.Helper()
	return filepath.Join(dir, "cli-comrade", "update_check.json")
}

// TestUpdateNoticeSkipsBareInvocation proves a true bare `comrade` (no
// args) never even attempts a background check.
func TestUpdateNoticeSkipsBareInvocation(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	fetcher := fakeReleaseFetcher{release: update.Release{TagName: "v9.9.9"}}

	out, _, err := execRootWithFetcher(t, "1.2.3", fetcher)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(out, "comrade version 1.2.3"))

	_, statErr := os.Stat(updateCheckStatePath(t, dir))
	assert.True(t, os.IsNotExist(statErr), "bare invocation must never touch update_check.json")
}

// TestUpdateNoticeSkipsUpgradeCommand proves `comrade upgrade` itself
// never triggers the passive notice.
func TestUpdateNoticeSkipsUpgradeCommand(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	fetcher := fakeReleaseFetcher{release: update.Release{TagName: "v9.9.9"}}

	// "dev" refuses inside upgrade's own RunE regardless, but the point
	// here is that PersistentPostRunE's own upgrade-name skip exists —
	// use a real version so RunE would otherwise succeed and reach
	// PersistentPostRunE.
	_, _, _ = execRootWithFetcher(t, "1.2.3", fetcher, "upgrade", "--check")

	_, statErr := os.Stat(updateCheckStatePath(t, dir))
	assert.True(t, os.IsNotExist(statErr))
}

// TestUpdateNoticeSkipsDevBuild proves a successful, non-bare command on
// a "dev" build never performs the background check.
func TestUpdateNoticeSkipsDevBuild(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	fetcher := fakeReleaseFetcher{release: update.Release{TagName: "v9.9.9"}}

	out, _, err := execRootWithFetcher(t, "dev", fetcher, "config", "path")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	_, statErr := os.Stat(updateCheckStatePath(t, dir))
	assert.True(t, os.IsNotExist(statErr))
}

// TestUpdateNoticeHonorsUpdateCheckDisabled proves
// general.update_check=false skips the background check entirely.
func TestUpdateNoticeHonorsUpdateCheckDisabled(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	loader, err := newTestLoaderFactory()()
	require.NoError(t, err)
	_, _, err = loader.Load()
	require.NoError(t, err)
	require.NoError(t, loader.SetAndSave("general.update_check", false))

	fetcher := fakeReleaseFetcher{release: update.Release{TagName: "v9.9.9"}}
	out, _, err := execRootWithFetcher(t, "1.2.3", fetcher, "config", "path")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	_, statErr := os.Stat(updateCheckStatePath(t, dir))
	assert.True(t, os.IsNotExist(statErr), "update_check=false must skip the background check entirely")
}

// TestUpdateNoticePrintsWhenNewerAvailable is the full successful path:
// due for a check, update_check enabled, a real (non-dev) version, and
// the fake fetcher reports a newer release — the notice must be printed
// to stderr and the state file must record the observed version.
func TestUpdateNoticePrintsWhenNewerAvailable(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	fetcher := fakeReleaseFetcher{release: update.Release{TagName: "v2.0.0"}}

	out, errOut, err := execRootWithFetcher(t, "v1.0.0", fetcher, "config", "path")
	require.NoError(t, err)
	assert.NotEmpty(t, out)
	assert.Contains(t, errOut, "v2.0.0")
	assert.Contains(t, errOut, "v1.0.0")
	assert.Contains(t, errOut, "comrade upgrade")

	got := update.ReadState(updateCheckStatePath(t, dir))
	assert.Equal(t, "v2.0.0", got.LatestKnownVersion)
	assert.False(t, got.LastCheckedAt.IsZero())
}

// TestUpdateNoticeSilentWhenAlreadyUpToDate proves the up-to-date case
// prints nothing while still persisting state (so the throttle takes
// effect regardless of outcome).
func TestUpdateNoticeSilentWhenAlreadyUpToDate(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	fetcher := fakeReleaseFetcher{release: update.Release{TagName: "v1.0.0"}}

	_, errOut, err := execRootWithFetcher(t, "v1.0.0", fetcher, "config", "path")
	require.NoError(t, err)
	assert.Empty(t, errOut)

	got := update.ReadState(updateCheckStatePath(t, dir))
	assert.Equal(t, "v1.0.0", got.LatestKnownVersion)
}

// TestUpdateNoticeSilentOnFetchFailure proves a failed background check
// (offline, API error, ...) is completely silent — no error surfaces to
// the user, and the command's own success is unaffected — while still
// persisting the attempt's timestamp so an offline machine is throttled
// to one attempt per CheckInterval instead of retrying every command.
func TestUpdateNoticeSilentOnFetchFailure(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	fetcher := fakeReleaseFetcher{err: errors.New("network down")}

	out, errOut, err := execRootWithFetcher(t, "v1.0.0", fetcher, "config", "path")
	require.NoError(t, err, "a background-check failure must never fail the command")
	assert.NotEmpty(t, out)
	assert.Empty(t, errOut, "a background-check failure must never print anything")

	statePath := updateCheckStatePath(t, dir)
	_, statErr := os.Stat(statePath)
	require.NoError(t, statErr, "a failed attempt must still be persisted so it's throttled to once per week")
	got := update.ReadState(statePath)
	assert.Empty(t, got.LatestKnownVersion, "no version was actually observed on a failed fetch")
}

// TestUpdateNoticeSkipsWhenRecentlyChecked proves ShouldCheck's weekly
// throttle is honored end-to-end: pre-seeding update_check.json with a
// very recent timestamp must leave it untouched and never call the
// fetcher (a fetcher call would flip fetcherCalled to true).
func TestUpdateNoticeSkipsWhenRecentlyChecked(t *testing.T) {
	withIsolatedConfigDir(t)

	statePath, err := update.DefaultStatePath()
	require.NoError(t, err)
	seeded := update.CheckState{LastCheckedAt: time.Now().Add(-time.Hour), LatestKnownVersion: "v0.0.1"}
	require.NoError(t, update.WriteState(statePath, seeded))

	fetcherCalled := false
	fetcher := countingFetcher{called: &fetcherCalled, release: update.Release{TagName: "v9.9.9"}}

	_, errOut, err := execRootWithFetcher(t, "v1.0.0", fetcher, "config", "path")
	require.NoError(t, err)
	assert.Empty(t, errOut)
	assert.False(t, fetcherCalled, "a recently-checked state must skip the network call entirely")

	got := update.ReadState(statePath)
	assert.Equal(t, "v0.0.1", got.LatestKnownVersion, "a recent check must not be overwritten")
}

// countingFetcher wraps fakeReleaseFetcher's behavior while recording
// whether LatestRelease was ever actually invoked.
type countingFetcher struct {
	called  *bool
	release update.Release
}

func (f countingFetcher) LatestRelease(context.Context) (update.Release, error) {
	*f.called = true
	return f.release, nil
}
