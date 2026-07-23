package context

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"
)

// maxLastCommandBytes bounds how much of last_command.json
// ReadLastCommand will ever read into memory. The file is a single
// small JSON object FAZ 4's shell hooks write via WriteLastCommand
// (command text + a bounded stderr/stdout tail); anything anywhere
// near this size is not a file comrade itself ever produced.
const maxLastCommandBytes = 1 << 20 // 1 MiB

// LastCommand is the shape of the last_command.json file written by the
// FAZ 4 shell hooks and read here. FAZ 3 owns and defines this format
// (FAZ 4 will only ever write it): {command, exit_code, stderr_tail,
// stdout_tail, timestamp, shell}, per docs/history/UYGULAMA_PLANI.md's FAZ 3 spec.
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
//
// The unix branches use path.Join (always "/"), not filepath.Join, for
// the same reason config.ResolveDir/audit.PathFor do (see their doc
// comments): filepath.Join uses the separator of the OS the calling
// process actually runs on, which would silently produce backslash
// paths if this function is ever called with an injected
// goos="linux"/"darwin" on a Windows host — keeping the unix branches on
// literal "/" ties the output to goos, not runtime.GOOS.
func LastCommandPath(goos string, getenv func(string) string) (string, error) {
	if goos == "windows" {
		localAppData := getenv("LOCALAPPDATA")
		if localAppData == "" {
			return "", fmt.Errorf("resolve last_command.json path: LOCALAPPDATA environment variable is not set")
		}
		return localAppData + `\cli-comrade\last_command.json`, nil
	}

	if xdg := getenv("XDG_STATE_HOME"); xdg != "" {
		return path.Join(xdg, "cli-comrade", "last_command.json"), nil
	}

	home := getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("resolve last_command.json path: HOME environment variable is not set")
	}
	return path.Join(home, ".local", "state", "cli-comrade", "last_command.json"), nil
}

// WriteLastCommand serializes cmd to JSON and atomically writes it to
// path — write to a temp file in path's own directory, then os.Rename,
// so a reader (ReadLastCommand) never observes a partially written file
// — creating path's parent directory first if it does not exist. FAZ
// 4's hidden "comrade hook record" subcommand is the sole writer of this
// file (see internal/shellinit and internal/cli/hook.go): shell hooks
// never hand-assemble this JSON themselves, since shell-escaping
// arbitrary command text (quotes, unicode, embedded newlines) into a
// JSON literal from inside a shell script is unsafe. They instead exec
// the comrade binary, which does the encoding here with Go's
// encoding/json.
func WriteLastCommand(path string, cmd LastCommand) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("write last command: create directory %s: %w", dir, err)
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("write last command: marshal: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".last_command-*.json.tmp")
	if err != nil {
		return fmt.Errorf("write last command: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write last command: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write last command: close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("write last command: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("write last command: rename temp file into place: %w", err)
	}
	renamed = true
	return nil
}

// ReadLastCommand reads and parses the last_command.json file at path. A
// missing file, an unreadable file, an oversized file (over
// maxLastCommandBytes), or corrupt JSON is not an error: ok is false and
// the zero LastCommand is returned, so callers always get a uniform "not
// available" result instead of having to special-case os.IsNotExist
// versus a json.Unmarshal failure.
func ReadLastCommand(path string) (cmd LastCommand, ok bool) {
	if path == "" {
		return LastCommand{}, false
	}
	f, err := os.Open(path) // #nosec G304 -- path is this process's own fixed XDG-state last_command.json location (LastCommandPath), not attacker-controlled input
	if err != nil {
		return LastCommand{}, false
	}
	defer func() { _ = f.Close() }()

	// Read at most maxLastCommandBytes+1: this process's own memory
	// footprint never exceeds ~1 MiB regardless of the file's actual
	// on-disk size, and if the file is larger, the extra byte tips
	// len(data) over the cap below so the oversized file is rejected
	// outright rather than parsed from a truncated JSON fragment.
	data, err := io.ReadAll(io.LimitReader(f, maxLastCommandBytes+1))
	if err != nil {
		return LastCommand{}, false
	}
	if len(data) > maxLastCommandBytes {
		return LastCommand{}, false
	}
	if err := json.Unmarshal(data, &cmd); err != nil {
		return LastCommand{}, false
	}
	return cmd, true
}
