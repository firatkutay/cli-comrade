package redact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// awsExampleKey is AWS's own well-known placeholder access key ID, used
// throughout AWS's public documentation — safe to use verbatim in a
// golden test.
const awsExampleKey = "AKIAIOSFODNN7EXAMPLE"

// jwtIOExampleJWT is the canonical three-segment example JWT from
// jwt.io's own docs.
const jwtIOExampleJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"

const rsaPrivateKeyBlock = `-----BEGIN RSA PRIVATE KEY-----
MIIBOgIBAAJBAK8wjX3lIcBK9O9OiPqYQOh2KMFLo4dPBQmMFljVWZC3ZQpRA5+g
h6b8p3aRb8lVX6MjhwQBBoBv7pQe1AAWU/kCAwEAAQJAX1B2q1B2VtF+e2M1yZ2E
-----END RSA PRIVATE KEY-----`

func TestApplyMandatoryPatterns(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantMasked string // substring that must appear in the output
		rawSecret  string // substring that must NOT appear in the output
	}{
		{
			name:       "api_key sk- prefix",
			input:      "here is my key sk-ABCDEFGHIJ1234567890KL and more",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "sk-ABCDEFGHIJ1234567890KL",
		},
		{
			name:       "api_key ghp_ prefix",
			input:      "token: ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ12",
			wantMasked: "[REDACTED:credential]", // caught by credential kv (token:) before the generic ghp_ pattern
			rawSecret:  "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ12",
		},
		{
			name:       "api_key ghp_ prefix standalone",
			input:      "the key is ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ12 for the bot",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ12",
		},
		{
			name:       "api_key gho_ prefix standalone",
			input:      "oauth token gho_ABCDEFGHIJKLMNOPQRSTUVWXYZ12 leaked",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "gho_ABCDEFGHIJKLMNOPQRSTUVWXYZ12",
		},
		{
			name:       "api_key AKIA prefix",
			input:      "aws access key " + awsExampleKey + " in the log",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  awsExampleKey,
		},
		{
			name:       "api_key xoxb slack prefix",
			input:      "slack token xoxb-1234567890-abcdefGHIJKL leaked",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "xoxb-1234567890-abcdefGHIJKL",
		},
		{
			name:       "jwt standalone",
			input:      "auth payload was " + jwtIOExampleJWT + " in the request",
			wantMasked: "[REDACTED:jwt]",
			rawSecret:  jwtIOExampleJWT,
		},
		{
			name:       "private_key PEM block",
			input:      "leaked key:\n" + rsaPrivateKeyBlock + "\nend of log",
			wantMasked: "[REDACTED:private_key]",
			rawSecret:  "MIIBOgIBAAJBAK8wjX3lIcBK9O9OiPqYQOh2KMFLo4dPBQmMFljVWZC3ZQpRA5",
		},
		{
			name:       "credential kv password=",
			input:      "connecting with password=hunter2forever",
			wantMasked: "password=[REDACTED:credential]",
			rawSecret:  "hunter2forever",
		},
		{
			name:       "credential kv passwd colon",
			input:      "passwd: hunter2forever",
			wantMasked: "passwd: [REDACTED:credential]",
			rawSecret:  "hunter2forever",
		},
		{
			name:       "credential kv pwd with spaces around equals",
			input:      "pwd = hunter2forever",
			wantMasked: "pwd = [REDACTED:credential]",
			rawSecret:  "hunter2forever",
		},
		{
			name:       "credential kv secret=",
			input:      "secret=topsecretvalue123",
			wantMasked: "secret=[REDACTED:credential]",
			rawSecret:  "topsecretvalue123",
		},
		{
			name:       "credential kv api_key= case insensitive",
			input:      "API_KEY=sk-ABCDEFGHIJ1234567890KL",
			wantMasked: "API_KEY=[REDACTED:credential]",
			rawSecret:  "sk-ABCDEFGHIJ1234567890KL",
		},
		{
			name:       "credential kv apikey=",
			input:      "apikey=zzzzzzzzzz9999999999",
			wantMasked: "apikey=[REDACTED:credential]",
			rawSecret:  "zzzzzzzzzz9999999999",
		},
		{
			name:       "bearer header form",
			input:      "Authorization: Bearer abcdefGHIJKL1234567890",
			wantMasked: "Authorization: Bearer [REDACTED:bearer]",
			rawSecret:  "abcdefGHIJKL1234567890",
		},
		{
			name:       "bearer bare form",
			input:      "curl -H \"Bearer abcdefGHIJKL1234567890\" https://api.example.com",
			wantMasked: "Bearer [REDACTED:bearer]",
			rawSecret:  "abcdefGHIJKL1234567890",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := New(false, false)
			got := r.Apply(tc.input)
			assert.Contains(t, got, tc.wantMasked)
			assert.NotContains(t, got, tc.rawSecret)
		})
	}
}

func TestApplyCredentialKVKeepsKeyNameVisible(t *testing.T) {
	r := New(false, false)
	got := r.Apply("password=hunter2forever")
	assert.Equal(t, "password=[REDACTED:credential]", got)
}

func TestApplyBearerHeaderKeepsAuthorizationPrefix(t *testing.T) {
	r := New(false, false)
	got := r.Apply("Authorization: Bearer abcdefGHIJKL1234567890")
	assert.Equal(t, "Authorization: Bearer [REDACTED:bearer]", got)
}

func TestApplyCredentialKVDoubleQuotedValueWithSpaces(t *testing.T) {
	r := New(false, false)
	got := r.Apply(`password="a b c"`)
	assert.Equal(t, "password=[REDACTED:credential]", got)
	assert.NotContains(t, got, "a b c")
	assert.NotContains(t, got, `"`)
}

func TestApplyCredentialKVSingleQuotedValueWithSpaces(t *testing.T) {
	r := New(false, false)
	got := r.Apply(`token='x y'`)
	assert.Equal(t, "token=[REDACTED:credential]", got)
	assert.NotContains(t, got, "x y")
	assert.NotContains(t, got, "'")
}

// TestApplyCredentialKVQuotedValueFollowedByCommaStaysIdempotent pins the
// exact regression the coordinator's review caught: a quoted value
// directly followed by a delimiter (no space) leaves that delimiter
// outside the first match; a second Apply pass must not then treat the
// "[REDACTED:credential]" marker + delimiter as one bare token and
// swallow the delimiter.
func TestApplyCredentialKVQuotedValueFollowedByCommaStaysIdempotent(t *testing.T) {
	r := New(false, false)
	input := `password="a b c", token='x y', done`
	once := r.Apply(input)
	twice := r.Apply(once)
	assert.Equal(t, "password=[REDACTED:credential], token=[REDACTED:credential], done", once)
	assert.Equal(t, once, twice)
}

func TestApplyCredentialKVQuotedValueTrailingContextPreserved(t *testing.T) {
	r := New(false, false)
	got := r.Apply(`config: password="a b c" and continue after`)
	assert.Equal(t, `config: password=[REDACTED:credential] and continue after`, got)
}

func TestApplyCredentialKVSingleQuotedValueWithColonSeparator(t *testing.T) {
	r := New(false, false)
	got := r.Apply(`secret: 'top secret value'`)
	assert.Equal(t, "secret: [REDACTED:credential]", got)
	assert.NotContains(t, got, "top secret value")
}

func TestApplyFalsePositives(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"risk-free prose does not trigger sk- pattern", "risk-free skydiving is a great hobby"},
		{"tokens= plural key does not trigger token= credential pattern", "tokens=5 is fine for this test"},
		{"passwords prose without separator does not trigger credential pattern", "passwords are important for security"},
		{"sk- shorter than 20 chars does not trigger api_key pattern", "the code is sk-short123 today"},
		{"lone Bearer word does not trigger bearer pattern", "Just say Bearer. That's it."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := New(false, false)
			got := r.Apply(tc.input)
			assert.Equal(t, tc.input, got, "false-positive case must be left untouched")
		})
	}
}

func TestApplyEmailOptedOut(t *testing.T) {
	r := New(false, false)
	got := r.Apply("contact me at alice@example.com please")
	assert.Contains(t, got, "alice@example.com", "email must be left intact when redact_emails=false")
}

func TestApplyEmailOptedIn(t *testing.T) {
	r := New(true, false)
	got := r.Apply("contact me at alice@example.com please")
	assert.NotContains(t, got, "alice@example.com")
	assert.Contains(t, got, "[REDACTED:email]")
}

func TestApplyIPOptedOut(t *testing.T) {
	r := New(false, false)
	got := r.Apply("server is at 203.0.113.42 right now")
	assert.Contains(t, got, "203.0.113.42", "IP must be left intact when redact_ips=false")
}

func TestApplyIPv4OptedIn(t *testing.T) {
	r := New(false, true)
	got := r.Apply("server is at 203.0.113.42 right now")
	assert.NotContains(t, got, "203.0.113.42")
	assert.Contains(t, got, "[REDACTED:ip]")
}

func TestApplyIPv6OptedIn(t *testing.T) {
	r := New(false, true)
	got := r.Apply("server is at 2001:db8:85a3:0:0:8a2e:370:7334 right now")
	assert.NotContains(t, got, "2001:db8:85a3:0:0:8a2e:370:7334")
	assert.Contains(t, got, "[REDACTED:ip]")
}

func TestApplyLocalAddressesNeverMasked(t *testing.T) {
	r := New(false, true)
	for _, addr := range []string{"127.0.0.1", "0.0.0.0", "::1"} {
		got := r.Apply("ollama is running on " + addr + ":11434")
		assert.Contains(t, got, addr, "local address %s must never be masked", addr)
	}
}

func TestApplyIdempotent(t *testing.T) {
	input := "key sk-ABCDEFGHIJ1234567890KL, password=hunter2forever, " +
		`password="a b c", token='x y', ` +
		"Authorization: Bearer abcdefGHIJKL1234567890, " + jwtIOExampleJWT + ", " +
		"contact alice@example.com from 203.0.113.42\n" + rsaPrivateKeyBlock

	r := New(true, true)
	once := r.Apply(input)
	twice := r.Apply(once)
	assert.Equal(t, once, twice, "applying Apply twice must be a no-op the second time")
}

func TestApplyPEMMultilineRoundTrip(t *testing.T) {
	input := "before\n" + rsaPrivateKeyBlock + "\nafter"
	r := New(false, false)
	got := r.Apply(input)

	assert.Contains(t, got, "before")
	assert.Contains(t, got, "after")
	assert.Contains(t, got, "[REDACTED:private_key]")
	assert.NotContains(t, got, "-----BEGIN RSA PRIVATE KEY-----")
	assert.NotContains(t, got, "-----END RSA PRIVATE KEY-----")
	assert.NotContains(t, got, "MIIBOgIBAAJBAK8wjX3lIcBK9O9OiPqYQOh2KMFLo4dPBQmMFljVWZC3ZQpRA5")
}

func TestApplyUTF8Safe(t *testing.T) {
	r := New(true, false)
	input := "şifre bilgisi güvenli değil: password=hunter2forever ve e-posta alice@example.com"
	got := r.Apply(input)

	assert.True(t, strings.Contains(got, "şifre bilgisi güvenli değil"), "non-ASCII prose must survive untouched")
	assert.Contains(t, got, "password=[REDACTED:credential]")
	assert.Contains(t, got, "[REDACTED:email]")
	assert.NotContains(t, got, "hunter2forever")
	assert.NotContains(t, got, "alice@example.com")
}
