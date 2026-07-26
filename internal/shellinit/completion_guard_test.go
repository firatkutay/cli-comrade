package shellinit_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

// This file covers the real-world regression reported against
// snippets/powershell.ps1: an npm install that lands the dispatcher
// (npm/main/bin/comrade.js) without its platform binary leaves "comrade"
// resolvable on PATH but failing on every invocation. Before this fix,
// every one of the four generated shell hooks unconditionally piped (or
// sourced) `comrade completion <shell>`'s output straight into an
// eval-like sink (source/eval/Invoke-Expression) with no check that the
// output was non-empty -- PowerShell's Invoke-Expression throws outright
// on an empty command string, and even where the interpreter tolerates
// an empty eval (bash/zsh's `source`, fish's `source`), the child's own
// diagnostic (written to stderr, inherited straight to the terminal by
// process substitution / a pipe) still spammed every new shell.
//
// Two kinds of proof below:
//   - requireAdjacentLines pins each snippet's SHAPE: the eval/source/
//     Invoke-Expression call must sit immediately after the line that
//     captures output and checks it is non-empty, not merely appear
//     somewhere in the same file.
//   - the Test*SnippetSilentWhenComradeBroken tests actually source/run
//     the generated snippet, with a fake "comrade" on PATH that
//     reproduces the exact reported failure (diagnostic to stderr, empty
//     stdout, non-zero exit), and assert the shell exits 0 with empty
//     stderr -- the real proof, not a string match.

// requireAdjacentLines asserts body contains want[0], want[1], ... in
// that exact order, separated only by whitespace/newlines. This is what
// makes the assertion a shape check rather than a mere substring check:
// a regression that kept the guard's non-empty test somewhere in the
// file but moved the eval/source/Invoke-Expression outside of it (e.g.
// back to an unconditional call elsewhere) fails this, where a plain
// "does the guard string appear anywhere" check would not.
func requireAdjacentLines(t *testing.T, body string, want ...string) {
	t.Helper()
	parts := make([]string, len(want))
	for i, w := range want {
		parts[i] = regexp.QuoteMeta(w)
	}
	pattern := strings.Join(parts, `\s*\n\s*`)
	re, err := regexp.Compile(pattern)
	require.NoError(t, err)
	assert.Regexp(t, re, body, "expected these lines adjacent (inside the same guard), in order:\n%s", strings.Join(want, "\n"))
}

func TestBashCompletionEvalIsGuardedByNonEmptyCheck(t *testing.T) {
	snippet, err := shellinit.Snippet(shellinit.Bash)
	require.NoError(t, err)

	requireAdjacentLines(t, snippet,
		`__comrade_completion="$(comrade completion bash 2>/dev/null)"`,
		`[ -n "$__comrade_completion" ] && eval "$__comrade_completion"`,
	)
	// The exact pre-fix unconditional line (not just the substring
	// "source <(comrade completion bash)", which also legitimately
	// appears inside the guard's own explanatory comment above).
	assert.NotContains(t, snippet, "comrade >/dev/null 2>&1 && source <(comrade completion bash)",
		"must never source comrade's completion output unconditionally")
}

func TestZshCompletionEvalIsGuardedByNonEmptyCheck(t *testing.T) {
	snippet, err := shellinit.Snippet(shellinit.Zsh)
	require.NoError(t, err)

	requireAdjacentLines(t, snippet,
		`__comrade_completion="$(comrade completion zsh 2>/dev/null)"`,
		`[ -n "$__comrade_completion" ] && eval "$__comrade_completion"`,
	)
	// The exact pre-fix unconditional line.
	assert.NotContains(t, snippet, "whence compdef >/dev/null 2>&1 && source <(comrade completion zsh)",
		"must never source comrade's completion output unconditionally")
}

func TestFishCompletionsSourceIsGuardedByNonEmptyCheck(t *testing.T) {
	script := shellinit.FishCompletionsScript()

	requireAdjacentLines(t, script,
		`set -l __comrade_completion (comrade completion fish 2>/dev/null)`,
		`if test (count $__comrade_completion) -gt 0`,
		`string join \n -- $__comrade_completion | source`,
	)
	assert.NotContains(t, script, "comrade completion fish | source",
		"must never pipe comrade's completion output straight into source unconditionally")
}

func TestPowerShellCompletionInvokeIsGuardedByNonEmptyCheck(t *testing.T) {
	snippet, err := shellinit.Snippet(shellinit.PowerShell)
	require.NoError(t, err)
	snippet = normalizeCRLF(snippet)

	requireAdjacentLines(t, snippet,
		`$__comradeCompletionScript = comrade completion powershell 2>$null | Out-String`,
		`if ($__comradeCompletionScript -and $__comradeCompletionScript.Trim()) {`,
		`Invoke-Expression $__comradeCompletionScript`,
	)
	assert.NotContains(t, snippet, "comrade completion powershell | Out-String | Invoke-Expression",
		"must never Invoke-Expression comrade's completion output unconditionally -- this is the exact reported bug")
}

// ---------------------------------------------------------------------
// Real execution: source/run the actual generated snippet with a fake
// "comrade" reproducing the field-reported failure mode, and prove the
// shell survives it silently.
// ---------------------------------------------------------------------

// writeFakeComrade writes an executable "comrade" script into dir. body
// is a POSIX shell script body (a plain "#!/usr/bin/env bash" shebang is
// resolvable as an external command by bash, zsh, and fish alike -- none
// of them care what interpreter the target binary itself uses).
func writeFakeComrade(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, "comrade")
	require.NoError(t, os.WriteFile(path, []byte("#!/usr/bin/env bash\n"+body+"\n"), 0o755))
}

// brokenComradeScript reproduces the exact field-reported failure: an
// npm install that landed the dispatcher without its platform binary.
// The diagnostic goes to stderr (npm/main/bin/comrade.js's own
// console.error), stdout is empty, and the process exits non-zero.
const brokenComradeScript = `echo 'cli-comrade: no prebuilt binary available for this platform ("win32-x64").' >&2
exit 1`

// workingComradeScript is a minimal stand-in for a healthy install: a
// real (if trivial) POSIX-shell completion script on stdout, so the
// "comrade works fine" path can be proven not to have regressed by this
// fix. Used by the bash/zsh tests, which source (eval) this output as
// POSIX shell.
const workingComradeScript = `if [ "$1" = "completion" ]; then
  echo '__comrade_fake_completion_marker() { :; }'
  exit 0
fi
exit 0`

// fishWorkingComradeScript is workingComradeScript's fish counterpart:
// fish-completions.fish sources comrade's completion output as fish
// script, not POSIX shell, so the emitted function definition must use
// fish's own function syntax.
const fishWorkingComradeScript = `if [ "$1" = "completion" ]; then
  echo 'function __comrade_fake_completion_marker; end'
  exit 0
fi
exit 0`

func runSourcingShell(t *testing.T, shellPath, sourceLine string, pathDirs []string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(shellPath, "-c", sourceLine)
	cmd.Env = []string{"PATH=" + strings.Join(pathDirs, string(os.PathListSeparator)), "HOME=" + t.TempDir()}
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run %s: %v", shellPath, err)
		}
	}
	return outBuf.String(), errBuf.String(), code
}

func writeSnippetFile(t *testing.T, shell shellinit.Shell) string {
	t.Helper()
	body, err := shellinit.Snippet(shell)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "snippet.sh")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestBashSnippetSilentWhenComradeAbsentFromPath(t *testing.T) {
	snippetPath := writeSnippetFile(t, shellinit.Bash)
	stdout, stderr, code := runSourcingShell(t, "bash",
		"source "+snippetPath, []string{"/usr/bin", "/bin"})

	assert.Equal(t, 0, code, "stdout: %s\nstderr: %s", stdout, stderr)
	assert.Empty(t, stderr, "sourcing the hook with comrade entirely absent from PATH must never print anything")
}

func TestBashSnippetSilentWhenComradeBrokenOnPath(t *testing.T) {
	fakeBinDir := t.TempDir()
	writeFakeComrade(t, fakeBinDir, brokenComradeScript)
	snippetPath := writeSnippetFile(t, shellinit.Bash)

	stdout, stderr, code := runSourcingShell(t, "bash",
		"source "+snippetPath, []string{fakeBinDir, "/usr/bin", "/bin"})

	assert.Equal(t, 0, code, "stdout: %s\nstderr: %s", stdout, stderr)
	assert.Empty(t, stderr, "a comrade that is on PATH but broken (npm dispatcher without a platform binary) must never spam the shell on every startup -- this is the exact field-reported bug")
}

func TestBashSnippetLoadsCompletionWhenComradeWorks(t *testing.T) {
	fakeBinDir := t.TempDir()
	writeFakeComrade(t, fakeBinDir, workingComradeScript)
	snippetPath := writeSnippetFile(t, shellinit.Bash)

	stdout, stderr, code := runSourcingShell(t, "bash",
		"source "+snippetPath+"; type __comrade_fake_completion_marker >/dev/null 2>&1 && echo DEFINED || echo NOT_DEFINED",
		[]string{fakeBinDir, "/usr/bin", "/bin"})

	assert.Equal(t, 0, code, "stdout: %s\nstderr: %s", stdout, stderr)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "DEFINED", "a healthy comrade's completion script must still be loaded -- this fix must not regress the working case")
}

func TestZshSnippetSilentWhenComradeBrokenOnPath(t *testing.T) {
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not found on PATH; skipping zsh execution check")
	}

	fakeBinDir := t.TempDir()
	writeFakeComrade(t, fakeBinDir, brokenComradeScript)
	snippetPath := writeSnippetFile(t, shellinit.Zsh)

	stdout, stderr, code := runSourcingShell(t, zshPath,
		"source "+snippetPath, []string{fakeBinDir, "/usr/bin", "/bin"})

	assert.Equal(t, 0, code, "stdout: %s\nstderr: %s", stdout, stderr)
	assert.Empty(t, stderr, "a comrade that is on PATH but broken must never spam the shell on every startup")
}

func TestZshSnippetLoadsCompletionWhenComradeWorks(t *testing.T) {
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not found on PATH; skipping zsh execution check")
	}

	fakeBinDir := t.TempDir()
	writeFakeComrade(t, fakeBinDir, workingComradeScript)
	snippetPath := writeSnippetFile(t, shellinit.Zsh)

	stdout, stderr, code := runSourcingShell(t, zshPath,
		"source "+snippetPath+"; type __comrade_fake_completion_marker >/dev/null 2>&1 && echo DEFINED || echo NOT_DEFINED",
		[]string{fakeBinDir, "/usr/bin", "/bin"})

	assert.Equal(t, 0, code, "stdout: %s\nstderr: %s", stdout, stderr)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "DEFINED", "a healthy comrade's completion script must still be loaded")
}

func TestFishCompletionsScriptSilentWhenComradeBrokenOnPath(t *testing.T) {
	fishPath, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not found on PATH; skipping fish execution check")
	}

	fakeBinDir := t.TempDir()
	writeFakeComrade(t, fakeBinDir, brokenComradeScript)
	path := filepath.Join(t.TempDir(), "completions.fish")
	require.NoError(t, os.WriteFile(path, []byte(shellinit.FishCompletionsScript()), 0o644))

	stdout, stderr, code := runSourcingShell(t, fishPath,
		"source "+path, []string{fakeBinDir, "/usr/bin", "/bin"})

	assert.Equal(t, 0, code, "stdout: %s\nstderr: %s", stdout, stderr)
	assert.Empty(t, stderr, "a comrade that is on PATH but broken must never spam the shell on every startup")
}

func TestFishCompletionsScriptLoadsCompletionWhenComradeWorks(t *testing.T) {
	fishPath, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not found on PATH; skipping fish execution check")
	}

	fakeBinDir := t.TempDir()
	writeFakeComrade(t, fakeBinDir, fishWorkingComradeScript)
	path := filepath.Join(t.TempDir(), "completions.fish")
	require.NoError(t, os.WriteFile(path, []byte(shellinit.FishCompletionsScript()), 0o644))

	stdout, stderr, code := runSourcingShell(t, fishPath,
		"source "+path+"; functions -q __comrade_fake_completion_marker; and echo DEFINED; or echo NOT_DEFINED",
		[]string{fakeBinDir, "/usr/bin", "/bin"})

	assert.Equal(t, 0, code, "stdout: %s\nstderr: %s", stdout, stderr)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "DEFINED", "a healthy comrade's completion script must still be loaded")
}

func TestFishCompletionsScriptIsSyntacticallyValid(t *testing.T) {
	fishPath, err := exec.LookPath("fish")
	if err != nil {
		t.Skip("fish not found on PATH; skipping fish syntax check")
	}

	path := filepath.Join(t.TempDir(), "completions.fish")
	require.NoError(t, os.WriteFile(path, []byte(shellinit.FishCompletionsScript()), 0o644))

	cmd := exec.Command(fishPath, "-n", path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "fish -n snippets/fish-completions.fish failed: %s", out)
}
