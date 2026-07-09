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

// ResolveDir computes cli-comrade's platform config DIRECTORY — the parent
// of config.toml (see ResolvePath) and, as of FAZ 8, of
// internal/secrets's file-fallback "credentials" file — for the given
// target OS ("windows" or anything else) using getenv to read
// environment variables. It takes goos and getenv as parameters (rather
// than reading runtime.GOOS/os.Getenv directly) so tests can exercise
// every branch — including the Windows branch — without depending on the
// OS the test binary actually runs on.
//
// Resolution rules (normative, see UYGULAMA_PLANI.md FAZ 1 / CLAUDE.md):
//   - windows: %APPDATA%\cli-comrade
//   - otherwise: $XDG_CONFIG_HOME/cli-comrade, falling back to
//     ~/.config/cli-comrade when XDG_CONFIG_HOME is unset.
//
// The windows branch is built with an explicit backslash rather than
// filepath.Join: filepath.Join uses the separator of the OS the test
// binary is actually running on, which would silently produce a
// forward-slash path when this branch is exercised on a Linux/macOS CI
// runner.
func ResolveDir(goos string, getenv func(string) string) (string, error) {
	if goos == "windows" {
		appData := getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("resolve config directory: APPDATA environment variable is not set")
		}
		return appData + `\` + configDirName, nil
	}

	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, configDirName), nil
	}

	home := getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("resolve config directory: HOME environment variable is not set")
	}
	return filepath.Join(home, ".config", configDirName), nil
}

// ResolvePath computes the config file path for the given target OS,
// built from ResolveDir plus configFileName. See ResolveDir for the
// resolution rules and the rationale for goos/getenv as parameters
// instead of runtime.GOOS/os.Getenv.
func ResolvePath(goos string, getenv func(string) string) (string, error) {
	dir, err := ResolveDir(goos, getenv)
	if err != nil {
		return "", fmt.Errorf("resolve config path: %w", err)
	}
	if goos == "windows" {
		return dir + `\` + configFileName, nil
	}
	return filepath.Join(dir, configFileName), nil
}

// DefaultDir resolves the config directory for the OS and environment
// this process is actually running under.
func DefaultDir() (string, error) {
	return ResolveDir(runtime.GOOS, os.Getenv)
}

// DefaultPath resolves the config file path for the OS and environment
// this process is actually running under.
func DefaultPath() (string, error) {
	return ResolvePath(runtime.GOOS, os.Getenv)
}
