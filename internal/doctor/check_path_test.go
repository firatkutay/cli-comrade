package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

func TestPathCheckFailsWhenNotOnPath(t *testing.T) {
	deps := baseDeps()
	deps.GOOS = "linux"
	deps.LookPath = func(string) (string, error) { return "", errNotFound{} }

	result := PathCheck(context.Background(), deps)

	assert.Equal(t, SeverityFail, result.Severity)
	assert.Equal(t, i18n.MsgDoctorPathNotFound, result.Summary)
	assert.Equal(t, []any{"comrade"}, result.SummaryArgs)
	assert.Equal(t, pathFixInstallUnix, result.Fix, "Fix must be the bare, copy-pasteable install one-liner, not a prose sentence")
}

// TestPathCheckFixIsBareCommandNotProse is issue #39's core regression
// guard: check_path.go's Fix used to be an English sentence ("re-run
// scripts/install.sh, or add comrade's install directory to your PATH
// manually") — the ONE remaining prose Fix in this package (every other
// check's Fix is already a bare, copy-pasteable command; see the
// issue's own survey). Fix must never contain the tell-tale prose
// fragments of that old sentence, for either the not-found or the
// stale-copy result, on either OS.
func TestPathCheckFixIsBareCommandNotProse(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			deps := baseDeps()
			deps.GOOS = goos
			deps.LookPath = func(string) (string, error) { return "", errNotFound{} }

			result := PathCheck(context.Background(), deps)

			assert.NotContains(t, result.Fix, "re-run", "Fix must not be a prose sentence")
			assert.NotContains(t, result.Fix, "or add", "Fix must not be a prose sentence")
			assert.NotContains(t, result.Fix, "manually", "Fix must not be a prose sentence")
			assert.NotEmpty(t, result.Fix)
		})
	}
}

// TestPathCheckNotFoundSummaryRendersExactTurkishText verifies (issue
// #39's "verify the TR rendering at runtime" requirement) that the
// explanation moved OUT of Fix now lives in the translated Summary
// line, rendered through a real i18n.Translator — not a substring check.
func TestPathCheckNotFoundSummaryRendersExactTurkishText(t *testing.T) {
	deps := baseDeps()
	deps.GOOS = "linux"
	deps.LookPath = func(string) (string, error) { return "", errNotFound{} }

	result := PathCheck(context.Background(), deps)

	tr := i18n.NewTranslator(i18n.LangTR)
	got := tr.T(result.Summary, result.SummaryArgs...)
	want := `"comrade", PATH üzerinde bulunamadı — kurulum betiğini yeniden çalıştırın (aşağıdaki Fix'e bakın) veya comrade'in kurulum dizinini PATH'inize kendiniz ekleyin`
	assert.Equal(t, want, got)
	assert.Equal(t, pathFixInstallUnix, result.Fix)
}

func TestPathCheckUsesWindowsBinaryName(t *testing.T) {
	deps := baseDeps()
	deps.GOOS = "windows"
	var lookedUp string
	deps.LookPath = func(name string) (string, error) { lookedUp = name; return "", errNotFound{} }

	_ = PathCheck(context.Background(), deps)

	assert.Equal(t, "comrade.exe", lookedUp)
}

func TestPathCheckOKWhenLookPathResolvesToRunningBinary(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "comrade")
	require.NoError(t, os.WriteFile(binPath, []byte("bin"), 0o755))

	deps := baseDeps()
	deps.GOOS = "linux"
	deps.LookPath = func(string) (string, error) { return binPath, nil }
	deps.Executable = func() (string, error) { return binPath, nil }

	result := PathCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorPathOK, result.Summary)
	assert.Equal(t, []any{binPath}, result.SummaryArgs)
}

// TestPathCheckWarnsOnStaleCopy is the "found on PATH but a DIFFERENT
// binary than the one running this diagnostic" case — the classic stale
// Homebrew/manual-copy scenario.
func TestPathCheckWarnsOnStaleCopy(t *testing.T) {
	dir := t.TempDir()
	stalePath := filepath.Join(dir, "comrade-stale")
	runningPath := filepath.Join(dir, "comrade-running")
	require.NoError(t, os.WriteFile(stalePath, []byte("old"), 0o755))
	require.NoError(t, os.WriteFile(runningPath, []byte("new"), 0o755))

	deps := baseDeps()
	deps.GOOS = "linux"
	deps.LookPath = func(string) (string, error) { return stalePath, nil }
	deps.Executable = func() (string, error) { return runningPath, nil }

	result := PathCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, i18n.MsgDoctorPathStale, result.Summary)
	assert.Equal(t, []any{stalePath}, result.SummaryArgs)
	assert.Equal(t, pathFixInstallUnix, result.Fix, "Fix must be the bare, copy-pasteable install one-liner, not a prose sentence")
}

// TestPathCheckStaleSummaryRendersExactTurkishText mirrors
// TestPathCheckNotFoundSummaryRendersExactTurkishText for the stale-copy
// case, verified against a real i18n.Translator, exact string.
func TestPathCheckStaleSummaryRendersExactTurkishText(t *testing.T) {
	dir := t.TempDir()
	stalePath := filepath.Join(dir, "comrade-stale")
	runningPath := filepath.Join(dir, "comrade-running")
	require.NoError(t, os.WriteFile(stalePath, []byte("old"), 0o755))
	require.NoError(t, os.WriteFile(runningPath, []byte("new"), 0o755))

	deps := baseDeps()
	deps.GOOS = "windows"
	deps.LookPath = func(string) (string, error) { return stalePath, nil }
	deps.Executable = func() (string, error) { return runningPath, nil }

	result := PathCheck(context.Background(), deps)

	tr := i18n.NewTranslator(i18n.LangTR)
	got := tr.T(result.Summary, result.SummaryArgs...)
	want := `PATH, şu anda çalışan comrade ikili dosyasından farklı bir kopyaya işaret ediyor (` + stalePath + `) — kurulum betiğini yeniden çalıştırın (aşağıdaki Fix'e bakın) veya comrade'in kurulum dizinini PATH'inize kendiniz ekleyin`
	assert.Equal(t, want, got)
	assert.Equal(t, pathFixInstallWindows, result.Fix)
}
