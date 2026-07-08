package config

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultMatchesPlanExactly pins every default value from
// UYGULAMA_PLANI.md's FAZ 1 schema block. If defaultConfigTOML drifts from
// the plan, this is the test that must catch it.
func TestDefaultMatchesPlanExactly(t *testing.T) {
	cfg := Default()

	assert.Equal(t, "ask", cfg.General.Mode)
	assert.Equal(t, "auto", cfg.General.Language)
	assert.Equal(t, true, cfg.General.Color)

	assert.Equal(t, "anthropic", cfg.LLM.Provider)
	assert.Equal(t, "", cfg.LLM.Model)
	assert.Equal(t, []string{}, cfg.LLM.Fallback)
	assert.Equal(t, 60, cfg.LLM.TimeoutSeconds)
	assert.Equal(t, 2048, cfg.LLM.MaxTokens)
	assert.Equal(t, "https://api.openai.com/v1", cfg.LLM.OpenAICompat.BaseURL)
	assert.Equal(t, "http://localhost:11434", cfg.LLM.Ollama.BaseURL)

	assert.Equal(t, true, cfg.Safety.ConfirmDestructive)
	assert.Equal(t, true, cfg.Safety.ConfirmElevated)
	assert.Equal(t, []string{}, cfg.Safety.DenylistExtra)
	assert.Equal(t, 10, cfg.Safety.MaxAutoSteps)

	assert.Equal(t, false, cfg.Context.SendHistory)
	assert.Equal(t, 5, cfg.Context.HistoryDepth)
	assert.Equal(t, false, cfg.Context.SendEnvNames)

	assert.Equal(t, false, cfg.Privacy.RedactEmails)
	assert.Equal(t, false, cfg.Privacy.RedactIPs)
	assert.Equal(t, false, cfg.Privacy.Telemetry)

	assert.Equal(t, true, cfg.Audit.Enabled)
	assert.Equal(t, 90, cfg.Audit.RetentionDays)
}

// TestDefaultDoesNotPanic guards the Default() panic paths: if
// defaultConfigTOML is ever edited into something that fails to parse or
// no longer matches the Config struct, this test turns that programmer
// error into a normal failing test instead of a runtime panic discovered
// later.
func TestDefaultDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() { Default() })
}

// flattenStructTags walks t (expected to be a struct type, possibly
// containing nested structs) and returns every leaf field's dotted
// mapstructure-tag path.
func flattenStructTags(rt reflect.Type, prefix string) []string {
	var keys []string
	for i := 0; i < rt.NumField(); i++ {
		f := rt.Field(i)
		tag := f.Tag.Get("mapstructure")
		if tag == "" {
			tag = strings.ToLower(f.Name)
		}
		full := tag
		if prefix != "" {
			full = prefix + "." + tag
		}
		if f.Type.Kind() == reflect.Struct {
			keys = append(keys, flattenStructTags(f.Type, full)...)
			continue
		}
		keys = append(keys, full)
	}
	return keys
}

// flattenSettings walks a nested map[string]any (as produced by
// viper.AllSettings) and returns every leaf key's dotted path.
func flattenSettings(m map[string]any, prefix string) []string {
	var keys []string
	for k, v := range m {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		if nested, ok := v.(map[string]any); ok {
			keys = append(keys, flattenSettings(nested, full)...)
			continue
		}
		keys = append(keys, full)
	}
	return keys
}

// TestKeyDefsMatchConfigStruct is a bidirectional drift guard between the
// Config struct (schema.go) and the keyDefs validation registry
// (validate.go): a field added to one without the other fails this test,
// per the project's "no unguarded hand-maintained mirrors" rule.
func TestKeyDefsMatchConfigStruct(t *testing.T) {
	structKeys := flattenStructTags(reflect.TypeOf(Config{}), "")
	sort.Strings(structKeys)

	assert.Equal(t, structKeys, Keys())
}

// TestKeyDefsMatchDefaultConfigTOML is a bidirectional drift guard between
// defaultConfigTOML (the on-disk source of truth for defaults) and the
// keyDefs validation registry: every key written to a fresh config file
// must be settable via `comrade config set`, and vice versa.
func TestKeyDefsMatchDefaultConfigTOML(t *testing.T) {
	v := viper.New()
	v.SetConfigType("toml")
	require.NoError(t, v.ReadConfig(strings.NewReader(defaultConfigTOML)))

	tomlKeys := flattenSettings(v.AllSettings(), "")
	sort.Strings(tomlKeys)

	assert.Equal(t, tomlKeys, Keys())
}
