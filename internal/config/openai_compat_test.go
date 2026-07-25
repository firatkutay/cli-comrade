package config

import (
	"strings"
	"testing"
)

// TestIsDefaultOpenAICompatBaseURL pins IsDefaultOpenAICompatBaseURL's
// exact identity rule (see its own doc comment): trailing-slash and
// scheme/host-case variants of the shipped default must still count as
// "the default", while a genuinely different endpoint (or garbage input)
// must not. Also pins the anti-spoofing property an adversarial probe
// exercised manually (userinfo spoof, percent-encoded host, homograph,
// protocol-relative, path-embedded host, and pathological-length inputs)
// as committed, re-runnable tests rather than a one-off check.
func TestIsDefaultOpenAICompatBaseURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "exact default", raw: "https://api.openai.com/v1", want: true},
		{name: "single trailing slash", raw: "https://api.openai.com/v1/", want: true},
		{name: "double trailing slash", raw: "https://api.openai.com/v1//", want: true},
		{name: "uppercase scheme", raw: "HTTPS://api.openai.com/v1", want: true},
		{name: "uppercase host", raw: "https://API.OPENAI.COM/v1", want: true},
		{name: "uppercase scheme and host with trailing slash", raw: "HTTPS://API.OpenAI.com/v1/", want: true},
		{name: "leading/trailing whitespace", raw: "  https://api.openai.com/v1  ", want: true},
		{name: "empty string", raw: "", want: false},
		{name: "genuinely different provider — Qwen-style gateway", raw: "https://dashscope.aliyuncs.com/compatible-mode/v1", want: false},
		{name: "different path under the same host", raw: "https://api.openai.com/v2", want: false},
		{name: "path case must NOT be folded", raw: "https://api.openai.com/V1", want: false},
		{name: "unparseable garbage", raw: "not-a-url", want: false},

		// N2 regression: a trailing slash followed by a query string is a
		// MID-STRING slash, not a whole-string-final one — a naive
		// pre-parse TrimRight(raw, "/") cannot see it. Parsing first, then
		// trimming the PARSED path, must still recognize this as the
		// default (matching the equivalent no-trailing-slash query-string
		// spelling right below it).
		{name: "trailing slash before a query string", raw: "https://api.openai.com/v1/?x=1", want: true},
		{name: "query string, no trailing slash", raw: "https://api.openai.com/v1?x=1", want: true},

		// Anti-spoofing table — pins the adversarial probe's findings as
		// real, committed tests rather than a one-off manual check. Every
		// case here MUST fail closed (want: false): each superficially
		// resembles or embeds the real default somewhere in the string,
		// but none of them is net/url's own Host field equal to
		// "api.openai.com".
		{name: "userinfo spoof — fake host used as userinfo", raw: "https://api.openai.com@evil.example.com/v1", want: false},
		{name: "percent-encoded host", raw: "https://api%2Eopenai.com/v1", want: false},
		{name: "homograph host — Cyrillic а instead of Latin a", raw: "https://аpi.openai.com/v1", want: false},
		{name: "protocol-relative — no scheme at all", raw: "//api.openai.com/v1", want: false},
		{name: "path-embedded host — real host only appears in the path", raw: "https://evil.example.com/api.openai.com/v1", want: false},
		{name: "very long unparseable-shaped input does not panic", raw: strings.Repeat("a", 200_000), want: false},
		{name: "very long slash run does not panic", raw: "https://api.openai.com/v1" + strings.Repeat("/", 100_000), want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsDefaultOpenAICompatBaseURL(tc.raw)
			if got != tc.want {
				t.Errorf("IsDefaultOpenAICompatBaseURL(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
