package config

import (
	"net/url"
	"strings"
)

// IsDefaultOpenAICompatBaseURL reports whether raw is the SAME endpoint as
// llm.openai_compat.base_url's shipped default
// (Default().LLM.OpenAICompat.BaseURL, "https://api.openai.com/v1") —
// the single source of truth two independent call sites need the exact
// same answer from: internal/llm/pricing.go's EstimateUSD (only apply
// OpenAI's own price table against OpenAI's own endpoint) and
// internal/doctor/check_baseurl.go's BaseURLCheck (only warn about a
// suspected-vendor key when base_url is STILL the default — see that
// check's own doc comment). Both used to run their own bare `!=` string
// compare against the default, which is not "same endpoint" — a
// perfectly legal, config.CheckBaseURL-accepted trailing slash
// (https://api.openai.com/v1/, exactly what `comrade auth login`'s own
// promptOpenAICompatBaseURLIfDefault persists verbatim) or a
// differently-cased scheme/host would silently fail that compare even
// though newOpenAICompatConnector's own strings.TrimRight(baseURL, "/")
// treats it as the identical endpoint at request time.
//
// Comparison rule: both raw and the shipped default are first trimmed of
// leading/trailing whitespace and ANY number of trailing slashes — the
// same identity newOpenAICompatConnector's own TrimRight applies before
// ever issuing a request. If what remains parses as an absolute URL
// (scheme + host present), the scheme and host are compared
// case-insensitively (HTTP scheme/host are case-insensitive by
// definition) while the path is compared EXACTLY as the connector sees
// it — no case-folding, since an OpenAI-compatible gateway's path
// segment is not guaranteed case-insensitive. A raw value that does not
// parse as an absolute URL (no scheme, no host — including "") can never
// equal a real default and is compared as a case-insensitive raw string
// purely so it never panics or false-matches.
func IsDefaultOpenAICompatBaseURL(raw string) bool {
	return normalizeOpenAICompatBaseURL(raw) == normalizeOpenAICompatBaseURL(Default().LLM.OpenAICompat.BaseURL)
}

// normalizeOpenAICompatBaseURL is IsDefaultOpenAICompatBaseURL's own
// identity function — see that function's doc comment for the exact
// rule. Not exported: callers only ever need the boolean comparison, and
// keeping this unexported means the normalized FORM (arbitrary today) is
// never itself a public contract.
func normalizeOpenAICompatBaseURL(raw string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")

	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.ToLower(trimmed)
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + u.Path
}
