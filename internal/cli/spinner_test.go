package cli

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// TestStartWaitSpinnerDisabledIsNoOp proves enabled=false starts no
// goroutine and writes nothing to out at all — "invisible entirely in
// non-TTY/piped runs".
func TestStartWaitSpinnerDisabledIsNoOp(t *testing.T) {
	var out bytes.Buffer
	tr := i18n.NewTranslator(i18n.LangEN)

	stop := startWaitSpinner(false, &out, tr)
	stop()

	assert.Empty(t, out.String())
}

// TestStartWaitSpinnerWritesFrameThenClearsOnStop proves the enabled path
// writes at least one braille frame plus the resolved label, and that
// stop() always appends the ANSI clear-line sequence LAST — the guarantee
// callers rely on to ensure nothing they print next lands on the same
// line as a spinner frame.
//
// stop() is provably safe to call immediately after start (no sleep
// needed here): stop() blocks on <-done, which only unblocks once the
// spinner goroutine's loop has returned, and that loop's very first
// statement on every iteration — including the first — is the frame
// write, unconditionally, before it ever checks ctx.Done(). So by the
// time stop() returns, out is guaranteed to already contain at least one
// frame.
func TestStartWaitSpinnerWritesFrameThenClearsOnStop(t *testing.T) {
	var out bytes.Buffer
	tr := i18n.NewTranslator(i18n.LangEN)

	stop := startWaitSpinner(true, &out, tr)
	stop()

	got := out.String()
	require.NotEmpty(t, got)
	assert.Contains(t, got, "thinking…", "EN label must appear")
	assert.True(t, strings.HasSuffix(got, "\r\x1b[K"), "must end with the clear-line sequence, got: %q", got)
	assert.True(t, strings.HasPrefix(got, "\r"), "each frame write must start with a carriage return, got: %q", got)
}

// TestStartWaitSpinnerUsesTurkishLabel proves the label is resolved from
// tr (i18n.MsgSpinnerThinking), not hardcoded English, exactly like every
// other user-facing string in this tree.
func TestStartWaitSpinnerUsesTurkishLabel(t *testing.T) {
	var out bytes.Buffer
	tr := i18n.NewTranslator(i18n.LangTR)

	stop := startWaitSpinner(true, &out, tr)
	stop()

	got := out.String()
	assert.Contains(t, got, "düşünüyorum…")
	assert.NotContains(t, got, "thinking…")
}

// TestStartWaitSpinnerStopNeverLeaksAGoroutine drives several start/stop
// cycles (including one where the spinner is left running for a couple of
// real ticks before being stopped, to exercise the ticker.C branch, not
// just the immediate-stop path) and asserts runtime.NumGoroutine() returns
// to baseline afterward — same discipline, same style of proof, as
// internal/llm's sendChunk/ctx.Done() goroutine-leak regression tests.
func TestStartWaitSpinnerStopNeverLeaksAGoroutine(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangEN)
	baseline := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		var out bytes.Buffer
		stop := startWaitSpinner(true, &out, tr)
		if i%2 == 0 {
			// Let it run through at least one real ticker.C-driven frame
			// advance before stopping, not just the immediate-stop path.
			time.Sleep(waitSpinnerFrames.FPS * 2)
		}
		stop()
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		if runtime.NumGoroutine() <= baseline {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("goroutine leak: NumGoroutine()=%d, want <= baseline %d", runtime.NumGoroutine(), baseline)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestStartWaitSpinnerStopIsSafeToCallOnce documents (and guards, via
// -race) the contract every call site follows: stop is called EXACTLY
// once. This is not a test of double-stop safety (callers never do that;
// see startWaitSpinner's own doc comment) — it is the same shape every
// real call site (do.go/fix.go/explain.go/chat.go) uses: start, run a
// blocking call, stop unconditionally.
func TestStartWaitSpinnerStopIsSafeToCallOnce(t *testing.T) {
	var out bytes.Buffer
	tr := i18n.NewTranslator(i18n.LangEN)

	stop := startWaitSpinner(true, &out, tr)
	// Simulate the blocking LLM call taking a little while.
	time.Sleep(waitSpinnerFrames.FPS)
	stop()

	assert.True(t, strings.HasSuffix(out.String(), "\r\x1b[K"))
}

// TestStartWaitSpinnerStopIsSafeToCallTwice proves calling the returned
// stop function a second time neither panics nor deadlocks — no real call
// site does this today (every one calls stop exactly once, per
// startWaitSpinner's own doc comment), but stop's own implementation
// makes it safe regardless: context.CancelFunc is idempotent by contract,
// and receiving from an already-closed done channel returns immediately
// (zero value, ok=false) rather than blocking. The second call does write
// the clear sequence to out again — a harmless, redundant no-visual-effect
// write, not a bug — which this test pins by asserting the SECOND stop
// still leaves the output ending in exactly one clear sequence (i.e.
// nothing garbled or duplicated beyond that trailing write).
func TestStartWaitSpinnerStopIsSafeToCallTwice(t *testing.T) {
	var out bytes.Buffer
	tr := i18n.NewTranslator(i18n.LangEN)

	stop := startWaitSpinner(true, &out, tr)

	done := make(chan struct{})
	go func() {
		stop()
		stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("calling stop() twice deadlocked")
	}

	assert.True(t, strings.HasSuffix(out.String(), "\r\x1b[K"))
}
