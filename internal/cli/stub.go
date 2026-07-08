package cli

import (
	"fmt"
	"io"
)

// notReadyMessage builds the single hardcoded English message printed by
// every unimplemented subcommand stub. FAZ 9 replaces this helper's body
// with an internal/i18n catalog lookup so every call site below stays
// unchanged.
func notReadyMessage(feature string) string {
	return fmt.Sprintf("comrade %s: this feature is not ready yet.", feature)
}

// printNotReady writes the shared "not ready" message for the given
// feature name to w, followed by a newline. Write errors on the command's
// own output stream are not actionable here, so they are intentionally
// discarded.
func printNotReady(w io.Writer, feature string) {
	_, _ = fmt.Fprintln(w, notReadyMessage(feature))
}
