package cli

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// captureOpenAICompatBaseURLWarning swaps auth.go's
// emitOpenAICompatBaseURLWarning seam for the duration of fn, returning
// every string it was called with (in call order). Reassigning the real
// os.Stderr variable would NOT work here: config.EmitBaseURLWarning
// writes through config's own package-level baseURLWarningWriter, which
// already captured the original *os.File at THAT package's var-init
// time, before this test ever ran — only the injectable seam (or a
// non-portable OS-level fd dup2) can intercept it.
func captureOpenAICompatBaseURLWarning(t *testing.T, fn func()) []string {
	t.Helper()
	var got []string
	original := emitOpenAICompatBaseURLWarning
	emitOpenAICompatBaseURLWarning = func(warning string) { got = append(got, warning) }
	t.Cleanup(func() { emitOpenAICompatBaseURLWarning = original })

	fn()

	return got
}

// withMockKeychain switches go-keyring's package-level provider to its
// in-memory mock for the duration of t, so newSecretsStore's underlying
// detectKeychainAvailable probe reports "available" and every test using
// it exercises the keychain backend deterministically, regardless of
// whether this sandbox's own headless environment happens to have a
// reachable Secret Service. t.Cleanup restores an unavailable-keychain
// state afterward, so a later test in this same package's test binary
// that forgets to call either helper still gets deterministic
// (file-fallback) behavior instead of silently inheriting this test's
// mock state — see internal/secrets/store_test.go's identical pair for
// the full rationale.
func withMockKeychain(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	t.Cleanup(func() { keyring.MockInitWithError(keyring.ErrUnsupportedPlatform) })
}

// withUnavailableKeychain forces every keychain operation to fail, so
// newSecretsStore's Store falls back to the file backend deterministically.
func withUnavailableKeychain(t *testing.T) {
	t.Helper()
	keyring.MockInitWithError(keyring.ErrUnsupportedPlatform)
	t.Cleanup(func() { keyring.MockInitWithError(keyring.ErrUnsupportedPlatform) })
}

// fakePasswordReader is the passwordReader test double `comrade auth
// login`'s tests inject in place of golang.org/x/term.ReadPassword, which
// requires a real terminal file descriptor this test binary does not
// have.
func fakePasswordReader(value string) passwordReader {
	return func(int) ([]byte, error) { return []byte(value), nil }
}

// fakeTTY is the isTerminalFunc test double every `comrade auth login`
// test that needs to get PAST the QA MINOR-5 TTY guard injects — go
// test's own stdin is essentially never a real TTY (locally or in CI),
// so relying on the real term.IsTerminal here would make every one of
// those tests fail regardless of what they're actually testing.
// fakeTTY(false) is MINOR-5's own dedicated test's way of simulating the
// non-interactive case without needing a real piped/redirected stdin.
func fakeTTY(present bool) isTerminalFunc {
	return func(int) bool { return present }
}

// newTestLoaderFactory returns a loaderFactory resolving against the
// process environment as it stands right now — the same thing
// NewRootCmd's own newLoader does — for tests that construct a leaf
// command (e.g. newAuthLoginCmd) directly instead of going through the
// full root command tree.
func newTestLoaderFactory() loaderFactory {
	return func() (*config.Loader, error) { return config.NewLoader("") }
}

// findTableRow returns the first line of output whose trimmed text
// starts with prefix — tabwriter.Flush renders columns as
// space-padded text, not literal tabs, so asserting on a whole row's
// content (rather than a literal "\t"-joined string) is what actually
// survives that alignment.
func findTableRow(output, prefix string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return line
		}
	}
	return ""
}

func TestAuthLoginRejectsOllama(t *testing.T) {
	_, _, err := execRootSplit(t, "dev", "auth", "login", "ollama")

	assert.ErrorContains(t, err, "ollama needs no API key")
}

func TestAuthLoginRejectsUnknownProvider(t *testing.T) {
	_, _, err := execRootSplit(t, "dev", "auth", "login", "bogus-provider")

	assert.ErrorContains(t, err, `unknown provider "bogus-provider"`)
}

func TestAuthLoginStoresKeyAndReportsPingSuccess(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-5.4-mini","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-test-key-123"), fakeTTY(true))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute())

	assert.Equal(t, "Bearer sk-test-key-123", gotAuth)
	assert.Contains(t, stdout.String(), "Stored key for openai_compat")
	assert.Contains(t, stdout.String(), "Test request succeeded")
	assert.NotContains(t, stdout.String(), "sk-test-key-123", "the entered key must never be echoed back")

	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	key, source, err := store.Get(context.Background(), "openai_compat")
	require.NoError(t, err)
	assert.Equal(t, "sk-test-key-123", key)
	assert.Equal(t, secrets.SourceKeychain, source)
}

// TestAuthLoginOpenAICompatPromptsForBaseURLWhenStillDefault is this
// bug's own regression test: llm.openai_compat.base_url is left at its
// shipped default value (never pre-set here, unlike every other
// openai_compat test in this file) which hardcodes OpenAI's own API.
// Without the fix, pingProvider below would silently ping api.openai.com
// with a wrong-provider (e.g. Qwen) key and fail with a 401 from OpenAI
// itself instead of ever reaching the fake "Qwen" endpoint — this test
// proves the interactive base_url prompt fires instead, that the fake
// endpoint (not api.openai.com) is what actually receives the request,
// and that the entered value is persisted before the ping runs.
func TestAuthLoginOpenAICompatPromptsForBaseURLWhenStillDefault(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	var requestCount int
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		gotHost = r.Host
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"qwen-max","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-qwen-test-key"), fakeTTY(true))
	cmd.SetIn(strings.NewReader(srv.URL + "\n"))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute())

	assert.Equal(t, 1, requestCount, "the fake endpoint must receive exactly one request")
	wantHost := strings.TrimPrefix(srv.URL, "http://")
	assert.Equal(t, wantHost, gotHost, "the request must land on the fake endpoint")
	assert.NotEqual(t, "api.openai.com", gotHost, "the request must NOT land on OpenAI's real API")

	loader, err := config.NewLoader("")
	require.NoError(t, err)
	got, err := loader.Get("llm.openai_compat.base_url")
	require.NoError(t, err)
	assert.Equal(t, srv.URL, got, "the entered base_url must be persisted")

	assert.Contains(t, stdout.String(), "Provider address (base_url) [current: https://api.openai.com/v1]")
	assert.Contains(t, stdout.String(), "Saved llm.openai_compat.base_url = "+srv.URL)
	assert.Contains(t, stdout.String(), "Model — enter this provider's model name", "the model prompt must fire once base_url is no longer the OpenAI default")
	assert.Contains(t, stdout.String(), "Stored key for openai_compat")
	assert.Contains(t, stdout.String(), "Test request succeeded")

	got, err = loader.Get("llm.model")
	require.NoError(t, err)
	assert.Equal(t, "", got, "no model line was supplied on stdin, so llm.model must stay empty")
}

// TestAuthLoginOpenAICompatPromptsForBaseURLWhenStillDefaultInTurkish is
// the same proof in Turkish — this project's established TR-smoke-test
// convention (exact substring pin, not merely "TR appears").
func TestAuthLoginOpenAICompatPromptsForBaseURLWhenStillDefaultInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	t.Setenv("COMRADE_LANG", "tr")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"qwen-max","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-qwen-test-key"), fakeTTY(true))
	cmd.SetIn(strings.NewReader(srv.URL + "\n"))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute())

	assert.Contains(t, stdout.String(), "Sağlayıcı adresi (base_url) [şu an: https://api.openai.com/v1]")
	assert.Contains(t, stdout.String(), "llm.openai_compat.base_url = "+srv.URL+" olarak kaydedildi")
	assert.Contains(t, stdout.String(), "Model — bu sağlayıcının model adını gir", "the model prompt must fire once base_url is no longer the OpenAI default")

	loader, err := config.NewLoader("")
	require.NoError(t, err)
	got, err := loader.Get("llm.openai_compat.base_url")
	require.NoError(t, err)
	assert.Equal(t, srv.URL, got)
}

// TestAuthLoginOpenAICompatSkipsPromptWhenBaseURLAlreadySet proves the
// prompt is a no-op once llm.openai_compat.base_url's effective VALUE no
// longer equals the shipped default (set explicitly via `comrade config
// set` here — the same precondition every OTHER openai_compat test in
// this file already relies on): stdin is pre-loaded with a URL that must
// NEVER be read — if the prompt fired anyway, it would persist that
// sentinel value over the real one and the ping would hit the wrong
// host — so leaving base_url == srv.URL after Execute, and the ping
// actually landing on srv (proven by the captured Authorization header),
// is proof the flow is unchanged and no extra prompt was ever emitted.
func TestAuthLoginOpenAICompatSkipsPromptWhenBaseURLAlreadySet(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-5.4-mini","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)
	// llm.model is ALSO pre-set here — deliberately, and required for this
	// test's own sentinel-input proof to still hold: promptOpenAICompatModelIfEmpty
	// (this task's own new addition) fires whenever base_url is non-default
	// AND llm.model is empty, regardless of whether base_url was customized
	// just now or, as here, already set beforehand — so leaving llm.model
	// empty would make the sentinel line below a LEGITIMATE model-prompt
	// read instead of the never-consumed poison this test needs it to be.
	// promptOpenAICompatModelIfEmpty's own firing behavior is covered by
	// TestAuthLoginOpenAICompatPromptsForModelWhenBaseURLNonDefaultAndModelEmpty.
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.model", "gpt-5.4-mini")
	require.NoError(t, err)

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-test-key-123"), fakeTTY(true))
	cmd.SetIn(strings.NewReader("http://should-never-be-read.invalid\n"))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute())

	assert.Equal(t, "Bearer sk-test-key-123", gotAuth, "the ping must land on the pre-configured srv.URL, not the unread sentinel")
	assert.Contains(t, stdout.String(), "Stored key for openai_compat")
	assert.NotContains(t, stdout.String(), "Provider address (base_url)", "no base_url prompt must be emitted when base_url is already explicitly set")
	assert.NotContains(t, stdout.String(), "Model —", "no model prompt must be emitted when llm.model is already explicitly set")

	loader, err := config.NewLoader("")
	require.NoError(t, err)
	got, err := loader.Get("llm.openai_compat.base_url")
	require.NoError(t, err)
	assert.Equal(t, srv.URL, got, "base_url must be unchanged — the unread sentinel must never be persisted")
}

// TestPromptOpenAICompatBaseURLEmitsCleartextWarningForWarnClassURL is
// the security-review fix's own regression test: entering a warned-but-
// allowed endpoint (http:// to a non-loopback host — config.CheckBaseURL's
// warn class, not its reject class) at the prompt must surface the SAME
// cleartext-credential warning `comrade config set` prints for the exact
// same value (config.EmitBaseURLWarning — reused, not duplicated), not
// silently accept it. Calls promptOpenAICompatBaseURL directly (rather
// than through the full auth-login flow) so the warn-class host never
// needs an actual live connection — pingProvider is not involved here at
// all.
func TestPromptOpenAICompatBaseURLEmitsCleartextWarningForWarnClassURL(t *testing.T) {
	withIsolatedConfigDir(t)

	loader, err := config.NewLoader("")
	require.NoError(t, err)

	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("http://192.168.1.50:11434\n"))
	var stdout strings.Builder
	cmd.SetOut(&stdout)

	warnings := captureOpenAICompatBaseURLWarning(t, func() {
		promptErr := promptOpenAICompatBaseURL(cmd, loader, i18n.NewTranslator(i18n.LangEN), "https://api.openai.com/v1", bufio.NewReader(cmd.InOrStdin()))
		require.NoError(t, promptErr)
	})

	require.Len(t, warnings, 1, "the accept path must emit exactly one warning")
	assert.Equal(t,
		"warning: llm.openai_compat.base_url is set to an http:// URL (192.168.1.50:11434); the API key will be sent unencrypted over the network to this host",
		warnings[0])

	got, err := loader.Get("llm.openai_compat.base_url")
	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.50:11434", got, "a warn-class (not reject-class) value must still be accepted and saved")
}

// TestPromptOpenAICompatBaseURLEmitsNoWarningForHTTPSURL proves the
// warning is genuinely conditional — an https:// value (no cleartext
// risk) must save without ever invoking the warning seam, exactly like
// config.CheckBaseURL's own warning == "" case.
func TestPromptOpenAICompatBaseURLEmitsNoWarningForHTTPSURL(t *testing.T) {
	withIsolatedConfigDir(t)

	loader, err := config.NewLoader("")
	require.NoError(t, err)

	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("https://dashscope.aliyuncs.com/compatible-mode/v1\n"))
	var stdout strings.Builder
	cmd.SetOut(&stdout)

	warnings := captureOpenAICompatBaseURLWarning(t, func() {
		promptErr := promptOpenAICompatBaseURL(cmd, loader, i18n.NewTranslator(i18n.LangEN), "https://api.openai.com/v1", bufio.NewReader(cmd.InOrStdin()))
		require.NoError(t, promptErr)
	})

	assert.Empty(t, warnings, "an https:// endpoint carries no cleartext risk and must not warn")

	got, err := loader.Get("llm.openai_compat.base_url")
	require.NoError(t, err)
	assert.Equal(t, "https://dashscope.aliyuncs.com/compatible-mode/v1", got)
}

// TestPromptOpenAICompatModelIfEmptyPromptsAndSavesWhenEligible is this
// task's own core proof: a non-OpenAI base_url (Qwen, here) combined with
// an empty llm.model must prompt for — and persist — a model name, so
// buildProvider (client.go) never silently falls back to
// llm.DefaultOpenAICompatModel() (an OpenAI-specific name) against a
// provider that has never heard of it.
func TestPromptOpenAICompatModelIfEmptyPromptsAndSavesWhenEligible(t *testing.T) {
	withIsolatedConfigDir(t)

	loader, err := config.NewLoader("")
	require.NoError(t, err)

	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("qwen-plus\n"))
	var stdout strings.Builder
	cmd.SetOut(&stdout)

	cfg := config.Default()
	cfg.LLM.OpenAICompat.BaseURL = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	cfg.LLM.Model = ""

	promptErr := promptOpenAICompatModelIfEmpty(cmd, loader, cfg, i18n.NewTranslator(i18n.LangEN), bufio.NewReader(cmd.InOrStdin()))
	require.NoError(t, promptErr)

	assert.Contains(t, stdout.String(), "Model — enter this provider's model name")

	got, err := loader.Get("llm.model")
	require.NoError(t, err)
	assert.Equal(t, "qwen-plus", got, "a non-empty model line must be persisted")
}

// TestPromptOpenAICompatModelIfEmptySkipsWhenBaseURLIsDefault proves the
// prompt never fires for a genuine OpenAI user (base_url still the
// shipped default) — the stdin sentinel below must never be read, since a
// blocking read here with no input queued would hang the test.
func TestPromptOpenAICompatModelIfEmptySkipsWhenBaseURLIsDefault(t *testing.T) {
	withIsolatedConfigDir(t)

	loader, err := config.NewLoader("")
	require.NoError(t, err)

	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("should-never-be-read\n"))
	var stdout strings.Builder
	cmd.SetOut(&stdout)

	cfg := config.Default()
	cfg.LLM.Model = ""

	promptErr := promptOpenAICompatModelIfEmpty(cmd, loader, cfg, i18n.NewTranslator(i18n.LangEN), bufio.NewReader(cmd.InOrStdin()))
	require.NoError(t, promptErr)

	assert.Empty(t, stdout.String(), "no prompt must be emitted when base_url is still the OpenAI default")

	got, err := loader.Get("llm.model")
	require.NoError(t, err)
	assert.Equal(t, "", got, "the unread sentinel must never be persisted as a model name")
}

// TestPromptOpenAICompatModelIfEmptySkipsWhenModelAlreadySet proves the
// prompt never fires once llm.model already has a value — same
// never-read-the-sentinel proof as the base_url-default case above, this
// time with base_url non-default but llm.model already populated.
func TestPromptOpenAICompatModelIfEmptySkipsWhenModelAlreadySet(t *testing.T) {
	withIsolatedConfigDir(t)

	loader, err := config.NewLoader("")
	require.NoError(t, err)

	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("should-never-be-read\n"))
	var stdout strings.Builder
	cmd.SetOut(&stdout)

	cfg := config.Default()
	cfg.LLM.OpenAICompat.BaseURL = "https://dashscope-intl.aliyuncs.com/compatible-mode/v1"
	cfg.LLM.Model = "qwen-max"

	promptErr := promptOpenAICompatModelIfEmpty(cmd, loader, cfg, i18n.NewTranslator(i18n.LangEN), bufio.NewReader(cmd.InOrStdin()))
	require.NoError(t, promptErr)

	assert.Empty(t, stdout.String(), "no prompt must be emitted when llm.model is already set")

	got, err := loader.Get("llm.model")
	require.NoError(t, err)
	assert.Equal(t, "", got, "loader must be untouched — the pre-set value came from cfg, not from SetAndSave")
}

func TestAuthLoginStoresKeyEvenWhenPingFails(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-still-stored"), fakeTTY(true))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute(), "a failed ping must not turn auth login into a command error")

	assert.Contains(t, stdout.String(), "Key saved")
	assert.Contains(t, stdout.String(), "Couldn't verify it right now")

	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	key, _, err := store.Get(context.Background(), "openai_compat")
	require.NoError(t, err)
	assert.Equal(t, "sk-still-stored", key, "the key must be stored regardless of whether the ping succeeded")
}

// TestAuthLoginOpenAICompatReportsModelNotFoundOn404WithModelMessage is
// this task's own ping-classification proof: a 404 response whose body
// mentions "model" (openai_compat's real-world shape for an unknown-model
// error) must render the dedicated MsgAuthModelNotFound notice — naming
// the model that was actually pinged — rather than the generic
// MsgAuthStoredKeyPingFailed framing, which would misleadingly read as a
// network hiccup. Non-fatal: the key is stored and Execute returns nil,
// exactly like every other ping-failure class except ErrAuthRejected.
// llm.model is pre-set via `config set` (rather than left for the
// interactive model prompt to fill) so the effective model name is
// deterministic and the test needs no stdin at all.
func TestAuthLoginOpenAICompatReportsModelNotFoundOn404WithModelMessage(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"The model 'gpt-5.4-mini' does not exist"}}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.model", "gpt-5.4-mini")
	require.NoError(t, err)

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-still-stored"), fakeTTY(true))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute(), "an unknown-model 404 must not turn auth login into a command error")

	assert.Contains(t, stdout.String(),
		"Key saved ✓  But model 'gpt-5.4-mini' doesn't exist on this provider.\n› Pick a model:  comrade config models   then:  comrade config set llm.model <model>\n")
	assert.NotContains(t, stdout.String(), "Couldn't verify it right now", "must not use the generic ping-failed framing for an unknown-model 404")

	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	key, _, err := store.Get(context.Background(), "openai_compat")
	require.NoError(t, err)
	assert.Equal(t, "sk-still-stored", key, "the key must be stored — the model, not the key, is the problem")
}

// TestAuthLoginOpenAICompatReportsPingFailedOn404WithoutModelWording
// proves the 404 classification is scoped to messages that actually
// mention "model" — a 404 for an unrelated reason (route not found,
// resource not found, ...) must fall through to the generic
// MsgAuthStoredKeyPingFailed framing instead of misreporting an unrelated
// 404 as a model problem.
func TestAuthLoginOpenAICompatReportsPingFailedOn404WithoutModelWording(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"resource not found"}}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.model", "gpt-5.4-mini")
	require.NoError(t, err)

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-still-stored"), fakeTTY(true))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute())

	assert.Contains(t, stdout.String(), "Couldn't verify it right now")
	assert.NotContains(t, stdout.String(), "doesn't exist on this provider", "a 404 whose message never mentions \"model\" must not be misreported as an unknown-model error")
}

// TestAuthLoginOpenAICompatReportsPingFailedOnOfflineError proves
// llm.ErrOffline (a transport-level failure — connection refused here,
// since srv is closed before Execute ever runs) renders the generic
// MsgAuthStoredKeyPingFailed notice, not MsgAuthModelNotFound — the
// 404-model classification must never fire for a class of failure that
// never even reached the provider's HTTP layer.
func TestAuthLoginOpenAICompatReportsPingFailedOnOfflineError(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachableURL := srv.URL
	srv.Close() // closed before Execute: every request against unreachableURL now gets connection-refused

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", unreachableURL)
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.model", "gpt-5.4-mini")
	require.NoError(t, err)

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-still-stored"), fakeTTY(true))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute(), "an offline/unreachable provider must not turn auth login into a command error")

	assert.Contains(t, stdout.String(), "Couldn't verify it right now")
	assert.NotContains(t, stdout.String(), "doesn't exist on this provider")

	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	key, _, err := store.Get(context.Background(), "openai_compat")
	require.NoError(t, err)
	assert.Equal(t, "sk-still-stored", key)
}

// TestAuthLoginStoresKeyAndReportsBaseURLUnsafeInsteadOfPingFailed is the
// reviewer-flagged residual's fix: when the active provider's base_url
// reaches config via a COMRADE_ env var (the same bypass path
// TestDoRefusesToBuildClientForMetadataBaseURLActiveProvider (do_test.go)
// documents — `config set` itself still hard-rejects this value directly,
// so it cannot be the source here) is a cloud-metadata address,
// pingProvider's own llm.New call never even attempts a request —
// isBaseURLRejection recognizes the SAME *config.InvalidValueError
// do/fix/explain/chat's translateBaseURLRejectedError does. The key must
// still be stored (buildProvider refuses before any network call, so it
// was never transmitted — storing it locally is harmless), but the
// printed message must be the translated, base_url-focused
// MsgAuthStoredKeyBaseURLUnsafe — NOT the generic
// MsgAuthStoredKeyPingFailed "could not verify it right now" framing,
// which would misleadingly read as a network hiccup rather than a
// security refusal, and NOT the raw, untranslated
// *config.InvalidValueError.Error() text either.
func TestAuthLoginStoresKeyAndReportsBaseURLUnsafeInsteadOfPingFailed(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", "http://169.254.169.254/latest/meta-data/")

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-still-stored"), fakeTTY(true))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute(), "a base_url-reject ping must not turn auth login into a command error")

	assert.Contains(t, stdout.String(),
		`Stored key for openai_compat. Skipped the live test — llm.openai_compat.base_url (currently "http://169.254.169.254/latest/meta-data/") is not a safe endpoint, so your key was never sent there; fix it with: comrade config set llm.openai_compat.base_url <valid-url>`+"\n")
	assert.NotContains(t, stdout.String(), "Could not verify it right now", "must not use the generic ping-failed framing for a base_url refusal")
	assert.NotContains(t, stdout.String(), "InvalidValueError")

	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	key, _, err := store.Get(context.Background(), "openai_compat")
	require.NoError(t, err)
	assert.Equal(t, "sk-still-stored", key, "the key must still be stored — buildProvider refuses before any network call, so it was never transmitted")
}

// TestAuthLoginStoresKeyAndReportsBaseURLUnsafeInTurkish is the same proof
// in Turkish — this project's established TR-smoke-test convention (exact
// full-string pin, not merely "TR appears").
func TestAuthLoginStoresKeyAndReportsBaseURLUnsafeInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	t.Setenv("COMRADE_LANG", "tr")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", "http://169.254.169.254/latest/meta-data/")

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-still-stored"), fakeTTY(true))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute())

	assert.Contains(t, stdout.String(),
		`openai_compat için anahtar kaydedildi. Canlı test atlandı — llm.openai_compat.base_url (şu an "http://169.254.169.254/latest/meta-data/") güvenli bir uç nokta değil, bu yüzden anahtarınız oraya hiç gönderilmedi; düzeltmek için: comrade config set llm.openai_compat.base_url <geçerli-url>`+"\n")

	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	key, _, err := store.Get(context.Background(), "openai_compat")
	require.NoError(t, err)
	assert.Equal(t, "sk-still-stored", key)
}

// TestAuthLoginNeverWritesKeyWhenProviderRejectsIt is QA MAJOR-2's
// branch (a), reordered per review: pingProvider verifies the IN-MEMORY
// key directly (its own llm.WithKeyResolver closure, never the store),
// so a 401/403 response (llm.ErrAuthRejected) is now known BEFORE any
// store.Set call ever happens — auth login must return a genuine
// (nonzero-exit) command error, the i18n'd MsgAuthKeyRejected message,
// and the credentials file must NEVER be created at all (proving the key
// was never written in the first place, not written-then-deleted — the
// file backend, not the keychain mock, is used here specifically because
// "the file was never created" is directly observable on disk, where a
// write-then-delete would still have created it, just later emptied it).
func TestAuthLoginNeverWritesKeyWhenProviderRejectsIt(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	withUnavailableKeychain(t)
	credentialsPath := filepath.Join(dir, "cli-comrade", credentialsFileName)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)
	_, statErr := os.Stat(credentialsPath)
	require.True(t, os.IsNotExist(statErr), "precondition: no credentials file yet from the config-set calls above")

	var stdout strings.Builder
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-definitely-bad"), fakeTTY(true))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	err = cmd.Execute()

	require.Error(t, err, "a definitively rejected key must be a nonzero-exit command error")
	assert.Contains(t, err.Error(), "The provider rejected this key for openai_compat")
	assert.Contains(t, err.Error(), `comrade auth login openai_compat`)
	assert.NotContains(t, stdout.String(), "Stored key for openai_compat", "the storage confirmation must not print once the key is known-bad")

	_, statErr = os.Stat(credentialsPath)
	assert.True(t, os.IsNotExist(statErr), "the credentials file must never be created when the provider rejects the key — proof of ping-before-store, not store-then-delete")

	store, storeErr := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, storeErr)
	_, source, getErr := store.Get(context.Background(), "openai_compat")
	require.NoError(t, getErr)
	assert.Equal(t, secrets.SourceNone, source, "a key the provider rejected must not remain stored")
}

// TestAuthLoginNeverWritesKeyWhenProviderRejectsItInTurkish is the same
// proof in Turkish — this project's established TR-smoke-test convention
// (exact full-string pin, not merely "TR appears").
func TestAuthLoginNeverWritesKeyWhenProviderRejectsItInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	withUnavailableKeychain(t)
	t.Setenv("COMRADE_LANG", "tr")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"forbidden"}}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-definitely-bad"), fakeTTY(true))
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	err = cmd.Execute()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "openai_compat için bu anahtar sağlayıcı tarafından reddedildi")
	assert.Contains(t, err.Error(), `comrade auth login openai_compat`)
}

// TestAuthLoginNonInteractiveStdinReportsFriendlyError is QA MINOR-5's
// fix: without it, a non-TTY stdin reached x/term.ReadPassword and
// failed with a raw platform errno ("inappropriate ioctl for device" on
// Unix) instead of a message a non-expert user could act on. The
// password reader is never even invoked once the guard fires (asserted
// via a reader that panics if called), and no key is stored.
func TestAuthLoginNonInteractiveStdinReportsFriendlyError(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	panicReader := func(int) ([]byte, error) {
		panic("readPassword must not be called once the TTY guard fires")
	}
	cmd := newAuthLoginCmd(newTestLoaderFactory(), panicReader, fakeTTY(false))
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"anthropic"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Equal(t, "auth login needs an interactive terminal (stdin is not a TTY) — run it directly in a terminal, not piped or redirected.", err.Error())

	store, storeErr := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, storeErr)
	_, source, getErr := store.Get(context.Background(), "anthropic")
	require.NoError(t, getErr)
	assert.Equal(t, secrets.SourceNone, source)
}

// TestAuthLoginNonInteractiveStdinReportsFriendlyErrorInTurkish is the
// same proof in Turkish.
func TestAuthLoginNonInteractiveStdinReportsFriendlyErrorInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	t.Setenv("COMRADE_LANG", "tr")

	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("unused"), fakeTTY(false))
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"anthropic"})

	err := cmd.Execute()

	require.Error(t, err)
	assert.Equal(t, "auth login, etkileşimli bir terminal gerektirir (stdin bir TTY değil) — doğrudan bir terminalde çalıştırın, boru hattına yönlendirmeyin.", err.Error())
}

func TestAuthLoginRejectsEmptyKey(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("   "), fakeTTY(true))
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"anthropic"})

	err := cmd.Execute()

	assert.ErrorContains(t, err, "no key entered")

	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	_, source, err := store.Get(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, secrets.SourceNone, source, "an empty key must never be stored")
}

func TestAuthLogoutRemovesStoredKey(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), "anthropic", "sk-to-remove"))

	stdout, _, err := execRootSplit(t, "dev", "auth", "logout", "anthropic")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Removed stored key for anthropic")

	_, source, err := store.Get(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, secrets.SourceNone, source)
}

func TestAuthLogoutNoStoredKeyReportsWithoutError(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	stdout, _, err := execRootSplit(t, "dev", "auth", "logout", "anthropic")

	require.NoError(t, err)
	assert.Contains(t, stdout, "No stored key for anthropic")
}

func TestAuthStatusShowsNotSetForEveryProviderByDefault(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	for _, envVar := range []string{
		"COMRADE_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY",
		"COMRADE_OPENAI_COMPAT_API_KEY", "OPENAI_API_KEY",
		"COMRADE_GOOGLE_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY",
	} {
		t.Setenv(envVar, "")
	}

	stdout, _, err := execRootSplit(t, "dev", "auth", "status")
	require.NoError(t, err)

	assert.Contains(t, findTableRow(stdout, "anthropic"), "not set")
	assert.Contains(t, findTableRow(stdout, "openai_compat"), "not set")
	assert.Contains(t, findTableRow(stdout, "google"), "not set")
	assert.Contains(t, findTableRow(stdout, "ollama"), "no key required")
}

func TestAuthStatusShowsEnvSourceWhenNoStoredKey(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	t.Setenv("COMRADE_ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-env")

	stdout, _, err := execRootSplit(t, "dev", "auth", "status")
	require.NoError(t, err)

	assert.Contains(t, findTableRow(stdout, "anthropic"), "set (env: ANTHROPIC_API_KEY)")
}

func TestAuthStatusPrefersStoredKeychainOverEnv(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	t.Setenv("ANTHROPIC_API_KEY", "sk-from-env")
	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), "anthropic", "sk-from-keychain"))

	stdout, _, err := execRootSplit(t, "dev", "auth", "status")
	require.NoError(t, err)

	assert.Contains(t, findTableRow(stdout, "anthropic"), "set (keychain)")
}

func TestAuthStatusShowsFileSourceWhenKeychainUnavailable(t *testing.T) {
	withIsolatedConfigDir(t)
	withUnavailableKeychain(t)
	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), "google", "sk-file-fallback"))

	stdout, _, err := execRootSplit(t, "dev", "auth", "status")
	require.NoError(t, err)

	assert.Contains(t, findTableRow(stdout, "google"), "set (file)")
}

// TestAuthStatusFileFallbackWarningIsSoftenedAndTranslated is QA
// MINOR-4's fix: the file-fallback notice now routes through i18n
// (rather than internal/secrets' own hardcoded English default), with
// softened wording — the security-relevant fact (base64, not encrypted)
// stays, the earlier more alarming phrasing does not.
func TestAuthStatusFileFallbackWarningIsSoftenedAndTranslated(t *testing.T) {
	withIsolatedConfigDir(t)
	withUnavailableKeychain(t)
	// Pre-touch the config file so this call's own stderr contains
	// nothing but the file-fallback warning under test — MsgFirstRunNotice
	// would otherwise also land on stderr the first time ANY command
	// touches a freshly isolated config dir.
	_, _, err := execRootSplit(t, "dev", "config", "list")
	require.NoError(t, err)

	_, stderr, err := execRootSplit(t, "dev", "auth", "status")

	require.NoError(t, err)
	assert.Equal(t, "cli-comrade: no system keychain found, so API keys are being saved to a local file instead (base64-encoded, not encrypted — see the file's own header for details).\n", stderr)
}

// TestAuthStatusFileFallbackWarningIsSoftenedAndTranslatedInTurkish is
// the same proof in Turkish.
func TestAuthStatusFileFallbackWarningIsSoftenedAndTranslatedInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	withUnavailableKeychain(t)
	_, _, err := execRootSplit(t, "dev", "config", "set", "general.language", "tr")
	require.NoError(t, err)

	_, stderr, err := execRootSplit(t, "dev", "auth", "status")

	require.NoError(t, err)
	assert.Equal(t, "cli-comrade: sistem anahtarlığı bulunamadı, bu yüzden API anahtarları yerel bir dosyaya kaydediliyor (base64 ile kodlanmış, şifrelenmemiş — ayrıntılar için dosyanın kendi başlığına bakın).\n", stderr)
}

func TestAuthStatusNeverPrintsKeyValues(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	const sentinel = "sk-super-secret-sentinel-value"
	store, err := newSecretsStore(io.Discard, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), "anthropic", sentinel))
	t.Setenv("GOOGLE_API_KEY", sentinel)

	stdout, stderr, err := execRootSplit(t, "dev", "auth", "status")
	require.NoError(t, err)

	assert.NotContains(t, stdout, sentinel)
	assert.NotContains(t, stderr, sentinel)
}

// TestAuthLoginWrongArgCountShowsTranslatedUsageError proves `comrade
// auth login`'s Args (translatedExactArgs, auth.go) renders a friendly,
// i18n'd usage error — naming every secrets.KnownProviders entry, per
// the same pattern MsgAuthUnknownProviderError already uses — instead
// of cobra's raw English "accepts 1 arg(s), received 0/2", for both too
// few (0) and too many (2+) arguments, under both halves of
// bestEffortTranslator's resolution chain: (a) a config file with
// general.language="tr" and every language env var explicitly emptied
// (proving config alone, with no matching env var, is enough — the
// exact case config_test.go's own "...FromConfigLanguageAlone" sibling
// tests pin for `config set`), and (b) a totally fresh install (no
// config file yet) with LANG=en_US and no COMRADE_LANG/LC_ALL set
// (proving the English default, and that the usage-error path still
// creates the first-run config file exactly like every other command's
// first invocation).
func TestAuthLoginWrongArgCountShowsTranslatedUsageError(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T)
		want  string
	}{
		{
			name: "turkish from config general.language alone",
			setup: func(t *testing.T) {
				dir := withIsolatedConfigDir(t)
				t.Setenv("COMRADE_LANG", "")
				t.Setenv("LANG", "")
				t.Setenv("LC_ALL", "")
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "cli-comrade"), 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, "cli-comrade", "config.toml"),
					[]byte("[general]\nlanguage = \"tr\"\n"), 0o600))
			},
			want: "kullanım: comrade auth login <sağlayıcı> (beklenen: anthropic, openai_compat, google)",
		},
		{
			name: "english default with no config and LANG=en_US",
			setup: func(t *testing.T) {
				withIsolatedConfigDir(t)
				t.Setenv("COMRADE_LANG", "")
				t.Setenv("LANG", "en_US")
				t.Setenv("LC_ALL", "")
			},
			want: "usage: comrade auth login <provider> (expected one of: anthropic, openai_compat, google)",
		},
	}

	for _, tc := range cases {
		for _, extraArgs := range [][]string{{}, {"a", "b"}} {
			t.Run(tc.name+"/"+strings.Join(extraArgs, ","), func(t *testing.T) {
				tc.setup(t)
				args := append([]string{"auth", "login"}, extraArgs...)
				_, _, err := execRootSplit(t, "dev", args...)
				require.Error(t, err)
				assert.Equal(t, tc.want, err.Error())
			})
		}
	}
}

// TestAuthLogoutWrongArgCountShowsTranslatedUsageError is `comrade auth
// logout`'s counterpart to TestAuthLoginWrongArgCountShowsTranslatedUsageError
// — a leaner single-case proof (0 args, default English) since logout's
// Args wiring is otherwise identical (translatedExactArgs, same
// provider-list message shape).
func TestAuthLogoutWrongArgCountShowsTranslatedUsageError(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "auth", "logout")

	require.Error(t, err)
	assert.Equal(t, "usage: comrade auth logout <provider> (expected one of: anthropic, openai_compat, google)", err.Error())
}

// TestAuthUnknownSubcommandShowsTranslatedError proves `comrade auth
// <bogus>` (no subcommand named "bogus") renders a friendly, i18n'd
// "unknown subcommand" error (translatedUnknownSubcommand,
// argvalidation.go) naming every real subcommand, instead of silently
// printing help and exiting 0 (auth's prior behavior — see
// translatedUnknownSubcommand's own doc comment for why this was never
// actually cobra's raw "unknown command" text either).
func TestAuthUnknownSubcommandShowsTranslatedError(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "auth", "bogus")

	require.Error(t, err)
	assert.Equal(t, `unknown subcommand "bogus" for comrade auth (expected one of: login, logout, status)`, err.Error())
}

// TestAuthUnknownSubcommandShowsTranslatedErrorInTurkish is the same
// proof under COMRADE_LANG=tr.
func TestAuthUnknownSubcommandShowsTranslatedErrorInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "auth", "bogus")

	require.Error(t, err)
	assert.Equal(t, `"bogus": comrade auth için bilinmeyen alt komut (beklenen: login, logout, status)`, err.Error())
}

// TestAuthBareInvocationStillPrintsHelpAndExitsZero proves a bare
// "comrade auth" (no subcommand at all) keeps its pre-existing behavior
// unchanged: help text, exit 0 — translatedUnknownSubcommand only ever
// fires for len(args) > 0, and newAuthCmd's own RunE (added alongside
// it, mirroring newHookCmd) reproduces exactly what cobra's non-Runnable
// default used to do for this exact case.
func TestAuthBareInvocationStillPrintsHelpAndExitsZero(t *testing.T) {
	withIsolatedConfigDir(t)

	stdout, _, err := execRootSplit(t, "dev", "auth")

	require.NoError(t, err)
	assert.Contains(t, stdout, "Usage:")
	assert.Contains(t, stdout, "comrade auth")
	assert.Contains(t, stdout, "login")
}

// TestAuthLoginSubcommandResolutionBypassesParentUnknownSubcommandCheck
// verifies the assumption translatedUnknownSubcommand's doc comment
// states as fact: cobra's Find() resolves a REAL subcommand name (e.g.
// "login") all the way down to that leaf command, so the PARENT's own
// Args validator (translatedUnknownSubcommand) never runs for it at all
// — only the leaf's own Args (translatedExactArgs) does. Proven here by
// the exact error text: it must be login's own usage error (naming
// "login"/providers), never the parent's "unknown subcommand" message
// naming "login" as the unmatched arg.
func TestAuthLoginSubcommandResolutionBypassesParentUnknownSubcommandCheck(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "auth", "login")

	require.Error(t, err)
	assert.Equal(t, "usage: comrade auth login <provider> (expected one of: anthropic, openai_compat, google)", err.Error())
	assert.NotContains(t, err.Error(), "unknown subcommand")
}
