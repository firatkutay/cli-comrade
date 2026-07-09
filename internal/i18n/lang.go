package i18n

import "strings"

// Lang is one of cli-comrade's two supported UI languages.
type Lang string

const (
	// LangEN is English — the catalog's own fallback language (see
	// Translator.T) and the default when nothing else resolves a
	// language.
	LangEN Lang = "en"
	// LangTR is Turkish.
	LangTR Lang = "tr"
)

// String renders l as its canonical two-letter code.
func (l Lang) String() string {
	if l == LangTR {
		return string(LangTR)
	}
	return string(LangEN)
}

// ResolveLanguage implements CLAUDE.md's "Dil seçimi" precedence — the
// single source of truth for every language decision in cli-comrade,
// consolidating what used to be internal/engine's own resolveLanguage
// (FAZ 5/7) with FAZ 9's full i18n catalog:
//
//  1. configLanguage: an explicit, non-"auto" config general.language
//     value ("tr" or "en") wins outright.
//  2. COMRADE_LANG: this project's own COMRADE_-prefixed override
//     (consistent with FAZ 1's COMRADE_ env convention — see
//     UYGULAMA_PLANI.md FAZ 9's acceptance criterion, which drives this
//     from `COMRADE_LANG=tr comrade explain ...`). A value starting with
//     "tr" (case-insensitive) resolves to Turkish; any other non-empty
//     value resolves to English.
//  3. LANG, then LC_ALL: the first of the two that is set is parsed the
//     same way glibc locale names are (e.g. "tr_TR.UTF-8" starts with
//     "tr") — a "tr"-prefixed value resolves to Turkish, anything else to
//     English.
//  4. Otherwise: English.
//
// getenv is injectable (production passes os.Getenv) so this stays a pure,
// testable function; a nil getenv is treated as "nothing set" rather than
// panicking.
func ResolveLanguage(configLanguage string, getenv func(string) string) Lang {
	switch configLanguage {
	case "tr":
		return LangTR
	case "en":
		return LangEN
	}

	if getenv == nil {
		return LangEN
	}

	if v := getenv("COMRADE_LANG"); v != "" {
		return parseLocaleLikeValue(v)
	}

	for _, name := range []string{"LANG", "LC_ALL"} {
		if v := getenv(name); v != "" {
			return parseLocaleLikeValue(v)
		}
	}

	return LangEN
}

// parseLocaleLikeValue resolves a single non-empty environment value (a
// glibc-style locale name for LANG/LC_ALL, or a plain language code for
// COMRADE_LANG) to a Lang: a "tr"-prefixed value (case-insensitive) is
// Turkish, everything else is English.
func parseLocaleLikeValue(v string) Lang {
	if strings.HasPrefix(strings.ToLower(v), "tr") {
		return LangTR
	}
	return LangEN
}
