package envkeys

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStripManagedRemovesExactCaseMatch proves the common case: an
// entry whose key is byte-for-byte ManagedByEnvVar is removed.
func TestStripManagedRemovesExactCaseMatch(t *testing.T) {
	in := []string{"PATH=/usr/bin", "COMRADE_MANAGED_BY=npm", "HOME=/home/user"}

	got := StripManaged(in)

	assert.Equal(t, []string{"PATH=/usr/bin", "HOME=/home/user"}, got)
}

// TestStripManagedIsCaseInsensitive is PR #37 review's N2 regression
// guard: Windows environment-variable lookups are case-insensitive at
// the OS level, so update.IsNPMManaged's env signal can DETECT a
// lower/mixed-case variant of ManagedByEnvVar on that platform — the
// strip must remove every case variant too, or the variable would be
// detected but survive into a spawned child unstripped.
func TestStripManagedIsCaseInsensitive(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"uppercase (the documented/canonical form)", "COMRADE_MANAGED_BY"},
		{"lowercase", "comrade_managed_by"},
		{"mixed case", "Comrade_Managed_By"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := []string{"PATH=/usr/bin", tt.key + "=npm"}

			got := StripManaged(in)

			assert.Equal(t, []string{"PATH=/usr/bin"}, got, "key %q must be stripped regardless of case", tt.key)
		})
	}
}

// TestStripManagedPreservesEverythingElse proves the strip is scoped to
// exactly ManagedByEnvVar — every other entry, including one whose KEY
// merely contains "MANAGED_BY" as a substring, must survive unchanged.
func TestStripManagedPreservesEverythingElse(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"SOME_OTHER_MANAGED_BY_THING=untouched",
		"COMRADE_MANAGED_BY_SUFFIXED=untouched",
		"HOME=/home/user",
	}

	got := StripManaged(in)

	assert.Equal(t, in, got, "no entry other than an EXACT (case-insensitive) key match must ever be removed")
}

// TestStripManagedEmptyInput proves an empty/nil input is handled
// without panicking and returns an empty (not nil-vs-empty-ambiguous)
// result callers can safely range over.
func TestStripManagedEmptyInput(t *testing.T) {
	got := StripManaged(nil)

	assert.Empty(t, got)
}
