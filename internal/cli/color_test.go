package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// resolveColorEnabledTestCfg returns a config.Config with only
// General.Color set, matching how every real call site actually varies
// this decision (the rest of the struct is irrelevant to color
// resolution).
func resolveColorEnabledTestCfg(colorConfig bool) config.Config {
	return config.Config{General: config.GeneralConfig{Color: colorConfig}}
}

// TestResolveColorEnabled is the table-driven test for internal/cli's
// single color-decision point: cfg.General.Color as the master opt-out,
// refined by TTY detection (simulated via colorprofile's own TTY_FORCE
// test hook, since a real *os.File TTY is not available under `go test`)
// and the NO_COLOR / CLICOLOR_FORCE env conventions.
func TestResolveColorEnabled(t *testing.T) {
	tests := []struct {
		name        string
		colorConfig bool
		environ     []string
		want        bool
	}{
		{
			name:        "config color=false always disables, even on a forced TTY",
			colorConfig: false,
			environ:     []string{"TTY_FORCE=1"},
			want:        false,
		},
		{
			name:        "config color=false always disables, even with CLICOLOR_FORCE",
			colorConfig: false,
			environ:     []string{"CLICOLOR_FORCE=1"},
			want:        false,
		},
		{
			name:        "config color=true, non-TTY, no CLICOLOR_FORCE stays plain",
			colorConfig: true,
			environ:     []string{},
			want:        false,
		},
		{
			// TERM is required alongside TTY_FORCE here: colorprofile
			// treats an unset $TERM as a "dumb" terminal (NoTTY) on
			// non-Windows regardless of isatty, matching a real terminal
			// session, which always has $TERM set.
			name:        "config color=true, real TTY, no NO_COLOR enables",
			colorConfig: true,
			environ:     []string{"TTY_FORCE=1", "TERM=xterm-256color"},
			want:        true,
		},
		{
			name:        "config color=true, TTY, NO_COLOR=1 disables",
			colorConfig: true,
			environ:     []string{"TTY_FORCE=1", "TERM=xterm-256color", "NO_COLOR=1"},
			want:        false,
		},
		{
			name:        "config color=true, non-TTY (piped), CLICOLOR_FORCE=1 forces on",
			colorConfig: true,
			environ:     []string{"CLICOLOR_FORCE=1"},
			want:        true,
		},
		{
			// noColorSet's unconditional pre-check wins over
			// CLICOLOR_FORCE here too (colorprofile.Detect is never even
			// reached).
			name:        "config color=true, TTY, CLICOLOR_FORCE=1 AND NO_COLOR=1 on a TTY: NO_COLOR wins",
			colorConfig: true,
			environ:     []string{"TTY_FORCE=1", "TERM=xterm-256color", "CLICOLOR_FORCE=1", "NO_COLOR=1"},
			want:        false,
		},
		{
			// colorprofile v0.4.3's OWN internal NO_COLOR check is gated
			// on isatty (see env.go's colorProfile: that branch only runs
			// `&& isatty`), so left to colorprofile alone, CLICOLOR_FORCE
			// would win over NO_COLOR when piped. resolveColorEnabled
			// deliberately does NOT delegate to that precedence: its own
			// noColorSet pre-check runs unconditionally, before
			// colorprofile.Detect is ever reached, so NO_COLOR wins here
			// too — matching no-color.org's stated intent ("NO_COLOR ...
			// should disable color", no isatty carve-out) rather than this
			// one dependency's own narrower behavior.
			name:        "config color=true, non-TTY, CLICOLOR_FORCE=1 AND NO_COLOR=1 while piped: NO_COLOR still wins",
			colorConfig: true,
			environ:     []string{"CLICOLOR_FORCE=1", "NO_COLOR=1"},
			want:        false,
		},
		{
			name:        "config color=true, non-TTY, CLICOLOR_FORCE=1 without NO_COLOR still forces on (QA path unaffected)",
			colorConfig: true,
			environ:     []string{"CLICOLOR_FORCE=1"},
			want:        true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			got := resolveColorEnabled(resolveColorEnabledTestCfg(tt.colorConfig), tt.environ, &out)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestResolveColorEnabledSkipsWindowsANSIEnableForNonFileWriter proves the
// *os.File type assertion guard doesn't panic (and simply skips the
// Windows VT-enable call) when out is a plain io.Writer, as it always is
// under `go test` — this is what keeps this test suite from ever touching
// a real console mode.
func TestResolveColorEnabledSkipsWindowsANSIEnableForNonFileWriter(t *testing.T) {
	var out bytes.Buffer
	environ := []string{"TTY_FORCE=1", "TERM=xterm-256color"}
	assert.NotPanics(t, func() {
		got := resolveColorEnabled(resolveColorEnabledTestCfg(true), environ, &out)
		assert.True(t, got, "sanity: this environ must actually resolve to enabled for the guard to be exercised")
	})
}
