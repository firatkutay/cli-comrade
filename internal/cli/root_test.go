package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// execRoot runs the root command with the given args and returns combined
// stdout/stderr output.
func execRoot(t *testing.T, version string, args ...string) string {
	t.Helper()
	root := NewRootCmd(version)
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}
	return buf.String()
}

func TestRootCmdBareInvocationPrintsVersionAndHelp(t *testing.T) {
	out := execRoot(t, "1.2.3")

	assert.True(t, strings.HasPrefix(out, "comrade version 1.2.3\n\n"),
		"expected output to start with the version banner, got: %q", out)
	assert.Contains(t, out, "Usage:")
	assert.Contains(t, out, "comrade is a cross-platform AI CLI companion for the terminal")
}

func TestRootCmdVersionFlagPrintsExactVersionString(t *testing.T) {
	out := execRoot(t, "9.9.9", "--version")

	assert.Equal(t, "comrade version 9.9.9\n", out)
}

func TestRootCmdDefaultVersionIsDevWhenUnset(t *testing.T) {
	out := execRoot(t, "dev", "--version")

	assert.Equal(t, "comrade version dev\n", out)
}

func TestSubcommandStubsPrintNotReadyMessage(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"fix", []string{"fix"}, "comrade fix: this feature is not ready yet.\n"},
		{"explain", []string{"explain", "ls"}, "comrade explain: this feature is not ready yet.\n"},
		{"chat", []string{"chat"}, "comrade chat: this feature is not ready yet.\n"},
		{"config", []string{"config"}, "comrade config: this feature is not ready yet.\n"},
		{"init", []string{"init"}, "comrade init: this feature is not ready yet.\n"},
		{"history", []string{"history"}, "comrade history: this feature is not ready yet.\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := execRoot(t, "dev", tc.args...)
			assert.Equal(t, tc.want, out)
		})
	}
}
