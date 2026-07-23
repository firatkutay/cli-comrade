package context

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxHistoryBytes bounds how much of a shell history file ReadHistory
// will ever read into memory. Shell history files are user-controlled
// and can grow unbounded over years of interactive use (unlike
// last_command.json, which comrade itself writes and keeps small) —
// 4 MiB comfortably covers the "last few thousand commands" ReadHistory
// actually needs (depth is always small) while still bounding the read.
const maxHistoryBytes = 4 << 20 // 4 MiB

// zshExtendedPrefix strips zsh's optional "extended_history" prefix
// (`: <epoch>:<duration>;`) that precedes each command when that shell
// option is enabled, leaving just the command text.
var zshExtendedPrefix = regexp.MustCompile(`^: \d+:\d+;`)

// fishCmdPrefix is the line prefix fish's history YAML uses for the
// command text of each entry ("- cmd: <command>").
const fishCmdPrefix = "- cmd: "

// HistoryPath resolves shell's history file path, using getenv for
// environment lookups (same injectable pattern as LastCommandPath /
// config.ResolvePath). ok is false for a shell this package does not
// know a history file location for, or when the environment variable
// the location depends on is unset.
func HistoryPath(shell string, getenv func(string) string) (path string, ok bool) {
	switch shell {
	case "bash":
		home := getenv("HOME")
		if home == "" {
			return "", false
		}
		return filepath.Join(home, ".bash_history"), true
	case "zsh":
		home := getenv("HOME")
		if home == "" {
			return "", false
		}
		return filepath.Join(home, ".zsh_history"), true
	case "fish":
		home := getenv("HOME")
		if home == "" {
			return "", false
		}
		return filepath.Join(home, ".local", "share", "fish", "fish_history"), true
	case "powershell":
		appData := getenv("APPDATA")
		if appData == "" {
			return "", false
		}
		return filepath.Join(appData, "Microsoft", "Windows", "PowerShell", "PSReadLine", "ConsoleHost_history.txt"), true
	default:
		return "", false
	}
}

// ReadHistory returns up to depth of the most recent commands from
// shell's history file. It is best-effort per shell: an unrecognized
// shell, an unset environment variable, a missing/unreadable file, or
// depth<=0 all return nil rather than an error — opt-in history is
// grounding context, never something a read failure should abort on.
func ReadHistory(shell string, getenv func(string) string, depth int) []string {
	if depth <= 0 {
		return nil
	}
	path, ok := HistoryPath(shell, getenv)
	if !ok {
		return nil
	}
	data, err := readHistoryTail(path, maxHistoryBytes)
	if err != nil {
		return nil
	}

	var lines []string
	switch shell {
	case "bash", "powershell":
		lines = plainHistoryLines(string(data))
	case "zsh":
		lines = zshHistoryLines(string(data))
	case "fish":
		lines = fishHistoryLines(string(data))
	default:
		return nil
	}

	return lastN(lines, depth)
}

// readHistoryTail reads path bounded to at most limit bytes. A file no
// larger than limit is read in full, unchanged from before. A larger file
// has its LEADING bytes skipped (via Seek) so the bounded read captures
// the file's last limit bytes instead of its first — ReadHistory only
// ever wants the most recent entries (lastN's own trim to `depth`
// applies after this), so keeping the tail of an oversized file is what
// actually degrades gracefully here, unlike last_command.json's
// all-or-nothing JSON blob. The one entry straddling the truncation
// boundary may come out malformed (a partial line, or — for zsh/fish's
// own line-prefixed formats — a line missing its prefix); that entry is
// simply one more line for lastN to potentially drop, not a read
// failure.
func readHistoryTail(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path) // #nosec G304 -- path comes from HistoryPath's own fixed, well-known shell-history conventions (e.g. $HOME/.bash_history), not attacker-controlled input
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	if info, err := f.Stat(); err == nil && info.Size() > limit {
		if _, err := f.Seek(-limit, io.SeekEnd); err != nil {
			return nil, err
		}
	}
	return io.ReadAll(io.LimitReader(f, limit))
}

// plainHistoryLines splits data into non-empty lines, used for shells
// (bash, PowerShell's PSReadLine file) whose history is one command per
// line with no extra framing.
func plainHistoryLines(data string) []string {
	var out []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// zshHistoryLines splits data into non-empty lines and strips zsh's
// optional extended_history timestamp prefix from each one.
func zshHistoryLines(data string) []string {
	var out []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		out = append(out, zshExtendedPrefix.ReplaceAllString(line, ""))
	}
	return out
}

// fishHistoryLines extracts the command text from fish's YAML-ish
// history format, keeping only "- cmd: <command>" lines.
func fishHistoryLines(data string) []string {
	var out []string
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, fishCmdPrefix) {
			out = append(out, strings.TrimPrefix(line, fishCmdPrefix))
		}
	}
	return out
}

// lastN returns the last n elements of lines (or all of them if there
// are fewer than n).
func lastN(lines []string, n int) []string {
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}
