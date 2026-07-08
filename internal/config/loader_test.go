package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
