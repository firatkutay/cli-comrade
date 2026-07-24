package config

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
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
	// KindBaseURL is an http(s) endpoint URL for a provider connector
	// (llm.openai_compat.base_url, llm.ollama.base_url). See checkBaseURL
	// for the exact hard-error / warning rules — this fixes SAST finding
	// #3: an unvalidated base_url sends the provider API key, as an
	// Authorization: Bearer header, to whatever host it names.
	KindBaseURL
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
	{Key: "general.show_usage", Kind: KindBool},
	// general.profile: no Enum — the set of legal values is dynamic (any
	// currently-defined [profiles.<name>] table name, or "" for none),
	// which keyDefs' static Enum mechanism cannot express. `comrade
	// config profile use <name>` is the fully-validated path (name-format
	// regex + existence check — see ValidateProfileName/profile.go); a
	// raw `comrade config set general.profile <name>` accepts any string
	// syntactically, same as llm.model, and an undefined/bogus value is
	// instead caught (WARN, never fail) at load time by
	// applyProfileOverlay.
	{Key: "general.profile", Kind: KindString},
	{Key: "general.plan_review", Kind: KindString, Enum: []string{"off", "ask"}},

	{Key: "llm.provider", Kind: KindString, Enum: []string{"anthropic", "openai_compat", "google", "ollama"}},
	{Key: "llm.model", Kind: KindString},
	{Key: "llm.fallback", Kind: KindStringSlice},
	{Key: "llm.timeout_seconds", Kind: KindPositiveInt},
	{Key: "llm.idle_timeout_seconds", Kind: KindNonNegativeInt},
	{Key: "llm.max_tokens", Kind: KindPositiveInt},
	{Key: "llm.openai_compat.base_url", Kind: KindBaseURL},
	{Key: "llm.ollama.base_url", Kind: KindBaseURL},

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
	// ReasonNotURL means raw doesn't parse as a URL, or its scheme isn't
	// http/https, or it has no host (KindBaseURL).
	ReasonNotURL
	// ReasonMetadataOrLinkLocal means raw parses fine as an http(s) URL
	// but names a cloud-metadata / link-local host — 169.254.0.0/16
	// (which covers the 169.254.169.254 metadata endpoint used by AWS/GCP/
	// Azure) or the IPv6 link-local range fe80::/10 (KindBaseURL).
	ReasonMetadataOrLinkLocal
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
	case ReasonNotURL:
		return fmt.Sprintf("invalid value %q for %s: must be a valid http:// or https:// URL with a host", e.Raw, e.Key)
	case ReasonMetadataOrLinkLocal:
		return fmt.Sprintf("invalid value %q for %s: cloud metadata / link-local address not allowed", e.Raw, e.Key)
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

	case KindBaseURL:
		warning, err := checkBaseURL(key, raw)
		if err != nil {
			return nil, err
		}
		emitBaseURLWarning(warning)
		return raw, nil

	default:
		return nil, fmt.Errorf("config: key %s has unhandled kind %v", key, kd.Kind)
	}
}

// baseURLWarningWriter is where checkBaseURL's non-fatal cleartext-
// credential warning is printed. A package-level var (not a hardcoded
// os.Stderr call) purely so tests can capture it in a bytes.Buffer instead
// of spawning a subprocess or redirecting the real os.Stderr.
var baseURLWarningWriter io.Writer = os.Stderr

// checkBaseURL enforces this package's SAST-finding-#3 fix for
// llm.openai_compat.base_url / llm.ollama.base_url — an unvalidated
// base_url sends the provider API key, as an Authorization: Bearer header,
// to whatever host it names, in the clear if that host is reached over
// plain http://. It is the single source of truth for both `comrade config
// set` (via Validate, above) and config-load-time re-validation (via
// validateLoadedConfig in loader.go), so the two paths cannot drift apart.
//
// Hard-rejected (returns a non-nil *InvalidValueError, warning == ""):
//   - raw fails to parse as a URL, or its scheme isn't http/https, or it
//     has no host (ReasonNotURL) — e.g. "file://...", "ftp://x",
//     "not-a-url", "https://" with no host.
//   - raw's host is a literal cloud-metadata / link-local address:
//     169.254.0.0/16 (which covers the 169.254.169.254 metadata endpoint
//     used by AWS/GCP/Azure) or the IPv6 link-local range fe80::/10
//     (ReasonMetadataOrLinkLocal). net.IP.IsLinkLocalUnicast reports
//     exactly this pair of ranges, so it is used directly rather than
//     hand-rolling the two CIDRs.
//
// Warned-but-ALLOWED (returns "", a non-empty warning string, nil error):
//   - scheme is http and the host is not loopback (the literal
//     "localhost", or an IP in net.IP.IsLoopback's range: 127.0.0.0/8,
//     ::1). This is deliberate: a hard https requirement would break
//     legitimate self-hosted LAN Ollama/LM-Studio setups, so only the
//     cleartext-credential risk is warned about. General private ranges
//     (10/8, 192.168/16, 172.16/12) are legitimate for self-hosted LLMs
//     and fall into this same warned-but-allowed case, never rejected.
func checkBaseURL(key, raw string) (warning string, err error) {
	u, parseErr := url.Parse(raw)
	if parseErr != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", &InvalidValueError{Key: key, Raw: raw, Reason: ReasonNotURL}
	}

	host := u.Hostname()
	if ip := net.ParseIP(host); ip != nil && ip.IsLinkLocalUnicast() {
		return "", &InvalidValueError{Key: key, Raw: raw, Reason: ReasonMetadataOrLinkLocal}
	}

	if u.Scheme == "http" && !isLoopbackHost(host) {
		return fmt.Sprintf("warning: %s is set to an http:// URL (%s); the API key will be sent unencrypted over the network to this host", key, u.Host), nil
	}
	return "", nil
}

// CheckBaseURL is checkBaseURL exported for internal/llm's point-of-use
// enforcement of this same SAST-finding-#3 rule: when llm.New actually
// constructs the openai_compat/ollama connector for an attempt in the
// fallback chain (internal/llm/client.go's buildProvider), it calls this
// with that attempt's base_url and returns the rejection as a hard error —
// the API key must never be handed to a connector pointed at a dangerous
// host, even though config-load time (validateLoadedConfig, loader.go) now
// only warns about the very same value, never fails. See checkBaseURL's own
// doc comment above for the full reject/warn classification shared by all
// three enforcement points (`comrade config set`, Load(), and client
// construction).
func CheckBaseURL(key, raw string) (warning string, err error) {
	return checkBaseURL(key, raw)
}

// isLoopbackHost reports whether host (already extracted via
// url.URL.Hostname(), so any brackets/port are stripped) is a loopback
// address: the literal "localhost", or an IP within net.IP.IsLoopback's
// range (127.0.0.0/8, ::1).
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// emitBaseURLWarning prints warning to baseURLWarningWriter, doing nothing
// when warning is empty (the common, no-warning-applies case).
func emitBaseURLWarning(warning string) {
	if warning == "" {
		return
	}
	fmt.Fprintln(baseURLWarningWriter, warning) //nolint:errcheck // best-effort stderr warning; a write failure here has no recovery action
}

// EmitBaseURLWarning is emitBaseURLWarning exported for internal/cli's
// `comrade auth login openai_compat` base_url prompt (auth.go's
// promptOpenAICompatBaseURL): the SAME warned-but-allowed cleartext-http
// notice `comrade config set` prints via CheckBaseURL's own warning
// return value must also surface when a user types a warn-class (http://
// to a non-loopback host) endpoint into that interactive prompt — a
// credential-entry point, exactly the case this warning exists for.
// Calling this instead of writing a second, near-duplicate message keeps
// the wording and destination (baseURLWarningWriter) identical between
// both entry points.
func EmitBaseURLWarning(warning string) {
	emitBaseURLWarning(warning)
}

// validateLoadedConfig re-runs checkBaseURL against the ACTIVE provider's
// (cfg.LLM.Provider) base_url only, from the fully-resolved config
// (built-in defaults merged with the on-disk file merged with COMRADE_
// environment variables) that Loader.Load just produced — WARN-ONLY, never
// an error, and this function has no return value for exactly that reason.
//
// This is deliberately weaker than an earlier version of this function,
// which hard-failed Load() for a bad base_url reaching the file some other
// way than `comrade config set` (a hand-edited TOML file, or a
// COMRADE_LLM_OPENAI_COMPAT_BASE_URL / COMRADE_LLM_OLLAMA_BASE_URL
// environment variable) — closing SAST finding #3's gap, but at the cost of
// a second, worse bug: Load() runs at the top of EVERY command, including
// `comrade config set`/`comrade config edit`/`comrade config get`, so a
// single bad value bricked the entire tool with no in-tool way back in —
// not even the repair commands worked. Load() must never fail because of a
// base_url value; a HARD reject for the value that actually matters (the
// one about to receive the API key) now happens instead at the point an
// LLM client is actually built for the active provider — see
// internal/llm/client.go's buildProvider, which calls the same CheckBaseURL
// this function uses. That path is safe to hard-fail: `comrade config set`
// never builds an LLM client, so it stays reachable to fix the value.
//
// Scoped to cfg.LLM.Provider only (not both base_url keys, unconditionally,
// like the pre-fix version did) so an unused provider's bad/edited
// base_url — one you are not even sending your API key to — stays
// completely silent instead of printing per-invocation warning noise about
// a provider nobody is using.
func validateLoadedConfig(cfg *Config) {
	var key, value string
	switch cfg.LLM.Provider {
	case "openai_compat":
		key, value = "llm.openai_compat.base_url", cfg.LLM.OpenAICompat.BaseURL
	case "ollama":
		key, value = "llm.ollama.base_url", cfg.LLM.Ollama.BaseURL
	default:
		// anthropic/google (or an already-invalid llm.provider value —
		// llm.provider's own enum validity is a separate concern this
		// function doesn't police) never read a base_url at all.
		return
	}

	warning, err := checkBaseURL(key, value)
	if err != nil {
		// What would be a hard reject at `comrade config set`/client-build
		// time is downgraded to the same warning treatment here — see this
		// function's own doc comment for why Load() itself must never fail.
		emitBaseURLWarning("warning: " + err.Error())
		return
	}
	emitBaseURLWarning(warning)
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

// ProfileKeyNotAllowedError is ValidateProfileKey's structured error for
// the one keyDefs key explicitly disallowed inside a [profiles.<name>]
// table: general.profile itself. Allowing it would let one profile
// activate another (unbounded recursion, and a confusing "which profile
// is actually active" question with no good answer) — every OTHER known
// key is allowed verbatim inside a profile, with Validate's usual enum/
// int/base_url rules still applying identically.
type ProfileKeyNotAllowedError struct {
	Key string
}

func (e *ProfileKeyNotAllowedError) Error() string {
	return fmt.Sprintf("config key %q cannot be set inside a profile (it selects the active profile itself)", e.Key)
}

// ValidateProfileKey validates key/raw for use inside a [profiles.<name>]
// table: general.profile is rejected outright (see
// ProfileKeyNotAllowedError); every other keyDefs key is validated by
// calling Validate wholesale, so the exact same enum/bool/int/base_url
// rules a top-level `comrade config set` enforces apply identically to
// the same key set inside a profile — no separate, potentially-drifting
// validation logic for the profile case.
func ValidateProfileKey(key, raw string) (any, error) {
	if key == "general.profile" {
		return nil, &ProfileKeyNotAllowedError{Key: key}
	}
	return Validate(key, raw)
}
