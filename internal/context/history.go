package context

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

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
	data, err := os.ReadFile(path)
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
