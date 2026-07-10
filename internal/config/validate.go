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
	// KindNonNegativeInt is an integer that must be >= 0, for a key where
	// 0 is itself a meaningful value (e.g. "disabled") rather than an
	// invalid one — the only difference from KindPositiveInt.
	KindNonNegativeInt
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
	{Key: "llm.idle_timeout_seconds", Kind: KindNonNegativeInt},
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

// UnknownKeyError is Validate/Loader.Get/Loader.Source/Loader.Set's
// structured error for an unrecognized config key. Its own Error() text
// is exactly the plain-English sentence this package has always
// returned (so any pre-existing caller/test asserting on that text is
// unaffected); internal/cli's config.go additionally uses errors.As to
// pull Key/ValidKeys back out and re-render the SAME information through
// an i18n.Translator, instead of parsing this string — a QA-found gap
// (`comrade config set`'s validation errors bypassed i18n entirely,
// unlike every other user-facing message in this tree).
type UnknownKeyError struct {
	Key       string
	ValidKeys []string
}

func (e *UnknownKeyError) Error() string {
	return fmt.Sprintf("unknown config key %q; valid keys are: %s", e.Key, strings.Join(e.ValidKeys, ", "))
}

// unknownKeyError builds the "helpful message listing valid keys" required
// for a rejected `comrade config set`/`get` on an unrecognized key.
func unknownKeyError(key string) error {
	return &UnknownKeyError{Key: key, ValidKeys: Keys()}
}

// InvalidValueReason classifies why Validate rejected a value that DID
// name a known key — internal/cli's config.go switches on this (via
// errors.As on *InvalidValueError) to pick the matching i18n message,
// rather than parsing English error text.
type InvalidValueReason int

const (
	// ReasonInvalidEnum means raw isn't one of Key's Enum values.
	ReasonInvalidEnum InvalidValueReason = iota
	// ReasonNotBoolean means raw doesn't parse as a bool (KindBool).
	ReasonNotBoolean
	// ReasonNotInteger means raw doesn't parse as an integer at all
	// (KindPositiveInt/KindNonNegativeInt).
	ReasonNotInteger
	// ReasonNotPositive means raw parsed as an integer but is <= 0
	// (KindPositiveInt).
	ReasonNotPositive
	// ReasonNotNonNegative means raw parsed as an integer but is < 0
	// (KindNonNegativeInt).
	ReasonNotNonNegative
)

// InvalidValueError is Validate's structured error for every rejection
// that follows a successful key lookup (Key is always valid; UnknownKeyError
// is the separate, earlier failure mode). Error() renders the exact same
// plain-English sentence this package always has, MINUS the raw wrapped
// strconv error `fmt.Errorf`'s old `: %w` tail used to append (e.g.
// `strconv.ParseBool: parsing "x": invalid syntax`) — that internal
// detail is dropped from the rendered text (Key/Raw/Reason already say
// everything a user needs), matching this task's "never surface raw
// internal/library detail in a user-facing message" rule; the original
// strconv error is still reachable via Unwrap() for any caller that wants
// it.
type InvalidValueError struct {
	Key    string
	Raw    string
	Reason InvalidValueReason
	Enum   []string // populated only for ReasonInvalidEnum
	err    error    // wrapped strconv.ParseBool/Atoi error, if any; see Unwrap
}

func (e *InvalidValueError) Error() string {
	switch e.Reason {
	case ReasonInvalidEnum:
		return fmt.Sprintf("invalid value %q for %s; must be one of: %s", e.Raw, e.Key, strings.Join(e.Enum, ", "))
	case ReasonNotBoolean:
		return fmt.Sprintf("invalid value %q for %s: must be a boolean (true/false)", e.Raw, e.Key)
	case ReasonNotInteger:
		return fmt.Sprintf("invalid value %q for %s: must be an integer", e.Raw, e.Key)
	case ReasonNotPositive:
		return fmt.Sprintf("invalid value %q for %s: must be greater than 0", e.Raw, e.Key)
	case ReasonNotNonNegative:
		return fmt.Sprintf("invalid value %q for %s: must be 0 or greater", e.Raw, e.Key)
	default:
		return fmt.Sprintf("invalid value %q for %s", e.Raw, e.Key)
	}
}

// Unwrap exposes the wrapped strconv.ParseBool/Atoi error (nil for
// ReasonInvalidEnum/ReasonNotPositive/ReasonNotNonNegative, which never
// wrap one), so errors.Is/As still reaches it if a caller needs to.
func (e *InvalidValueError) Unwrap() error { return e.err }

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
			return nil, &InvalidValueError{Key: key, Raw: raw, Reason: ReasonInvalidEnum, Enum: kd.Enum}
		}
		return raw, nil

	case KindBool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, &InvalidValueError{Key: key, Raw: raw, Reason: ReasonNotBoolean, err: err}
		}
		return b, nil

	case KindPositiveInt:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, &InvalidValueError{Key: key, Raw: raw, Reason: ReasonNotInteger, err: err}
		}
		if n <= 0 {
			return nil, &InvalidValueError{Key: key, Raw: raw, Reason: ReasonNotPositive}
		}
		return n, nil

	case KindNonNegativeInt:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, &InvalidValueError{Key: key, Raw: raw, Reason: ReasonNotInteger, err: err}
		}
		if n < 0 {
			return nil, &InvalidValueError{Key: key, Raw: raw, Reason: ReasonNotNonNegative}
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
