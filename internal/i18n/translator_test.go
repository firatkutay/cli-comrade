package i18n

import "testing"

func TestNewTranslatorSelectsCatalogByLang(t *testing.T) {
	en := NewTranslator(LangEN)
	if en.Lang() != LangEN {
		t.Fatalf("NewTranslator(LangEN).Lang() = %q, want %q", en.Lang(), LangEN)
	}
	tr := NewTranslator(LangTR)
	if tr.Lang() != LangTR {
		t.Fatalf("NewTranslator(LangTR).Lang() = %q, want %q", tr.Lang(), LangTR)
	}
	unknown := NewTranslator(Lang("fr"))
	if unknown.Lang() != LangEN {
		t.Fatalf("NewTranslator of an unrecognized Lang must default to English, got %q", unknown.Lang())
	}
}

func TestTranslatorTAppliesActiveCatalogAndInterpolates(t *testing.T) {
	tr := NewTranslator(LangTR)
	got := tr.T(MsgFirstRunNotice, "/tmp/config.toml")
	want := "Varsayılan ayar dosyası oluşturuldu: /tmp/config.toml\n"
	if got != want {
		t.Fatalf("tr.T(MsgFirstRunNotice, ...) = %q, want %q", got, want)
	}

	en := NewTranslator(LangEN)
	got = en.T(MsgFirstRunNotice, "/tmp/config.toml")
	want = "Created default config at /tmp/config.toml\n"
	if got != want {
		t.Fatalf("en.T(MsgFirstRunNotice, ...) = %q, want %q", got, want)
	}
}

func TestTranslatorTZeroArgsReturnsFormatUnchanged(t *testing.T) {
	tr := Translator{lang: LangEN, active: Catalog{"pct": "100% done"}, fallback: Catalog{}}
	if got := tr.T("pct"); got != "100% done" {
		t.Fatalf("T with zero args must return the format string unchanged even with a literal %%, got %q", got)
	}
}

// TestTranslatorTFallsBackToEnglishThenBareID directly exercises
// Translator's 3-tier resolution (active catalog -> fallback catalog ->
// bare MessageID) using hand-built catalogs, independent of the real
// catalogEN/catalogTR content, so this test cannot be defeated by the two
// real catalogs happening to define the exact same key set (which
// TestCatalogsCoverIdenticalKeys in catalog_test.go requires anyway).
func TestTranslatorTFallsBackToEnglishThenBareID(t *testing.T) {
	active := Catalog{"only_in_active": "active: %s"}
	fallback := Catalog{"only_in_fallback": "fallback: %s", "only_in_active": "SHOULD NOT WIN"}
	tr := Translator{lang: LangTR, active: active, fallback: fallback}

	if got := tr.T("only_in_active", "x"); got != "active: x" {
		t.Fatalf("a key present in the active catalog must use it, got %q", got)
	}
	if got := tr.T("only_in_fallback", "y"); got != "fallback: y" {
		t.Fatalf("a key missing from the active catalog must fall back to the fallback catalog, got %q", got)
	}
	if got := tr.T("missing_everywhere"); got != "missing_everywhere" {
		t.Fatalf("a key missing from both catalogs must degrade to the bare MessageID, got %q", got)
	}
}
