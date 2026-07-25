package i18n

import (
	"regexp"
	"strings"
	"testing"
)

// formatVerbRe matches one fmt-style format verb (%s, %d, %q, %v, ...).
// Applied AFTER stripping literal "%%" escapes (fmt's own rule for a
// literal percent sign, which introduces no interpolation argument), so
// countFormatVerbs never miscounts a literal "%" as a verb.
var formatVerbRe = regexp.MustCompile(`%[a-zA-Z]`)

// countFormatVerbs returns how many fmt format verbs appear in s.
func countFormatVerbs(s string) int {
	return len(formatVerbRe.FindAllString(strings.ReplaceAll(s, "%%", ""), -1))
}

// TestCatalogsCoverIdenticalKeys is the bidirectional drift guard
// docs/history/UYGULAMA_PLANI.md FAZ 9 calls for: catalogEN and catalogTR must define
// exactly the same MessageID set, checked in BOTH directions, so a
// MessageID added to only one catalog (in either direction) fails CI
// instead of silently falling back at runtime — see Translator.T's
// fallback behavior, which would otherwise mask the gap.
func TestCatalogsCoverIdenticalKeys(t *testing.T) {
	for id := range catalogEN {
		if _, ok := catalogTR[id]; !ok {
			t.Errorf("MessageID %q is defined in catalogEN but missing from catalogTR", id)
		}
	}
	for id := range catalogTR {
		if _, ok := catalogEN[id]; !ok {
			t.Errorf("MessageID %q is defined in catalogTR but missing from catalogEN", id)
		}
	}
}

// TestCatalogsHaveNoEmptyValues guards against a copy-paste placeholder
// (an accidentally empty translation) in either catalog.
func TestCatalogsHaveNoEmptyValues(t *testing.T) {
	for id, v := range catalogEN {
		if v == "" {
			t.Errorf("catalogEN[%q] is empty", id)
		}
	}
	for id, v := range catalogTR {
		if v == "" {
			t.Errorf("catalogTR[%q] is empty", id)
		}
	}
}

// TestCatalogsHaveMatchingFormatVerbCounts is the format-verb parity
// guard TestCatalogsCoverIdenticalKeys/TestCatalogsHaveNoEmptyValues
// don't cover: for every MessageID present in both catalogs, EN and TR
// must use the SAME NUMBER of fmt format verbs (%s/%d/%q/%v/...). Only
// key-presence and non-emptiness were guarded before this test — an
// arg-count mismatch between languages (a translation that drops or
// adds an interpolation) shipped silently and would only surface as a
// "%!s(MISSING)"/"%!(EXTRA ...)" artifact in exactly ONE language, at
// render time, in production.
func TestCatalogsHaveMatchingFormatVerbCounts(t *testing.T) {
	for id, en := range catalogEN {
		tr, ok := catalogTR[id]
		if !ok {
			continue // TestCatalogsCoverIdenticalKeys already reports this gap
		}
		enCount := countFormatVerbs(en)
		trCount := countFormatVerbs(tr)
		if enCount != trCount {
			t.Errorf("MessageID %q: catalogEN has %d format verb(s) (%q) but catalogTR has %d (%q)", id, enCount, en, trCount, tr)
		}
	}
}
