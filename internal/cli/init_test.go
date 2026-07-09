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

	// NOTE (pre-existing, out of scope for this feature): confirmYesNo
	// only accepts literal "y"/"yes" regardless of the active language,
	// even though the TR prompt itself displays "[e/H]" — "e"/"evet" is
	// NOT actually accepted. Using "y" here to reach the install path;
	// this discrepancy is unrelated to the multi-profile PowerShell
	// change and is not fixed by it.
	out := execInitCmd(t, deps, "y\n", "powershell")
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
