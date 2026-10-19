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

// TestApplyNewSecretShapes is the golden table for the additional
// high-confidence secret shapes added to close the SAST redaction-coverage
// finding: each case asserts the secret is gone, the expected marker is
// present, and applying Apply a second time is a no-op (idempotency).
func TestApplyNewSecretShapes(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		wantMasked string // substring that must appear in the output
		rawSecret  string // substring that must NOT appear in the output
	}{
		{
			name:       "Google/Gemini API key standalone",
			input:      "the gemini key is AIzaODjfcRNL2EDLbdDZ1c5jAU2rjTbrNLwMtsh for the bot",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "AIzaODjfcRNL2EDLbdDZ1c5jAU2rjTbrNLwMtsh",
		},
		{
			name:       "GitHub fine-grained PAT",
			input:      "leaked: github_pat_F6PwKluYIFdlKdMwj6uUvtaiJVfU7wicpHdEoziI in the log",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "github_pat_F6PwKluYIFdlKdMwj6uUvtaiJVfU7wicpHdEoziI",
		},
		{
			name:       "GitHub app token",
			input:      "installation token ghs_N68k4tUNpfZ46pdJQIPvjiQv expired",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "ghs_N68k4tUNpfZ46pdJQIPvjiQv",
		},
		{
			name:       "GitLab personal access token",
			input:      "gitlab token glpat-2zucR-LGOTU2Ixw7gBOirOl3 leaked",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "glpat-2zucR-LGOTU2Ixw7gBOirOl3",
		},
		{
			name:       "Stripe live secret key",
			input:      "stripe key sk_live_testvalidkey1234567890a in payload",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "sk_live_testvalidkey1234567890a",
		},
		{
			name:       "Stripe test secret key",
			input:      "stripe key sk_test_testvalidkey0987654321b in payload",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "sk_test_testvalidkey0987654321b",
		},
		{
			name:       "Google OAuth client secret",
			input:      "oauth secret GOCSPX-KK_IQQ8Vh2bZnzv45PfcIrCd leaked",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "GOCSPX-KK_IQQ8Vh2bZnzv45PfcIrCd",
		},
		{
			name:       "SendGrid API key",
			input:      "sendgrid key SG.test-fake-key-12345678.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx in env",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "SG.test-fake-key-12345678.xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		},
		{
			name:       "npm access token",
			input:      "npm token npm_JMSN9DlviDvUDDleg62hKD9gF2LEmEr3PZH8 in .npmrc",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "npm_JMSN9DlviDvUDDleg62hKD9gF2LEmEr3PZH8",
		},
		{
			name:       "GCP OAuth access token",
			input:      "gcp access token ya29.fFK1ohaoehyQm6oJB6MJbhQsIfvkU4 expired",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "ya29.fFK1ohaoehyQm6oJB6MJbhQsIfvkU4",
		},
		{
			name:       "Slack incoming webhook URL",
			input:      "webhook https://hooks.slack.com/services/TTEST000/BTEST0000/ZZZZZZZZZZZZZZZZZZtest in config",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "https://hooks.slack.com/services/TTEST000/BTEST0000/ZZZZZZZZZZZZZZZZZZtest",
		},
		{
			name:       "Slack workflow webhook URL",
			input:      "export W=https://hooks.slack.com/workflows/TTEST000GHZ/ATEST0000FF7AA/442660231TESTTEST/FTESTZZreCkhPmwBtaqbNtest",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "https://hooks.slack.com/workflows/TTEST000GHZ/ATEST0000FF7AA/442660231TESTTEST/FTESTZZreCkhPmwBtaqbNtest",
		},
		{
			name:       "Slack trigger webhook URL",
			input:      "SLACK_WEBHOOK_URL=https://hooks.slack.com/triggers/TTEST00ABC456/8675309TEST/abcd1234efghTESTtest",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "https://hooks.slack.com/triggers/TTEST00ABC456/8675309TEST/abcd1234efghTESTtest",
		},
		{
			name:       "Slack incoming webhook URL over http scheme",
			input:      "curl http://hooks.slack.com/services/T024TTTTT/BTEST72BBL/AZTESTu0pA4ad666eMgbitest",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "http://hooks.slack.com/services/T024TTTTT/BTEST72BBL/AZTESTu0pA4ad666eMgbitest",
		},
		{
			name:       "Slack incoming webhook URL with underscore and hyphen in token",
			input:      "url: https://hooks.slack.com/services/T024TTTTT/BTEST72BBL/AZTESTu0pA4_d666eMgbi-test",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "https://hooks.slack.com/services/T024TTTTT/BTEST72BBL/AZTESTu0pA4_d666eMgbi-test",
		},
		{
			name:       "Slack webhook URL with no scheme at all",
			input:      "webhook hooks.slack.com/services/TTEST000/BTEST0000/ZZTESTtoken pasted bare",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "hooks.slack.com/services/TTEST000/BTEST0000/ZZTESTtoken",
		},
		{
			name:       "Slack webhook URL with uppercase HTTPS scheme",
			input:      "HTTPS://hooks.slack.com/services/TTEST000/BTEST0000/ZZTESTtoken in script",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "HTTPS://hooks.slack.com/services/TTEST000/BTEST0000/ZZTESTtoken",
		},
		{
			name:       "Slack app-level token (xapp-)",
			input:      "socket mode token xapp-1-TESTfake9Zk1 in config",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "xapp-1-TESTfake9Zk1",
		},
		{
			name:       "Slack token-rotation refresh token (xoxe-)",
			input:      "refresh token xoxe-1-TESTfake7Qm2 in payload",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "xoxe-1-TESTfake7Qm2",
		},
		{
			name:       "Slack browser session token (xoxc-)",
			input:      "session token xoxc-1234-TESTfake5Rb leaked",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "xoxc-1234-TESTfake5Rb",
		},
		{
			name:       "Slack bot token with underscore in token body",
			input:      "bot token xoxb-fake_TEST_9Zk1 in payload",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "xoxb-fake_TEST_9Zk1",
		},
		{
			name:       "Slack compound rotation token (xoxe.xoxp-)",
			input:      "compound token xoxe.xoxp-1-TESTfake3Nc8 leaked",
			wantMasked: "[REDACTED:api_key]",
			rawSecret:  "xoxe.xoxp-1-TESTfake3Nc8",
		},
		{
			name:       "Azure storage AccountKey",
			input:      "DefaultEndpointsProtocol=https;AccountName=foo;AccountKey=pTyGJMuHbEL31IeL2HPcHyGcFRl1SPnXNYvMIHa/2o==;EndpointSuffix=core.windows.net",
			wantMasked: "AccountKey=[REDACTED:credential]",
			rawSecret:  "pTyGJMuHbEL31IeL2HPcHyGcFRl1SPnXNYvMIHa/2o==",
		},
		{
			name:       "Authorization Basic header",
			input:      "Authorization: Basic dXNlcm5hbWU6cGFzc3dvcmQxMjM0NTY3ODk=",
			wantMasked: "Authorization: Basic [REDACTED:basic]",
			rawSecret:  "dXNlcm5hbWU6cGFzc3dvcmQxMjM0NTY3ODk=",
		},
		{
			name:       "postgres connection string with embedded password",
			input:      `psql "postgres://admin:S3cr3tPassw0rd@db.internal/app"`,
			wantMasked: "postgres://admin:[REDACTED:credential]@db.internal/app",
			rawSecret:  "S3cr3tPassw0rd",
		},
		{
			name:       "mongodb connection string with embedded password",
			input:      "mongodb://dbuser:hunter2forever@cluster0.example.net:27017/mydb",
			wantMasked: "mongodb://dbuser:[REDACTED:credential]@cluster0.example.net:27017/mydb",
			rawSecret:  "hunter2forever",
		},
		{
			name:       "redis connection string with empty user (password-only DSN)",
			input:      "redis://:S3cr3t@localhost:6379/0",
			wantMasked: "redis://:[REDACTED:credential]@localhost:6379/0",
			rawSecret:  "S3cr3t",
		},
		{
			name:       "credential kv DB_PASSWORD compound key",
			input:      "DB_PASSWORD=hunter2forever in env",
			wantMasked: "DB_PASSWORD=[REDACTED:credential]",
			rawSecret:  "hunter2forever",
		},
		{
			name:       "credential kv access_token compound key",
			input:      "access_token=abc123opaquevalue in response",
			wantMasked: "access_token=[REDACTED:credential]",
			rawSecret:  "abc123opaquevalue",
		},
		{
			name:       "credential kv client_secret compound key",
			input:      "client_secret=zzsupersecretvalue in config",
			wantMasked: "client_secret=[REDACTED:credential]",
			rawSecret:  "zzsupersecretvalue",
		},
		{
			name:       "credential kv AWS_SECRET_ACCESS_KEY compound key",
			input:      "AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY in env",
			wantMasked: "AWS_SECRET_ACCESS_KEY=[REDACTED:credential]",
			rawSecret:  "wJalrXUtnFEMIK7MDENGbPxRfiCYEXAMPLEKEY",
		},
		{
			name:       "credential kv GITHUB_TOKEN compound key",
			input:      "GITHUB_TOKEN=ghp_opaqueGithubActionsToken1234 in workflow env",
			wantMasked: "GITHUB_TOKEN=[REDACTED:credential]",
			rawSecret:  "ghp_opaqueGithubActionsToken1234",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := New(false, false)
			once := r.Apply(tc.input)
			assert.Contains(t, once, tc.wantMasked)
			assert.NotContains(t, once, tc.rawSecret)

			twice := r.Apply(once)
			assert.Equal(t, once, twice, "applying Apply twice must be a no-op the second time")
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

// TestApplySlackWebhookWithUnderscoreMasksWholeToken pins the exact
// regression this token family was originally fixed for: under the old
// regex, an underscore inside the token's final path segment truncated the
// match and left the token's tail on the wire, while the table test above
// only asserted Contains/NotContains — both of which also hold for that
// truncated old output, so that test alone did not guard the regression.
// This asserts full byte-for-byte equality so the whole URL, not just a
// prefix of it, is gone.
func TestApplySlackWebhookWithUnderscoreMasksWholeToken(t *testing.T) {
	r := New(false, false)
	got := r.Apply("url: https://hooks.slack.com/services/T024TTTTT/BTEST72BBL/AZTESTu0pA4_d666eMgbi-test")
	assert.Equal(t, "url: [REDACTED:api_key]", got)
}

// TestApplySlackTokenSentenceFinalPeriodNotSwallowed pins the boundary
// safety the widened xox*/xapp- token charset (now including ".") must
// preserve: a "."-joined continuation segment is only consumed when it is
// followed by more token-shaped text, never when it's followed by
// whitespace/EOL, so a sentence-final period stays outside the match.
func TestApplySlackTokenSentenceFinalPeriodNotSwallowed(t *testing.T) {
	r := New(false, false)
	got := r.Apply("bot token xoxb-ABCDEFGHIJKL. Next sentence.")
	assert.Equal(t, "bot token [REDACTED:api_key]. Next sentence.", got)
}

// TestApplyWebhookURLsMaskedExactly covers the bearer-equivalent webhook
// URL families beyond Slack (#58). It asserts full byte-for-byte output,
// not Contains/NotContains, so a match that stops partway through the
// token (leaving its tail on the wire) fails the test.
// fakeGoogleAPIKey is a TEST-marked, AIza-shaped fixture (AIza + 35
// chars, the shape apiKeyPatterns masks). It is assembled by concatenation
// so the literal key shape never appears in the source, which keeps
// GitHub secret scanning from raising a "Google API Key" alert on it.
const fakeGoogleAPIKey = "AIza" + "SyTESTfakeKey" + "0000000000000000000000"

func TestApplyWebhookURLsMaskedExactly(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Discord webhook URL",
			input: "curl -X POST https://discord.com/api/webhooks/1234567890TEST/TESTfakeToken_abc-DEF -d @msg.json",
			want:  "curl -X POST [REDACTED:api_key] -d @msg.json",
		},
		{
			name:  "Discord webhook URL on legacy discordapp.com with versioned API path",
			input: "DISCORD_URL=https://discordapp.com/api/v10/webhooks/1234567890TEST/TESTfakeToken_abc-DEF",
			want:  "DISCORD_URL=[REDACTED:api_key]",
		},
		{
			name:  "Discord webhook URL on canary host, no scheme, sentence-final period",
			input: "use canary.discord.com/api/webhooks/1234567890TEST/TESTfakeToken. Then retry.",
			want:  "use [REDACTED:api_key]. Then retry.",
		},
		{
			name:  "Discord webhook URL with uppercase scheme",
			input: "HTTPS://discord.com/api/webhooks/1234567890TEST/TESTfakeToken in script",
			want:  "[REDACTED:api_key] in script",
		},
		{
			name:  "Microsoft Teams incoming webhook URL",
			input: "url: https://contosoTEST.webhook.office.com/webhookb2/00000000-TEST-0000-0000-000000000000@11111111-TEST-1111-1111-111111111111/IncomingWebhook/TESTfake0123456789abcdef/22222222-TEST-2222-2222-222222222222/V2TESTfake_token-abc",
			want:  "url: [REDACTED:api_key]",
		},
		{
			name:  "Microsoft Teams legacy outlook.office.com webhook URL",
			input: "posting to https://outlook.office.com/webhook/00000000-TEST-0000@11111111-TEST/IncomingWebhook/TESTfake/22222222-TEST failed",
			want:  "posting to [REDACTED:api_key] failed",
		},
		{
			name:  "Microsoft Teams Workflows URL masks the sig= signature",
			input: "https://prod-00.westusTEST.logic.azure.com:443/workflows/TESTfake0123/triggers/manual/paths/invoke?api-version=2016-06-01&sp=%2Ftriggers%2Fmanual%2Frun&sv=1.0&sig=TESTfakeSignature_abc-123",
			want:  "https://prod-00.westusTEST.logic.azure.com:443/workflows/TESTfake0123/triggers/manual/paths/invoke?api-version=2016-06-01&sp=%2Ftriggers%2Fmanual%2Frun&sv=1.0&sig=[REDACTED:credential]",
		},
		{
			name:  "Mattermost incoming webhook URL",
			input: "curl -i -X POST http://localhost:8065/hooks/test0fake0hook0id0abcdefgh -d payload",
			want:  "curl -i -X POST [REDACTED:api_key] -d payload",
		},
		{
			name:  "Mattermost webhook path in an access log line without a host",
			input: `"POST /hooks/test0fake0hook0id0abcdefgh HTTP/1.1" 200`,
			want:  `"POST [REDACTED:api_key] HTTP/1.1" 200`,
		},
		{
			name:  "PagerDuty v1 integration URL",
			input: "endpoint https://events.pagerduty.com/integration/TESTfake0123456789abcdef0000test/enqueue returned 400",
			want:  "endpoint [REDACTED:api_key] returned 400",
		},
		{
			name:  "PagerDuty EU custom-event URL",
			input: "events.eu.pagerduty.com/x-ere/TESTfake0123456789abcdef0000test",
			want:  "[REDACTED:api_key]",
		},
		{
			name:  "Discord webhook URL with explicit port",
			input: "https://discord.com:443/api/webhooks/1234567890TEST/TESTfakeToken_abc-DEF sent",
			want:  "[REDACTED:api_key] sent",
		},
		{
			name:  "Microsoft Teams webhook URL with explicit port",
			input: "https://contosoTEST.webhook.office.com:443/webhookb2/1111-TEST@2222-TEST/IncomingWebhook/TESTfake/3333-TEST sent",
			want:  "[REDACTED:api_key] sent",
		},
		{
			name:  "Microsoft Teams webhook URL with percent-encoded @",
			input: "https://contosoTEST.webhook.office.com/webhookb2/1111-TEST%402222-TEST/IncomingWebhook/TESTfake/3333-TEST sent",
			want:  "[REDACTED:api_key] sent",
		},
		{
			name:  "Microsoft Teams webhook URL on a multi-label host",
			input: "https://a.contosoTEST.webhook.office.com/webhookb2/1111-TEST@2222-TEST/IncomingWebhook/TESTfake sent",
			want:  "[REDACTED:api_key] sent",
		},
		{
			name:  "Microsoft Teams legacy outlook.office.com webhook URL with explicit port",
			input: "https://outlook.office.com:443/webhook/1111-TEST@2222-TEST/IncomingWebhook/TESTfake/3333-TEST sent",
			want:  "[REDACTED:api_key] sent",
		},
		{
			name:  "PagerDuty integration URL with explicit port",
			input: "https://events.pagerduty.com:443/integration/TESTfake0123456789abcdef0000test/enqueue sent",
			want:  "[REDACTED:api_key] sent",
		},
		{
			name:  "Slack webhook URL with explicit port",
			input: "https://hooks.slack.com:443/services/TTEST000/BTEST0000/ZZTESTtoken sent",
			want:  "[REDACTED:api_key] sent",
		},
		{
			name:  "PagerDuty routing_key in Alertmanager config",
			input: "    routing_key: TESTfake0123456789abcdef0000test",
			want:  "    routing_key: [REDACTED:credential]",
		},
		{
			name:  "PagerDuty service_key in Alertmanager config",
			input: "    service_key: TESTfake0123456789abcdef0000test",
			want:  "    service_key: [REDACTED:credential]",
		},
		{
			name:  "Google Chat webhook URL masks both key and token query params",
			input: "https://chat.googleapis.com/v1/spaces/AAAATESTfake/messages?key=" + fakeGoogleAPIKey + "&token=TESTfakeChatToken_abc%3D",
			want:  "https://chat.googleapis.com/v1/spaces/AAAATESTfake/messages?key=[REDACTED:api_key]&token=[REDACTED:credential]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := New(false, false)
			once := r.Apply(tc.input)
			assert.Equal(t, tc.want, once)
			assert.Equal(t, once, r.Apply(once), "applying Apply twice must be a no-op the second time")
		})
	}
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
		{"compound key prose without separator does not trigger credential pattern", "mypassword_field_label is a form field name"},
		{"Stripe publishable key pk_live_ is left intact (not a secret)", "publishable key pk_live_51H8xYzABCDEFGHIJKLMNOPQR is safe to embed client-side"},
		{"credential-less HTTPS URL does not trigger connStringPattern", "see https://example.com/path for details"},
		{"HTTP URL with port but no @ does not trigger connStringPattern", "server is at http://host:8080/db right now"},
		{"Slack API docs URL does not trigger the webhook pattern", "see https://api.slack.com/messaging/webhooks for details"},
		{"Discord docs URL does not trigger the webhook pattern", "see https://discord.com/developers/docs/resources/webhook for details"},
		{"Outlook web mail URL does not trigger the Teams webhook pattern", "open https://outlook.office.com/mail/inbox to check"},
		{"GitHub repo hook API path does not trigger the Mattermost pattern", "GET https://api.github.com/repos/octo/demo/hooks/12345 returned 404"},
		{"PagerDuty v2 enqueue URL carries no key in its path", "POST https://events.pagerduty.com/v2/enqueue with a JSON body"},
		{"signature= and design= do not trigger the sig= credential key", "signature=abc123 design=flat"},
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
