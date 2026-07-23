package update

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"
)

// maxStateBytes bounds how many bytes ReadState will read from
// update_check.json. Its real content is always a tiny JSON object (a
// timestamp and a short version string); this caps memory use if the
// file were ever replaced by something unexpectedly large (LOW#10).
const maxStateBytes = 64 << 10 // 64 KiB

// stateFileName is the JSON file's name within the resolved state
// directory — deliberately NOT alongside config.toml: this is transient,
// machine-local bookkeeping (a last-checked timestamp and the version
// last observed), never something a user hand-edits, so it belongs next
// to audit.jsonl/last_command.json (internal/audit, internal/context)
// under the platform's state directory, not the config directory.
const stateFileName = "update_check.json"

// CheckState is the on-disk shape of update_check.json: the passive
// version-notification feature's (docs/history/UYGULAMA_PLANI.md FAZ 10 item 4)
// entire persisted state — when it last asked GitHub, and what it found.
type CheckState struct {
	LastCheckedAt      time.Time `json:"last_checked_at"`
	LatestKnownVersion string    `json:"latest_known_version,omitempty"`
}

// StatePathFor resolves update_check.json's path for goos, using getenv
// for environment lookups — the same injectable-goos pattern as
// audit.PathFor/context.LastCommandPath/config.ResolvePath, so the
// windows branch is testable regardless of the OS the test binary
// actually runs on.
//
//   - windows: %LOCALAPPDATA%\cli-comrade\update_check.json
//   - otherwise: $XDG_STATE_HOME/cli-comrade/update_check.json, falling
//     back to ~/.local/state/cli-comrade/update_check.json when
//     XDG_STATE_HOME is unset
//
// The unix branches use path.Join (always "/"), not filepath.Join — see
// config.ResolveDir/audit.PathFor/context.LastCommandPath's doc comments
// for why: filepath.Join uses the separator of the OS the calling
// process actually runs on, which would silently produce backslash
// paths if this function is called with an injected
// goos="linux"/"darwin" on a Windows host.
func StatePathFor(goos string, getenv func(string) string) (string, error) {
	if goos == "windows" {
		localAppData := getenv("LOCALAPPDATA")
		if localAppData == "" {
			return "", fmt.Errorf("resolve update_check.json path: LOCALAPPDATA environment variable is not set")
		}
		return localAppData + `\cli-comrade\` + stateFileName, nil
	}

	if xdg := getenv("XDG_STATE_HOME"); xdg != "" {
		return path.Join(xdg, "cli-comrade", stateFileName), nil
	}

	home := getenv("HOME")
	if home == "" {
		return "", fmt.Errorf("resolve update_check.json path: HOME environment variable is not set")
	}
	return path.Join(home, ".local", "state", "cli-comrade", stateFileName), nil
}

// DefaultStatePath resolves update_check.json's path for the OS and
// environment this process is actually running under.
func DefaultStatePath() (string, error) {
	return StatePathFor(runtime.GOOS, os.Getenv)
}

// ReadState reads and parses update_check.json at path. A missing file,
// an unreadable file, or corrupt JSON is not an error — the zero
// CheckState is returned instead (LastCheckedAt is time.Time{}, the
// earliest possible value, which ShouldCheck always treats as "due") —
// so a first-ever run degrades to "always check" rather than failing,
// exactly like context.ReadLastCommand's uniform not-available result.
func ReadState(path string) CheckState {
	if path == "" {
		return CheckState{}
	}
	f, err := os.Open(path) // #nosec G304 -- path is this process's own fixed XDG-state update-check location (DefaultStatePath), not attacker-controlled input
	if err != nil {
		return CheckState{}
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, maxStateBytes))
	if err != nil {
		return CheckState{}
	}
	var st CheckState
	if err := json.Unmarshal(data, &st); err != nil {
		return CheckState{}
	}
	return st
}

// WriteState serializes st to JSON and atomically writes it to path —
// write to a temp file in path's own directory, then os.Rename, so a
// concurrent ReadState never observes a partially written file —
// creating path's parent directory first if it does not exist. Mirrors
// context.WriteLastCommand's exact atomic-write shape.
func WriteState(path string, st CheckState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("update: write state: create directory %s: %w", dir, err)
	}

	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("update: write state: marshal: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".update_check-*.json.tmp")
	if err != nil {
		return fmt.Errorf("update: write state: create temp file: %w", err)
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
		return fmt.Errorf("update: write state: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("update: write state: close temp file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("update: write state: chmod temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("update: write state: rename temp file into place: %w", err)
	}
	renamed = true
	return nil
}

// CheckInterval is the passive version-notification feature's maximum
// check frequency — docs/history/UYGULAMA_PLANI.md FAZ 10 item 4's "haftada en fazla
// bir kez" (at most once per week).
const CheckInterval = 7 * 24 * time.Hour

// ShouldCheck reports whether a background version check is due: true
// when at least CheckInterval has elapsed since st.LastCheckedAt (the
// zero value counts as infinitely long ago, so a first-ever run is
// always due).
func ShouldCheck(now time.Time, st CheckState) bool {
	return now.Sub(st.LastCheckedAt) >= CheckInterval
}
