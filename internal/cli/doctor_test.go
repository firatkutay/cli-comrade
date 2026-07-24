package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/doctor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/update"
)

// errDoctorTestLookPathNotFound is the fixed error every doctor_test.go
// fake LookPath/Executable returns for "not found", matching
// upgrade_test.go's own small-sentinel-error convention.
var errDoctorTestLookPathNotFound = errors.New("not found")

// testDoctorDeps builds doctorDeps wired entirely to deterministic
// fakes/stubs — no test in this file ever reaches the real network,
// keychain, or a real running executable, matching testUpgradeDeps'
// established shape in upgrade_test.go.
func testDoctorDeps() doctorDeps {
	return doctorDeps{
		version:    "v1.0.0",
		goos:       "linux",
		getenv:     func(string) string { return "" },
		lookPath:   func(string) (string, error) { return "", errDoctorTestLookPathNotFound },
		executable: func() (string, error) { return "", errDoctorTestLookPathNotFound },
		run:        nil,
		fetcher:    fakeReleaseFetcher{release: update.Release{TagName: "v1.0.0"}},
		httpClient: &http.Client{Timeout: 2 * time.Second},
		now:        func() time.Time { return time.Date(2026, 7, 24, 0, 0, 0, 0, time.UTC) },
		livePing: func(context.Context, config.Config, string, string) (llm.CompletionResponse, time.Duration, error) {
			return llm.CompletionResponse{}, 0, nil
		},
	}
}

func execDoctorCmd(t *testing.T, deps doctorDeps, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := newDoctorCmd(newTestLoaderFactory(), deps)
	outBuf := &strings.Builder{}
	errBuf := &strings.Builder{}
	cmd.SetOut(outBuf)
	cmd.SetErr(errBuf)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err = cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// TestDoctorExitsZeroWhenNoCheckFails drives every check to a non-Fail
// outcome (dev build skips version, shellhook skips on an undetected
// shell, an httptest server makes reach/baseurl/key all resolve OK) and
// proves the whole command exits 0 (nil error) — P-1's "warnings are
// non-fatal" rule, exercised end to end through the real command tree
// rather than asserting on internal/doctor's severities directly.
func TestDoctorExitsZeroWhenNoCheckFails(t *testing.T) {
	withIsolatedConfigDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	deps := testDoctorDeps()
	deps.version = "dev"
	deps.lookPath = func(string) (string, error) { return "/usr/local/bin/comrade", nil }
	deps.executable = func() (string, error) { return "/usr/local/bin/comrade", nil }
	deps.getenv = func(name string) string {
		if name == "OPENAI_API_KEY" {
			return "test-key"
		}
		return ""
	}

	stdout, stderr, err := execDoctorCmd(t, deps)

	require.NoError(t, err, "stdout: %s\nstderr: %s", stdout, stderr)
	assert.Contains(t, stdout, "[OK]")
	assert.NotContains(t, stdout, "[FAIL]")
}

// TestDoctorExitsOneWithFixedExitCodeWhenACheckFails forces exactly the
// path check to Fail (LookPath never finds "comrade") while every other
// check resolves to a non-Fail outcome, and proves: (a) RunE returns a
// non-nil error, (b) that error implements ExitCode() int returning 1 —
// P-1's exit-code rule — and (c) the rendered checklist shows [FAIL] for
// exactly that row.
func TestDoctorExitsOneWithFixedExitCodeWhenACheckFails(t *testing.T) {
	withIsolatedConfigDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	deps := testDoctorDeps()
	deps.version = "dev"
	deps.getenv = func(name string) string {
		if name == "OPENAI_API_KEY" {
			return "test-key"
		}
		return ""
	}
	// deps.lookPath keeps its testDoctorDeps default: always "not found".

	stdout, _, err := execDoctorCmd(t, deps)

	require.Error(t, err)
	var ec interface{ ExitCode() int }
	require.ErrorAs(t, err, &ec, "doctorFailedError must implement ExitCode() int")
	assert.Equal(t, 1, ec.ExitCode())
	assert.Contains(t, stdout, "[FAIL]")
}

// TestDoctorJSONOutputsOneSeverityStringObjectPerLine proves --json
// mirrors `comrade history --json`'s one-JSON-object-per-line shape,
// with every result's Severity rendered as its stable lowercase string
// (doctor.Severity.String()) and Summary already resolved to plain text.
func TestDoctorJSONOutputsOneSeverityStringObjectPerLine(t *testing.T) {
	withIsolatedConfigDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	deps := testDoctorDeps()
	deps.version = "dev"
	deps.lookPath = func(string) (string, error) { return "/usr/local/bin/comrade", nil }
	deps.executable = func() (string, error) { return "/usr/local/bin/comrade", nil }
	deps.getenv = func(name string) string {
		if name == "OPENAI_API_KEY" {
			return "test-key"
		}
		return ""
	}

	stdout, _, err := execDoctorCmd(t, deps, "--json")
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	require.Len(t, lines, len(doctor.NewRunner().Checks))

	var sawVersionRow bool
	for _, line := range lines {
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		require.Contains(t, entry, "id")
		require.Contains(t, entry, "severity")
		require.Contains(t, entry, "summary")

		sev, _ := entry["severity"].(string)
		assert.Contains(t, []string{"ok", "warn", "fail", "skip"}, sev)

		if entry["id"] == "version" {
			sawVersionRow = true
			assert.Equal(t, "skip", sev, "dev build must report the version check as skip")
		}
	}
	assert.True(t, sawVersionRow)
}

// TestDoctorNeverCallsLivePingWithoutLiveFlag proves the default-mode
// "never sends a credential anywhere" guarantee end to end through the
// real command tree: LivePing must not be invoked at all unless --live
// is explicitly given.
func TestDoctorNeverCallsLivePingWithoutLiveFlag(t *testing.T) {
	withIsolatedConfigDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	deps := testDoctorDeps()
	deps.version = "dev"
	deps.lookPath = func(string) (string, error) { return "/usr/local/bin/comrade", nil }
	deps.executable = func() (string, error) { return "/usr/local/bin/comrade", nil }
	deps.getenv = func(name string) string {
		if name == "OPENAI_API_KEY" {
			return "test-key"
		}
		return ""
	}
	livePingCalled := false
	deps.livePing = func(context.Context, config.Config, string, string) (llm.CompletionResponse, time.Duration, error) {
		livePingCalled = true
		return llm.CompletionResponse{}, 0, nil
	}

	_, _, err = execDoctorCmd(t, deps)

	require.NoError(t, err)
	assert.False(t, livePingCalled)
}

// TestDoctorLiveFlagInvokesLivePing proves --live actually reaches
// LivePing (the shared pingProviderWithKey path — see llmping.go).
func TestDoctorLiveFlagInvokesLivePing(t *testing.T) {
	withIsolatedConfigDir(t)
	// reachCheckLive's key resolution (internal/doctor.resolveKeyForLive)
	// falls back to llm.ResolveEnvKey, which reads the REAL process
	// environment — not doctorDeps.getenv (that seam only feeds
	// internal/doctor's own KeyCheck) — so this test sets the real env
	// var via t.Setenv rather than deps.getenv.
	t.Setenv("OPENAI_API_KEY", "test-key")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	deps := testDoctorDeps()
	deps.version = "dev"
	deps.lookPath = func(string) (string, error) { return "/usr/local/bin/comrade", nil }
	deps.executable = func() (string, error) { return "/usr/local/bin/comrade", nil }
	deps.getenv = func(name string) string {
		if name == "OPENAI_API_KEY" {
			return "test-key"
		}
		return ""
	}
	livePingCalled := false
	deps.livePing = func(context.Context, config.Config, string, string) (llm.CompletionResponse, time.Duration, error) {
		livePingCalled = true
		return llm.CompletionResponse{Model: "gpt-5.4-mini"}, 5 * time.Millisecond, nil
	}

	stdout, _, err := execDoctorCmd(t, deps, "--live")

	require.NoError(t, err)
	assert.True(t, livePingCalled)
	assert.Contains(t, stdout, "[OK]")
}

// TestDoctorStrayArgShowsTranslatedUsageError proves `comrade doctor`'s
// Args (translatedNoArgs) renders a friendly, i18n'd usage error instead
// of cobra's raw English "accepts 0 arg(s), received 1" — mirrors
// history_test.go's own TestHistoryStrayArgShowsTranslatedUsageError.
func TestDoctorStrayArgShowsTranslatedUsageError(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "doctor", "unexpected")

	require.Error(t, err)
	assert.Equal(t, "comrade doctor does not take any arguments", err.Error())
}

// TestDoctorHelpDescribesJSONAndLiveFlags proves both flags are
// registered and documented.
func TestDoctorHelpDescribesJSONAndLiveFlags(t *testing.T) {
	withIsolatedConfigDir(t)
	deps := testDoctorDeps()

	stdout, _, err := execDoctorCmd(t, deps, "--help")

	require.NoError(t, err)
	assert.Contains(t, stdout, "--json")
	assert.Contains(t, stdout, "--live")
}

// TestDoctorTitlesCoverEveryRegisteredCheck is this package's
// derive-or-guard test for doctorTitles (doctor.go): every check ID
// internal/doctor.NewRunner() actually registers must have a row-title
// mapping here, so a future check added to that registry without a
// matching entry in doctorTitles fails immediately instead of silently
// rendering its own raw, untranslated ID as a fallback title.
func TestDoctorTitlesCoverEveryRegisteredCheck(t *testing.T) {
	for _, c := range doctor.NewRunner().Checks {
		title, ok := doctorTitles[c.ID]
		assert.True(t, ok, "doctorTitles is missing an entry for check ID %q", c.ID)
		assert.NotEqual(t, i18n.MessageID(""), title, "doctorTitles[%q] must not be the empty MessageID", c.ID)
	}
}

// TestDoctorFailedSummaryRendersInTurkish is this feature's TR smoke
// test, matching this project's established per-feature TR-smoke
// convention (see upgrade_test.go's own release-not-found Turkish case).
func TestDoctorFailedSummaryRendersInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	deps := testDoctorDeps()
	deps.version = "dev"
	deps.getenv = func(name string) string {
		if name == "OPENAI_API_KEY" {
			return "test-key"
		}
		return ""
	}
	// lookPath's default (testDoctorDeps) always fails -> path check Fails.

	_, _, err = execDoctorCmd(t, deps)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "kontrol başarısız")
}
