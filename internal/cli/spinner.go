package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// waitSpinnerFrames borrows only bubbles/v2/spinner's frame DATA
// (spinner.MiniDot — a braille frame set, exactly what Part 2(d) asks for)
// rather than its tea.Model/Update/View machinery. Every one of this
// spinner's four call sites (do/fix planning+diagnosis, explain, chat's
// "/do") calls its blocking LLM request from a plain synchronous function —
// none of them run inside an active bubbletea.Program at the point of that
// call (chat's "/do" specifically ReleaseTerminal()s its outer Program
// first — see chatmodel.go's newRealChatDoRunner) — so driving bubbles'
// own spinner.Model would mean spinning up a second, otherwise-pointless
// tea.Program around one blocking call; a plain goroutine + time.Ticker
// reusing bubbles' own well-tested frame/FPS table is the same visual
// result with far less machinery, matching this task's own "prefer no new
// dependency, minimal hand-rolled spinner is fine" guidance.
var waitSpinnerFrames = spinner.MiniDot

// waitSpinnerStyle is the spinner glyph+label's color — the SAME pastel
// lavender used for --help's section headers (help.go's helpHeaderStyle),
// for one consistent "this is UI chrome, not program output" accent color
// across both surfaces, again a fixed ANSI256 code rather than
// AdaptiveColor/compat for the same live-terminal-query cost documented on
// help.go's helpHeaderStyle.
var waitSpinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(paletteLavender))

// startWaitSpinner starts an animated braille spinner labeled per tr
// (i18n.MsgSpinnerThinking) on out, and returns a stop function the caller
// MUST call exactly once, unconditionally — on success, on error, and on
// context cancellation alike. The expected call shape at every call site
// is:
//
//	stop := startWaitSpinner(enabled, cmd.ErrOrStderr(), tr)
//	result, err := blockingLLMCall(...)
//	stop()
//	if err != nil { ... }
//
// stop() always blocks until the spinner goroutine has fully exited AND
// the line has been cleared (an ANSI "erase in line" sequence, not a
// fixed-width space-overwrite, so it is correct regardless of the active
// language's label length) BEFORE returning — so nothing the caller prints
// next (a stream chunk, an error message, the final result) can ever land
// on the same line as a spinner frame, and the goroutine can never
// outlive its caller: the same unconditional-cleanup discipline
// internal/llm's sendChunk/ctx.Done() guard established for the Stream
// goroutine-leak fix earlier in this project's history.
//
// enabled is the caller's own resolveColorEnabled(cfg, os.Environ(),
// <stderr>) result — internal/cli's single color-decision point,
// evaluated against stderr specifically (this spinner always renders
// there, regardless of where the command's own real output goes), per
// Part 2(c)'s explicit "don't create a second color-decision path"
// instruction. enabled=false (out is not a TTY, general.color=false,
// NO_COLOR is set, or CLICOLOR_FORCE was needed but absent) makes this a
// complete no-op: no goroutine is ever started, out is never written to at
// all — "invisible entirely in non-TTY/piped runs".
func startWaitSpinner(enabled bool, out io.Writer, tr i18n.Translator) (stop func()) {
	if !enabled {
		return func() {}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	label := tr.T(i18n.MsgSpinnerThinking)

	go func() {
		defer close(done)
		ticker := time.NewTicker(waitSpinnerFrames.FPS)
		defer ticker.Stop()
		frame := 0
		for {
			fmt.Fprint(out, "\r"+waitSpinnerStyle.Render(waitSpinnerFrames.Frames[frame%len(waitSpinnerFrames.Frames)])+" "+label) //nolint:errcheck // best-effort spinner frame write; nowhere to report a stderr write failure mid-animation
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				frame++
			}
		}
	}()

	return func() {
		cancel()
		<-done
		fmt.Fprint(out, "\r\x1b[K") //nolint:errcheck // best-effort clear; same rationale as the frame write above
	}
}
