package i18n

import "testing"

func TestResolveLanguagePrecedence(t *testing.T) {
	getenvWith := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	locale := func(v string) func() string {
		return func() string { return v }
	}

	cases := []struct {
		name           string
		configLanguage string
		getenv         func(string) string
		systemLocale   func() string
		want           Lang
	}{
		{"explicit config tr wins outright over every env var", "tr", getenvWith(map[string]string{
			"COMRADE_LANG": "en", "LANG": "en_US.UTF-8", "LC_ALL": "en_US.UTF-8",
		}), nil, LangTR},
		{"explicit config en wins outright over every env var", "en", getenvWith(map[string]string{
			"COMRADE_LANG": "tr", "LANG": "tr_TR.UTF-8",
		}), nil, LangEN},
		{"explicit config en wins outright over a tr system locale too", "en", getenvWith(nil), locale("tr-TR"), LangEN},

		{"auto + COMRADE_LANG=tr resolves tr", "auto", getenvWith(map[string]string{"COMRADE_LANG": "tr"}), nil, LangTR},
		{"auto + COMRADE_LANG=TR (case-insensitive) resolves tr", "auto", getenvWith(map[string]string{"COMRADE_LANG": "TR"}), nil, LangTR},
		{"auto + COMRADE_LANG=en resolves en", "auto", getenvWith(map[string]string{"COMRADE_LANG": "en"}), nil, LangEN},
		{"COMRADE_LANG wins over LANG", "auto", getenvWith(map[string]string{
			"COMRADE_LANG": "tr", "LANG": "en_US.UTF-8",
		}), nil, LangTR},

		{"auto + tr_TR.UTF-8 LANG resolves tr", "auto", getenvWith(map[string]string{"LANG": "tr_TR.UTF-8"}), nil, LangTR},
		{"auto + en_US.UTF-8 LANG resolves en", "auto", getenvWith(map[string]string{"LANG": "en_US.UTF-8"}), nil, LangEN},
		{"LANG wins over LC_ALL when both set", "auto", getenvWith(map[string]string{
			"LANG": "en_US.UTF-8", "LC_ALL": "tr_TR.UTF-8",
		}), nil, LangEN},
		{"LC_ALL used when LANG unset", "auto", getenvWith(map[string]string{"LC_ALL": "tr_TR.UTF-8"}), nil, LangTR},

		// -- step 4: system locale, reached only once env vars are all empty --

		{"env vars win over a tr system locale (LANG=en beats systemLocale=tr-TR)", "auto", getenvWith(map[string]string{"LANG": "en_US.UTF-8"}), locale("tr-TR"), LangEN},
		{"env vars win over a tr system locale (COMRADE_LANG=en beats systemLocale=tr-TR)", "auto", getenvWith(map[string]string{"COMRADE_LANG": "en"}), locale("tr-TR"), LangEN},
		{"no env vars set: system locale tr-TR (Windows BCP-47) resolves tr", "auto", getenvWith(nil), locale("tr-TR"), LangTR},
		{"no env vars set: system locale TR-tr (mixed case) resolves tr", "auto", getenvWith(nil), locale("TR-tr"), LangTR},
		{"no env vars set: system locale bare tr resolves tr", "auto", getenvWith(nil), locale("tr"), LangTR},
		{"no env vars set: system locale en-US resolves en", "auto", getenvWith(nil), locale("en-US"), LangEN},
		{"no env vars set: system locale de-DE resolves en", "auto", getenvWith(nil), locale("de-DE"), LangEN},
		{"no env vars set: empty system locale (non-Windows, or a failed Windows syscall) resolves en", "auto", getenvWith(nil), locale(""), LangEN},
		{"nil systemLocale behaves exactly like today (no step 4 at all)", "auto", getenvWith(nil), nil, LangEN},

		{"auto + nothing set resolves en", "auto", getenvWith(nil), nil, LangEN},
		{"empty config language treated like auto", "", getenvWith(map[string]string{"LANG": "tr_TR.UTF-8"}), nil, LangTR},
		{"unrecognized config language falls through to env", "fr", getenvWith(map[string]string{"LANG": "tr_TR.UTF-8"}), nil, LangTR},
		{"nil getenv resolves en", "auto", nil, nil, LangEN},
		{"nil getenv still falls through to a tr system locale", "auto", nil, locale("tr-TR"), LangTR},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveLanguage(tc.configLanguage, tc.getenv, tc.systemLocale)
			if got != tc.want {
				t.Fatalf("ResolveLanguage(%q, ...) = %q, want %q", tc.configLanguage, got, tc.want)
			}
		})
	}
}

// TestResolveLanguageSystemLocaleCalledOnlyWhenReached proves systemLocale
// is consulted lazily — never called at all when an earlier step in the
// chain already resolved the language — so a real GetUserDefaultLocaleName
// syscall on Windows never runs on the far more common "config explicit"
// or "an env var was already set" paths (cold-start budget: this is a
// single fast syscall, but only on the path that actually needs it).
func TestResolveLanguageSystemLocaleCalledOnlyWhenReached(t *testing.T) {
	callCount := func() (probe func() string, calls *int) {
		n := 0
		return func() string { n++; return "tr-TR" }, &n
	}

	t.Run("not called when config is explicit", func(t *testing.T) {
		probe, calls := callCount()
		ResolveLanguage("en", func(string) string { return "" }, probe)
		if *calls != 0 {
			t.Fatalf("systemLocale called %d times, want 0", *calls)
		}
	})

	t.Run("not called when an env var already resolved it", func(t *testing.T) {
		probe, calls := callCount()
		ResolveLanguage("auto", func(name string) string {
			if name == "LANG" {
				return "en_US.UTF-8"
			}
			return ""
		}, probe)
		if *calls != 0 {
			t.Fatalf("systemLocale called %d times, want 0", *calls)
		}
	})

	t.Run("called exactly once when the chain actually reaches it", func(t *testing.T) {
		probe, calls := callCount()
		got := ResolveLanguage("auto", func(string) string { return "" }, probe)
		if *calls != 1 {
			t.Fatalf("systemLocale called %d times, want 1", *calls)
		}
		if got != LangTR {
			t.Fatalf("ResolveLanguage(...) = %q, want %q", got, LangTR)
		}
	})
}

func TestLangString(t *testing.T) {
	if Lang("tr").String() != "tr" {
		t.Fatalf("LangTR.String() = %q, want \"tr\"", Lang("tr").String())
	}
	if Lang("en").String() != "en" {
		t.Fatalf("LangEN.String() = %q, want \"en\"", Lang("en").String())
	}
	if Lang("garbage").String() != "en" {
		t.Fatalf("an unrecognized Lang value must render as \"en\", got %q", Lang("garbage").String())
	}
}
