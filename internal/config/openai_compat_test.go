package config

import "testing"

// TestIsDefaultOpenAICompatBaseURL pins IsDefaultOpenAICompatBaseURL's
// exact identity rule (see its own doc comment): trailing-slash and
// scheme/host-case variants of the shipped default must still count as
// "the default", while a genuinely different endpoint (or garbage input)
// must not.
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
