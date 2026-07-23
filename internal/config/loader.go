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
)

// envAliases lists the explicit, named environment variable aliases
// required by docs/history/UYGULAMA_PLANI.md's FAZ 1 in addition to the generic
// COMRADE_<SECTION>_<KEY> mapping every key already gets from
// viper.AutomaticEnv(). Both forms work for these three keys; see
// bindEnvAliases and Loader.Source.
var envAliases = map[string][]string{
	"general.mode": {"COMRADE_MODE"},
	"llm.provider": {"COMRADE_PROVIDER"},
	"llm.model":    {"COMRADE_MODEL"},
}

// Loader loads, persists, and resolves the effective value of cli-comrade's
// TOML config file at a fixed path. It holds no global state — callers
// construct one (typically via NewLoader("") to use the platform default
// path) and pass it down explicitly.
type Loader struct {
	path string
}

// NewLoader constructs a Loader for the config file at path. If path is
// empty, the platform-default path (DefaultPath) is resolved and used.
func NewLoader(path string) (*Loader, error) {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	return &Loader{path: path}, nil
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
// the config file, or the built-in default.
func (l *Loader) Source(key string) (Source, error) {
	if !IsValidKey(key) {
		return "", unknownKeyError(key)
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
	if fv.IsSet(key) {
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

	v := viper.New()
	v.SetConfigType("toml")
	if err := v.MergeConfig(strings.NewReader(defaultConfigTOML)); err != nil {
		return fmt.Errorf("config: parse built-in defaults: %w", err)
	}
	v.SetConfigFile(l.path)
	if err := v.MergeInConfig(); err != nil {
		return fmt.Errorf("read config file %s: %w", l.path, err)
	}

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

// newEffectiveViper builds a viper instance layered as: built-in defaults,
// merged with the on-disk file, with COMRADE_ environment variables
// (generic COMRADE_<SECTION>_<KEY> plus the explicit envAliases) bound on
// top. l.path must already exist.
func (l *Loader) newEffectiveViper() (*viper.Viper, error) {
	v := viper.New()
	v.SetConfigType("toml")
	if err := v.MergeConfig(strings.NewReader(defaultConfigTOML)); err != nil {
		return nil, fmt.Errorf("config: parse built-in defaults: %w", err)
	}

	v.SetConfigFile(l.path)
	if err := v.MergeInConfig(); err != nil {
		return nil, fmt.Errorf("read config file %s: %w", l.path, err)
	}

	v.SetEnvPrefix("comrade")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	bindEnvAliases(v)

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
