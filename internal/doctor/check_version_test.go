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

// npmManagedTestGetenv is a GetenvFunc-shaped fake reporting
// COMRADE_MANAGED_BY=npm and every other variable unset, shared by every
// npm-managed VersionCheck test in this file.
func npmManagedTestGetenv(name string) string {
	if name == "COMRADE_MANAGED_BY" {
		return "npm"
	}
	return ""
}

// TestVersionCheckAgreesWithUpgradeRefusalUnderNPMManagedEnv is HIGH-1's
// regression guard (PR #37 review): `comrade doctor` must never advise
// the bare `comrade upgrade` as the Fix when `comrade upgrade` itself
// refuses to run — both must call the exact same update.IsNPMManaged
// detection, and this pins that agreement directly under
// COMRADE_MANAGED_BY=npm rather than trusting the two call sites to stay
// in sync by inspection alone.
//
// Per P2 (PR #37 review's second pass): the Node-managed caveat lives in
// the TRANSLATED Summary (MsgDoctorVersionBehindNodeManaged), not in
// Fix — Fix itself stays a bare, copy-pasteable shell command exactly
// like the non-managed case, per doctor.Result.Fix's own "a shell
// command, not prose" contract (matching the existing `ollama pull
// llama3.1` precedent). This test asserts BOTH halves: Fix is the exact
// npm command (never the bare "comrade upgrade" the real CLI refuses),
// AND Summary is the dedicated, non-misleading MessageID rather than
// the plain MsgDoctorVersionBehind.
func TestVersionCheckAgreesWithUpgradeRefusalUnderNPMManagedEnv(t *testing.T) {
	deps := baseDeps()
	deps.Version = "v1.0.0"
	deps.Fetcher = fakeFetcher{release: update.Release{TagName: "v1.2.0"}}
	deps.Getenv = npmManagedTestGetenv

	// Sanity-check the premise: the env var this test sets is genuinely
	// what update.IsNPMManaged (the SAME function comrade upgrade calls)
	// detects, using the exact seams VersionCheck itself reads from deps.
	require.True(t, update.IsNPMManaged(deps.Getenv, deps.Executable, filepath.EvalSymlinks),
		"test setup: COMRADE_MANAGED_BY=npm must be what IsNPMManaged detects")

	result := VersionCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, "npm update -g cli-comrade", result.Fix,
		"Fix must be the bare, copy-pasteable command — never the plain \"comrade upgrade\" comrade upgrade itself refuses under a Node-managed install")
	assert.Equal(t, i18n.MsgDoctorVersionBehindNodeManaged, result.Summary,
		"Summary must be the dedicated Node-managed MessageID, not the plain MsgDoctorVersionBehind, so the caveat is actually translated")
	assert.Equal(t, []any{"v1.2.0", "v1.0.0"}, result.SummaryArgs)
}

// TestVersionCheckBehindNodeManagedSummaryRendersInTurkish is P2's
// "verify the TR rendering at runtime" requirement: it actually
// constructs an i18n.Translator(LangTR) and renders the exact
// MessageID/SummaryArgs pair VersionCheck returned, rather than merely
// eyeballing the catalog string — proving the Turkish translation
// interpolates correctly (this project's established per-feature
// TR-smoke convention; see upgrade_test.go's own release-not-found
// Turkish case).
func TestVersionCheckBehindNodeManagedSummaryRendersInTurkish(t *testing.T) {
	deps := baseDeps()
	deps.Version = "v1.0.0"
	deps.Fetcher = fakeFetcher{release: update.Release{TagName: "v1.2.0"}}
	deps.Getenv = npmManagedTestGetenv

	result := VersionCheck(context.Background(), deps)

	tr := i18n.NewTranslator(i18n.LangTR)
	rendered := tr.T(result.Summary, result.SummaryArgs...)

	assert.Equal(t,
		"daha yeni bir sürüm mevcut: v1.2.0 (mevcut sürümünüz: v1.0.0) — comrade bir Node paket yöneticisiyle (ör. npm, pnpm, yarn, bun) kuruldu; bunun yerine o paket yöneticisiyle güncelleyin (örnek olarak npm gösterilmiştir)",
		rendered)
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
