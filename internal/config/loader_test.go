package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoaderDefaultConfigLoadsWithoutErrorOrWarning pins the "must stay
// non-breaking" requirement for SAST finding #3's base_url validation: the
// two shipped defaults (an https public host, and http on loopback) must
// keep loading cleanly with no error and no cleartext warning.
func TestLoaderDefaultConfigLoadsWithoutErrorOrWarning(t *testing.T) {
	buf := captureBaseURLWarnings(t)
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, "https://api.openai.com/v1", cfg.LLM.OpenAICompat.BaseURL)
	assert.Equal(t, "http://localhost:11434", cfg.LLM.Ollama.BaseURL)
	assert.Empty(t, buf.String())
}

// TestLoaderDoesNotBrickOnMetadataBaseURLForInactiveProvider is the
// regression guard for the "un-brick Load()" fix: a base_url reaching the
// file some other way than `comrade config set` (here, a hand-edited file)
// used to make Load() FAIL for this exact fixture — including for every
// command that calls Load() before it can do anything else, `comrade
// config set`/`get`/`edit`/`path` among them — with no in-tool way back
// in. llm.provider here is left at its default ("anthropic"), so
// llm.openai_compat.base_url is a value nobody is even using: Load() must
// now succeed AND stay silent (no warning either — see
// validateLoadedConfig's own doc comment for why an unused provider's bad
// value produces no per-invocation noise). The companion active-provider
// case (warns, still succeeds) is
// TestLoaderWarnsOnMetadataBaseURLForActiveProvider below; the
// companion "repair commands survive this file" proof, at the actual
// `comrade config` command surface, is
// TestConfigSetGetPathWorkOnFileWithMetadataBaseURLForInactiveProvider in
// internal/cli/config_test.go.
func TestLoaderDoesNotBrickOnMetadataBaseURLForInactiveProvider(t *testing.T) {
	buf := captureBaseURLWarnings(t)
	path := tempConfigPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	toml := "[llm.openai_compat]\nbase_url = \"https://169.254.169.254/v1\"\n"
	require.NoError(t, os.WriteFile(path, []byte(toml), 0o644))

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()

	require.NoError(t, err, "Load() must never fail because of a base_url value — see validateLoadedConfig")
	assert.Equal(t, "anthropic", cfg.LLM.Provider, "precondition: openai_compat must be inactive")
	assert.Equal(t, "https://169.254.169.254/v1", cfg.LLM.OpenAICompat.BaseURL)
	assert.Empty(t, buf.String(), "an inactive provider's bad base_url must stay silent")

	// The repair path itself: Get/SetAndSave (what `comrade config
	// get`/`set` call) must still work against this exact file.
	value, err := loader.Get("llm.openai_compat.base_url")
	require.NoError(t, err)
	assert.Equal(t, "https://169.254.169.254/v1", value)

	require.NoError(t, loader.SetAndSave("llm.openai_compat.base_url", "https://api.openai.com/v1"))
	cfg, _, err = loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "https://api.openai.com/v1", cfg.LLM.OpenAICompat.BaseURL, "the repaired value must persist")
}

// TestLoaderWarnsOnMetadataBaseURLForActiveProvider is
// TestLoaderDoesNotBrickOnMetadataBaseURLForInactiveProvider's counterpart:
// the SAME reject-class value, but for the ACTIVE provider this time —
// Load() must still succeed (never hard-fail; see validateLoadedConfig's
// own doc comment), but now it must warn, since this is the base_url an
// LLM client would actually be built against (and — separately —
// internal/llm.buildProvider is where that HARD reject actually happens,
// once a client is built for do/fix/chat/explain; see client_test.go).
func TestLoaderWarnsOnMetadataBaseURLForActiveProvider(t *testing.T) {
	buf := captureBaseURLWarnings(t)
	path := tempConfigPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	toml := "[llm]\nprovider = \"openai_compat\"\n\n[llm.openai_compat]\nbase_url = \"https://169.254.169.254/v1\"\n"
	require.NoError(t, os.WriteFile(path, []byte(toml), 0o644))

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, "https://169.254.169.254/v1", cfg.LLM.OpenAICompat.BaseURL)
	assert.Contains(t, buf.String(), "llm.openai_compat.base_url")
	assert.Contains(t, buf.String(), "cloud metadata / link-local address not allowed")
}

// TestLoaderWarnsOnCleartextBaseURLFromEnv exercises the other bypass path
// (a COMRADE_ environment variable) and confirms it still only warns, never
// rejects, for a legitimate self-hosted LAN endpoint over http:// — for the
// ACTIVE provider (COMRADE_PROVIDER=ollama alongside
// COMRADE_LLM_OLLAMA_BASE_URL): validateLoadedConfig scopes its warning to
// cfg.LLM.Provider only, so without this the env-supplied ollama base_url
// would go unchecked while the default "anthropic" provider stays active.
func TestLoaderWarnsOnCleartextBaseURLFromEnv(t *testing.T) {
	buf := captureBaseURLWarnings(t)
	path := tempConfigPath(t)
	t.Setenv("COMRADE_PROVIDER", "ollama")
	t.Setenv("COMRADE_LLM_OLLAMA_BASE_URL", "http://192.168.1.50:11434")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, "http://192.168.1.50:11434", cfg.LLM.Ollama.BaseURL)
	assert.Contains(t, buf.String(), "llm.ollama.base_url")
	assert.Contains(t, buf.String(), "unencrypted")
}

// TestLoaderStaysSilentOnCleartextBaseURLForInactiveProviderFromEnv is
// TestLoaderWarnsOnCleartextBaseURLFromEnv's negative counterpart: the
// SAME env-supplied value, but with llm.provider left at its default
// ("anthropic") — must load cleanly with no warning at all, since
// validateLoadedConfig only ever looks at the active provider's base_url.
func TestLoaderStaysSilentOnCleartextBaseURLForInactiveProviderFromEnv(t *testing.T) {
	buf := captureBaseURLWarnings(t)
	path := tempConfigPath(t)
	t.Setenv("COMRADE_LLM_OLLAMA_BASE_URL", "http://192.168.1.50:11434")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()

	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.LLM.Provider, "precondition: ollama must be inactive")
	assert.Equal(t, "http://192.168.1.50:11434", cfg.LLM.Ollama.BaseURL)
	assert.Empty(t, buf.String(), "an inactive provider's base_url must never warn, even from env")
}

func tempConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config.toml")
}

func TestLoaderFirstRunCreatesFileWithDefaults(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err), "precondition: config file must not exist yet")

	cfg, created, err := loader.Load()
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, Default(), *cfg)

	_, err = os.Stat(path)
	assert.NoError(t, err, "config file should now exist on disk")
}

func TestLoaderSecondLoadDoesNotReportCreation(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	_, created, err := loader.Load()
	require.NoError(t, err)
	require.True(t, created)

	_, created, err = loader.Load()
	require.NoError(t, err)
	assert.False(t, created)
}

func TestLoaderPartialFileFillsInMissingSectionsWithDefaults(t *testing.T) {
	path := tempConfigPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("[general]\nmode = \"auto\"\n"), 0o644))

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, created, err := loader.Load()
	require.NoError(t, err)
	assert.False(t, created, "an already-existing file must not be reported as created")

	assert.Equal(t, "auto", cfg.General.Mode, "explicit value from the partial file must win")
	assert.Equal(t, "auto", cfg.General.Language, "missing key must fall back to default")
	assert.Equal(t, "anthropic", cfg.LLM.Provider, "missing section must fall back to default")
	assert.Equal(t, 60, cfg.LLM.TimeoutSeconds)
	assert.Equal(t, 90, cfg.Audit.RetentionDays)
}

func TestLoaderEnvOverridesFileValue(t *testing.T) {
	path := tempConfigPath(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("[general]\nmode = \"auto\"\n"), 0o644))
	t.Setenv("COMRADE_GENERAL_MODE", "info")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "info", cfg.General.Mode, "env must beat the file value")
}

func TestLoaderEnvOverridesDefaultWhenFileHasNoValue(t *testing.T) {
	path := tempConfigPath(t)
	t.Setenv("COMRADE_LLM_PROVIDER", "google")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "google", cfg.LLM.Provider, "env must beat the built-in default")
}

func TestLoaderNamedEnvAliasesWork(t *testing.T) {
	cases := []struct {
		name    string
		envVar  string
		envVal  string
		extract func(*Config) string
	}{
		{"COMRADE_MODE", "COMRADE_MODE", "auto", func(c *Config) string { return c.General.Mode }},
		{"COMRADE_PROVIDER", "COMRADE_PROVIDER", "ollama", func(c *Config) string { return c.LLM.Provider }},
		{"COMRADE_MODEL", "COMRADE_MODEL", "llama3.1", func(c *Config) string { return c.LLM.Model }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := tempConfigPath(t)
			t.Setenv(tc.envVar, tc.envVal)

			loader, err := NewLoader(path)
			require.NoError(t, err)
			cfg, _, err := loader.Load()
			require.NoError(t, err)
			assert.Equal(t, tc.envVal, tc.extract(cfg))
		})
	}
}

func TestLoaderGetReturnsEffectiveValue(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	got, err := loader.Get("general.mode")
	require.NoError(t, err)
	assert.Equal(t, "ask", got)
}

func TestLoaderGetRejectsUnknownKey(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	_, err = loader.Get("general.bogus")
	assert.ErrorContains(t, err, "unknown config key")
}

func TestLoaderSetAndSavePersistsAcrossReload(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	value, err := Validate("general.mode", "auto")
	require.NoError(t, err)
	require.NoError(t, loader.SetAndSave("general.mode", value))

	reloaded, err := NewLoader(path)
	require.NoError(t, err)
	cfg, _, err := reloaded.Load()
	require.NoError(t, err)
	assert.Equal(t, "auto", cfg.General.Mode)
	// unrelated keys must still hold their defaults after the write-back.
	assert.Equal(t, "anthropic", cfg.LLM.Provider)
	assert.Equal(t, 2048, cfg.LLM.MaxTokens)
}

func TestLoaderSetAndSaveRejectsUnknownKey(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	err = loader.SetAndSave("general.bogus", "x")
	assert.ErrorContains(t, err, "unknown config key")
}

func TestLoaderSourceReportsDefaultThenFileThenEnv(t *testing.T) {
	path := tempConfigPath(t)
	// A hand-edited, partial file that never mentions general.mode: the
	// key's effective value must be reported as coming from the built-in
	// default, not the file (the file's presence-on-disk is not by
	// itself "the key is set in the file").
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("[llm]\nprovider = \"ollama\"\n"), 0o644))

	loader, err := NewLoader(path)
	require.NoError(t, err)

	src, err := loader.Source("general.mode")
	require.NoError(t, err)
	assert.Equal(t, SourceDefault, src)

	value, err := Validate("general.mode", "auto")
	require.NoError(t, err)
	require.NoError(t, loader.SetAndSave("general.mode", value))

	src, err = loader.Source("general.mode")
	require.NoError(t, err)
	assert.Equal(t, SourceFile, src)

	t.Setenv("COMRADE_GENERAL_MODE", "info")
	src, err = loader.Source("general.mode")
	require.NoError(t, err)
	assert.Equal(t, SourceEnv, src)
}

func TestLoaderSourceRejectsUnknownKey(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	_, err = loader.Source("general.bogus")
	assert.ErrorContains(t, err, "unknown config key")
}

// --- config profiles ---

func writeConfigFile(t *testing.T, path, toml string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(toml), 0o644))
}

func TestLoaderProfileOverlayAppliesOverTopLevelValue(t *testing.T) {
	path := tempConfigPath(t)
	writeConfigFile(t, path, "[general]\nprofile = \"work\"\n\n[llm]\nprovider = \"anthropic\"\n\n[profiles.work]\nllm.provider = \"openai_compat\"\nllm.model = \"gpt-4o\"\n")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "openai_compat", cfg.LLM.Provider, "the active profile's value must win over the file's own top-level value")
	assert.Equal(t, "gpt-4o", cfg.LLM.Model)
	assert.Equal(t, 60, cfg.LLM.TimeoutSeconds, "a key the profile doesn't touch must still fall through to its own default/file value")
}

func TestLoaderProfileOverlayLeavesConfigUntouchedWhenNoProfileActive(t *testing.T) {
	path := tempConfigPath(t)
	writeConfigFile(t, path, "[llm]\nprovider = \"anthropic\"\n\n[profiles.work]\nllm.provider = \"openai_compat\"\n")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "anthropic", cfg.LLM.Provider, "a defined-but-inactive profile must have zero effect")
}

func TestLoaderProfileFlagOverridesEnvAndFile(t *testing.T) {
	path := tempConfigPath(t)
	writeConfigFile(t, path, "[general]\nprofile = \"personal\"\n\n[profiles.personal]\nllm.provider = \"google\"\n\n[profiles.work]\nllm.provider = \"openai_compat\"\n")
	t.Setenv("COMRADE_PROFILE", "personal")

	loader, err := NewLoaderWithProfile(path, "work")
	require.NoError(t, err)

	cfg, _, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "openai_compat", cfg.LLM.Provider, "--profile flag must win over both COMRADE_PROFILE and the file's general.profile")
}

func TestLoaderProfileEnvOverridesFileValue(t *testing.T) {
	path := tempConfigPath(t)
	writeConfigFile(t, path, "[general]\nprofile = \"personal\"\n\n[profiles.personal]\nllm.provider = \"google\"\n\n[profiles.work]\nllm.provider = \"openai_compat\"\n")
	t.Setenv("COMRADE_PROFILE", "work")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "openai_compat", cfg.LLM.Provider, "COMRADE_PROFILE must win over the file's own general.profile")
}

// TestLoaderEnvStaysKingOverActiveProfile is the whole reason applyProfileOverlay
// merges via MergeConfigMap instead of viper.Set: a COMRADE_ environment
// variable for a key the active profile ALSO overrides must still win —
// env is the outermost, highest-precedence layer regardless of profiles.
func TestLoaderEnvStaysKingOverActiveProfile(t *testing.T) {
	path := tempConfigPath(t)
	writeConfigFile(t, path, "[general]\nprofile = \"work\"\n\n[profiles.work]\nllm.provider = \"openai_compat\"\n")
	t.Setenv("COMRADE_PROVIDER", "ollama")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()
	require.NoError(t, err)
	assert.Equal(t, "ollama", cfg.LLM.Provider, "COMRADE_PROVIDER must win over the active profile's own override")
}

func TestLoaderWarnsOnUndefinedActiveProfileButNeverFails(t *testing.T) {
	buf := captureProfileWarnings(t)
	path := tempConfigPath(t)
	writeConfigFile(t, path, "[general]\nprofile = \"ghost\"\n")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()
	require.NoError(t, err, "an undefined active profile must never fail Load()")
	assert.Equal(t, "ask", cfg.General.Mode, "config must otherwise load normally")
	assert.Contains(t, buf.String(), `"ghost"`)
	assert.Contains(t, buf.String(), "is not defined")
}

func TestLoaderWarnsOnUnknownKeyInsideProfileButNeverFails(t *testing.T) {
	buf := captureProfileWarnings(t)
	path := tempConfigPath(t)
	writeConfigFile(t, path, "[general]\nprofile = \"work\"\n\n[profiles.work]\nllm.provider = \"openai_compat\"\nllm.probider = \"typo\"\n")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	cfg, _, err := loader.Load()
	require.NoError(t, err, "an unknown key inside a defined profile must never fail Load()")
	assert.Equal(t, "openai_compat", cfg.LLM.Provider, "the recognized key must still apply")
	assert.Contains(t, buf.String(), `"work"`)
	assert.Contains(t, buf.String(), `"llm.probider"`)
	assert.Contains(t, buf.String(), "unknown key")
}

func TestLoaderSourceReportsProfileForKeyTheActiveProfileOverrides(t *testing.T) {
	path := tempConfigPath(t)
	writeConfigFile(t, path, "[general]\nprofile = \"work\"\n\n[llm]\nprovider = \"anthropic\"\n\n[profiles.work]\nllm.provider = \"openai_compat\"\n")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	src, err := loader.Source("llm.provider")
	require.NoError(t, err)
	assert.Equal(t, SourceProfile, src)

	// A key the profile does NOT touch still reports its own normal source.
	src, err = loader.Source("llm.timeout_seconds")
	require.NoError(t, err)
	assert.Equal(t, SourceDefault, src)
}

func TestLoaderSourceReportsEnvOverProfile(t *testing.T) {
	path := tempConfigPath(t)
	writeConfigFile(t, path, "[general]\nprofile = \"work\"\n\n[profiles.work]\nllm.provider = \"openai_compat\"\n")
	t.Setenv("COMRADE_PROVIDER", "ollama")

	loader, err := NewLoader(path)
	require.NoError(t, err)

	src, err := loader.Source("llm.provider")
	require.NoError(t, err)
	assert.Equal(t, SourceEnv, src)
}

// TestSetAndSavePreservesProfileTables is the spec-mandated regression
// proof: SetAndSave's full-map rewrite (its own WriteConfigAs call) must
// carry every existing [profiles.*] table through untouched when it sets
// an unrelated top-level key.
func TestSetAndSavePreservesProfileTables(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	require.NoError(t, loader.CreateProfile("work", map[string]any{"llm.provider": "openai_compat"}))

	value, err := Validate("general.mode", "auto")
	require.NoError(t, err)
	require.NoError(t, loader.SetAndSave("general.mode", value))

	reloaded, err := NewLoader(path)
	require.NoError(t, err)
	cfg, _, err := reloaded.Load()
	require.NoError(t, err)
	assert.Equal(t, "auto", cfg.General.Mode)
	require.Contains(t, cfg.Profiles, "work", "the profile table must survive SetAndSave's unrelated top-level write")
	llmSection, ok := cfg.Profiles["work"]["llm"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "openai_compat", llmSection["provider"])
}

func TestLoaderCreateProfileEmptyIsListedWithZeroKeysAndSurvivesReload(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	require.NoError(t, loader.CreateProfile("empty", nil))

	reloaded, err := NewLoader(path)
	require.NoError(t, err)
	cfg, _, err := reloaded.Load()
	require.NoError(t, err)
	require.Contains(t, cfg.Profiles, "empty")
	assert.Empty(t, ProfileKeys(cfg.Profiles["empty"]), "an empty profile must show zero real keys")
}

func TestLoaderCreateProfileRejectsDuplicateName(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	require.NoError(t, loader.CreateProfile("work", nil))
	err = loader.CreateProfile("work", nil)
	require.Error(t, err)
	var existsErr *ProfileExistsError
	assert.ErrorAs(t, err, &existsErr)
}

func TestLoaderCreateProfileRejectsInvalidName(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	err = loader.CreateProfile("Bad Name", nil)
	require.Error(t, err)
	var invalidName *InvalidProfileNameError
	assert.ErrorAs(t, err, &invalidName)
}

func TestLoaderSetProfileKeyPersistsAndRejectsUnknownProfile(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	require.NoError(t, loader.CreateProfile("work", nil))

	parsed, err := ValidateProfileKey("llm.provider", "openai_compat")
	require.NoError(t, err)
	require.NoError(t, loader.SetProfileKey("work", "llm.provider", parsed))

	reloaded, err := NewLoader(path)
	require.NoError(t, err)
	cfg, _, err := reloaded.Load()
	require.NoError(t, err)
	llmSection, ok := cfg.Profiles["work"]["llm"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "openai_compat", llmSection["provider"])

	err = loader.SetProfileKey("ghost", "llm.provider", "x")
	require.Error(t, err)
	var notFound *ProfileNotFoundError
	assert.ErrorAs(t, err, &notFound)
}

func TestLoaderRemoveProfileDeletesOnlyThatProfile(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	require.NoError(t, loader.CreateProfile("work", map[string]any{"llm.provider": "openai_compat"}))
	require.NoError(t, loader.CreateProfile("personal", map[string]any{"llm.provider": "google"}))

	require.NoError(t, loader.RemoveProfile("work"))

	reloaded, err := NewLoader(path)
	require.NoError(t, err)
	cfg, _, err := reloaded.Load()
	require.NoError(t, err)
	assert.NotContains(t, cfg.Profiles, "work")
	require.Contains(t, cfg.Profiles, "personal", "the sibling profile must survive removal of another")
	llmSection, ok := cfg.Profiles["personal"]["llm"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "google", llmSection["provider"])
}

func TestLoaderRemoveProfileClearsGeneralProfileWhenItPointedThere(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	require.NoError(t, loader.CreateProfile("work", map[string]any{"llm.provider": "openai_compat"}))
	require.NoError(t, loader.SetAndSave("general.profile", "work"))

	require.NoError(t, loader.RemoveProfile("work"))

	reloaded, err := NewLoader(path)
	require.NoError(t, err)
	cfg, _, err := reloaded.Load()
	require.NoError(t, err)
	assert.Equal(t, "", cfg.General.Profile, "general.profile must be cleared once the profile it pointed to is removed")
}

func TestLoaderRemoveProfileRejectsUnknownName(t *testing.T) {
	path := tempConfigPath(t)
	loader, err := NewLoader(path)
	require.NoError(t, err)

	err = loader.RemoveProfile("ghost")
	require.Error(t, err)
	var notFound *ProfileNotFoundError
	assert.ErrorAs(t, err, &notFound)
}

func TestNewLoaderUsesDefaultPathWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}

	want, err := DefaultPath()
	require.NoError(t, err)

	loader, err := NewLoader("")
	require.NoError(t, err)
	assert.Equal(t, want, loader.Path())
}
