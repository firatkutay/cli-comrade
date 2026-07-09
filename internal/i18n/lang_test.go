package i18n

import "testing"

func TestResolveLanguagePrecedence(t *testing.T) {
	getenvWith := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}

	cases := []struct {
		name           string
		configLanguage string
		getenv         func(string) string
		want           Lang
	}{
		{"explicit config tr wins outright over every env var", "tr", getenvWith(map[string]string{
			"COMRADE_LANG": "en", "LANG": "en_US.UTF-8", "LC_ALL": "en_US.UTF-8",
		}), LangTR},
		{"explicit config en wins outright over every env var", "en", getenvWith(map[string]string{
			"COMRADE_LANG": "tr", "LANG": "tr_TR.UTF-8",
		}), LangEN},

		{"auto + COMRADE_LANG=tr resolves tr", "auto", getenvWith(map[string]string{"COMRADE_LANG": "tr"}), LangTR},
		{"auto + COMRADE_LANG=TR (case-insensitive) resolves tr", "auto", getenvWith(map[string]string{"COMRADE_LANG": "TR"}), LangTR},
		{"auto + COMRADE_LANG=en resolves en", "auto", getenvWith(map[string]string{"COMRADE_LANG": "en"}), LangEN},
		{"COMRADE_LANG wins over LANG", "auto", getenvWith(map[string]string{
			"COMRADE_LANG": "tr", "LANG": "en_US.UTF-8",
		}), LangTR},

		{"auto + tr_TR.UTF-8 LANG resolves tr", "auto", getenvWith(map[string]string{"LANG": "tr_TR.UTF-8"}), LangTR},
		{"auto + en_US.UTF-8 LANG resolves en", "auto", getenvWith(map[string]string{"LANG": "en_US.UTF-8"}), LangEN},
		{"LANG wins over LC_ALL when both set", "auto", getenvWith(map[string]string{
			"LANG": "en_US.UTF-8", "LC_ALL": "tr_TR.UTF-8",
		}), LangEN},
		{"LC_ALL used when LANG unset", "auto", getenvWith(map[string]string{"LC_ALL": "tr_TR.UTF-8"}), LangTR},

		{"auto + nothing set resolves en", "auto", getenvWith(nil), LangEN},
		{"empty config language treated like auto", "", getenvWith(map[string]string{"LANG": "tr_TR.UTF-8"}), LangTR},
		{"unrecognized config language falls through to env", "fr", getenvWith(map[string]string{"LANG": "tr_TR.UTF-8"}), LangTR},
		{"nil getenv resolves en", "auto", nil, LangEN},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveLanguage(tc.configLanguage, tc.getenv)
			if got != tc.want {
				t.Fatalf("ResolveLanguage(%q, ...) = %q, want %q", tc.configLanguage, got, tc.want)
			}
		})
	}
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
