// Package i18n holds cli-comrade's Turkish and English message catalogs
// and the Translator every command resolves its user-facing output
// through — CLAUDE.md's "Kullanıcıya görünen TÜM metinler internal/i18n
// kataloglarından" rule, and docs/history/UYGULAMA_PLANI.md FAZ 9.
//
// This package is a leaf: it imports only the standard library, so any
// package may depend on it (internal/engine and internal/cli both do)
// without creating an import cycle.
//
// There is no global/package-level Translator anywhere in this package or
// its callers — CLAUDE.md's "Global state yok" rule. Every command
// receives its own Translator, built once per invocation from the
// resolved Lang (see ResolveLanguage) and injected via dependency
// injection, exactly like internal/config.Loader and internal/llm.Client
// already are.
package i18n

import "fmt"

// Catalog maps every MessageID cli-comrade defines to its rendered text in
// one language. A Catalog value is a %-style fmt format string (as fmt
// itself defines "format string" — most messages take no verbs at all).
type Catalog map[MessageID]string

// Translator resolves a MessageID to formatted, user-facing text in one
// resolved language, falling back to English for any key missing from a
// non-English active catalog, and finally to the bare MessageID string
// for a key missing from both — Translator.T never panics on an unknown
// ID.
type Translator struct {
	lang     Lang
	active   Catalog
	fallback Catalog
}

// NewTranslator builds a Translator for lang. Any Lang other than LangTR
// resolves to English — this is also NewTranslator's own fallback for an
// unrecognized Lang value, matching Lang.String's identical default.
func NewTranslator(lang Lang) Translator {
	if lang == LangTR {
		return Translator{lang: LangTR, active: catalogTR, fallback: catalogEN}
	}
	return Translator{lang: LangEN, active: catalogEN, fallback: catalogEN}
}

// Lang reports which language t was built for.
func (t Translator) Lang() Lang {
	if t.active == nil {
		return LangEN
	}
	return t.lang
}

// T resolves id to its formatted text in t's active language: t's own
// catalog first, then the English fallback catalog, then — for an id no
// catalog defines at all — the bare id string itself, so a lookup miss
// degrades to visible-but-harmless text instead of a panic. args are
// applied via fmt.Sprintf exactly when at least one is given; a
// zero-argument call returns the catalog string unchanged (so a message
// containing a literal "%" with no args is never misinterpreted as a
// format verb).
func (t Translator) T(id MessageID, args ...any) string {
	format, ok := t.active[id]
	if !ok {
		format, ok = t.fallback[id]
	}
	if !ok {
		format = string(id)
	}
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}
