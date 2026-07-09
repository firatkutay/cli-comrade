package cli

import (
	"bytes"
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

// testInitDeps builds an initDeps that never touches the real OS: goos
// and getenv are fully controlled, and lookPath/run always fail (so a
// PowerShell RCPath resolution attempt deterministically falls into its
// "not found" branch unless a test overrides them).
func testInitDeps(goos string, env map[string]string) initDeps {
	return initDeps{
		goos:   goos,
		getenv: func(k string) string { return env[k] },
		lookPath: func(string) (string, error) {
			return "", errors.New("not found")
		},
		run: func(stdctx.Context, string, ...string) ([]byte, error) {
			return nil, errors.New("not available")
		},
	}
}

// execInitCmd runs "comrade init" (via its own cobra command, not the
// full root tree — hook.go/config.go etc. are irrelevant here) with
// args and stdin, returning combined stdout+stderr output.
func execInitCmd(t *testing.T, deps initDeps, stdin string, args ...string) string {
	t.Helper()
	cmd := newInitCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	cmd.SetContext(stdctx.Background())
	err := cmd.Execute()
	require.NoError(t, err)
	return buf.String()
}

// execInitCmdErr is execInitCmd for the error-returning cases.
func execInitCmdErr(t *testing.T, deps initDeps, args ...string) error {
	t.Helper()
	cmd := newInitCmd(deps)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	cmd.SetContext(stdctx.Background())
	return cmd.Execute()
}

func TestInitPrintOnlyPrintsSnippetWithoutTouchingAnyFile(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	out := execInitCmd(t, deps, "", "bash", "--print")

	block, err := shellinit.Block(shellinit.Bash)
	require.NoError(t, err)
	assert.Equal(t, block+"\n", out)

	_, statErr := os.Stat(filepath.Join(dir, ".bashrc"))
	assert.True(t, os.IsNotExist(statErr), "expected .bashrc to not be created by --print")
}

func TestInitPrintAndRemoveAreMutuallyExclusive(t *testing.T) {
	deps := testInitDeps("linux", map[string]string{"HOME": t.TempDir()})
	err := execInitCmdErr(t, deps, "bash", "--print", "--remove")
	assert.ErrorContains(t, err, "mutually exclusive")
}

func TestInitUnsupportedShellArgErrors(t *testing.T) {
	deps := testInitDeps("linux", map[string]string{"HOME": t.TempDir()})
	err := execInitCmdErr(t, deps, "tcsh")
	assert.ErrorContains(t, err, "tcsh")
}

func TestInitNoArgDetectsShellFromEnv(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir, "SHELL": "/usr/bin/zsh"})

	out := execInitCmd(t, deps, "", "--print")

	block, err := shellinit.Block(shellinit.Zsh)
	require.NoError(t, err)
	assert.Equal(t, block+"\n", out)
}

func TestInitNoArgErrorsWhenShellCannotBeDetected(t *testing.T) {
	deps := testInitDeps("linux", map[string]string{"HOME": t.TempDir()}) // no SHELL set
	err := execInitCmdErr(t, deps)
	assert.ErrorContains(t, err, "could not detect")
}

func TestInitNoArgErrorsWhenDetectedShellUnsupported(t *testing.T) {
	// windows without PSModulePath detects as "cmd", which init does not support.
	deps := testInitDeps("windows", map[string]string{})
	err := execInitCmdErr(t, deps)
	assert.ErrorContains(t, err, "not supported")
}

func TestInitInstallsIntoFreshRCFileWithConfirmation(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	out := execInitCmd(t, deps, "y\n", "bash")

	assert.Contains(t, out, "The following will be added to")
	assert.Contains(t, out, "Installed cli-comrade shell integration in")

	data, err := os.ReadFile(filepath.Join(dir, ".bashrc"))
	require.NoError(t, err)
	block, err := shellinit.Block(shellinit.Bash)
	require.NoError(t, err)
	assert.Equal(t, block+"\n", string(data))
}

func TestInitDeclinedConfirmationMakesNoChanges(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	out := execInitCmd(t, deps, "n\n", "bash")

	assert.Contains(t, out, "Aborted; no changes made.")
	_, err := os.Stat(filepath.Join(dir, ".bashrc"))
	assert.True(t, os.IsNotExist(err))
}

func TestInitYesFlagSkipsConfirmationPrompt(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	// Empty stdin: if the command tried to read a confirmation line, a
	// naive implementation would still see EOF-as-false; --yes must
	// avoid reading stdin at all and install unconditionally.
	out := execInitCmd(t, deps, "", "bash", "--yes")

	assert.Contains(t, out, "Installed cli-comrade shell integration in")
	_, err := os.Stat(filepath.Join(dir, ".bashrc"))
	assert.NoError(t, err)
}

func TestInitSecondRunReportsAlreadyInstalled(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	execInitCmd(t, deps, "", "bash", "--yes")
	before, err := os.ReadFile(filepath.Join(dir, ".bashrc"))
	require.NoError(t, err)

	out := execInitCmd(t, deps, "", "bash", "--yes")
	assert.Contains(t, out, "already installed")

	after, err := os.ReadFile(filepath.Join(dir, ".bashrc"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after))
}

func TestInitUpgradesOlderBlockInPlace(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")
	oldBlock := shellinit.MarkerBegin + "\n# an old cli-comrade hook body\n" + shellinit.MarkerEnd
	require.NoError(t, os.WriteFile(rcPath, []byte("# my rc\n"+oldBlock+"\n"), 0o644))

	deps := testInitDeps("linux", map[string]string{"HOME": dir})
	out := execInitCmd(t, deps, "", "bash", "--yes")
	assert.Contains(t, out, "Installed cli-comrade shell integration in")

	data, err := os.ReadFile(rcPath)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), shellinit.MarkerBegin))
	assert.NotContains(t, string(data), "an old cli-comrade hook body")
}

func TestInitRemoveDeletesInstalledBlock(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	execInitCmd(t, deps, "", "bash", "--yes")

	out := execInitCmd(t, deps, "", "bash", "--remove")
	assert.Contains(t, out, "Removed cli-comrade shell integration from")

	data, err := os.ReadFile(filepath.Join(dir, ".bashrc"))
	require.NoError(t, err)
	assert.Equal(t, "", string(data))
}

func TestInitRemoveWithNoMarkersIsFriendlyNoop(t *testing.T) {
	dir := t.TempDir()
	rcPath := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(rcPath, []byte("# untouched rc\n"), 0o644))

	deps := testInitDeps("linux", map[string]string{"HOME": dir})
	out := execInitCmd(t, deps, "", "bash", "--remove")
	assert.Contains(t, out, "nothing to do")

	data, err := os.ReadFile(rcPath)
	require.NoError(t, err)
	assert.Equal(t, "# untouched rc\n", string(data))
}

func TestInitPowerShellFallsBackToPrintingWhenProfileCannotBeResolved(t *testing.T) {
	// testInitDeps' lookPath always fails, so RCPath's PowerShell branch
	// cannot resolve $PROFILE — this exercises the "keep honest" fallback.
	deps := testInitDeps("linux", map[string]string{})
	out := execInitCmd(t, deps, "", "powershell", "--yes")

	assert.Contains(t, out, "Could not automatically locate a profile file")
	assert.Contains(t, out, shellinit.MarkerBegin)
}
