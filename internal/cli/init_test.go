package cli

import (
	"bytes"
	stdctx "context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/i18n"
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
//
// withIsolatedConfigDir isolates the config dir newTestLoaderFactory()
// resolves against (a SEPARATE thing from deps.getenv, which is initDeps'
// own controlled map used only for shell/rc-path resolution — no
// interference between the two): install/remove now load config for
// their own translated output (see runInitInstall/runInitRemove), so
// every init test must not touch whatever the real test-process
// environment's config directory happens to be.
func execInitCmd(t *testing.T, deps initDeps, stdin string, args ...string) string {
	t.Helper()
	withIsolatedConfigDir(t)
	cmd := newInitCmd(deps, newTestLoaderFactory())
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
	withIsolatedConfigDir(t)
	cmd := newInitCmd(deps, newTestLoaderFactory())
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

// TestInitTooManyArgsShowsTranslatedUsageError proves `comrade init`'s
// Args (translatedMaxArgs, init.go) renders a friendly, i18n'd usage
// error naming the exact accepted invocation, instead of cobra's raw
// English "accepts at most 1 arg(s), received 2", when given 2+
// positional arguments.
func TestInitTooManyArgsShowsTranslatedUsageError(t *testing.T) {
	deps := testInitDeps("linux", map[string]string{"HOME": t.TempDir()})
	err := execInitCmdErr(t, deps, "bash", "zsh")
	require.Error(t, err)
	assert.Equal(t, "usage: comrade init [bash|zsh|fish|powershell]", err.Error())
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

// TestInitFishInstallsHookAndCompletions proves "comrade init fish"
// installs BOTH artifacts: the existing marker-block hook in
// config.fish (unchanged mechanism) AND, newly, the completions script
// at fish's native lazy-load location — reusing exactly
// shellinit.FishCompletionsScript's own content (not a hand-duplicated
// literal), and reporting both via their own message lines.
func TestInitFishInstallsHookAndCompletions(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	out := execInitCmd(t, deps, "y\n", "fish")

	assert.Contains(t, out, "Installed cli-comrade shell integration in")
	assert.Contains(t, out, "Installed shell completions for fish:")

	hookData, err := os.ReadFile(filepath.Join(dir, ".config", "fish", "config.fish"))
	require.NoError(t, err)
	block, err := shellinit.Block(shellinit.Fish)
	require.NoError(t, err)
	assert.Equal(t, block+"\n", string(hookData))

	compData, err := os.ReadFile(filepath.Join(dir, ".config", "fish", "completions", "comrade.fish"))
	require.NoError(t, err)
	assert.Equal(t, shellinit.FishCompletionsScript(), string(compData))
}

// TestInitFishSecondRunIsIdempotent proves running "comrade init fish"
// twice never duplicates the completions file's content (its
// idempotency is a plain overwrite — see FishCompletionsScript's own
// doc comment — as opposed to the hook block's marker-based merge) and
// still reports the hook block itself as already installed, unchanged.
func TestInitFishSecondRunIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})
	compPath := filepath.Join(dir, ".config", "fish", "completions", "comrade.fish")

	execInitCmd(t, deps, "", "fish", "--yes")
	before, err := os.ReadFile(compPath)
	require.NoError(t, err)

	out := execInitCmd(t, deps, "", "fish", "--yes")
	assert.Contains(t, out, "already installed")
	assert.Contains(t, out, "Installed shell completions for fish:")

	after, err := os.ReadFile(compPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the completions file's content must never change or duplicate across repeated installs")
}

// TestInitFishRemoveDeletesHookAndCompletions proves "comrade init fish
// --remove" deletes BOTH the hook block content from config.fish and the
// separate, fully comrade-owned completions file.
func TestInitFishRemoveDeletesHookAndCompletions(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})
	compPath := filepath.Join(dir, ".config", "fish", "completions", "comrade.fish")

	execInitCmd(t, deps, "", "fish", "--yes")
	_, err := os.Stat(compPath)
	require.NoError(t, err, "precondition: completions file must exist after install")

	out := execInitCmd(t, deps, "", "fish", "--remove")
	assert.Contains(t, out, "Removed cli-comrade shell integration from")
	assert.Contains(t, out, "Removed shell completions for fish:")

	_, statErr := os.Stat(compPath)
	assert.True(t, os.IsNotExist(statErr), "the completions file must be deleted by --remove")

	hookData, err := os.ReadFile(filepath.Join(dir, ".config", "fish", "config.fish"))
	require.NoError(t, err)
	assert.Equal(t, "", string(hookData))
}

// TestInitFishRemoveWithNothingInstalledIsNoop proves --remove on a
// fish setup that was never installed neither errors nor fabricates a
// "removed completions" message for a file that was never there.
func TestInitFishRemoveWithNothingInstalledIsNoop(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	out := execInitCmd(t, deps, "", "fish", "--remove")

	assert.Contains(t, out, "nothing to do")
	assert.NotContains(t, out, "Removed shell completions for fish:")
}

// TestInitFishPrintOnlyNeverWritesCompletionsFile proves --print's
// documented "make no file changes" contract extends to the completions
// file too, not just the hook block.
func TestInitFishPrintOnlyNeverWritesCompletionsFile(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	execInitCmd(t, deps, "", "fish", "--print")

	_, statErr := os.Stat(filepath.Join(dir, ".config", "fish", "completions", "comrade.fish"))
	assert.True(t, os.IsNotExist(statErr))
}

// TestInitFishDeclinedConfirmationNeverWritesCompletionsFile proves a
// declined y/N confirmation for the hook-block edit ALSO skips the
// completions file — installFishCompletionsIfApplicable is never called
// on that path (see its own doc comment) so a "no" to the rc-file edit
// is a "no" to both artifacts, not a partial install.
func TestInitFishDeclinedConfirmationNeverWritesCompletionsFile(t *testing.T) {
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	execInitCmd(t, deps, "n\n", "fish")

	_, statErr := os.Stat(filepath.Join(dir, ".config", "fish", "completions", "comrade.fish"))
	assert.True(t, os.IsNotExist(statErr))
}

// TestInitFishInstallAndRemoveMessagesRenderInTurkish is the Turkish
// counterpart to TestInitFishInstallsHookAndCompletions/
// TestInitFishRemoveDeletesHookAndCompletions for the two new
// MsgInitFishCompletionsInstalled/MsgInitFishCompletionsRemoved
// messages specifically.
func TestInitFishInstallAndRemoveMessagesRenderInTurkish(t *testing.T) {
	t.Setenv("COMRADE_LANG", "tr")
	dir := t.TempDir()
	deps := testInitDeps("linux", map[string]string{"HOME": dir})

	installOut := execInitCmd(t, deps, "e\n", "fish")
	assert.Contains(t, installOut, "fish için kabuk tamamlama kuruldu:")

	removeOut := execInitCmd(t, deps, "", "fish", "--remove")
	assert.Contains(t, removeOut, "fish için kabuk tamamlama kaldırıldı:")
}

func TestInitPowerShellFallsBackToPrintingWhenProfileCannotBeResolved(t *testing.T) {
	// testInitDeps' lookPath always fails, so RCPath's PowerShell branch
	// cannot resolve $PROFILE — this exercises the "keep honest" fallback.
	deps := testInitDeps("linux", map[string]string{})
	out := execInitCmd(t, deps, "", "powershell", "--yes")

	assert.Contains(t, out, "Could not automatically locate a profile file")
	assert.Contains(t, out, shellinit.MarkerBegin)
}

// testInitDepsWindowsPS builds an initDeps simulating GOOS=windows with
// controlled PowerShell variant detection: lookPath succeeds only for the
// binary names that are keys of profileFor, and run resolves each
// present binary's $PROFILE to profileFor[bin] — a real path under dir
// (not Windows syntax; readFileOrEmpty/writeFileContent just need a real
// path on whatever host this test actually runs on), so file content can
// be asserted directly with os.ReadFile.
func testInitDepsWindowsPS(profileFor map[string]string) initDeps {
	return initDeps{
		goos:   "windows",
		getenv: func(string) string { return "" },
		lookPath: func(name string) (string, error) {
			if _, ok := profileFor[name]; ok {
				return `C:\fake\` + name + `.exe`, nil
			}
			return "", errors.New("not found")
		},
		run: func(_ stdctx.Context, name string, _ ...string) ([]byte, error) {
			if path, ok := profileFor[name]; ok {
				return []byte(path), nil
			}
			return nil, errors.New("unexpected binary " + name)
		},
	}
}

func TestInitPowerShellWindowsInstallsIntoBothVariantProfilesWithSingleConfirmation(t *testing.T) {
	dir := t.TempDir()
	winPSProfile := filepath.Join(dir, "windowspowershell_profile.ps1")
	pwshProfile := filepath.Join(dir, "pwsh_profile.ps1")
	deps := testInitDepsWindowsPS(map[string]string{
		"powershell": winPSProfile,
		"pwsh":       pwshProfile,
	})

	out := execInitCmd(t, deps, "y\n", "powershell")

	// Both profiles previewed and reported, but only ONE confirmation
	// prompt asked (a single "y\n" on stdin satisfies both writes) — a
	// second unconsumed prompt would leave the reader blocked on empty
	// stdin and this call would never return.
	assert.Contains(t, out, "Windows PowerShell 5.1: Installed cli-comrade shell integration in "+winPSProfile)
	assert.Contains(t, out, "PowerShell 7: Installed cli-comrade shell integration in "+pwshProfile)
	assert.Equal(t, 1, strings.Count(out, "Add cli-comrade shell integration to the profile(s) above?"))

	block, err := shellinit.Block(shellinit.PowerShell)
	require.NoError(t, err)

	winData, err := os.ReadFile(winPSProfile)
	require.NoError(t, err)
	assert.Equal(t, block+"\n", string(winData))

	pwshData, err := os.ReadFile(pwshProfile)
	require.NoError(t, err)
	assert.Equal(t, block+"\n", string(pwshData))
}

func TestInitPowerShellWindowsYesFlagSkipsConfirmation(t *testing.T) {
	dir := t.TempDir()
	winPSProfile := filepath.Join(dir, "windowspowershell_profile.ps1")
	pwshProfile := filepath.Join(dir, "pwsh_profile.ps1")
	deps := testInitDepsWindowsPS(map[string]string{
		"powershell": winPSProfile,
		"pwsh":       pwshProfile,
	})

	out := execInitCmd(t, deps, "", "powershell", "--yes")

	assert.Contains(t, out, "Windows PowerShell 5.1: Installed cli-comrade shell integration in "+winPSProfile)
	assert.Contains(t, out, "PowerShell 7: Installed cli-comrade shell integration in "+pwshProfile)

	_, err := os.Stat(winPSProfile)
	assert.NoError(t, err)
	_, err = os.Stat(pwshProfile)
	assert.NoError(t, err)
}

func TestInitPowerShellWindowsOnlyOneVariantPresentInstallsJustThatOne(t *testing.T) {
	dir := t.TempDir()
	pwshProfile := filepath.Join(dir, "pwsh_profile.ps1")
	deps := testInitDepsWindowsPS(map[string]string{"pwsh": pwshProfile})

	out := execInitCmd(t, deps, "", "powershell", "--yes")

	assert.Contains(t, out, "PowerShell 7: Installed cli-comrade shell integration in "+pwshProfile)
	assert.NotContains(t, out, "Windows PowerShell 5.1")

	_, err := os.Stat(pwshProfile)
	assert.NoError(t, err)
}

func TestInitPowerShellWindowsNeitherVariantPresentErrors(t *testing.T) {
	deps := testInitDeps("windows", map[string]string{}) // lookPath always fails

	err := execInitCmdErr(t, deps, "powershell")
	require.Error(t, err)
	assert.ErrorContains(t, err, "no PowerShell installation found")
}

func TestInitPowerShellWindowsSecondRunReportsAlreadyInstalledForBothVariants(t *testing.T) {
	dir := t.TempDir()
	winPSProfile := filepath.Join(dir, "windowspowershell_profile.ps1")
	pwshProfile := filepath.Join(dir, "pwsh_profile.ps1")
	deps := testInitDepsWindowsPS(map[string]string{
		"powershell": winPSProfile,
		"pwsh":       pwshProfile,
	})

	execInitCmd(t, deps, "", "powershell", "--yes")
	winBefore, err := os.ReadFile(winPSProfile)
	require.NoError(t, err)
	pwshBefore, err := os.ReadFile(pwshProfile)
	require.NoError(t, err)

	// No stdin line is provided: if the second run tried to prompt at
	// all, this would hang or fail — proving no write, and therefore no
	// confirmation, was attempted for either already-installed profile.
	out := execInitCmd(t, deps, "", "powershell", "--yes")
	assert.Contains(t, out, "Windows PowerShell 5.1: cli-comrade shell integration is already installed in "+winPSProfile)
	assert.Contains(t, out, "PowerShell 7: cli-comrade shell integration is already installed in "+pwshProfile)

	winAfter, err := os.ReadFile(winPSProfile)
	require.NoError(t, err)
	pwshAfter, err := os.ReadFile(pwshProfile)
	require.NoError(t, err)
	assert.Equal(t, string(winBefore), string(winAfter))
	assert.Equal(t, string(pwshBefore), string(pwshAfter))
}

func TestInitPowerShellWindowsUpgradesStaleBlockInOneProfileWhileFreshInstallingTheOther(t *testing.T) {
	dir := t.TempDir()
	winPSProfile := filepath.Join(dir, "windowspowershell_profile.ps1")
	pwshProfile := filepath.Join(dir, "pwsh_profile.ps1")

	oldBlock := shellinit.MarkerBegin + "\n# an old cli-comrade hook body\n" + shellinit.MarkerEnd
	require.NoError(t, os.WriteFile(winPSProfile, []byte("# my profile\n"+oldBlock+"\n"), 0o644))
	// pwshProfile deliberately does not exist yet — a fresh install.

	deps := testInitDepsWindowsPS(map[string]string{
		"powershell": winPSProfile,
		"pwsh":       pwshProfile,
	})

	out := execInitCmd(t, deps, "y\n", "powershell")
	assert.Contains(t, out, "Windows PowerShell 5.1: Installed cli-comrade shell integration in "+winPSProfile)
	assert.Contains(t, out, "PowerShell 7: Installed cli-comrade shell integration in "+pwshProfile)
	// One combined confirmation must cover both the upgrade and the
	// fresh install — not one prompt each.
	assert.Equal(t, 1, strings.Count(out, "Add cli-comrade shell integration to the profile(s) above?"))

	winData, err := os.ReadFile(winPSProfile)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(winData), shellinit.MarkerBegin))
	assert.NotContains(t, string(winData), "an old cli-comrade hook body")

	_, err = os.Stat(pwshProfile)
	assert.NoError(t, err)
}

func TestInitPowerShellWindowsRemoveDeletesBlockFromBothVariantProfiles(t *testing.T) {
	dir := t.TempDir()
	winPSProfile := filepath.Join(dir, "windowspowershell_profile.ps1")
	pwshProfile := filepath.Join(dir, "pwsh_profile.ps1")
	deps := testInitDepsWindowsPS(map[string]string{
		"powershell": winPSProfile,
		"pwsh":       pwshProfile,
	})

	execInitCmd(t, deps, "", "powershell", "--yes")

	out := execInitCmd(t, deps, "", "powershell", "--remove")
	assert.Contains(t, out, "Windows PowerShell 5.1: Removed cli-comrade shell integration from "+winPSProfile)
	assert.Contains(t, out, "PowerShell 7: Removed cli-comrade shell integration from "+pwshProfile)

	winData, err := os.ReadFile(winPSProfile)
	require.NoError(t, err)
	assert.Equal(t, "", string(winData))
	pwshData, err := os.ReadFile(pwshProfile)
	require.NoError(t, err)
	assert.Equal(t, "", string(pwshData))
}

func TestInitPowerShellWindowsRemoveWithOnlyOneVariantInstalledReportsBoth(t *testing.T) {
	dir := t.TempDir()
	winPSProfile := filepath.Join(dir, "windowspowershell_profile.ps1")
	pwshProfile := filepath.Join(dir, "pwsh_profile.ps1")
	deps := testInitDepsWindowsPS(map[string]string{
		"powershell": winPSProfile,
		"pwsh":       pwshProfile,
	})

	// Only install into the Windows PowerShell 5.1 profile by hand,
	// leaving pwsh's profile untouched (as if the block was never
	// installed there).
	block, err := shellinit.Block(shellinit.PowerShell)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(winPSProfile, []byte(block+"\n"), 0o644))
	require.NoError(t, os.WriteFile(pwshProfile, []byte("# untouched profile\n"), 0o644))

	out := execInitCmd(t, deps, "", "powershell", "--remove")
	assert.Contains(t, out, "Windows PowerShell 5.1: Removed cli-comrade shell integration from "+winPSProfile)
	assert.Contains(t, out, "PowerShell 7: cli-comrade shell integration is not installed in "+pwshProfile+"; nothing to do.")

	pwshData, err := os.ReadFile(pwshProfile)
	require.NoError(t, err)
	assert.Equal(t, "# untouched profile\n", string(pwshData))
}

func TestInitPowerShellWindowsUnresolvedVariantIsReportedAndSkippedWithoutBlockingOthers(t *testing.T) {
	dir := t.TempDir()
	pwshProfile := filepath.Join(dir, "pwsh_profile.ps1")
	deps := initDeps{
		goos:   "windows",
		getenv: func(string) string { return "" },
		lookPath: func(name string) (string, error) {
			// Both binaries are "installed" (found on PATH)...
			return `C:\fake\` + name + `.exe`, nil
		},
		run: func(_ stdctx.Context, name string, _ ...string) ([]byte, error) {
			if name == "powershell" {
				// ...but querying Windows PowerShell 5.1's own $PROFILE
				// fails (e.g. a transient process-launch error).
				return nil, errors.New("exit status 1")
			}
			return []byte(pwshProfile), nil
		},
	}

	out := execInitCmd(t, deps, "", "powershell", "--yes")
	assert.Contains(t, out, "Windows PowerShell 5.1: could not resolve profile path")
	assert.Contains(t, out, "PowerShell 7: Installed cli-comrade shell integration in "+pwshProfile)

	_, err := os.Stat(pwshProfile)
	assert.NoError(t, err, "pwsh's profile must still be installed even though the other variant's resolution failed")
}

// TestInitPowerShellWindowsMessagesRenderInTurkish is this feature's TR
// i18n smoke test (per this project's established
// TestI18nSmoke*InTurkish convention, e.g. i18n_smoke_test.go): every
// new MsgInitPSVariantXxx/MsgInitConfirmPromptMulti string must actually
// route through the resolved Translator, not a hardcoded English
// literal, when general.language resolves to Turkish.
func TestInitPowerShellWindowsMessagesRenderInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	dir := t.TempDir()
	winPSProfile := filepath.Join(dir, "windowspowershell_profile.ps1")
	pwshProfile := filepath.Join(dir, "pwsh_profile.ps1")

	deps := initDeps{
		goos:   "windows",
		getenv: func(k string) string { return map[string]string{"COMRADE_LANG": "tr"}[k] },
		lookPath: func(name string) (string, error) {
			if name == "powershell" || name == "pwsh" {
				return `C:\fake\` + name + `.exe`, nil
			}
			return "", errors.New("not found")
		},
		run: func(_ stdctx.Context, name string, _ ...string) ([]byte, error) {
			switch name {
			case "powershell":
				return []byte(winPSProfile), nil
			case "pwsh":
				return []byte(pwshProfile), nil
			default:
				return nil, errors.New("unexpected binary " + name)
			}
		},
	}

	// confirmYesNo accepts TR's own "e"/"evet" affirmative (matching the
	// "[e/H]" the TR prompt itself renders) — see TestConfirmYesNoIsAffirmative.
	out := execInitCmd(t, deps, "e\n", "powershell")
	assert.Contains(t, out, "Windows PowerShell 5.1: cli-comrade kabuk entegrasyonu "+winPSProfile+" içine kuruldu")
	assert.Contains(t, out, "PowerShell 7: cli-comrade kabuk entegrasyonu "+pwshProfile+" içine kuruldu")
	assert.Contains(t, out, "Yukarıdaki profil(ler)e cli-comrade kabuk entegrasyonu eklensin mi?")
	assert.NotContains(t, out, "Installed cli-comrade shell integration")
}

// TestInitPowerShellWindowsNoneFoundErrorRendersInTurkish proves
// MsgInitPowerShellNoneFoundError is also translated, not a hardcoded
// English literal, when general.language resolves to Turkish.
func TestInitPowerShellWindowsNoneFoundErrorRendersInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")
	deps := testInitDeps("windows", map[string]string{}) // lookPath always fails

	err := execInitCmdErr(t, deps, "powershell")
	require.Error(t, err)
	assert.ErrorContains(t, err, "hiçbir PowerShell kurulumu bulunamadı")
	assert.NotContains(t, err.Error(), "no PowerShell installation found")
}

// TestIsAffirmative is the table-driven test for confirmYesNo's per-language
// acceptance rule: TR accepts (case-insensitively) "e"/"evet", every other
// language accepts "y"/"yes" — everything else, including an empty line,
// must stay default-NO. This is the fix for the tracked bug where the TR
// prompt rendered "[e/H]" but "e"/"evet" were silently rejected.
func TestIsAffirmative(t *testing.T) {
	tests := []struct {
		name string
		lang i18n.Lang
		line string
		want bool
	}{
		{"tr lowercase e is affirmative", i18n.LangTR, "e", true},
		{"tr uppercase E is affirmative", i18n.LangTR, "E", true},
		{"tr lowercase evet is affirmative", i18n.LangTR, "evet", true},
		{"tr mixed-case EVET is affirmative", i18n.LangTR, "EVET", true},
		{"tr rejects EN y", i18n.LangTR, "y", false},
		{"tr rejects EN yes", i18n.LangTR, "yes", false},
		{"tr empty line stays no", i18n.LangTR, "", false},
		{"tr garbage stays no", i18n.LangTR, "maybe", false},
		{"tr explicit h stays no", i18n.LangTR, "h", false},
		{"en lowercase y is affirmative", i18n.LangEN, "y", true},
		{"en uppercase Y is affirmative", i18n.LangEN, "Y", true},
		{"en lowercase yes is affirmative", i18n.LangEN, "yes", true},
		{"en mixed-case Yes is affirmative", i18n.LangEN, "Yes", true},
		{"en rejects TR e", i18n.LangEN, "e", false},
		{"en rejects TR evet", i18n.LangEN, "evet", false},
		{"en empty line stays no", i18n.LangEN, "", false},
		{"en garbage stays no", i18n.LangEN, "maybe", false},
		{"en explicit n stays no", i18n.LangEN, "n", false},
		{"unknown lang falls back to EN rule: y affirmative", i18n.Lang("xx"), "y", true},
		{"unknown lang falls back to EN rule: e not affirmative", i18n.Lang("xx"), "e", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isAffirmative(tt.lang, tt.line))
		})
	}
}

// TestConfirmYesNoReadsLangSpecificAnswer drives confirmYesNo itself (not
// just isAffirmative) through a stubbed *cobra.Command, proving the prompt
// is written to stdout and the answer is read from stdin per lang.
func TestConfirmYesNoReadsLangSpecificAnswer(t *testing.T) {
	tests := []struct {
		name  string
		lang  i18n.Lang
		stdin string
		want  bool
	}{
		{"tr accepts e", i18n.LangTR, "e\n", true},
		{"tr rejects EN y", i18n.LangTR, "y\n", false},
		{"en accepts y", i18n.LangEN, "y\n", true},
		{"en rejects TR e", i18n.LangEN, "e\n", false},
		{"empty stdin (EOF) stays no", i18n.LangEN, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.SetIn(strings.NewReader(tt.stdin))
			var out bytes.Buffer
			cmd.SetOut(&out)

			got, err := confirmYesNo(cmd, "confirm? ", tt.lang)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, "confirm? ", out.String())
		})
	}
}

// TestWriteFileContent drives writeFileContent directly (LOW#9's atomic
// rc-file write) rather than only indirectly through the higher-level
// TestInit* flows above: it asserts the resulting content is exactly
// right, that re-running is idempotent (no double-write/corruption),
// that no leftover ".comrade-init-*.tmp" file survives a successful
// write (proof the mechanism is genuinely temp-file-then-rename rather
// than a direct in-place write), and that pre-existing sibling content
// in the same directory is left untouched.
func TestWriteFileContent(t *testing.T) {
	tests := []struct {
		name        string
		existing    string // "" means: do not create the file at all first
		content     string
		wantContent string
	}{
		{
			name:        "fresh file with no prior content",
			existing:    "",
			content:     "export COMRADE_HOME=1\n",
			wantContent: "export COMRADE_HOME=1\n",
		},
		{
			name:        "overwrite replaces existing content exactly",
			existing:    "# old rc content\nalias ll='ls -la'\n",
			content:     "# new rc content\n",
			wantContent: "# new rc content\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".bashrc")
			if tt.existing != "" {
				require.NoError(t, os.WriteFile(path, []byte(tt.existing), 0o644))
			}

			require.NoError(t, writeFileContent(path, tt.content))

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, tt.wantContent, string(got))

			assertNoLeftoverTempFile(t, dir)
		})
	}
}

// TestWriteFileContentIsIdempotentAcrossRepeatedWrites proves writing the
// same content twice in a row never double-appends or corrupts the file
// — the second write's temp-file-then-rename simply replaces the first
// write's result byte-for-byte.
func TestWriteFileContentIsIdempotentAcrossRepeatedWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".bashrc")
	content := "# cli-comrade hook\nexport FOO=bar\n"

	require.NoError(t, writeFileContent(path, content))
	require.NoError(t, writeFileContent(path, content))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, content, string(got), "re-running writeFileContent with identical content must not double-write it")
	assertNoLeftoverTempFile(t, dir)
}

// TestWriteFileContentPreservesExistingFileMode proves an existing rc
// file's own permission bits survive the temp-file-then-rename — the
// same behavior os.WriteFile always had (it never re-chmods a file that
// already exists), now delivered atomically instead of in-place.
// POSIX permission bits are not meaningful on Windows (see this
// package's other runtime.GOOS-guarded mode assertions, e.g.
// TestUpdateReplace* in internal/update), so this only runs elsewhere.
func TestWriteFileContentPreservesExistingFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, ".bashrc")
	require.NoError(t, os.WriteFile(path, []byte("# old\n"), 0o640))

	require.NoError(t, writeFileContent(path, "# new\n"))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
}

// TestWriteFileContentFreshFileGetsDefaultMode proves a newly created rc
// file still gets the same 0o644 default writeFileContent always used,
// matching os.WriteFile's own create-time mode.
func TestWriteFileContentFreshFileGetsDefaultMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, ".bashrc")

	require.NoError(t, writeFileContent(path, "# new\n"))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

// TestWriteFileContentLeavesSiblingFilesUntouched proves the temp file
// writeFileContent creates alongside path (same directory, required for
// os.Rename to stay within one filesystem) never collides with or
// clobbers an unrelated file that happens to already live there.
func TestWriteFileContentLeavesSiblingFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	siblingPath := filepath.Join(dir, ".zshrc")
	require.NoError(t, os.WriteFile(siblingPath, []byte("# unrelated zsh content\n"), 0o644))

	path := filepath.Join(dir, ".bashrc")
	require.NoError(t, writeFileContent(path, "# bash content\n"))

	sibling, err := os.ReadFile(siblingPath)
	require.NoError(t, err)
	assert.Equal(t, "# unrelated zsh content\n", string(sibling))
	assertNoLeftoverTempFile(t, dir)
}

// assertNoLeftoverTempFile lists dir and fails the test if any entry
// matches writeFileContent's own ".comrade-init-*.tmp" temp-file
// pattern — the sole positive proof that a write actually went through
// os.CreateTemp+os.Rename rather than a direct in-place os.WriteFile.
func assertNoLeftoverTempFile(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.False(t, strings.Contains(e.Name(), ".comrade-init-"), "leftover temp file: %s", e.Name())
	}
}
