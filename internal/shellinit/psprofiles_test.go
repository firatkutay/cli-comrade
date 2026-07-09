package shellinit_test

import (
	stdctx "context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

// lookPathAmong returns a lookPath stub that succeeds for exactly the
// binaries named in found, failing for everything else.
func lookPathAmong(found ...string) func(string) (string, error) {
	set := map[string]bool{}
	for _, f := range found {
		set[f] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return `C:\fake\` + name + `.exe`, nil
		}
		return "", errors.New("not found")
	}
}

// runReturning returns a CommandRunner stub that always answers
// `-Command '$PROFILE'` with profileFor[bin], asserting the exact
// -NoProfile/-Command/$PROFILE argument shape RCPath itself already
// pins.
func runReturning(t *testing.T, profileFor map[string]string) shellinit.CommandRunner {
	t.Helper()
	return func(_ stdctx.Context, bin string, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"-NoProfile", "-Command", "$PROFILE"}, args)
		path, ok := profileFor[bin]
		if !ok {
			return nil, errors.New("unexpected bin " + bin)
		}
		return []byte(path), nil
	}
}

func TestResolvePowerShellProfilesWindowsBothVariantsPresent(t *testing.T) {
	lookPath := lookPathAmong("powershell", "pwsh")
	run := runReturning(t, map[string]string{
		"powershell": `C:\Users\alice\Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1`,
		"pwsh":       `C:\Users\alice\Documents\PowerShell\Microsoft.PowerShell_profile.ps1`,
	})

	profiles, err := shellinit.ResolvePowerShellProfiles(stdctx.Background(), "windows", lookPath, run)
	require.NoError(t, err)
	require.Len(t, profiles, 2)

	assert.Equal(t, shellinit.PSVariantWindowsPowerShell, profiles[0].Variant)
	assert.True(t, profiles[0].OK)
	assert.Equal(t, `C:\Users\alice\Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1`, profiles[0].Path)

	assert.Equal(t, shellinit.PSVariantPwsh, profiles[1].Variant)
	assert.True(t, profiles[1].OK)
	assert.Equal(t, `C:\Users\alice\Documents\PowerShell\Microsoft.PowerShell_profile.ps1`, profiles[1].Path)
}

func TestResolvePowerShellProfilesWindowsOnlyWindowsPowerShellPresent(t *testing.T) {
	lookPath := lookPathAmong("powershell")
	run := runReturning(t, map[string]string{
		"powershell": `C:\Users\alice\Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1`,
	})

	profiles, err := shellinit.ResolvePowerShellProfiles(stdctx.Background(), "windows", lookPath, run)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, shellinit.PSVariantWindowsPowerShell, profiles[0].Variant)
	assert.True(t, profiles[0].OK)
}

func TestResolvePowerShellProfilesWindowsOnlyPwshPresent(t *testing.T) {
	lookPath := lookPathAmong("pwsh")
	run := runReturning(t, map[string]string{
		"pwsh": `C:\Users\alice\Documents\PowerShell\Microsoft.PowerShell_profile.ps1`,
	})

	profiles, err := shellinit.ResolvePowerShellProfiles(stdctx.Background(), "windows", lookPath, run)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, shellinit.PSVariantPwsh, profiles[0].Variant)
	assert.True(t, profiles[0].OK)
}

func TestResolvePowerShellProfilesWindowsNeitherPresentErrors(t *testing.T) {
	lookPath := lookPathAmong() // nothing found
	run := runReturning(t, map[string]string{})

	profiles, err := shellinit.ResolvePowerShellProfiles(stdctx.Background(), "windows", lookPath, run)
	require.Error(t, err)
	assert.True(t, errors.Is(err, shellinit.ErrNoPowerShellFound))
	assert.Nil(t, profiles)
}

func TestResolvePowerShellProfilesWindowsOneVariantResolvesOneDoesNot(t *testing.T) {
	lookPath := lookPathAmong("powershell", "pwsh")
	run := func(_ stdctx.Context, bin string, _ ...string) ([]byte, error) {
		if bin == "powershell" {
			return []byte(`C:\Users\alice\Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1`), nil
		}
		return nil, errors.New("exit status 1")
	}

	profiles, err := shellinit.ResolvePowerShellProfiles(stdctx.Background(), "windows", lookPath, run)
	require.NoError(t, err)
	require.Len(t, profiles, 2, "a query failure for one variant must not hide the other variant's success")

	assert.Equal(t, shellinit.PSVariantWindowsPowerShell, profiles[0].Variant)
	assert.True(t, profiles[0].OK)

	assert.Equal(t, shellinit.PSVariantPwsh, profiles[1].Variant)
	assert.False(t, profiles[1].OK)
	assert.Contains(t, profiles[1].Note, "failed")
}

func TestResolvePowerShellProfilesNonWindowsOnlyProbesPwsh(t *testing.T) {
	var probed []string
	lookPath := func(name string) (string, error) {
		probed = append(probed, name)
		return `/usr/bin/pwsh`, nil
	}
	run := runReturning(t, map[string]string{
		"pwsh": "/home/alice/.config/powershell/Microsoft.PowerShell_profile.ps1",
	})

	profiles, err := shellinit.ResolvePowerShellProfiles(stdctx.Background(), "linux", lookPath, run)
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, shellinit.PSVariantPwsh, profiles[0].Variant)
	assert.Equal(t, []string{"pwsh"}, probed, "non-Windows must never probe the \"powershell\" binary")
}

func TestResolvePowerShellProfilesNonWindowsErrorsWhenPwshMissing(t *testing.T) {
	lookPath := func(string) (string, error) { return "", errors.New("not found") }

	_, err := shellinit.ResolvePowerShellProfiles(stdctx.Background(), "linux", lookPath, nil)
	assert.True(t, errors.Is(err, shellinit.ErrNoPowerShellFound))
}

func TestPSVariantLabel(t *testing.T) {
	assert.Equal(t, "Windows PowerShell 5.1", shellinit.PSVariantWindowsPowerShell.Label())
	assert.Equal(t, "PowerShell 7", shellinit.PSVariantPwsh.Label())
	assert.Equal(t, "bogus", shellinit.PSVariant("bogus").Label())
}
