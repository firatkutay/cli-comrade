package redact

import "regexp"

// Redactor masks known secret shapes out of a string. Construct one
// with New; the five mandatory pattern families (api_key, jwt,
// private_key, credential kv, bearer) are always applied by Apply.
// maskEmails/maskIPs additionally enable the two optional families.
type Redactor struct {
	maskEmails bool
	maskIPs    bool
}

// New builds a Redactor. maskEmails and maskIPs enable the optional
// email/IP pattern families; the five mandatory families are always
// active regardless of these flags.
func New(maskEmails, maskIPs bool) *Redactor {
	return &Redactor{maskEmails: maskEmails, maskIPs: maskIPs}
}

// Compiled once at package init. See Apply's doc comment for the order
// these are applied in and why that order is load-bearing.
var (
	// privateKeyPattern matches a full PEM private-key block of any key
	// type ("RSA PRIVATE KEY", "EC PRIVATE KEY", "PRIVATE KEY", ...),
	// (?s) so "." also matches the embedded newlines.
	privateKeyPattern = regexp.MustCompile(`(?s)-----BEGIN (?:[A-Za-z0-9]+ )?PRIVATE KEY-----.*?-----END (?:[A-Za-z0-9]+ )?PRIVATE KEY-----`)

	// credentialKVPattern matches `key=value` / `key: value` /
	// `key = value` for a fixed set of credential-shaped key names,
	// case-insensitively. Group 1 is the key text and group 2 the
	// separator+spacing, both preserved as-typed by Apply's
	// replacement; group 3 (the value) is discarded. The value
	// alternation tries a double-quoted string, then a single-quoted
	// string, then a bare token — a quoted value (e.g.
	// `password="a b c"`) is consumed in full, including its embedded
	// spaces and closing quote, so nothing after the first space leaks
	// past the mask (a bare \S+ would stop at "a" and leave ` b c"` on
	// the wire). The bare-token alternative excludes trailing `,;)}`
	// rather than using a plain \S+: without that exclusion, a bare
	// value immediately followed by one of those (e.g. `token=abc123,`)
	// would swallow the punctuation into the match on the first Apply,
	// but a subsequent Apply over the *already redacted* text — where
	// "[REDACTED:credential]" now sits directly against a delimiter a
	// QUOTED value had left behind, e.g. `password="a b c",` →
	// `password=[REDACTED:credential],` — would then swallow just that
	// trailing comma on the second pass (since the marker text itself
	// has no comma to stop at), breaking idempotency. Excluding these
	// delimiters up front makes both passes agree. Note `]` is
	// deliberately NOT excluded: the "[REDACTED:credential]" marker
	// itself ends in `]`, and a bare-token exclusion of `]` would stop
	// the match one character short of the marker's own closing
	// bracket on a repeat pass, leaving a stray extra `]` behind — the
	// exact inverse idempotency bug. The trailing \b after the key
	// alternation stops "token" from matching the prefix of "tokens=5"
	// — see redact_test.go's false-positive suite.
	credentialKVPattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|api_key|apikey)\b(\s*(?::|=)\s*)("[^"]*"|'[^']*'|[^\s,;)}]+)`)

	// bearerHeaderPattern matches "Authorization: Bearer <token>" and
	// keeps the header text, masking only the token. bearerBarePattern
	// catches a standalone "Bearer <token>" not inside that header. Both
	// require an 8+ char token-shaped charset after "Bearer" so a short
	// following word (e.g. "the Bearer of good news") is never mistaken
	// for a token, and a lone trailing "Bearer" with nothing after it
	// never matches at all.
	bearerHeaderPattern = regexp.MustCompile(`(?i)(Authorization:\s*Bearer\s+)([A-Za-z0-9._~+/=-]{8,})`)
	bearerBarePattern   = regexp.MustCompile(`\bBearer\s+[A-Za-z0-9._~+/=-]{8,}`)

	// apiKeyPatterns are the raw key-format patterns. The leading \b
	// before each prefix is what keeps "sk-" from firing inside
	// "risk-free...": \b requires a transition between a word and a
	// non-word character, and "risk-free" has no such transition right
	// before its embedded "sk-" (it's preceded by the word character
	// "i").
	apiKeyPatterns = []*regexp.Regexp{
		regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}`),
		regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`\bgho_[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`\bAKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`),
	}

	// jwtPattern matches a three-segment base64url JWT. It runs after
	// bearer/credentialKV so a JWT already carried by "Bearer <jwt>" or
	// "token=<jwt>" is tagged [REDACTED:bearer]/[REDACTED:credential]
	// instead of being double-matched here.
	jwtPattern = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

	emailPattern = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`)

	ipv4Pattern = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])\.){3}(?:25[0-5]|2[0-4][0-9]|1?[0-9]?[0-9])\b`)
	// ipv6Pattern is deliberately "simple": it covers the common
	// full/compressed hextet forms plus the bare "::1" loopback, not
	// every valid RFC 4291 shorthand.
	ipv6Pattern = regexp.MustCompile(`\b(?:[0-9A-Fa-f]{1,4}:){2,7}[0-9A-Fa-f]{1,4}\b|::1\b`)
)

// localAddresses are never masked even when IP masking is enabled:
// redacting a well-known local address is noise, and would undermine
// guidance the LLM needs to give about a local service (e.g. Ollama
// running on 127.0.0.1).
var localAddresses = map[string]bool{
	"127.0.0.1": true,
	"0.0.0.0":   true,
	"::1":       true,
}

// Apply masks every mandatory pattern, then (if enabled at
// construction) the optional email/IP patterns, and returns the
// result. The order below is significant and must not be reordered
// without re-checking the interactions described:
//
//  1. private_key — full PEM blocks first, since they are multiline;
//     running this first keeps a PEM body's base64 content from ever
//     being examined by the single-line patterns below.
//  2. credential kv (password=/token=/...) — runs before the generic
//     api_key patterns so "api_key=sk-XXXX" is tagged
//     [REDACTED:credential] (which also keeps the key name visible)
//     rather than the less informative [REDACTED:api_key].
//  3. bearer — runs before the generic api_key patterns so
//     "Authorization: Bearer sk-XXXX" keeps its bearer framing instead
//     of the sk- pattern matching first and destroying the "Bearer "
//     prefix's context.
//  4. api_key — the raw key-format patterns.
//  5. jwt — standalone eyJ... tokens not already consumed by bearer or
//     credential-kv above.
//  6. email, ip — optional, only when enabled at construction.
//
// Apply is idempotent: none of the mandatory or optional patterns match
// their own "[REDACTED:...]" replacement text, so calling Apply twice on
// the same string returns the same string both times.
func (r *Redactor) Apply(s string) string {
	s = privateKeyPattern.ReplaceAllString(s, "[REDACTED:private_key]")
	s = credentialKVPattern.ReplaceAllString(s, "$1$2[REDACTED:credential]")
	s = bearerHeaderPattern.ReplaceAllString(s, "$1[REDACTED:bearer]")
	s = bearerBarePattern.ReplaceAllString(s, "Bearer [REDACTED:bearer]")

	for _, p := range apiKeyPatterns {
		s = p.ReplaceAllString(s, "[REDACTED:api_key]")
	}

	s = jwtPattern.ReplaceAllString(s, "[REDACTED:jwt]")

	if r.maskEmails {
		s = emailPattern.ReplaceAllString(s, "[REDACTED:email]")
	}
	if r.maskIPs {
		s = maskIPMatches(s)
	}

	return s
}

// maskIPMatches replaces IPv4/IPv6 literals with [REDACTED:ip], except
// localAddresses (see its doc comment).
func maskIPMatches(s string) string {
	replace := func(match string) string {
		if localAddresses[match] {
			return match
		}
		return "[REDACTED:ip]"
	}
	s = ipv4Pattern.ReplaceAllStringFunc(s, replace)
	s = ipv6Pattern.ReplaceAllStringFunc(s, replace)
	return s
}
