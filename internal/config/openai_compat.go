package config

import (
	"net/url"
	"strings"
)

// IsDefaultOpenAICompatBaseURL reports whether raw is the SAME endpoint as
// llm.openai_compat.base_url's shipped default
// (Default().LLM.OpenAICompat.BaseURL, "https://api.openai.com/v1") —
// the single source of truth every call site that needs to ask "is this
// still OpenAI's own endpoint" must share (internal/llm/pricing.go's
// EstimateUSD — only apply OpenAI's own price table against OpenAI's own
// endpoint; internal/doctor/check_baseurl.go's BaseURLCheck — only warn
// about a suspected-vendor key when base_url is STILL the default;
// internal/cli/auth.go's promptOpenAICompatBaseURLIfDefault and
// promptOpenAICompatModelIfEmpty — decide whether to ask for the real
// endpoint / a model name during `comrade auth login`). All four used to
// run their own bare `!=`/`==` string compare against the default, which
// is not "same endpoint" — a perfectly legal, config.CheckBaseURL-
// accepted trailing slash (https://api.openai.com/v1/, exactly what
// promptOpenAICompatBaseURLIfDefault's own prompt persists verbatim on a
// bare Enter) or a differently-cased scheme/host would silently fail
// that compare even though newOpenAICompatConnector's own
// strings.TrimRight(baseURL, "/") treats it as the identical endpoint at
// request time.
//
// Comparison rule: raw is first trimmed of leading/trailing whitespace,
// then PARSED as a URL — never slash-trimmed before parsing (a
// pre-parse whole-string TrimRight only ever strips a slash that is
// literally the last character of the raw input, so
// ".../v1/?x=1" — a trailing slash followed by a query string — would
// silently NOT match ".../v1?x=1" even though both resolve to the exact
// same path once parsed; parsing first and trimming the PARSED path
// avoids this). If what remains parses as an absolute URL (scheme + host
// present), the scheme and host are compared case-insensitively (HTTP
// scheme/host are case-insensitive by definition) while the path is
// trimmed of trailing slashes and then compared EXACTLY as the connector
// sees it — no case-folding, since an OpenAI-compatible gateway's path
// segment is not guaranteed case-insensitive, and no other component
// (userinfo, query, fragment) ever participates in the comparison at
// all, which is exactly what keeps a userinfo-spoofed
// (https://api.openai.com@evil.example.com/v1 — net/url's own Host field
// never includes the userinfo prefix) or path-embedded-host spoof
// (https://evil.example.com/api.openai.com/v1) failing closed: neither
// ever produces a Host equal to api.openai.com. A raw value that does
// not parse as an absolute URL (no scheme, no host — including "") can
// never equal a real default and is compared as a trimmed,
// case-insensitive raw string purely so it never panics or
// false-matches.
//
// Knowingly NOT folded (fail-closed by design, not an oversight):
// an explicit default port (https://api.openai.com:443/v1) and a
// trailing-FQDN-root-dot host (https://api.openai.com./v1) are both
// the same real endpoint the connector would reach, but this function
// still reports them as non-default — both are rare spellings a real
// user is unlikely to type or a config to carry, and treating one as
// "the same host" would require its own DNS-shaped equivalence rule
// (port-if-scheme-default, trailing-dot-insensitive) that adds parsing
// surface for a case this codebase has never observed; a false "not
// default" here only ever costs an extra harmless prompt or a
// cost-unknown line, never a wrong price or a suppressed security
// warning, so under-matching is the safe direction to leave unfixed.
func IsDefaultOpenAICompatBaseURL(raw string) bool {
	return normalizeOpenAICompatBaseURL(raw) == normalizeOpenAICompatBaseURL(Default().LLM.OpenAICompat.BaseURL)
}

// normalizeOpenAICompatBaseURL is IsDefaultOpenAICompatBaseURL's own
// identity function — see that function's doc comment for the exact
// rule. Not exported: callers only ever need the boolean comparison, and
// keeping this unexported means the normalized FORM (arbitrary today) is
// never itself a public contract.
func normalizeOpenAICompatBaseURL(raw string) string {
	trimmed := strings.TrimSpace(raw)

	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.ToLower(strings.TrimRight(trimmed, "/"))
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.Path, "/")
}
