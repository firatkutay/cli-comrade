package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigModelsAnthropicStaticListSelectionPersists(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "anthropic")
	require.NoError(t, err)

	stdout, _, err := execRootSplitWithStdin(t, "2\n", "config", "models")
	require.NoError(t, err)

	assert.Contains(t, stdout, "1) claude-haiku-4-5")
	assert.Contains(t, stdout, "2) claude-sonnet-5")
	assert.Contains(t, stdout, "3) claude-opus-4-8")
	assert.Contains(t, stdout, "docs.claude.com")
	assert.Contains(t, stdout, "llm.model = claude-sonnet-5")

	got, _, err := execRootSplit(t, "dev", "config", "get", "llm.model")
	require.NoError(t, err)
	assert.Equal(t, "claude-sonnet-5", strings.TrimSpace(got))
}

func TestConfigModelsGoogleStaticList(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "google")
	require.NoError(t, err)

	stdout, _, err := execRootSplitWithStdin(t, "1\n", "config", "models")
	require.NoError(t, err)

	assert.Contains(t, stdout, "1) gemini-3.5-flash")
	assert.Contains(t, stdout, "ai.google.dev")
	assert.Contains(t, stdout, "llm.model = gemini-3.5-flash")
}

func TestConfigModelsOllamaLiveListSelectionPersists(t *testing.T) {
	withIsolatedConfigDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tags", r.URL.Path)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.1"},{"name":"mistral"}]}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "ollama")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.ollama.base_url", srv.URL)
	require.NoError(t, err)

	stdout, _, err := execRootSplitWithStdin(t, "2\n", "config", "models")
	require.NoError(t, err)

	assert.Contains(t, stdout, "1) llama3.1")
	assert.Contains(t, stdout, "2) mistral")
	assert.Contains(t, stdout, "llm.model = mistral")
}

func TestConfigModelsOllamaUnreachableProducesFriendlyError(t *testing.T) {
	withIsolatedConfigDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "ollama")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.ollama.base_url", url)
	require.NoError(t, err)

	_, _, err = execRootSplitWithStdin(t, "1\n", "config", "models")

	require.Error(t, err)
	assert.ErrorContains(t, err, "does not appear to be running")
	assert.ErrorContains(t, err, "ollama serve")
}

func TestConfigModelsOpenAICompatLiveListUsesEnvKeyWhenNothingStored(t *testing.T) {
	withIsolatedConfigDir(t)
	withUnavailableKeychain(t)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "sk-test-listing")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.4-mini"},{"id":"gpt-5.4"}]}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	stdout, _, err := execRootSplitWithStdin(t, "2\n", "config", "models")
	require.NoError(t, err)

	assert.Equal(t, "Bearer sk-test-listing", gotAuth)
	assert.Contains(t, stdout, "llm.model = gpt-5.4")
}

func TestConfigModelsOutOfRangeSelectionErrorsWithoutPersisting(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "anthropic")
	require.NoError(t, err)

	_, _, err = execRootSplitWithStdin(t, "99\n", "config", "models")

	require.Error(t, err)
	assert.ErrorContains(t, err, "out of range")

	got, _, err := execRootSplit(t, "dev", "config", "get", "llm.model")
	require.NoError(t, err)
	assert.Equal(t, "", strings.TrimSpace(got), "an invalid selection must not persist anything to llm.model")
}

func TestConfigModelsNonNumericSelectionErrors(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "anthropic")
	require.NoError(t, err)

	_, _, err = execRootSplitWithStdin(t, "not-a-number\n", "config", "models")

	require.Error(t, err)
	assert.ErrorContains(t, err, "not a number")
}
