package envkeys

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNPMDispatcherEnvSignalMatchesGoConstants is the bidirectional
// drift guard for ManagedByEnvVar/ManagedByEnvValueNPM's cross-language
// mirror (PR #37 review, MEDIUM-3; moved here alongside the constants
// themselves per P3): npm/main/bin/comrade.js hardcodes the SAME env
// var name and value as a JS object-literal property, with no shared
// source of truth tying the two languages together — a rename on
// either side alone leaves the other silently dead. The review found
// exactly this: renaming only the Go const left every existing test
// green while the primary (env) signal quietly stopped firing for real
// npm installs, degrading to the path-only fallback.
//
// This reads the actual dispatcher source (not a copy/fixture) and
// asserts it sets ManagedByEnvVar to EXACTLY ManagedByEnvValueNPM — a
// bare two-substring check would be insufficient here, since the
// literal "npm" (ManagedByEnvValueNPM) appears throughout this file for
// unrelated reasons (package names, comments), so it must confirm the
// two appear together as one key:value pair, not merely somewhere each.
func TestNPMDispatcherEnvSignalMatchesGoConstants(t *testing.T) {
	dispatcherPath := filepath.Join("..", "..", "npm", "main", "bin", "comrade.js")
	src, err := os.ReadFile(dispatcherPath) // #nosec G304 -- fixed, repo-relative path to this repo's own source file, not attacker-controlled input
	require.NoError(t, err, "the npm dispatcher must be readable from internal/envkeys' own test at %s — did it move?", dispatcherPath)

	pattern := regexp.MustCompile(regexp.QuoteMeta(ManagedByEnvVar) + `\s*:\s*['"]` + regexp.QuoteMeta(ManagedByEnvValueNPM) + `['"]`)
	assert.Regexp(t, pattern, string(src),
		"npm/main/bin/comrade.js must set %s to exactly %q (as a JS object-literal property) — the Go and JS literals have drifted apart; update whichever side changed to match the other",
		ManagedByEnvVar, ManagedByEnvValueNPM)
}
