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
	assert.NotEmpty(t, result.Fix)
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
	assert.NotEmpty(t, result.Fix)
}
