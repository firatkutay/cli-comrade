package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

func TestShellHookCheckSkipsWhenShellUndetected(t *testing.T) {
	deps := baseDeps()
	deps.GOOS = "linux"
	deps.Getenv = func(string) string { return "" } // SHELL unset

	result := ShellHookCheck(context.Background(), deps)

	assert.Equal(t, SeveritySkip, result.Severity)
	assert.Equal(t, i18n.MsgDoctorShellHookUndetected, result.Summary)
}

func TestShellHookCheckSkipsUnsupportedShell(t *testing.T) {
	deps := baseDeps()
	deps.GOOS = "linux"
	deps.Getenv = func(name string) string {
		if name == "SHELL" {
			return "/bin/tcsh"
		}
		return ""
	}

	result := ShellHookCheck(context.Background(), deps)

	assert.Equal(t, SeveritySkip, result.Severity)
	assert.Equal(t, i18n.MsgDoctorShellHookUnsupported, result.Summary)
	assert.Equal(t, []any{"tcsh"}, result.SummaryArgs)
}

func TestShellHookCheckSkipsWhenRCPathUnresolved(t *testing.T) {
	deps := baseDeps()
	deps.GOOS = "linux"
	deps.Getenv = func(name string) string {
		if name == "SHELL" {
			return "/bin/bash"
		}
		return "" // HOME unset too -> RCPath can't resolve .bashrc
	}

	result := ShellHookCheck(context.Background(), deps)

	assert.Equal(t, SeveritySkip, result.Severity)
	assert.Equal(t, i18n.MsgDoctorShellHookUnresolved, result.Summary)
	assert.Equal(t, []any{"bash"}, result.SummaryArgs)
}

func TestShellHookCheckWarnsWhenBlockAbsent(t *testing.T) {
	home := t.TempDir()
	deps := baseDeps()
	deps.GOOS = "linux"
	deps.Getenv = func(name string) string {
		switch name {
		case "SHELL":
			return "/bin/bash"
		case "HOME":
			return home
		default:
			return ""
		}
	}
	// .bashrc does not exist at all yet.

	result := ShellHookCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, i18n.MsgDoctorShellHookMissing, result.Summary)
	assert.Equal(t, []any{"bash"}, result.SummaryArgs)
	assert.Equal(t, "comrade init bash", result.Fix)
}

func TestShellHookCheckWarnsWhenBlockOutdated(t *testing.T) {
	home := t.TempDir()
	staleBlock := shellinit.MarkerBegin + "\n# an older cli-comrade hook body\n" + shellinit.MarkerEnd
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# my bashrc\n\n"+staleBlock+"\n"), 0o644))

	deps := baseDeps()
	deps.GOOS = "linux"
	deps.Getenv = func(name string) string {
		switch name {
		case "SHELL":
			return "/bin/bash"
		case "HOME":
			return home
		default:
			return ""
		}
	}

	result := ShellHookCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, i18n.MsgDoctorShellHookMissing, result.Summary)
	assert.Equal(t, "comrade init bash", result.Fix)
}

func TestShellHookCheckOKWhenBlockCurrent(t *testing.T) {
	home := t.TempDir()
	block, err := shellinit.Block(shellinit.Bash)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(home, ".bashrc"), []byte("# my bashrc\n\n"+block+"\n"), 0o644))

	deps := baseDeps()
	deps.GOOS = "linux"
	deps.Getenv = func(name string) string {
		switch name {
		case "SHELL":
			return "/bin/bash"
		case "HOME":
			return home
		default:
			return ""
		}
	}

	result := ShellHookCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorShellHookOK, result.Summary)
	assert.Equal(t, []any{"bash"}, result.SummaryArgs)
	assert.Empty(t, result.Fix)
}
