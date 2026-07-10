package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModeStringNames(t *testing.T) {
	assert.Equal(t, "auto", ModeAuto.String())
	assert.Equal(t, "ask", ModeAsk.String())
	assert.Equal(t, "info", ModeInfo.String())
	assert.Equal(t, "unknown(99)", Mode(99).String())
}

func TestParseModeAcceptsCanonicalNames(t *testing.T) {
	cases := []struct {
		in   string
		want Mode
	}{
		{"auto", ModeAuto},
		{"ask", ModeAsk},
		{"info", ModeInfo},
	}
	for _, tc := range cases {
		got, err := ParseMode(tc.in)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
}

func TestParseModeRejectsUnknown(t *testing.T) {
	_, err := ParseMode("yolo")
	assert.Error(t, err)
}

// TestResolveModePrecedence pins docs/history/UYGULAMA_PLANI.md FAZ 6 item 2's exact
// mode precedence: an explicit flag wins over COMRADE_MODE, which wins
// over config general.mode.
func TestResolveModePrecedence(t *testing.T) {
	cases := []struct {
		name              string
		flag, env, config string
		want              Mode
	}{
		{"flag wins over env and config", "info", "auto", "ask", ModeInfo},
		{"env wins over config when flag unset", "", "auto", "ask", ModeAuto},
		{"config used when flag and env unset", "", "", "ask", ModeAsk},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveMode(tc.flag, tc.env, tc.config)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveModeRejectsInvalidCandidate(t *testing.T) {
	_, err := ResolveMode("bogus", "", "ask")
	assert.Error(t, err)
}

func TestResolveModeErrorsWhenNoSourceProvidesAValue(t *testing.T) {
	_, err := ResolveMode("", "", "")
	assert.Error(t, err)
}
