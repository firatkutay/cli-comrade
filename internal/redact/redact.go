package redact

import "regexp"

// Redactor masks known secret shapes out of a string. Construct one
// with New; the mandatory pattern families (api_key, jwt, private_key,
// credential kv, connection-string/Azure-AccountKey password,
// bearer/basic auth header) are always applied by Apply.
// maskEmails/maskIPs additionally enable the two optional families.
type Redactor struct {
	maskEmails bool
	maskIPs    bool
}

// New builds a Redactor. maskEmails and maskIPs enable the optional
// email/IP pattern families; the mandatory families are always active
// regardless of these flags.
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
	// case-insensitively — including compound/prefixed names like
	// `DB_PASSWORD=`, `access_token=`, `client_secret=`,
	// `AWS_SECRET_ACCESS_KEY=`, `GITHUB_TOKEN=`. Group 1 is the full key
	// text (prefix and/or suffix included, when present) and group 2 the
	// separator+spacing, both preserved as-typed by Apply's replacement;
	// group 3 (the value) is discarded. The optional
	// `(?:[A-Za-z0-9]+[_.\-])?` prefix and `(?:[_.\-][A-Za-z0-9]+)?`
	// suffix around the core credential word are what let
	// "AWS_SECRET_ACCESS_KEY" match via the "access_key" alternative
	// (prefix "SECRET_" + key "ACCESS_KEY") while still requiring a
	// `:`/`=` separator to follow immediately (with only whitespace in
	// between) — that separator requirement is what keeps a plain word
	// with no separator (e.g. "tokens=5", "mypassword_field_label" with
	// no trailing `=`/`:` at all) from matching; a bare `\b` after the
	// key word is no longer needed for that guard once the separator
	// itself is mandatory (see redact_test.go's false-positive suite).
	// The value alternation tries a double-quoted string, then a
	// single-quoted string, then a bare token — a quoted value (e.g.
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
	// exact inverse idempotency bug.
	credentialKVPattern = regexp.MustCompile(`(?i)((?:[A-Za-z0-9]+[_.\-])?(?:password|passwd|pwd|token|secret|api[_-]?key|access[_-]?key|client[_-]?secret)(?:[_.\-][A-Za-z0-9]+)?)(\s*(?::|=)\s*)("[^"]*"|'[^']*'|[^\s,;)}]+)`)

	// connStringPattern matches a `scheme://user:password@` connection
	// string (postgres://, mysql://, mongodb://, redis://, amqp://, ...)
	// and masks only the password (group 2), preserving the scheme,
	// user, and host/port (group 1 = "scheme://user:", group 3 = "@")
	// so the redacted line stays useful to the model. The user portion
	// (`[^:\s/@]*`) is zero-or-more, not one-or-more: password-only DSNs
	// with an empty user — `redis://:password@host` is the common
	// shape (Redis and others often carry no username at all) — must
	// still match. The mandatory trailing `@` (group 3) is what keeps
	// this from firing on a credential-less URL: `https://example.com/path`
	// and `http://host:8080/path` have no "user:pass@" segment at all, so
	// there is nothing for `[^@\s/]+` (the password, still one-or-more)
	// to anchor against before an `@` that doesn't exist.
	connStringPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.\-]*://[^:\s/@]*:)([^@\s/]+)(@)`)

	// azureAccountKeyPattern matches an Azure Storage connection
	// string's `AccountKey=<base64>` component and masks only the
	// value, keeping the "AccountKey=" text for context.
	azureAccountKeyPattern = regexp.MustCompile(`(\bAccountKey=)([0-9A-Za-z+/=]{40,})`)

	// bearerHeaderPattern matches "Authorization: Bearer <token>" and
	// keeps the header text, masking only the token. bearerBarePattern
	// catches a standalone "Bearer <token>" not inside that header. Both
	// require an 8+ char token-shaped charset after "Bearer" so a short
	// following word (e.g. "the Bearer of good news") is never mistaken
	// for a token, and a lone trailing "Bearer" with nothing after it
	// never matches at all.
	bearerHeaderPattern = regexp.MustCompile(`(?i)(Authorization:\s*Bearer\s+)([A-Za-z0-9._~+/=-]{8,})`)
	bearerBarePattern   = regexp.MustCompile(`\bBearer\s+[A-Za-z0-9._~+/=-]{8,}`)

	// basicHeaderPattern mirrors bearerHeaderPattern for
	// "Authorization: Basic <base64>", masking only the credential and
	// keeping the header text.
	basicHeaderPattern = regexp.MustCompile(`(?i)(Authorization:\s*Basic\s+)([A-Za-z0-9+/=]{8,})`)

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
		// Google API key (also covers Gemini keys — "google" is a
		// first-class provider).
		regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`),
		// GitHub fine-grained PAT and app token.
		regexp.MustCompile(`\bgithub_pat_[0-9A-Za-z_]{22,}`),
		regexp.MustCompile(`\bghs_[0-9A-Za-z]{20,}`),
		// GitLab personal access token.
		regexp.MustCompile(`\bglpat-[0-9A-Za-z_\-]{20,}`),
		// Stripe secret key (sk_live_/sk_test_ — distinct from the
		// hyphenated "sk-" pattern above; pk_live_/pk_test_
		// publishable keys are deliberately not matched here).
		regexp.MustCompile(`\bsk_(?:live|test)_[0-9A-Za-z]{20,}`),
		// Google OAuth client secret.
		regexp.MustCompile(`\bGOCSPX-[0-9A-Za-z_\-]{20,}`),
		// SendGrid API key.
		regexp.MustCompile(`\bSG\.[0-9A-Za-z_\-]{22}\.[0-9A-Za-z_\-]{43}`),
		// npm access token.
		regexp.MustCompile(`\bnpm_[0-9A-Za-z]{36}`),
		// GCP OAuth access token.
		regexp.MustCompile(`\bya29\.[0-9A-Za-z_\-]+`),
		// Slack incoming webhook URL.
		regexp.MustCompile(`https://hooks\.slack\.com/services/[A-Za-z0-9/]+`),
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
//  2. connection string (scheme://user:PASSWORD@host) — runs early,
//     alongside private_key/credential-kv, since it is itself a
//     credential-bearing key=value-shaped construct.
//  3. Azure AccountKey= — same rationale as the connection string.
//  4. credential kv (password=/token=/DB_PASSWORD=/access_token=/...)
//     — runs before the generic api_key patterns so "api_key=sk-XXXX"
//     is tagged [REDACTED:credential] (which also keeps the key name
//     visible) rather than the less informative [REDACTED:api_key].
//  5. bearer, basic — run before the generic api_key patterns so
//     "Authorization: Bearer sk-XXXX" / "Authorization: Basic ..."
//     keep their header framing instead of a raw-format pattern
//     matching first and destroying that context.
//  6. api_key — the raw key-format patterns.
//  7. jwt — standalone eyJ... tokens not already consumed by bearer or
//     credential-kv above.
//  8. email, ip — optional, only when enabled at construction.
//
// Apply is idempotent: calling Apply twice on the same string returns the
// same string both times. Most patterns achieve this by never matching
// their own "[REDACTED:...]" replacement text. The credential-kv and
// connection-string patterns are a partial exception in mechanism, not
// outcome: their value/password character classes (a bare token /
// "[^@\s/]+") are permissive enough to also match an already-emitted
// "[REDACTED:credential]" marker on a second pass — but since that value
// is always discarded and replaced with the same literal marker, the
// output is unchanged either way. (azureAccountKeyPattern's value class
// is a strict base64 alphabet with a 40-char minimum, which the 21-char
// marker never satisfies, so it achieves idempotency by non-match like
// the rest.)
func (r *Redactor) Apply(s string) string {
	s = privateKeyPattern.ReplaceAllString(s, "[REDACTED:private_key]")
	s = connStringPattern.ReplaceAllString(s, "${1}[REDACTED:credential]${3}")
	s = azureAccountKeyPattern.ReplaceAllString(s, "${1}[REDACTED:credential]")
	s = credentialKVPattern.ReplaceAllString(s, "$1$2[REDACTED:credential]")
	s = bearerHeaderPattern.ReplaceAllString(s, "$1[REDACTED:bearer]")
	s = bearerBarePattern.ReplaceAllString(s, "Bearer [REDACTED:bearer]")
	s = basicHeaderPattern.ReplaceAllString(s, "$1[REDACTED:basic]")

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
