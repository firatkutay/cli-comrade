package doctor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/update"
)

func TestVersionCheckSkipsDevBuild(t *testing.T) {
	deps := baseDeps()
	deps.Version = "dev"

	result := VersionCheck(context.Background(), deps)

	assert.Equal(t, SeveritySkip, result.Severity)
	assert.Equal(t, i18n.MsgDoctorVersionDevSkip, result.Summary)
}

func TestVersionCheckUpToDate(t *testing.T) {
	deps := baseDeps()
	deps.Version = "v1.0.0"
	deps.Fetcher = fakeFetcher{release: update.Release{TagName: "v1.0.0"}}

	result := VersionCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorVersionUpToDate, result.Summary)
	assert.Equal(t, []any{"v1.0.0"}, result.SummaryArgs)
	assert.Empty(t, result.Fix)
}

func TestVersionCheckBehindWarnsWithUpgradeFix(t *testing.T) {
	deps := baseDeps()
	deps.Version = "v1.0.0"
	deps.Fetcher = fakeFetcher{release: update.Release{TagName: "v1.2.0"}}

	result := VersionCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, i18n.MsgDoctorVersionBehind, result.Summary)
	assert.Equal(t, []any{"v1.2.0", "v1.0.0"}, result.SummaryArgs)
	assert.Equal(t, "comrade upgrade", result.Fix)
}

// TestVersionCheckAgreesWithUpgradeRefusalUnderNPMManagedEnv is HIGH-1's
// regression guard (PR #37 review): `comrade doctor` must never advise
// `comrade upgrade` as the Fix when `comrade upgrade` itself refuses to
// run — both must call the exact same update.IsNPMManaged detection, and
// this pins that agreement directly under COMRADE_MANAGED_BY=npm rather
// than trusting the two call sites to stay in sync by inspection alone.
func TestVersionCheckAgreesWithUpgradeRefusalUnderNPMManagedEnv(t *testing.T) {
	deps := baseDeps()
	deps.Version = "v1.0.0"
	deps.Fetcher = fakeFetcher{release: update.Release{TagName: "v1.2.0"}}
	deps.Getenv = func(name string) string {
		if name == "COMRADE_MANAGED_BY" {
			return "npm"
		}
		return ""
	}

	// Sanity-check the premise: the env var this test sets is genuinely
	// what update.IsNPMManaged (the SAME function comrade upgrade calls)
	// detects, using the exact seams VersionCheck itself reads from deps.
	require.True(t, update.IsNPMManaged(deps.Getenv, deps.Executable, filepath.EvalSymlinks),
		"test setup: COMRADE_MANAGED_BY=npm must be what IsNPMManaged detects")

	result := VersionCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.NotEqual(t, "comrade upgrade", result.Fix,
		"doctor must never recommend a command comrade upgrade itself refuses under a Node-managed install")
	assert.Contains(t, result.Fix, "Node package manager")
}

func TestVersionCheckFetchErrorIsWarnNotFail(t *testing.T) {
	deps := baseDeps()
	deps.Fetcher = fakeFetcher{err: errors.New("network unreachable")}

	result := VersionCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, i18n.MsgDoctorVersionFetchError, result.Summary)
	assert.Contains(t, result.Detail, "network unreachable")
}

// TestVersionCheckWritesStateOnSuccessfulFetch pins VersionCheck's
// documented side effect: a successful fetch (whether up to date or
// behind) writes update_check.json, feeding the SAME passive
// version-update notice every other command's background check does.
func TestVersionCheckWritesStateOnSuccessfulFetch(t *testing.T) {
	dir := t.TempDir()
	deps := baseDeps()
	deps.GOOS = "linux"
	deps.Getenv = func(name string) string {
		if name == "XDG_STATE_HOME" {
			return dir
		}
		return ""
	}
	fixedNow := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	deps.Now = func() time.Time { return fixedNow }
	deps.Fetcher = fakeFetcher{release: update.Release{TagName: "v1.2.0"}}

	_ = VersionCheck(context.Background(), deps)

	statePath, err := update.StatePathFor("linux", deps.Getenv)
	require.NoError(t, err)
	require.FileExists(t, statePath)

	st := update.ReadState(statePath)
	assert.Equal(t, "v1.2.0", st.LatestKnownVersion)
	assert.True(t, fixedNow.Equal(st.LastCheckedAt))
}

// TestVersionCheckDoesNotWriteStateOnFetchError proves a failed fetch
// never writes a MISLEADING "latest known version" — only a successful
// fetch (up to date or behind) updates the state file.
func TestVersionCheckDoesNotWriteStateOnFetchError(t *testing.T) {
	dir := t.TempDir()
	deps := baseDeps()
	deps.Getenv = func(name string) string {
		if name == "XDG_STATE_HOME" {
			return dir
		}
		return ""
	}
	deps.Fetcher = fakeFetcher{err: errors.New("boom")}

	_ = VersionCheck(context.Background(), deps)

	statePath, err := update.StatePathFor("linux", deps.Getenv)
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Dir(statePath))
	assert.True(t, os.IsNotExist(statErr), "no state file's directory should exist after a fetch error")
}
