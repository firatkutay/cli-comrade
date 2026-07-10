package cli

import (
	"io"
	"os"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// Pastel palette — every fixed ANSI256 color code this package's styling
// uses lives HERE, by name, so no style literal is ever scattered across
// help.go/spinner.go/chatmodel.go/chatdispatch.go as a bare quoted
// number. All six are deliberately fixed, mid-tone/desaturated codes
// rather than lipgloss's AdaptiveColor/compat package — see
// helpHeaderStyle's doc comment (help.go) for the full reasoning
// (charm.land/lipgloss/v2/compat's HasDarkBackground/Profile vars run an
// unconditional, up-to-2-second-blocking live terminal query at that
// package's own import/init time — a cold-start regression class this
// codebase has already hardened against once, see go.mod's atotto/
// clipboard replace comment). Every one of these six is chosen to read
// acceptably on both a light and a dark 256-color terminal without any
// runtime background detection.
const (
	// paletteLavender is help.go's section-header/spinner accent (bold).
	paletteLavender = "183"
	// paletteCyan is help.go's command-name accent.
	paletteCyan = "115"
	// palettePeach is help.go's flag-name accent.
	palettePeach = "216"
	// paletteGray is `comrade chat`'s own echoed transcript lines (the
	// user's own typed message text, NOT the "> " prefix — see
	// paletteYellow).
	paletteGray = "245"
	// paletteBlue is `comrade chat`'s command-like content — concretely,
	// today, the leading "/xxx" token on each row of "/help"'s rendered
	// slash-command list (see chatmodel.go's colorizeSlashCommandList).
	paletteBlue = "111"
	// paletteYellow is `comrade chat`'s input-prompt "> " symbol — both
	// the live bubbles/v2/textinput prompt and the transcript's own
	// echoed "> " prefix render with this same color, so the two visually
	// match.
	paletteYellow = "222"
)

// resolveColorEnabled is internal/cli's single color-decision point.
// Before this function existed, every call site read cfg.General.Color
// directly as the one and only signal, with no TTY/env awareness at all;
// every one of those call sites (chat.go, chatmodel.go, do.go, fix.go,
// explain.go, promptui.go, runtime.go) — and Part 2's new help/spinner
// code — now goes through this SAME function instead, so there is exactly
// one place that ever decides whether ANSI gets written, never two
// diverging paths.
//
// The decision: cfg.General.Color is still the master, explicit opt-out —
// general.color=false always disables color, full stop, exactly as before.
// NO_COLOR (https://no-color.org) is checked next, and UNCONDITIONALLY:
// this function's own pre-check (noColorSet, below) short-circuits to
// false before colorprofile.Detect ever runs, so NO_COLOR always wins —
// including over CLICOLOR_FORCE. This is a deliberate override of
// colorprofile v0.4.3's own behavior: that library's internal NO_COLOR
// check is gated on isatty (see env.go's colorProfile — the NO_COLOR
// branch only runs `&& isatty`), so on piped/non-TTY output its own logic
// lets CLICOLOR_FORCE win over NO_COLOR — verified by reading its source.
// no-color.org's own stated intent is unconditional ("NO_COLOR ... should
// disable ... color"), so this function does not merely delegate to
// colorprofile's precedence here.
//
// When color=true and NO_COLOR is not set, the actual per-invocation
// answer is refined by colorprofile.Detect(out, environ) — real TTY
// detection, plus https://bixense.com/clicolors/'s CLICOLOR_FORCE=1
// (forces color on even when out is not a TTY — the hook QA/tests use to
// check ANSI output non-interactively, e.g.
// `CLICOLOR_FORCE=1 comrade --help | cat -v`). Detect's own isatty check
// is what makes plain, byte-clean output the default for piped/non-TTY
// runs absent CLICOLOR_FORCE.
//
// On Windows, when the result is true, lipgloss.EnableLegacyWindowsANSI
// opts out's console into ENABLE_VIRTUAL_TERMINAL_PROCESSING: legacy
// conhost.exe (still what Windows PowerShell 5.1 typically runs in) does
// NOT interpret ANSI escape sequences until a process explicitly asks it
// to (unlike Windows Terminal, which negotiates VT support itself) — this
// is lipgloss v2's own documented mechanism for that
// (charm.land/lipgloss/v2 ansi_windows.go), confirmed by reading its
// source: a no-op on every non-Windows OS and a no-op if VT processing is
// already on (e.g. already inside Windows Terminal/PowerShell 7).
// EnableLegacyWindowsANSI takes a concrete *os.File (it needs the raw
// handle), so it is only invoked when out actually is one — in tests out
// is normally a plain io.Writer (bytes.Buffer), for which this call is
// skipped entirely, matching colorprofile.Detect's own graceful
// degradation for non-*os.File writers.
func resolveColorEnabled(cfg config.Config, environ []string, out io.Writer) bool {
	if !cfg.General.Color {
		return false
	}
	if noColorSet(environ) {
		return false
	}
	profile := colorprofile.Detect(out, environ)
	enabled := profile > colorprofile.ASCII
	if enabled {
		if f, ok := out.(*os.File); ok {
			lipgloss.EnableLegacyWindowsANSI(f)
		}
	}
	return enabled
}

// noColorSet reports whether environ's NO_COLOR entry parses as true
// (strconv.ParseBool — the same value convention colorprofile itself uses
// for NO_COLOR/CLICOLOR/CLICOLOR_FORCE, e.g. "NO_COLOR=1"; an unset or
// empty NO_COLOR, or one that fails to parse as a bool, is "not set").
// environ is a "KEY=VALUE" slice (colorprofile.Detect's own shape, e.g.
// os.Environ() in production), not a getenv func, so this scans it
// directly rather than calling out to any environ-lookup helper
// colorprofile itself keeps unexported.
func noColorSet(environ []string) bool {
	for _, kv := range environ {
		key, value, ok := strings.Cut(kv, "=")
		if !ok || key != "NO_COLOR" {
			continue
		}
		set, err := strconv.ParseBool(value)
		return err == nil && set
	}
	return false
}
