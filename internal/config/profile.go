package config

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

// profilePlaceholderKey is the internal bookkeeping leaf key
// CreateProfile seeds a brand-new, otherwise-empty profile with. It
// exists purely because viper's WriteConfigAs silently drops a TOML
// table that has no leaf keys under it at all — a genuinely empty
// [profiles.<name>] table does not survive a single write/reload cycle
// (verified empirically against this project's pinned viper version).
// This key is filtered out of ProfileKeys/ProfileSafetyOverrides and
// skipped by applyProfileOverlay's unknown-key warning, so it never
// surfaces to a user: an "empty" profile created via `comrade config
// profile add <name>` (no --from-current) shows zero keys everywhere it
// is listed, while still actually persisting across reloads.
const profilePlaceholderKey = "_placeholder"

// profileNamePattern is the legal profile-name shape: lowercase letters,
// digits, hyphen, underscore, starting with a letter or digit, 1-32
// characters. Enforced lowercase up front because viper itself lowercases
// every key it reads — a profile name containing an uppercase letter
// could never be looked up back out of the merged config again.
var profileNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// InvalidProfileNameError is ValidateProfileName's structured error for a
// name that doesn't match profileNamePattern.
type InvalidProfileNameError struct {
	Name string
}

func (e *InvalidProfileNameError) Error() string {
	return fmt.Sprintf("invalid profile name %q: must start with a lowercase letter or digit and contain only lowercase letters, digits, - or _ (max 32 characters)", e.Name)
}

// ProfileNotFoundError is the structured error for a profile name that
// isn't defined in the [profiles] table — returned by Loader.SetProfileKey/
// RemoveProfile, and used by internal/cli's `profile show/use/set/remove`
// subcommands to report the same condition, translated.
type ProfileNotFoundError struct {
	Name string
}

func (e *ProfileNotFoundError) Error() string {
	return fmt.Sprintf("profile %q is not defined", e.Name)
}

// ProfileExistsError is Loader.CreateProfile's structured error when
// name already names a defined profile — `comrade config profile add`
// never silently overwrites an existing profile.
type ProfileExistsError struct {
	Name string
}

func (e *ProfileExistsError) Error() string {
	return fmt.Sprintf("profile %q already exists", e.Name)
}

// ValidateProfileName reports whether name is a legal profile name (see
// profileNamePattern's own doc comment for the exact shape and rationale).
func ValidateProfileName(name string) error {
	if !profileNamePattern.MatchString(name) {
		return &InvalidProfileNameError{Name: name}
	}
	return nil
}

// ResolveActiveProfile implements config profiles' active-profile
// precedence, mirroring engine.ResolveMode's exact shape
// (internal/engine/mode.go): an explicit --profile flag wins outright,
// then COMRADE_PROFILE, then the file's general.profile value. An empty
// string at any source is treated as "not set" and falls through to the
// next; empty for all three means no profile is active. Unlike
// ResolveMode, there is no fixed enum to validate against — any
// non-empty string is a syntactically legal candidate; whether it names
// an actually-DEFINED profile is a separate question applyProfileOverlay
// (Load-time) or the CLI's own existence checks answer.
func ResolveActiveProfile(flagValue, envValue, fileValue string) string {
	for _, candidate := range []string{flagValue, envValue, fileValue} {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

// flattenProfileKeys recursively walks a profile's own nested map (as
// produced by viper/go-toml for a [profiles.<name>] table: every
// intermediate TOML section is its own map[string]any) and returns every
// leaf's dotted key path relative to the profile's own root — e.g.
// {"llm": {"provider": "x"}} yields ["llm.provider"]. Unsorted; callers
// needing determinism (ProfileKeys) sort the result themselves.
func flattenProfileKeys(m map[string]any, prefix string) []string {
	var keys []string
	for k, v := range m {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		if nested, ok := v.(map[string]any); ok {
			keys = append(keys, flattenProfileKeys(nested, full)...)
			continue
		}
		keys = append(keys, full)
	}
	return keys
}

// ProfileKeys returns every real (non-bookkeeping) dotted key profile
// actually sets, sorted — used by `comrade config profile list`'s key
// count and `comrade config profile show`'s key/value listing.
// profilePlaceholderKey is always excluded: it is never a real config
// key a user asked for, only the internal marker CreateProfile uses to
// keep an otherwise-empty profile durable across a write/reload cycle.
func ProfileKeys(profile map[string]any) []string {
	flat := flattenProfileKeys(profile, "")
	out := make([]string, 0, len(flat))
	for _, k := range flat {
		if k == profilePlaceholderKey {
			continue
		}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ProfileSafetyOverrides returns the sorted subset of ProfileKeys(profile)
// that fall under the safety.* section — P-5's "a profile MAY override
// safety.* keys, but `profile use`/`show` must print a HIGHLIGHTED
// warning whenever a profile overrides any safety.* key" decision.
// internal/cli's profile use/show commands call this to decide whether
// to print that warning; the runtime destructive/elevated confirmation
// gate itself is untouched by this — the --yolo escape hatch is what
// actually makes bypassing it safe, this is purely a visibility warning.
func ProfileSafetyOverrides(profile map[string]any) []string {
	var out []string
	for _, key := range ProfileKeys(profile) {
		if strings.HasPrefix(key, "safety.") {
			out = append(out, key)
		}
	}
	return out
}

// profileHasKey reports whether profile (a profile's own nested map, as
// ProfileKeys/flattenProfileKeys walk) sets key (a top-level dotted path
// like "llm.provider"), by walking key's dot-separated segments through
// profile's own nesting. Used by Loader.Source to report SourceProfile.
func profileHasKey(profile map[string]any, key string) bool {
	parts := strings.Split(key, ".")
	cur := profile
	for i, part := range parts {
		v, ok := cur[part]
		if !ok {
			return false
		}
		if i == len(parts)-1 {
			return true
		}
		nested, ok := v.(map[string]any)
		if !ok {
			return false
		}
		cur = nested
	}
	return false
}

// profileWarningWriter is where applyProfileOverlay's non-fatal
// undefined-profile/unknown-key warnings are printed — a package-level
// var (mirroring baseURLWarningWriter's own established pattern) purely
// so tests can capture it in a bytes.Buffer instead of redirecting the
// real os.Stderr.
var profileWarningWriter io.Writer = os.Stderr

// emitProfileWarning prints msg to profileWarningWriter. Always plain,
// untranslated English — exactly like validateLoadedConfig's own
// base_url warnings, which this package has always rendered
// untranslated (no i18n.Translator is in scope this deep in the config
// package); see validateLoadedConfig's doc comment for the same
// never-brick-Load() rationale this mirrors.
func emitProfileWarning(msg string) {
	fmt.Fprintln(profileWarningWriter, msg) //nolint:errcheck // best-effort stderr warning; a write failure here has no recovery action
}

// applyProfileOverlay merges the active profile named name's own table
// on top of the defaults+file layer already merged into v (v is
// expected to be mid-construction inside newEffectiveViper, BEFORE env
// binding — see that function's own doc comment for why env must always
// still win on top of this). Never fails: an undefined active profile,
// or a key inside a defined profile that isn't in keyDefs at all, is
// WARNED to stderr and otherwise ignored — Load() must never brick on a
// bad profile value any more than it may on a bad base_url (see
// validateLoadedConfig).
func applyProfileOverlay(v *viper.Viper, name string) {
	raw, ok := v.Get("profiles").(map[string]any)
	if !ok {
		emitProfileWarning(fmt.Sprintf("warning: active profile %q is not defined; ignoring", name))
		return
	}
	profile, ok := raw[name].(map[string]any)
	if !ok {
		emitProfileWarning(fmt.Sprintf("warning: active profile %q is not defined; ignoring", name))
		return
	}

	for _, key := range ProfileKeys(profile) {
		if !IsValidKey(key) {
			emitProfileWarning(fmt.Sprintf("warning: profile %q has unknown key %q; ignoring", name, key))
		}
	}

	_ = v.MergeConfigMap(profile)
}
