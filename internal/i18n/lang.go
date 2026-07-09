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
// (FAZ 5/7) with FAZ 9's full i18n catalog, extended for Windows (whose
// non-glibc environment leaves LANG/LC_ALL unset even when the user's OS
// locale IS Turkish — see locale_windows.go):
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
//  4. systemLocale: an OS-level locale probe, consulted ONLY once steps
//     1-3 all found nothing set. On Windows this calls
//     GetUserDefaultLocaleName (locale_windows.go), returning a BCP-47
//     tag like "tr-TR" — parsed by the exact same "tr"-prefix rule as
//     LANG/LC_ALL (parseLocaleLikeValue), verified by
//     TestResolveLanguagePrecedence's own BCP-47 cases. On every other
//     GOOS it is always "" (locale_other.go): LANG/LC_ALL already ARE
//     the OS locale mechanism on Linux/macOS, so this step is a genuine
//     no-op there, never a second chance at an answer steps 2-3 already
//     had every opportunity to give — Linux/macOS behavior is therefore
//     byte-identical to before this step existed.
//  5. Otherwise: English.
//
// getenv is injectable (production passes os.Getenv) so this stays a
// pure, testable function; a nil getenv is treated as "nothing set"
// rather than panicking. systemLocale is injectable the same way
// (production passes SystemLocale) so step 4 — including its Windows
// behavior — is table-testable from any host GOOS without touching a
// real Windows environment; a nil systemLocale is likewise treated as
// "nothing to probe". systemLocale is called at most once, and only when
// the chain actually reaches step 4 (never unconditionally/eagerly), so
// a real GetUserDefaultLocaleName syscall never runs on the far more
// common "an env var was already set" path.
func ResolveLanguage(configLanguage string, getenv func(string) string, systemLocale func() string) Lang {
	switch configLanguage {
	case "tr":
		return LangTR
	case "en":
		return LangEN
	}

	if getenv != nil {
		if v := getenv("COMRADE_LANG"); v != "" {
			return parseLocaleLikeValue(v)
		}

		for _, name := range []string{"LANG", "LC_ALL"} {
			if v := getenv(name); v != "" {
				return parseLocaleLikeValue(v)
			}
		}
	}

	if systemLocale != nil {
		if v := systemLocale(); v != "" {
			return parseLocaleLikeValue(v)
		}
	}

	return LangEN
}

// parseLocaleLikeValue resolves a single non-empty locale-like value — a
// glibc-style locale name for LANG/LC_ALL (e.g. "tr_TR.UTF-8"), a plain
// language code for COMRADE_LANG, or a Windows BCP-47 locale tag from
// SystemLocale (e.g. "tr-TR") — to a Lang: a "tr"-prefixed value
// (case-insensitive) is Turkish, everything else is English. All three
// source shapes share this one rule because all three put the language
// code first, however the rest of the value is formatted.
func parseLocaleLikeValue(v string) Lang {
	if strings.HasPrefix(strings.ToLower(v), "tr") {
		return LangTR
	}
	return LangEN
}
