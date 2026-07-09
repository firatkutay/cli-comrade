package context

import (
	stdctx "context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// shellVersionTimeout bounds how long ShellVersion will wait for the
// shell binary to answer "--version" (or equivalent) before giving up
// silently. Kept short since this is best-effort grounding, never a
// step the user is blocked on.
const shellVersionTimeout = 500 * time.Millisecond

// CommandRunner runs name with args under ctx and returns its combined
// output. Collector.RunCommand and ShellVersion take one as a
// parameter/field so tests can stub process execution instead of
// depending on a real shell binary being installed on the test machine.
type CommandRunner func(ctx stdctx.Context, name string, args ...string) ([]byte, error)

// RunCommand is the default CommandRunner, backed by exec.CommandContext.
func RunCommand(ctx stdctx.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) // #nosec G204 -- name is one of a small, fixed set of shell/version-probe binaries this package itself decides to invoke (e.g. detecting the shell's own --version), never attacker/LLM-controlled input
	return cmd.Output()
}

// DetectShell reports the current shell's name for goos, using getenv
// to read environment variables:
//
//   - windows: PSModulePath present ⇒ "powershell", otherwise "cmd"
//     (CLAUDE.md's stated heuristic; there is no reliable stdlib-only
//     way to distinguish an elevated/inherited shell further)
//   - otherwise: the basename of $SHELL, or "" if SHELL is unset
func DetectShell(goos string, getenv func(string) string) string {
	if goos == "windows" {
		if getenv("PSModulePath") != "" {
			return "powershell"
		}
		return "cmd"
	}

	shell := getenv("SHELL")
	if shell == "" {
		return ""
	}
	return filepath.Base(shell)
}

// ShellVersion best-effort runs shell's version command (under a short
// timeout derived from ctx) and returns the first line of its output.
// Any failure — shell == "", an unrecognized shell, a missing binary, a
// non-zero exit, or a timeout — is silent: it returns "" rather than an
// error, since a missing shell version is grounding context, never
// something to surface as a user-facing error.
func ShellVersion(ctx stdctx.Context, shell string, run CommandRunner) string {
	if shell == "" || run == nil {
		return ""
	}
	args := versionArgs(shell)
	if args == nil {
		return ""
	}

	timeoutCtx, cancel := stdctx.WithTimeout(ctx, shellVersionTimeout)
	defer cancel()

	out, err := run(timeoutCtx, shell, args...)
	if err != nil {
		return ""
	}
	return firstLine(string(out))
}

// versionArgs returns the argv (after the shell binary name) used to
// print a version string, or nil for a shell this package does not know
// how to query.
func versionArgs(shell string) []string {
	switch shell {
	case "bash", "zsh", "fish", "sh", "dash", "ksh":
		return []string{"--version"}
	case "powershell", "pwsh":
		return []string{"-NoProfile", "-Command", "$PSVersionTable.PSVersion.ToString()"}
	default:
		return nil
	}
}

// firstLine trims s and returns its first line, or the whole trimmed
// string if it has no newline.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return strings.TrimSpace(s[:idx])
	}
	return s
}
