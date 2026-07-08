package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// configDirName is the directory (under the platform's config root) that
// holds cli-comrade's config.toml.
const configDirName = "cli-comrade"

// configFileName is the config file's name within configDirName.
const configFileName = "config.toml"

// ResolvePath computes the config file path for the given target OS
// ("windows" or anything else) using getenv to read environment
// variables. It takes goos and getenv as parameters (rather than reading
// runtime.GOOS/os.Getenv directly) so tests can exercise every branch —
// including the Windows branch — without depending on the OS the test
// binary actually runs on.
//
// Resolution rules (normative, see UYGULAMA_PLANI.md FAZ 1 / CLAUDE.md):
//   - windows: %APPDATA%\cli-comrade\config.toml
//   - otherwise: $XDG_CONFIG_HOME/cli-comrade/config.toml, falling back to
//     ~/.config/cli-comrade/config.toml when XDG_CONFIG_HOME is unset.
//
// The windows branch is built with an explicit backslash rather than
// filepath.Join: filepath.Join uses the separator of the OS the test
// binary is actually running on, which would silently produce a
// forward-slash path when this branch is exercised on a Linux/macOS CI
// runner.
func ResolvePath(goos string, getenv func(string) string) (string, error) {
	if goos == "windows" {
		appData := getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("resolve config path: APPDATA environment variable is not set")
		}
		return appData + `\` + configDirName + `\` + configFileName, nil
	}

	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, configDirName, configFileName), nil
	}

	home := getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("resolve config path: HOME environment variable is not set")
	}
	return filepath.Join(home, ".config", configDirName, configFileName), nil
}

// DefaultPath resolves the config file path for the OS and environment
// this process is actually running under.
func DefaultPath() (string, error) {
	return ResolvePath(runtime.GOOS, os.Getenv)
}
