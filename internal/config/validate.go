package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Kind identifies the Go type a config key's value must parse into.
type Kind int

const (
	// KindString is a plain string value, optionally constrained to Enum.
	KindString Kind = iota
	// KindBool is a boolean value ("true"/"false"/"1"/"0"/...).
	KindBool
	// KindPositiveInt is an integer that must be strictly greater than 0.
	KindPositiveInt
	// KindStringSlice is a comma-separated list of strings.
	KindStringSlice
)

// KeyDef describes one settable config key: its dotted path (matching the
// TOML/mapstructure layout in schema.go), its value kind, and — for
// KindString keys with a closed set of legal values — the allowed values.
//
// keyDefs below is the single registry `comrade config set` validates
// against. It is checked bidirectionally against the Config struct's own
// field tags by TestKeyDefsMatchConfigSchema in validate_test.go: that
// test fails if a Config field has no KeyDef, or a KeyDef names a field
// that doesn't exist, so the two cannot silently drift apart.
type KeyDef struct {
	Key  string
	Kind Kind
	Enum []string // non-nil only for enum-constrained KindString keys
}

var keyDefs = []KeyDef{
	{Key: "general.mode", Kind: KindString, Enum: []string{"auto", "ask", "info"}},
	{Key: "general.language", Kind: KindString, Enum: []string{"auto", "tr", "en"}},
	{Key: "general.color", Kind: KindBool},
	{Key: "general.update_check", Kind: KindBool},

	{Key: "llm.provider", Kind: KindString, Enum: []string{"anthropic", "openai_compat", "google", "ollama"}},
	{Key: "llm.model", Kind: KindString},
	{Key: "llm.fallback", Kind: KindStringSlice},
	{Key: "llm.timeout_seconds", Kind: KindPositiveInt},
	{Key: "llm.max_tokens", Kind: KindPositiveInt},
	{Key: "llm.openai_compat.base_url", Kind: KindString},
	{Key: "llm.ollama.base_url", Kind: KindString},

	{Key: "safety.confirm_destructive", Kind: KindBool},
	{Key: "safety.confirm_elevated", Kind: KindBool},
	{Key: "safety.denylist_extra", Kind: KindStringSlice},
	{Key: "safety.max_auto_steps", Kind: KindPositiveInt},

	{Key: "context.send_history", Kind: KindBool},
	{Key: "context.history_depth", Kind: KindPositiveInt},
	{Key: "context.send_env_names", Kind: KindBool},

	{Key: "privacy.redact_emails", Kind: KindBool},
	{Key: "privacy.redact_ips", Kind: KindBool},
	{Key: "privacy.telemetry", Kind: KindBool},

	{Key: "audit.enabled", Kind: KindBool},
	{Key: "audit.retention_days", Kind: KindPositiveInt},

	{Key: "executor.step_timeout_seconds", Kind: KindPositiveInt},
}

// ProviderNames returns the fixed set of valid llm.provider values, in the
// same order keyDefs declares them — the same enum `comrade config set
// llm.provider` validates a new value against. It is exported so other
// packages (e.g. internal/secrets, which needs "every provider minus
// ollama" to know which providers can hold a stored credential) derive
// this list from keyDefs instead of hand-copying the enum, per this
// project's derive-or-guard rule: TestProviderNamesMatchesLLMProviderEnum
// in validate_test.go pins the exact returned slice, so keyDefs'
// llm.provider entry cannot drift from this function silently.
func ProviderNames() []string {
	kd, ok := lookup("llm.provider")
	if !ok {
		return nil
	}
	out := make([]string, len(kd.Enum))
	copy(out, kd.Enum)
	return out
}

// Keys returns every settable config key, sorted.
func Keys() []string {
	keys := make([]string, 0, len(keyDefs))
	for _, kd := range keyDefs {
		keys = append(keys, kd.Key)
	}
	sort.Strings(keys)
	return keys
}

// lookup finds the KeyDef for key, if any.
func lookup(key string) (KeyDef, bool) {
	for _, kd := range keyDefs {
		if kd.Key == key {
			return kd, true
		}
	}
	return KeyDef{}, false
}

// IsValidKey reports whether key is a known, settable config key.
func IsValidKey(key string) bool {
	_, ok := lookup(key)
	return ok
}

// unknownKeyError builds the "helpful message listing valid keys" required
// for a rejected `comrade config set`/`get` on an unrecognized key.
func unknownKeyError(key string) error {
	return fmt.Errorf("unknown config key %q; valid keys are: %s", key, strings.Join(Keys(), ", "))
}

// Validate parses raw (as given on the command line) into the Go value
// appropriate for key's Kind, applying enum/positivity checks. It returns
// an unknownKeyError if key isn't in the registry at all.
func Validate(key, raw string) (any, error) {
	kd, ok := lookup(key)
	if !ok {
		return nil, unknownKeyError(key)
	}

	switch kd.Kind {
	case KindString:
		if kd.Enum != nil && !contains(kd.Enum, raw) {
			return nil, fmt.Errorf("invalid value %q for %s; must be one of: %s", raw, key, strings.Join(kd.Enum, ", "))
		}
		return raw, nil

	case KindBool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q for %s: must be a boolean (true/false): %w", raw, key, err)
		}
		return b, nil

	case KindPositiveInt:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid value %q for %s: must be an integer: %w", raw, key, err)
		}
		if n <= 0 {
			return nil, fmt.Errorf("invalid value %q for %s: must be greater than 0", raw, key)
		}
		return n, nil

	case KindStringSlice:
		return parseStringSlice(raw), nil

	default:
		return nil, fmt.Errorf("config: key %s has unhandled kind %v", key, kd.Kind)
	}
}

// parseStringSlice splits a comma-separated CLI value into a trimmed,
// non-empty-entry string slice. An empty (or whitespace-only) raw value
// yields an empty slice rather than a slice containing one empty string.
func parseStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
