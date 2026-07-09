package context

import (
	stdctx "context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func fakeEnv(vars map[string]string) func(string) string {
	return func(key string) string {
		return vars[key]
	}
}

func TestDetectShellUnixReadsShellEnvBasename(t *testing.T) {
	cases := []struct {
		name  string
		shell string
		want  string
	}{
		{"bash absolute path", "/bin/bash", "bash"},
		{"zsh absolute path", "/usr/bin/zsh", "zsh"},
		{"fish absolute path", "/usr/local/bin/fish", "fish"},
		{"already bare name", "sh", "sh"},
		{"unset", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectShell("linux", fakeEnv(map[string]string{"SHELL": tc.shell}))
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDetectShellDarwinUsesSameShellEnvLogic(t *testing.T) {
	got := DetectShell("darwin", fakeEnv(map[string]string{"SHELL": "/bin/zsh"}))
	assert.Equal(t, "zsh", got)
}

func TestDetectShellWindowsPowerShellWhenPSModulePathSet(t *testing.T) {
	got := DetectShell("windows", fakeEnv(map[string]string{
		"PSModulePath": `C:\Program Files\WindowsPowerShell\Modules`,
	}))
	assert.Equal(t, "powershell", got)
}

func TestDetectShellWindowsCmdWhenPSModulePathUnset(t *testing.T) {
	got := DetectShell("windows", fakeEnv(map[string]string{}))
	assert.Equal(t, "cmd", got)
}

func TestShellVersionReturnsFirstLineOnSuccess(t *testing.T) {
	run := func(_ stdctx.Context, name string, args ...string) ([]byte, error) {
		assert.Equal(t, "bash", name)
		assert.Equal(t, []string{"--version"}, args)
		return []byte("GNU bash, version 5.2.15(1)-release\nCopyright ..."), nil
	}

	got := ShellVersion(stdctx.Background(), "bash", run)
	assert.Equal(t, "GNU bash, version 5.2.15(1)-release", got)
}

func TestShellVersionSilentOnCommandError(t *testing.T) {
	run := func(_ stdctx.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("exec: \"bash\": executable file not found in $PATH")
	}

	got := ShellVersion(stdctx.Background(), "bash", run)
	assert.Equal(t, "", got)
}

func TestShellVersionSilentOnEmptyShell(t *testing.T) {
	got := ShellVersion(stdctx.Background(), "", RunCommand)
	assert.Equal(t, "", got)
}

func TestShellVersionSilentOnUnknownShell(t *testing.T) {
	run := func(_ stdctx.Context, _ string, _ ...string) ([]byte, error) {
		t.Fatal("run must not be called for an unrecognized shell")
		return nil, nil
	}
	got := ShellVersion(stdctx.Background(), "tcsh", run)
	assert.Equal(t, "", got)
}

func TestShellVersionSilentOnNilRunner(t *testing.T) {
	got := ShellVersion(stdctx.Background(), "bash", nil)
	assert.Equal(t, "", got)
}

func TestShellVersionPowerShellUsesPSVersionCommand(t *testing.T) {
	var gotArgs []string
	run := func(_ stdctx.Context, _ string, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("7.4.1\n"), nil
	}
	got := ShellVersion(stdctx.Background(), "powershell", run)
	assert.Equal(t, "7.4.1", got)
	assert.Equal(t, []string{"-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()"}, gotArgs)
}
