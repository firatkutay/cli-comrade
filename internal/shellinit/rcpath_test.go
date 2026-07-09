package shellinit_test

import (
	stdctx "context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

func fakeEnv(vars map[string]string) func(string) string {
	return func(key string) string {
		return vars[key]
	}
}

func TestRCPathBashUsesHomeBashrc(t *testing.T) {
	path, ok, note := shellinit.RCPath(stdctx.Background(), shellinit.Bash, "linux",
		fakeEnv(map[string]string{"HOME": "/home/alice"}), nil, nil)
	require.True(t, ok)
	assert.Empty(t, note)
	assert.Equal(t, filepath.Join("/home/alice", ".bashrc"), path)
}

func TestRCPathBashErrorsWhenHomeUnset(t *testing.T) {
	_, ok, note := shellinit.RCPath(stdctx.Background(), shellinit.Bash, "linux", fakeEnv(nil), nil, nil)
	assert.False(t, ok)
	assert.Contains(t, note, "HOME")
}

func TestRCPathZshPrefersZDOTDIR(t *testing.T) {
	path, ok, _ := shellinit.RCPath(stdctx.Background(), shellinit.Zsh, "linux",
		fakeEnv(map[string]string{"HOME": "/home/alice", "ZDOTDIR": "/home/alice/.zsh-custom"}), nil, nil)
	require.True(t, ok)
	assert.Equal(t, filepath.Join("/home/alice/.zsh-custom", ".zshrc"), path)
}

func TestRCPathZshFallsBackToHomeWithoutZDOTDIR(t *testing.T) {
	path, ok, _ := shellinit.RCPath(stdctx.Background(), shellinit.Zsh, "linux",
		fakeEnv(map[string]string{"HOME": "/home/alice"}), nil, nil)
	require.True(t, ok)
	assert.Equal(t, filepath.Join("/home/alice", ".zshrc"), path)
}

func TestRCPathFishPrefersXDGConfigHome(t *testing.T) {
	path, ok, _ := shellinit.RCPath(stdctx.Background(), shellinit.Fish, "linux",
		fakeEnv(map[string]string{"HOME": "/home/alice", "XDG_CONFIG_HOME": "/home/alice/.config-custom"}), nil, nil)
	require.True(t, ok)
	assert.Equal(t, filepath.Join("/home/alice/.config-custom", "fish", "config.fish"), path)
}

func TestRCPathFishFallsBackToHomeConfig(t *testing.T) {
	path, ok, _ := shellinit.RCPath(stdctx.Background(), shellinit.Fish, "linux",
		fakeEnv(map[string]string{"HOME": "/home/alice"}), nil, nil)
	require.True(t, ok)
	assert.Equal(t, filepath.Join("/home/alice", ".config", "fish", "config.fish"), path)
}

func TestRCPathPowerShellResolvesViaPwshWhenAvailable(t *testing.T) {
	lookPath := func(name string) (string, error) {
		if name == "pwsh" {
			return "/usr/bin/pwsh", nil
		}
		return "", errors.New("not found")
	}
	run := func(_ stdctx.Context, name string, args ...string) ([]byte, error) {
		assert.Equal(t, "pwsh", name)
		assert.Equal(t, []string{"-NoProfile", "-Command", "$PROFILE"}, args)
		return []byte("/home/alice/.config/powershell/Microsoft.PowerShell_profile.ps1\n"), nil
	}

	path, ok, note := shellinit.RCPath(stdctx.Background(), shellinit.PowerShell, "linux", fakeEnv(nil), lookPath, run)
	require.True(t, ok)
	assert.Empty(t, note)
	assert.Equal(t, "/home/alice/.config/powershell/Microsoft.PowerShell_profile.ps1", path)
}

func TestRCPathPowerShellUsesPowershellBinaryOnWindows(t *testing.T) {
	var calledBin string
	lookPath := func(name string) (string, error) {
		calledBin = name
		return `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, nil
	}
	run := func(_ stdctx.Context, name string, _ ...string) ([]byte, error) {
		return []byte(`C:\Users\alice\Documents\WindowsPowerShell\profile.ps1`), nil
	}

	path, ok, _ := shellinit.RCPath(stdctx.Background(), shellinit.PowerShell, "windows", fakeEnv(nil), lookPath, run)
	require.True(t, ok)
	assert.Equal(t, "powershell", calledBin)
	assert.Equal(t, `C:\Users\alice\Documents\WindowsPowerShell\profile.ps1`, path)
}

func TestRCPathPowerShellNotOKWhenBinaryMissing(t *testing.T) {
	lookPath := func(string) (string, error) { return "", errors.New("not found") }
	run := func(stdctx.Context, string, ...string) ([]byte, error) {
		t.Fatal("run must not be called when the binary is not on PATH")
		return nil, nil
	}

	_, ok, note := shellinit.RCPath(stdctx.Background(), shellinit.PowerShell, "linux", fakeEnv(nil), lookPath, run)
	assert.False(t, ok)
	assert.Contains(t, note, "pwsh")
	assert.Contains(t, note, "not found")
}

func TestRCPathPowerShellNotOKWhenRunFails(t *testing.T) {
	lookPath := func(string) (string, error) { return "/usr/bin/pwsh", nil }
	run := func(stdctx.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("exit status 1")
	}

	_, ok, note := shellinit.RCPath(stdctx.Background(), shellinit.PowerShell, "linux", fakeEnv(nil), lookPath, run)
	assert.False(t, ok)
	assert.Contains(t, note, "failed")
}

func TestRCPathPowerShellNotOKWhenRunnersNil(t *testing.T) {
	_, ok, note := shellinit.RCPath(stdctx.Background(), shellinit.PowerShell, "linux", fakeEnv(nil), nil, nil)
	assert.False(t, ok)
	assert.NotEmpty(t, note)
}

func TestRCPathUnsupportedShellErrors(t *testing.T) {
	_, ok, note := shellinit.RCPath(stdctx.Background(), shellinit.Shell("tcsh"), "linux", fakeEnv(nil), nil, nil)
	assert.False(t, ok)
	assert.Contains(t, note, "tcsh")
}
