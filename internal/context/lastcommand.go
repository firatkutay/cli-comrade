package context

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LastCommand is the shape of the last_command.json file written by the
// FAZ 4 shell hooks and read here. FAZ 3 owns and defines this format
// (FAZ 4 will only ever write it): {command, exit_code, stderr_tail,
// stdout_tail, timestamp, shell}, per UYGULAMA_PLANI.md's FAZ 3 spec.
type LastCommand struct {
	Command    string    `json:"command"`
	ExitCode   int       `json:"exit_code"`
	StderrTail string    `json:"stderr_tail"`
	StdoutTail string    `json:"stdout_tail"`
	Timestamp  time.Time `json:"timestamp"`
	Shell      string    `json:"shell"`
}

// Age reports how long ago cmd.Timestamp was, relative to now. Staleness
// policy (e.g. FAZ 4's 10-minute freshness threshold for comrade fix's
// fallback chain) is the caller's decision — this type only reports the
// elapsed duration.
func (cmd LastCommand) Age(now time.Time) time.Duration {
	return now.Sub(cmd.Timestamp)
}

// LastCommandPath resolves the path to last_command.json for goos, using
// getenv for environment lookups — the same injectable-goos pattern as
// config.ResolvePath (internal/config/paths.go), so every branch,
// including windows, is testable regardless of the OS the test binary
// runs on.
//
//   - windows: %LOCALAPPDATA%\cli-comrade\last_command.json
//   - otherwise: $XDG_STATE_HOME/cli-comrade/last_command.json, falling
//     back to ~/.local/state/cli-comrade/last_command.json when
//     XDG_STATE_HOME is unset
func LastCommandPath(goos string, getenv func(string) string) (string, error) {
	if goos == "windows" {
		localAppData := getenv("LOCALAPPDATA")
		if localAppData == "" {
			return "", fmt.Errorf("resolve last_command.json path: LOCALAPPDATA environment variable is not set")
		}
		return localAppData + `\cli-comrade\last_command.json`, nil
	}

	if xdg := getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "cli-comrade", "last_command.json"), nil
	}

	home := getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("resolve last_command.json path: HOME environment variable is not set")
	}
	return filepath.Join(home, ".local", "state", "cli-comrade", "last_command.json"), nil
}

// ReadLastCommand reads and parses the last_command.json file at path. A
// missing file, an unreadable file, or corrupt JSON is not an error: ok
// is false and the zero LastCommand is returned, so callers always get a
// uniform "not available" result instead of having to special-case
// os.IsNotExist versus a json.Unmarshal failure.
func ReadLastCommand(path string) (cmd LastCommand, ok bool) {
	if path == "" {
		return LastCommand{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return LastCommand{}, false
	}
	if err := json.Unmarshal(data, &cmd); err != nil {
		return LastCommand{}, false
	}
	return cmd, true
}
