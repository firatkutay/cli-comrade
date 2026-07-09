package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// defaultConfigTOML is the single source of truth for every config key,
// its default value, and the on-disk layout written on first run. It is
// reproduced verbatim from UYGULAMA_PLANI.md's FAZ 1 schema block.
//
// Default() and the runtime Loader both derive from this constant instead
// of duplicating the same defaults as Go literals, so the schema cannot
// drift between "what we write to disk" and "what Get() returns for a
// missing key" — see docs/phases/FAZ-01.md.
const defaultConfigTOML = `[general]
mode = "ask"              # auto | ask | info
language = "auto"         # auto | tr | en
color = true

[llm]
provider = "anthropic"    # anthropic | openai_compat | google | ollama
model = ""                # empty means the provider's own default
fallback = []              # e.g. ["ollama/llama3.1", "openai_compat/gpt-4o-mini"]
timeout_seconds = 60
max_tokens = 2048

[llm.openai_compat]
base_url = "https://api.openai.com/v1"

[llm.ollama]
base_url = "http://localhost:11434"

[safety]
confirm_destructive = true   # required even in auto mode
confirm_elevated = true      # required for sudo/admin commands
denylist_extra = []           # user-supplied extra denylist regexes
max_auto_steps = 10           # max steps per request in auto mode

[context]
send_history = false
history_depth = 5
send_env_names = false

[privacy]
redact_emails = false
redact_ips = false
telemetry = false

[audit]
enabled = true
retention_days = 90

[executor]
step_timeout_seconds = 300   # max seconds a single executed step may run before being killed
`

// GeneralConfig holds the [general] section.
type GeneralConfig struct {
	Mode     string `mapstructure:"mode"`
	Language string `mapstructure:"language"`
	Color    bool   `mapstructure:"color"`
}

// OpenAICompatConfig holds the [llm.openai_compat] section.
type OpenAICompatConfig struct {
	BaseURL string `mapstructure:"base_url"`
}

// OllamaConfig holds the [llm.ollama] section.
type OllamaConfig struct {
	BaseURL string `mapstructure:"base_url"`
}

// LLMConfig holds the [llm] section.
type LLMConfig struct {
	Provider       string             `mapstructure:"provider"`
	Model          string             `mapstructure:"model"`
	Fallback       []string           `mapstructure:"fallback"`
	TimeoutSeconds int                `mapstructure:"timeout_seconds"`
	MaxTokens      int                `mapstructure:"max_tokens"`
	OpenAICompat   OpenAICompatConfig `mapstructure:"openai_compat"`
	Ollama         OllamaConfig       `mapstructure:"ollama"`
}

// SafetyConfig holds the [safety] section.
type SafetyConfig struct {
	ConfirmDestructive bool     `mapstructure:"confirm_destructive"`
	ConfirmElevated    bool     `mapstructure:"confirm_elevated"`
	DenylistExtra      []string `mapstructure:"denylist_extra"`
	MaxAutoSteps       int      `mapstructure:"max_auto_steps"`
}

// ContextConfig holds the [context] section.
type ContextConfig struct {
	SendHistory  bool `mapstructure:"send_history"`
	HistoryDepth int  `mapstructure:"history_depth"`
	SendEnvNames bool `mapstructure:"send_env_names"`
}

// PrivacyConfig holds the [privacy] section.
type PrivacyConfig struct {
	RedactEmails bool `mapstructure:"redact_emails"`
	RedactIPs    bool `mapstructure:"redact_ips"`
	Telemetry    bool `mapstructure:"telemetry"`
}

// AuditConfig holds the [audit] section.
type AuditConfig struct {
	Enabled       bool `mapstructure:"enabled"`
	RetentionDays int  `mapstructure:"retention_days"`
}

// ExecutorConfig holds the [executor] section — introduced in FAZ 6 for
// internal/executor's per-step timeout (UYGULAMA_PLANI.md FAZ 6 item 1:
// "timeout (adım başına config'den)").
type ExecutorConfig struct {
	StepTimeoutSeconds int `mapstructure:"step_timeout_seconds"`
}

// Config is the full, in-memory configuration schema for cli-comrade.
type Config struct {
	General  GeneralConfig  `mapstructure:"general"`
	LLM      LLMConfig      `mapstructure:"llm"`
	Safety   SafetyConfig   `mapstructure:"safety"`
	Context  ContextConfig  `mapstructure:"context"`
	Privacy  PrivacyConfig  `mapstructure:"privacy"`
	Audit    AuditConfig    `mapstructure:"audit"`
	Executor ExecutorConfig `mapstructure:"executor"`
}

// Default returns the schema's default configuration, parsed from
// defaultConfigTOML. It panics if defaultConfigTOML does not parse or does
// not match the Config struct — both are programmer errors caught
// immediately by TestDefaultParsesWithoutError / TestDefaultTOMLKeysMatchSchema
// in schema_test.go, never something a user-supplied file could trigger.
func Default() Config {
	v := viper.New()
	v.SetConfigType("toml")
	if err := v.ReadConfig(strings.NewReader(defaultConfigTOML)); err != nil {
		panic(fmt.Sprintf("config: defaultConfigTOML failed to parse: %v", err))
	}
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		panic(fmt.Sprintf("config: defaultConfigTOML does not match Config schema: %v", err))
	}
	return cfg
}
