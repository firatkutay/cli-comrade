package redact

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFAZ11ApplyInvalidUTF8BytesDoesNotPanic is UYGULAMA_PLANI.md FAZ 11
// item 2's "UTF-8 dışı çıktı" hardening test for the redaction pipeline
// specifically (as opposed to internal/executor's own non-UTF-8 capture
// test): a string built from genuinely invalid UTF-8 byte sequences —
// as internal/executor's capWriter would hand redact.Apply if that raw
// captured output were ever redacted before display — must never panic
// Go's regexp engine (which is byte/rune-oriented, not
// validity-checking), and a secret shape embedded either side of the
// invalid bytes must still be found and masked. This complements
// TestApplyUTF8Safe (FAZ 3), which only proves well-formed multi-byte
// UTF-8 (Turkish prose) survives untouched — a different concern from
// malformed bytes not crashing at all.
func TestFAZ11ApplyInvalidUTF8BytesDoesNotPanic(t *testing.T) {
	r := New(true, true)

	// 0xC3 0x28 is a textbook invalid UTF-8 sequence (0xC3 starts a
	// 2-byte sequence; 0x28 "(" is not a valid continuation byte).
	// Sandwiched between two real secret shapes so the test also proves
	// the invalid bytes don't derail matching on either side of them.
	invalid := []byte("password=hunter2 \xc3\x28 token=abc123")

	var got string
	require.NotPanics(t, func() {
		got = r.Apply(string(invalid))
	})

	assert.Contains(t, got, "password=[REDACTED:credential]")
	assert.Contains(t, got, "token=[REDACTED:credential]")
	assert.NotContains(t, got, "hunter2")
	assert.NotContains(t, got, "abc123")
}
