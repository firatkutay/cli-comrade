package i18n

import "testing"

// TestCatalogsCoverIdenticalKeys is the bidirectional drift guard
// UYGULAMA_PLANI.md FAZ 9 calls for: catalogEN and catalogTR must define
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
