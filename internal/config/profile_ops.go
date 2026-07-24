package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// CreateProfile creates profiles.<name> — an empty table when seed is
// nil/empty (see profilePlaceholderKey's own doc comment for why that
// still durably persists), or pre-populated with seed's key/value pairs
// (`comrade config profile add --from-current`'s snapshot of the
// current file-level [llm] section) — while preserving every other
// key/profile already in the file. Returns *ProfileExistsError if name
// already names a defined profile.
func (l *Loader) CreateProfile(name string, seed map[string]any) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	if _, err := l.ensureFileExists(); err != nil {
		return err
	}

	v, err := l.mergedFileViper()
	if err != nil {
		return err
	}

	if profiles, ok := v.Get("profiles").(map[string]any); ok {
		if _, exists := profiles[name]; exists {
			return &ProfileExistsError{Name: name}
		}
	}

	if len(seed) == 0 {
		v.Set("profiles."+name+"."+profilePlaceholderKey, true)
	} else {
		for key, value := range seed {
			v.Set("profiles."+name+"."+key, value)
		}
	}

	if err := v.WriteConfigAs(l.path); err != nil {
		return fmt.Errorf("write config file %s: %w", l.path, err)
	}
	return nil
}

// SetProfileKey validates that key/raw is legal via ValidateProfileKey,
// then persists the parsed value into profiles.<name>.<key> —
// preserving every other key/profile already in the file. Returns
// *ProfileNotFoundError if name isn't a defined profile — `comrade
// config profile set` never implicitly creates the profile it targets;
// use `comrade config profile add` for that.
func (l *Loader) SetProfileKey(name, key string, value any) error {
	if _, err := l.ensureFileExists(); err != nil {
		return err
	}

	v, err := l.mergedFileViper()
	if err != nil {
		return err
	}

	if !profileExists(v, name) {
		return &ProfileNotFoundError{Name: name}
	}

	v.Set("profiles."+name+"."+key, value)

	if err := v.WriteConfigAs(l.path); err != nil {
		return fmt.Errorf("write config file %s: %w", l.path, err)
	}
	return nil
}

// RemoveProfile deletes profiles.<name> entirely, clearing
// general.profile too when it pointed at name (so `comrade config
// profile list`/Load() never reports a just-removed profile as still
// "active"). Returns *ProfileNotFoundError if name isn't defined.
//
// Unlike CreateProfile/SetProfileKey (which only ever ADD a leaf key via
// viper.Set — safe on a viper instance that already has the old file
// merged in, since Set's override layer and the config layer merge
// key-by-key), an actual DELETION cannot be expressed that way: viper's
// AllSettings merge is additive, so v.Set("profiles", <map without
// name>) on a viper instance whose config layer still has profiles.name
// merges the old entry right back in (verified empirically against this
// project's pinned viper version — the override and config layers merge
// per-key, they do not replace each other's subtree wholesale). The fix
// is to compute the desired final settings as a plain Go map, delete
// name from it there, and write that map out through a completely FRESH
// viper instance that never had the old data merged into any layer at
// all.
func (l *Loader) RemoveProfile(name string) error {
	if _, err := l.ensureFileExists(); err != nil {
		return err
	}

	v, err := l.mergedFileViper()
	if err != nil {
		return err
	}

	settings := v.AllSettings()
	profiles, _ := settings["profiles"].(map[string]any)
	if _, exists := profiles[name]; !exists {
		return &ProfileNotFoundError{Name: name}
	}
	delete(profiles, name)

	if general, ok := settings["general"].(map[string]any); ok {
		if active, ok := general["profile"].(string); ok && active == name {
			general["profile"] = ""
		}
	}

	fresh := viper.New()
	fresh.SetConfigType("toml")
	if err := fresh.MergeConfigMap(settings); err != nil {
		return fmt.Errorf("config: rebuild settings after removing profile %q: %w", name, err)
	}
	if err := fresh.WriteConfigAs(l.path); err != nil {
		return fmt.Errorf("write config file %s: %w", l.path, err)
	}
	return nil
}

// profileExists reports whether v's merged "profiles" table defines name.
func profileExists(v *viper.Viper, name string) bool {
	profiles, ok := v.Get("profiles").(map[string]any)
	if !ok {
		return false
	}
	_, exists := profiles[name]
	return exists
}
