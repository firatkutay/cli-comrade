package cli

import (
	"crypto/rand"
	"encoding/hex"
)

// newRunID generates an 8-byte, cryptographically random, hex-encoded
// identifier (16 hex characters) that groups every audit.Entry a single
// do/fix/chat-"/do" invocation appends into one "run" — see
// audit.Entry.RunID's own doc comment, and `comrade undo`'s target
// selection, which is the reason this exists at all. A crypto/rand read
// failure (vanishingly rare — the OS CSPRNG being unavailable) degrades
// to an empty string rather than aborting the caller's real command:
// audit.Entry.RunID already treats an empty RunID identically to a
// pre-undo-support entry (never eligible for automatic undo, but every
// other part of the command still works), which is a strictly safer
// failure mode than crashing do/fix/chat over a run-grouping id.
func newRunID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}
