package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Source identifies where a config key's effective value came from.
type Source string

const (
	// SourceDefault means the key was not present in the file or the
	// environment; the value came from defaultConfigTOML.
	SourceDefault Source = "default"
	// SourceFile means the key was set explicitly in the config file.
	SourceFile Source = "file"
	// SourceEnv means a COMRADE_ environment variable overrode the key.
	SourceEnv Source = "env"
	// SourceProfile means the key's effective value came from the active
	// profile's own [profiles.<name>] table (config profiles' one new
	// overlay layer, between the file's own top-level value and env —
	// see newEffectiveViper's doc comment for the full precedence order).
	SourceProfile Source = "profile"
	// SourceFlag means the key's effective value came from a persistent
	// CLI flag override threaded into the Loader — today that is only
	// ever general.profile via the --profile flag (NewLoaderWithProfile's
	// profileOverride); no other key has a flag wired this deep. See
	// Loader.profileSource, general.profile's own dedicated Source path.
	SourceFlag Source = "flag"
)

// envAliases lists the explicit, named environment variable aliases
// required by docs/history/UYGULAMA_PLANI.md's FAZ 1 in addition to the generic
// COMRADE_<SECTION>_<KEY> mapping every key already gets from
// viper.AutomaticEnv(). Both forms work for these three keys; see
// bindEnvAliases and Loader.Source.
var envAliases = map[string][]string{
	"general.mode":    {"COMRADE_MODE"},
	"llm.provider":    {"COMRADE_PROVIDER"},
	"llm.model":       {"COMRADE_MODEL"},
	"general.profile": {"COMRADE_PROFILE"},
}

// Loader loads, persists, and resolves the effective value of cli-comrade's
// TOML config file at a fixed path. It holds no global state — callers
// construct one (typically via NewLoader("") to use the platform default
// path) and pass it down explicitly.
type Loader struct {
	path string
	// profileOverride is the --profile flag's value (the top precedence
	// tier ResolveActiveProfile resolves against), threaded in via
	// NewLoaderWithProfile. Empty for a Loader built with plain
	// NewLoader, meaning "no flag override" — COMRADE_PROFILE and the
	// file's own general.profile are still consulted normally.
	profileOverride string
}

// NewLoader constructs a Loader for the config file at path. If path is
// empty, the platform-default path (DefaultPath) is resolved and used.
func NewLoader(path string) (*Loader, error) {
	return NewLoaderWithProfile(path, "")
}

// NewLoaderWithProfile is NewLoader plus an explicit active-profile
// override — the --profile flag's value, threaded through from
// internal/cli's root command. An empty profileOverride behaves
// identically to NewLoader (COMRADE_PROFILE, then the file's own
// general.profile, are still consulted — see ResolveActiveProfile).
func NewLoaderWithProfile(path, profileOverride string) (*Loader, error) {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	return &Loader{path: path, profileOverride: profileOverride}, nil
}

// Path returns the config file path this Loader reads from and writes to.
func (l *Loader) Path() string {
	return l.path
}

// Load reads the effective configuration: built-in defaults, overlaid by
// the config file (created with defaults on first run), overlaid by
// COMRADE_ environment variables. The returned bool is true exactly when
// this call created the config file (i.e. this is the first run).
func (l *Loader) Load() (*Config, bool, error) {
	created, err := l.ensureFileExists()
	if err != nil {
		return nil, false, err
	}

	v, err := l.newEffectiveViper()
	if err != nil {
		return nil, created, err
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, created, fmt.Errorf("parse config file %s: %w", l.path, err)
	}
	// SAST finding #3: warn (never fail — see validateLoadedConfig's own
	// doc comment for why) about the ACTIVE provider's base_url here too,
	// not just at `comrade config set` time in Validate — a value can
	// reach the file via a hand-edit or a COMRADE_ env var, entirely
	// bypassing Validate.
	validateLoadedConfig(&cfg)
	return &cfg, created, nil
}

// Get returns the effective value (env > file > default) of a single
// known key, as a viper-decoded any (string, bool, int, or []string
// depending on the key's Kind).
func (l *Loader) Get(key string) (any, error) {
	if !IsValidKey(key) {
		return nil, unknownKeyError(key)
	}
	if _, err := l.ensureFileExists(); err != nil {
		return nil, err
	}
	v, err := l.newEffectiveViper()
	if err != nil {
		return nil, err
	}
	return v.Get(key), nil
}

// Source reports whether key's effective value came from the environment,
// the active profile's own table, the config file's top-level value, or
// the built-in default.
func (l *Loader) Source(key string) (Source, error) {
	if !IsValidKey(key) {
		return "", unknownKeyError(key)
	}

	// general.profile is special-cased entirely: newEffectiveViper forces
	// cfg.General.Profile to equal the exact same `active` value that
	// governs the profile-overlay decision (see that function's own doc
	// comment for the invariant and why), so general.profile's own
	// effective source IS wherever `active` came from — which needs
	// --profile flag > COMRADE_PROFILE > COMRADE_GENERAL_PROFILE > file
	// precedence, not the generic envCandidates loop below (that loop
	// cannot express "a flag beats env", and would misreport SourceEnv
	// even when l.profileOverride is ALSO set — see profileSource's own
	// doc comment).
	if key == "general.profile" {
		return l.profileSource()
	}

	for _, name := range envCandidates(key) {
		if os.Getenv(name) != "" {
			return SourceEnv, nil
		}
	}

	if _, err := l.ensureFileExists(); err != nil {
		return "", err
	}
	fv := viper.New()
	fv.SetConfigFile(l.path)
	fv.SetConfigType("toml")
	if err := fv.ReadInConfig(); err != nil {
		return "", fmt.Errorf("read config file %s: %w", l.path, err)
	}

	active := ResolveActiveProfile(l.profileOverride, os.Getenv("COMRADE_PROFILE"), os.Getenv("COMRADE_GENERAL_PROFILE"), fv.GetString("general.profile"))
	if active != "" {
		if raw, ok := fv.Get("profiles").(map[string]any); ok {
			if profile, ok := raw[active].(map[string]any); ok && profileHasKey(profile, key) {
				return SourceProfile, nil
			}
		}
	}

	if fv.IsSet(key) {
		return SourceFile, nil
	}
	return SourceDefault, nil
}

// profileSource is Source's dedicated path for the "general.profile" key
// itself. ValidateProfileKey already forbids a profile from ever setting
// general.profile INSIDE itself, so this key can never legitimately be
// SourceProfile — only SourceFlag, SourceEnv, SourceFile, or SourceDefault,
// checked in exactly ResolveActiveProfile's own precedence order so this
// never disagrees with which source actually decided the active profile
// (and therefore the overlay applied, and therefore cfg.General.Profile's
// own forced value — see newEffectiveViper).
func (l *Loader) profileSource() (Source, error) {
	if l.profileOverride != "" {
		return SourceFlag, nil
	}
	if os.Getenv("COMRADE_PROFILE") != "" || os.Getenv("COMRADE_GENERAL_PROFILE") != "" {
		return SourceEnv, nil
	}

	if _, err := l.ensureFileExists(); err != nil {
		return "", err
	}
	fv := viper.New()
	fv.SetConfigFile(l.path)
	fv.SetConfigType("toml")
	if err := fv.ReadInConfig(); err != nil {
		return "", fmt.Errorf("read config file %s: %w", l.path, err)
	}
	if fv.IsSet("general.profile") {
		return SourceFile, nil
	}
	return SourceDefault, nil
}

// SetAndSave validates that key is known, then persists value (already
// parsed/validated by Validate) to the config file, filling in every other
// key at its current effective file-or-default value (deliberately
// excluding any environment override, so a transient env var never gets
// baked into the file). Comments from a hand-edited file are not
// preserved — see docs/history/phases/FAZ-01.md.
func (l *Loader) SetAndSave(key string, value any) error {
	if !IsValidKey(key) {
		return unknownKeyError(key)
	}
	if _, err := l.ensureFileExists(); err != nil {
		return err
	}

	v, err := l.mergedFileViper()
	if err != nil {
		return err
	}

	// v already has every existing [profiles.*] table merged in from the
	// file (mergedFileViper's own MergeInConfig, above) — Set below only
	// ever touches the ONE top-level key path this call was asked to
	// change, so WriteConfigAs's full-map rewrite carries every profile
	// table through untouched. See TestSetAndSavePreservesProfileTables
	// (loader_test.go) for the pinned regression proof.
	v.Set(key, value)

	if err := v.WriteConfigAs(l.path); err != nil {
		return fmt.Errorf("write config file %s: %w", l.path, err)
	}
	return nil
}

// ensureFileExists creates the config file (with its parent directory and
// the built-in defaults) if it does not already exist. The returned bool
// is true exactly when this call created the file.
func (l *Loader) ensureFileExists() (bool, error) {
	_, err := os.Stat(l.path)
	switch {
	case err == nil:
		return false, nil
	case os.IsNotExist(err):
		if mkErr := os.MkdirAll(filepath.Dir(l.path), 0o750); mkErr != nil {
			return false, fmt.Errorf("create config directory for %s: %w", l.path, mkErr)
		}
		if wErr := os.WriteFile(l.path, []byte(defaultConfigTOML), 0o600); wErr != nil {
			return false, fmt.Errorf("write default config file %s: %w", l.path, wErr)
		}
		return true, nil
	default:
		return false, fmt.Errorf("stat config file %s: %w", l.path, err)
	}
}

// mergedFileViper builds a fresh viper layered as built-in defaults
// merged with the on-disk file (no env, no active-profile overlay) — the
// common starting point every read/write path in this file needs before
// applying its own next step (newEffectiveViper adds the profile overlay
// and env binding on top; SetAndSave/CreateProfile/RemoveProfile/
// SetProfileKey in profile_ops.go each apply their own one change and
// call WriteConfigAs). l.path must already exist.
func (l *Loader) mergedFileViper() (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType("toml")
	if err := v.MergeConfig(strings.NewReader(defaultConfigTOML)); err != nil {
		return nil, fmt.Errorf("config: parse built-in defaults: %w", err)
	}

	v.SetConfigFile(l.path)
	if err := v.MergeInConfig(); err != nil {
		return nil, fmt.Errorf("read config file %s: %w", l.path, err)
	}

	return v, nil
}

// newEffectiveViper builds a viper instance layered, low to high
// precedence, as: built-in defaults < the on-disk file's top-level
// values < the active profile's own [profiles.<name>] table (config
// profiles' one new overlay layer) < COMRADE_ environment variables
// (generic COMRADE_<SECTION>_<KEY> plus the explicit envAliases) — env
// always stays king, which is exactly why the profile overlay below is
// applied via MergeConfigMap (merges into viper's "config" precedence
// layer) and NOT viper.Set (which would write into the highest-priority
// "override" layer and invert this order — see applyProfileOverlay's own
// doc comment). l.path must already exist.
//
// INVARIANT: cfg.General.Profile must always name the profile actually in
// force — see the v.Set("general.profile", active) call at the end of
// this function. Without it, viper's own env precedence would leave the
// RESOLVED VALUE of general.profile (what cfg.General.Profile ends up as)
// disagreeing with the profile overlay actually applied above, in two
// independent ways: (1) viper v1.21.0's own find() checks the generic
// AutomaticEnv name (COMRADE_GENERAL_PROFILE) BEFORE an explicitly
// BindEnv-registered alias (COMRADE_PROFILE) — the OPPOSITE of the
// canonical-over-generic order ResolveActiveProfile (and this function's
// own `active`) uses — so with both env vars set to different profiles,
// the overlay applied would silently disagree with what
// cfg.General.Profile reports; (2) l.profileOverride (the --profile flag)
// never reaches viper's own value resolution AT ALL — it is a Go-level
// parameter to ResolveActiveProfile, not anything viper's find() ever
// sees — so without this v.Set, a --profile override would activate the
// right overlay while cfg.General.Profile kept reporting the file/env
// value instead. Forcing it here, via viper.Set (the highest-precedence
// "override" layer — see find()'s own precedence order, checked before
// pflags/env/config/defaults), makes the two questions ("which overlay
// applied" and "what does general.profile read as") the SAME question,
// answered once, by `active`.
func (l *Loader) newEffectiveViper() (*viper.Viper, error) {
	v, err := l.mergedFileViper()
	if err != nil {
		return nil, err
	}

	// Active-profile precedence mirrors ResolveMode's own shape:
	// l.profileOverride (the --profile flag) > COMRADE_PROFILE >
	// COMRADE_GENERAL_PROFILE (the generic COMRADE_<SECTION>_<KEY> form
	// general.profile gets for free from viper.AutomaticEnv, same as any
	// other key — see envAliases's own doc comment) > the file's own
	// general.profile value, read here BEFORE env binding is set up below
	// so this reflects defaults+file only, never an env override of
	// general.profile itself. Both env forms are read directly via
	// os.Getenv rather than through v itself, since v has no env binding
	// wired up yet at this point in construction.
	active := ResolveActiveProfile(l.profileOverride, os.Getenv("COMRADE_PROFILE"), os.Getenv("COMRADE_GENERAL_PROFILE"), v.GetString("general.profile"))
	if active != "" {
		applyProfileOverlay(v, active)
	}

	v.SetEnvPrefix("comrade")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	bindEnvAliases(v)

	// Force the invariant (see doc comment above): general.profile must
	// always read back as exactly `active`, the same value that decided
	// the overlay, regardless of viper's own env-precedence quirk or the
	// --profile flag never otherwise reaching viper's value resolution.
	// This is scoped to THIS ephemeral, read-only viper instance (used
	// only by Load()/Get(), both read-only) — it never reaches
	// mergedFileViper's own separate instance, so SetAndSave/CreateProfile/
	// SetProfileKey/RemoveProfile (all built on mergedFileViper, never
	// this function) keep persisting the file's own plain value, never an
	// env- or flag-derived one. See
	// TestSetAndSaveNeverPersistsEnvOrFlagDerivedProfile.
	v.Set("general.profile", active)

	return v, nil
}

// bindEnvAliases registers envAliases's explicit names on top of whatever
// generic COMRADE_<SECTION>_<KEY> name viper.AutomaticEnv already derives
// for that key.
func bindEnvAliases(v *viper.Viper) {
	for key, names := range envAliases {
		args := append([]string{key}, names...)
		_ = v.BindEnv(args...)
	}
}

// envCandidates returns every environment variable name that can override
// key: the generic COMRADE_<SECTION>_<KEY> mapping, plus any explicit
// envAliases entries for key.
func envCandidates(key string) []string {
	generic := "COMRADE_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
	return append([]string{generic}, envAliases[key]...)
}
