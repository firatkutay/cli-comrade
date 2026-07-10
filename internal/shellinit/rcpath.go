package shellinit

import (
	stdctx "context"
	"fmt"
	"path/filepath"
	"strings"
)

// CommandRunner runs name with args under ctx and returns its combined
// output. RCPath's PowerShell branch takes one as a parameter (rather
// than calling os/exec directly) purely so tests can stub process
// execution; internal/context.RunCommand (an unnamed func value of this
// same underlying signature) is what production callers pass in.
type CommandRunner func(ctx stdctx.Context, name string, args ...string) ([]byte, error)

// RCPath resolves the rc/profile file "comrade init" should install
// shell's block into, for goos, using getenv/lookPath/run to read
// environment state — the same injectable-goos pattern as
// config.ResolvePath and context.LastCommandPath, so every branch,
// including ones that only apply on an OS the test binary isn't
// actually running on, is testable.
//
//   - bash: $HOME/.bashrc
//   - zsh: $ZDOTDIR/.zshrc if ZDOTDIR is set, else $HOME/.zshrc
//   - fish: $XDG_CONFIG_HOME/fish/config.fish if set, else
//     $HOME/.config/fish/config.fish
//   - powershell: $PROFILE, as reported by actually invoking a
//     PowerShell binary ("powershell" on windows, "pwsh" everywhere
//     else, since Windows PowerShell is not expected off-Windows) with
//     `-NoProfile -Command '$PROFILE'`. If that binary isn't on PATH,
//     or invoking it fails, ok is false with note explaining why —
//     "comrade init" then falls back to printing the snippet with
//     manual instructions rather than guessing a path, per
//     docs/history/UYGULAMA_PLANI.md FAZ 4's "keep honest" requirement.
//
// ok is false only for the "can't locate a path at all" case; a
// resolution failure that has a concrete missing-environment-variable
// cause (HOME unset) is returned as note too, following the same
// (path, ok, note) shape for every shell so callers do not need a
// separate error type per branch.
func RCPath(ctx stdctx.Context, shell Shell, goos string, getenv func(string) string, lookPath func(string) (string, error), run CommandRunner) (path string, ok bool, note string) {
	switch shell {
	case Bash:
		home := getenv("HOME")
		if home == "" {
			return "", false, "cannot resolve bash rc file: HOME environment variable is not set"
		}
		return filepath.Join(home, ".bashrc"), true, ""

	case Zsh:
		if zdotdir := getenv("ZDOTDIR"); zdotdir != "" {
			return filepath.Join(zdotdir, ".zshrc"), true, ""
		}
		home := getenv("HOME")
		if home == "" {
			return "", false, "cannot resolve zsh rc file: HOME environment variable is not set"
		}
		return filepath.Join(home, ".zshrc"), true, ""

	case Fish:
		if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
			return filepath.Join(xdg, "fish", "config.fish"), true, ""
		}
		home := getenv("HOME")
		if home == "" {
			return "", false, "cannot resolve fish config file: HOME environment variable is not set"
		}
		return filepath.Join(home, ".config", "fish", "config.fish"), true, ""

	case PowerShell:
		return resolvePowerShellProfile(ctx, goos, lookPath, run)

	default:
		return "", false, fmt.Sprintf("unsupported shell %q", shell)
	}
}

// FishCompletionsPath resolves the path "comrade init fish" writes its
// shell-completions script to (FishCompletionsScript) — fish's own
// native lazy-load location for a single tool's completions (fish
// auto-sources any file placed in this exact directory the first time
// it needs to complete that command's name, no rc-file sourcing
// required) — using the EXACT same XDG_CONFIG_HOME-or-HOME resolution
// RCPath's own Fish branch already uses for config.fish above, one
// directory level deeper ("completions/comrade.fish" instead of
// "config.fish"). Kept as its own function (not folded into RCPath)
// because this path is written to unconditionally on every "comrade
// init fish" (full-file overwrite, no marker-block idempotency
// machinery — see FishCompletionsScript's own doc comment), never
// merged/diffed against existing content the way RCPath's other three
// shells' rc files are.
func FishCompletionsPath(getenv func(string) string) (path string, ok bool, note string) {
	if xdg := getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "fish", "completions", "comrade.fish"), true, ""
	}
	home := getenv("HOME")
	if home == "" {
		return "", false, "cannot resolve fish completions file: HOME environment variable is not set"
	}
	return filepath.Join(home, ".config", "fish", "completions", "comrade.fish"), true, ""
}

// resolvePowerShellProfile is RCPath's PowerShell branch: see RCPath's
// doc comment for the resolution strategy and rationale. It always
// queries exactly one binary, chosen purely from goos ("powershell" on
// Windows, "pwsh" everywhere else) — see ResolvePowerShellProfiles
// (psprofiles.go) for the Windows-aware variant that probes BOTH
// PowerShell 5.1 and PowerShell 7 instead of guessing one from goos,
// used by "comrade init powershell" on GOOS=windows so the hook reaches
// every installed variant's own profile, not just whichever one this
// function would have guessed.
func resolvePowerShellProfile(ctx stdctx.Context, goos string, lookPath func(string) (string, error), run CommandRunner) (string, bool, string) {
	bin := "pwsh"
	if goos == "windows" {
		bin = "powershell"
	}

	if lookPath == nil || run == nil {
		return "", false, fmt.Sprintf("cannot resolve PowerShell profile path: no way to query %s", bin)
	}
	if _, err := lookPath(bin); err != nil {
		return "", false, fmt.Sprintf("cannot resolve PowerShell profile path: %s not found on PATH", bin)
	}
	return queryProfilePath(ctx, bin, run)
}

// queryProfilePath runs bin (a PowerShell-family executable already
// confirmed to be on PATH) with `-NoProfile -Command '$PROFILE'` and
// returns bin's own profile path. This is the common tail shared by
// resolvePowerShellProfile (RCPath's single-guess-from-goos PowerShell
// branch) and ResolvePowerShellProfiles (psprofiles.go's multi-variant,
// lookPath-based resolver): both already know which specific binary to
// query by the time they reach this point, they just differ in HOW they
// arrived at that binary name.
func queryProfilePath(ctx stdctx.Context, bin string, run CommandRunner) (string, bool, string) {
	out, err := run(ctx, bin, "-NoProfile", "-Command", "$PROFILE")
	if err != nil {
		return "", false, fmt.Sprintf("cannot resolve PowerShell profile path: %s failed: %v", bin, err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", false, fmt.Sprintf("cannot resolve PowerShell profile path: %s returned an empty path", bin)
	}
	return path, true, ""
}
