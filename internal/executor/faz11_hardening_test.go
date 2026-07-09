package executor

import (
	"bytes"
	"context"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFAZ11RunCapsOneMegabyteOfStdoutWithoutUnboundedGrowth is
// UYGULAMA_PLANI.md FAZ 11 item 2's "çok uzun stderr (kuyruk kırpma)"
// hardening test, at a scale (~1.28MB, well over 100x maxCaptureBytes)
// meant to prove the cap holds under real volume, not just the smaller
// multiple TestRunCapCapturesOnlyTailOfLongOutput already exercises: the
// captured Result.Stdout never grows past maxCaptureBytes regardless of
// how much the child process actually writes (no unbounded buffer, no
// OOM), and the tail (not the head) survives truncation.
func TestFAZ11RunCapsOneMegabyteOfStdoutWithoutUnboundedGrowth(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := newUnixExecutor(&stdout, &stderr)

	// 40000 lines * 32 bytes/line ≈ 1.28MB of stdout, far more than
	// maxCaptureBytes (8KB) — only the capped tail may survive in
	// Result.Stdout, though the live-streamed copy (stdout above) still
	// receives the whole thing uncapped, proving the cap is specific to
	// the captured Result field.
	cmd := "for i in $(seq 1 40000); do printf 'line-%05d-XXXXXXXXXXXXXXXXXXXX\\n' \"$i\"; done"
	res, err := e.Run(context.Background(), cmd, Options{})

	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode)
	assert.LessOrEqual(t, len(res.Stdout), maxCaptureBytes, "captured Stdout must never exceed the documented cap")
	assert.Contains(t, res.Stdout, "line-40000", "the tail of the output must be preserved")
	assert.NotContains(t, res.Stdout, "line-00001-", "the head must have been truncated away")
	// The live-streamed copy is NOT capped — the full ~1.28MB is still
	// there, proving maxCaptureBytes only bounds Result.Stdout.
	assert.Greater(t, stdout.Len(), maxCaptureBytes*10)
}

// TestFAZ11RunNonUTF8StdoutDoesNotPanic is UYGULAMA_PLANI.md FAZ 11 item
// 2's "UTF-8 dışı çıktı" hardening test: a child process that writes
// invalid UTF-8 bytes to stdout must be captured without Run panicking
// or erroring, and the resulting Result.Stdout — while not valid UTF-8 —
// must still be a normal Go string a caller can safely pass to len(),
// strings.Contains, etc. capWriter.Write/String never assume valid
// UTF-8 (a Go string is just a byte sequence), so this is a fixed-shape
// regression test, not a bug this run found.
func TestFAZ11RunNonUTF8StdoutDoesNotPanic(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := newUnixExecutor(&stdout, &stderr)

	// printf's POSIX \NNN octal escapes emit raw bytes (dash's printf
	// builtin, unlike bash's, does not support \xHH): \303\050 is 0xC3
	// 0x28, a textbook invalid UTF-8 sequence (0xC3 starts a 2-byte
	// sequence, but 0x28 "(" is not a valid continuation byte).
	cmd := `printf 'before-\303\050-after'`

	require.NotPanics(t, func() {
		res, err := e.Run(context.Background(), cmd, Options{})
		require.NoError(t, err)
		assert.Equal(t, 0, res.ExitCode)
		assert.Contains(t, res.Stdout, "before-")
		assert.Contains(t, res.Stdout, "-after")
		assert.False(t, utf8.ValidString(res.Stdout), "the captured bytes are expected to be invalid UTF-8 — proving this doesn't crash is the point")
	})
}
