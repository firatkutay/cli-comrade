package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

// exeSuffix is the file-extension the OS this test binary is actually
// running on requires for an executable to be directly exec-able —
// ".exe" on Windows, "" everywhere else. Every test in this package that
// go-builds cmd/comrade and then execs the result (buildComradeBinary's
// callers) needs this, so it lives in one shared place rather than being
// duplicated per test.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// repoRoot resolves the module root from this test file's package
// directory (internal/cli), so the e2e test can `go build` the real
// comrade binary and locate scripts/ regardless of the working
// directory the test binary happens to run from.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolved repo root %s does not contain go.mod: %v", root, err)
	}
	return root
}

// buildComradeBinary builds the real comrade binary (the same
// cmd/comrade this repo ships) into a fresh temp directory and returns
// its path, so the bash E2E test below drives the actual "comrade hook
// record" code path rather than a stand-in.
func buildComradeBinary(t *testing.T) string {
	t.Helper()
	root := repoRoot(t)
	binPath := filepath.Join(t.TempDir(), "comrade"+exeSuffix())

	build := exec.Command("go", "build", "-o", binPath, "./cmd/comrade")
	build.Dir = root
	out, err := build.CombinedOutput()
	require.NoError(t, err, "go build ./cmd/comrade failed: %s", out)
	return binPath
}

// TestBashE2EHookRecordsFailedCommand is FAZ 4's linux-CI-safe bash
// end-to-end proof: it sources the real embedded bash snippet in an
// actual bash process, runs a failing command, fires the prompt hook,
// and asserts last_command.json (written by the real comrade binary)
// contains the failing command and its exit code.
//
// bash's history list is only auto-populated by the interactive
// readline loop, not by commands a script executes — so `history 1`
// would see nothing from a plain `false` run inside `bash -c`. This
// test makes that deterministic without a PTY by explicitly seeding
// the history list (`history -s "false"`) right after running the
// failing command, restoring $? to the failing command's exit code
// through a subshell (`( exit "$st" )`, whose own exit status becomes
// $st), and only then evaluating $PROMPT_COMMAND — exactly mirroring
// what an interactive shell does after each entered command, without
// needing one. See docs/phases/FAZ-04.md for this rationale in full.
func TestBashE2EHookRecordsFailedCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		// This test drives a real Unix bash hook end-to-end (history
		// seeding, $PROMPT_COMMAND semantics) that only make sense for
		// the bash/zsh/fish snippets; Windows users get the PowerShell
		// hook instead (internal/shellinit's powershell snippet, which
		// is golden-tested separately), so there is no equivalent
		// Windows path for this exact scenario to exercise. GitHub's
		// windows-latest runner does have a bash.exe on PATH (via Git
		// for Windows), so skip explicitly here rather than relying on
		// exec.LookPath("bash") to fail.
		t.Skip("bash hook E2E is Unix-only; Windows uses the PowerShell hook, golden-tested separately")
	}

	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found on PATH; skipping bash E2E hook test")
	}

	binPath := buildComradeBinary(t)
	binDir := filepath.Dir(binPath)

	snippetBody, err := shellinit.Snippet(shellinit.Bash)
	require.NoError(t, err)
	snippetFile := filepath.Join(t.TempDir(), "hook.sh")
	require.NoError(t, os.WriteFile(snippetFile, []byte(snippetBody), 0o644))

	stateDir := t.TempDir()
	home := t.TempDir()

	script := fmt.Sprintf(`
source %q

false
st=$?
history -s "false"
( exit "$st" )
eval "$PROMPT_COMMAND"
exit 0
`, snippetFile)

	cmd := exec.Command(bashPath, "--norc", "--noprofile", "-c", script)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"XDG_STATE_HOME="+stateDir,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "bash E2E script failed: %s", out)
	assert.Empty(t, out, "the hook must never print anything to the terminal")

	lastCmdPath := filepath.Join(stateDir, "cli-comrade", "last_command.json")
	got, ok := context.ReadLastCommand(lastCmdPath)
	require.True(t, ok, "expected last_command.json to exist and parse at %s", lastCmdPath)
	assert.Equal(t, "false", got.Command)
	assert.Equal(t, 1, got.ExitCode)
	assert.Equal(t, "bash", got.Shell)
}

// TestBashE2EHookSkipsWhenComradeNotOnPath proves the snippet's `command
// -v comrade` guard: with no comrade binary reachable, the hook must not
// write last_command.json (and must not error the shell either).
func TestBashE2EHookSkipsWhenComradeNotOnPath(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found on PATH; skipping bash E2E hook test")
	}

	snippetBody, err := shellinit.Snippet(shellinit.Bash)
	require.NoError(t, err)
	snippetFile := filepath.Join(t.TempDir(), "hook.sh")
	require.NoError(t, os.WriteFile(snippetFile, []byte(snippetBody), 0o644))

	stateDir := t.TempDir()
	home := t.TempDir()
	emptyBinDir := t.TempDir() // deliberately does not contain comrade

	script := fmt.Sprintf(`
source %q

false
st=$?
history -s "false"
( exit "$st" )
eval "$PROMPT_COMMAND"
exit 0
`, snippetFile)

	cmd := exec.Command(bashPath, "--norc", "--noprofile", "-c", script)
	cmd.Env = append([]string{}, // deliberately NOT inheriting os.Environ()'s real PATH
		"HOME="+home,
		"XDG_STATE_HOME="+stateDir,
		"PATH="+emptyBinDir,
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "bash E2E script failed: %s", out)
	assert.Empty(t, out)

	_, ok := context.ReadLastCommand(filepath.Join(stateDir, "cli-comrade", "last_command.json"))
	assert.False(t, ok, "last_command.json must not be written when comrade is not on PATH")
}
