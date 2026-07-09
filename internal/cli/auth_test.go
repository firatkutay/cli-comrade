package cli

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

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
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-test-key-123"))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute())

	assert.Equal(t, "Bearer sk-test-key-123", gotAuth)
	assert.Contains(t, stdout.String(), "Stored key for openai_compat")
	assert.Contains(t, stdout.String(), "Test request succeeded")
	assert.NotContains(t, stdout.String(), "sk-test-key-123", "the entered key must never be echoed back")

	store, err := newSecretsStore(io.Discard)
	require.NoError(t, err)
	key, source, err := store.Get(context.Background(), "openai_compat")
	require.NoError(t, err)
	assert.Equal(t, "sk-test-key-123", key)
	assert.Equal(t, secrets.SourceKeychain, source)
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
	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("sk-still-stored"))
	cmd.SetOut(&stdout)
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"openai_compat"})

	require.NoError(t, cmd.Execute(), "a failed ping must not turn auth login into a command error")

	assert.Contains(t, stdout.String(), "Stored key for openai_compat")
	assert.Contains(t, stdout.String(), "Test request failed")

	store, err := newSecretsStore(io.Discard)
	require.NoError(t, err)
	key, _, err := store.Get(context.Background(), "openai_compat")
	require.NoError(t, err)
	assert.Equal(t, "sk-still-stored", key, "the key must be stored regardless of whether the ping succeeded")
}

func TestAuthLoginRejectsEmptyKey(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)

	cmd := newAuthLoginCmd(newTestLoaderFactory(), fakePasswordReader("   "))
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"anthropic"})

	err := cmd.Execute()

	assert.ErrorContains(t, err, "no key entered")

	store, err := newSecretsStore(io.Discard)
	require.NoError(t, err)
	_, source, err := store.Get(context.Background(), "anthropic")
	require.NoError(t, err)
	assert.Equal(t, secrets.SourceNone, source, "an empty key must never be stored")
}

func TestAuthLogoutRemovesStoredKey(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	store, err := newSecretsStore(io.Discard)
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
	store, err := newSecretsStore(io.Discard)
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), "anthropic", "sk-from-keychain"))

	stdout, _, err := execRootSplit(t, "dev", "auth", "status")
	require.NoError(t, err)

	assert.Contains(t, findTableRow(stdout, "anthropic"), "set (keychain)")
}

func TestAuthStatusShowsFileSourceWhenKeychainUnavailable(t *testing.T) {
	withIsolatedConfigDir(t)
	withUnavailableKeychain(t)
	store, err := newSecretsStore(io.Discard)
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), "google", "sk-file-fallback"))

	stdout, _, err := execRootSplit(t, "dev", "auth", "status")
	require.NoError(t, err)

	assert.Contains(t, findTableRow(stdout, "google"), "set (file)")
}

func TestAuthStatusNeverPrintsKeyValues(t *testing.T) {
	withIsolatedConfigDir(t)
	withMockKeychain(t)
	const sentinel = "sk-super-secret-sentinel-value"
	store, err := newSecretsStore(io.Discard)
	require.NoError(t, err)
	require.NoError(t, store.Set(context.Background(), "anthropic", sentinel))
	t.Setenv("GOOGLE_API_KEY", sentinel)

	stdout, stderr, err := execRootSplit(t, "dev", "auth", "status")
	require.NoError(t, err)

	assert.NotContains(t, stdout, sentinel)
	assert.NotContains(t, stderr, sentinel)
}
