package shellinit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

func TestParseShellAcceptsEverySupportedName(t *testing.T) {
	cases := []struct {
		name string
		want shellinit.Shell
	}{
		{"bash", shellinit.Bash},
		{"zsh", shellinit.Zsh},
		{"fish", shellinit.Fish},
		{"powershell", shellinit.PowerShell},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := shellinit.ParseShell(tc.name)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseShellRejectsUnknownName(t *testing.T) {
	_, err := shellinit.ParseShell("tcsh")
	assert.ErrorContains(t, err, `unsupported shell "tcsh"`)
	assert.ErrorContains(t, err, "bash, zsh, fish, powershell")
}

func TestParseShellRejectsEmptyName(t *testing.T) {
	_, err := shellinit.ParseShell("")
	assert.Error(t, err)
}
