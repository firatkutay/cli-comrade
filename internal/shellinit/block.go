package shellinit

import (
	"fmt"
	"strings"
)

// Status reports what ApplyBlock did to the rc file content it was
// given.
type Status int

const (
	// StatusInstalled means no cli-comrade block existed yet; one was
	// appended.
	StatusInstalled Status = iota
	// StatusAlreadyInstalled means an existing block's content was
	// byte-identical to the current shell's Block; the input was
	// returned unchanged.
	StatusAlreadyInstalled
	// StatusUpgraded means an existing block's content differed (an
	// older snippet version) and was replaced in place.
	StatusUpgraded
)

// ApplyBlock returns rc file content with shell's Block installed:
// appended if MarkerBegin is absent, replaced in place if present with
// different content, or returned unchanged if present with identical
// content. This is the sole logic backing "comrade init"'s idempotency
// guarantee — running it twice on the same input+shell always yields
// exactly one block.
func ApplyBlock(original string, shell Shell) (updated string, status Status, err error) {
	block, err := Block(shell)
	if err != nil {
		return "", 0, err
	}

	beginIdx := strings.Index(original, MarkerBegin)
	if beginIdx == -1 {
		return appendBlock(original, block), StatusInstalled, nil
	}

	relEnd := strings.Index(original[beginIdx:], MarkerEnd)
	if relEnd == -1 {
		return "", 0, fmt.Errorf("apply block: found %q without a matching %q", MarkerBegin, MarkerEnd)
	}
	endIdx := beginIdx + relEnd + len(MarkerEnd)

	if original[beginIdx:endIdx] == block {
		return original, StatusAlreadyInstalled, nil
	}
	return original[:beginIdx] + block + original[endIdx:], StatusUpgraded, nil
}

// appendBlock appends block to original, adding a blank separator line
// before it when original has existing content (so the block reads as
// its own paragraph), and ensuring original ends in exactly one newline
// first if it didn't already.
func appendBlock(original, block string) string {
	if original == "" {
		return block + "\n"
	}
	if !strings.HasSuffix(original, "\n") {
		original += "\n"
	}
	return original + "\n" + block + "\n"
}

// RemoveBlock deletes the marker-delimited cli-comrade block from
// original, if present, along with the one trailing newline Block/
// appendBlock always leaves right after MarkerEnd and the one blank
// separator line appendBlock adds before it (undoing exactly what
// appendBlock added, so an install-then-remove round trip restores
// original's surrounding content). removed reports whether a block was
// found; a missing MarkerBegin (never installed, or already removed) is
// a no-op returning original unchanged and removed=false — including
// when MarkerBegin is present without a matching MarkerEnd, since a
// half-formed marker pair is left untouched rather than guessed at.
func RemoveBlock(original string) (updated string, removed bool) {
	beginIdx := strings.Index(original, MarkerBegin)
	if beginIdx == -1 {
		return original, false
	}
	relEnd := strings.Index(original[beginIdx:], MarkerEnd)
	if relEnd == -1 {
		return original, false
	}
	endIdx := beginIdx + relEnd + len(MarkerEnd)

	prefix := original[:beginIdx]
	suffix := strings.TrimPrefix(original[endIdx:], "\n")
	if strings.HasSuffix(prefix, "\n\n") {
		prefix = strings.TrimSuffix(prefix, "\n")
	}
	return prefix + suffix, true
}
