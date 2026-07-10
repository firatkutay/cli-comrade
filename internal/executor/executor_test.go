package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newUnixExecutor(stdout, stderr *bytes.Buffer) *Executor {
	return newForGOOS("linux", stdout, stderr)
}

// sleepThenTouchCmd returns a command string, appropriate for the host
// running the test (runtime.GOOS), that sleeps for seconds and then
// creates marker — used by the timeout/cancel-kill tests below, which
// must run their long-lived command through whichever shell the
// *real* platform's Executor actually invokes (sh on Unix, powershell
// on Windows — see buildCommand) so the kill behavior they exercise is
// the one that actually ships on that OS.
func sleepThenTouchCmd(seconds int, marker string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Start-Sleep -Seconds %d; New-Item -ItemType File -Path '%s' -Force | Out-Null", seconds, marker)
	}
	return fmt.Sprintf("sleep %d && touch %s", seconds, marker)
}

// killedExitCode is the Result.ExitCode a killed process reports, which
// is itself platform-dependent: killProcessGroup on Unix sends SIGKILL,
// and exec.ExitError.ExitCode() reports -1 for a signal-terminated
// process; on Windows, killProcessGroup calls Process.Kill(), which the
// standard library implements via TerminateProcess(handle, 1), so the
// killed process's own reported exit code is 1, not -1 (see
// os/exec_windows.go's Process.signal). Both are asserted exactly — no
// weakening — this constant just captures the platform difference in one
// place.
func killedExitCode() int {
	if runtime.GOOS == "windows" {
		return 1
	}
	return -1
}

func TestRunCapturesStdoutAndExitCodeZero(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := newUnixExecutor(&stdout, &stderr)

	res, err := e.Run(context.Background(), "echo hi", Options{})

	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode)
	assert.Contains(t, res.Stdout, "hi")
	assert.False(t, res.TimedOut)
	assert.False(t, res.Canceled)
}

func TestRunLiveStreamsToProvidedWriters(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := newUnixExecutor(&stdout, &stderr)

	_, err := e.Run(context.Background(), "echo out1; echo err1 1>&2", Options{})

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "out1", "live stdout writer must receive the child's output in real time")
	assert.Contains(t, stderr.String(), "err1", "live stderr writer must receive the child's output in real time")
}

func TestRunReportsExitCodeThree(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := newUnixExecutor(&stdout, &stderr)

	res, err := e.Run(context.Background(), "exit 3", Options{})

	require.NoError(t, err)
	assert.Equal(t, 3, res.ExitCode)
}

func TestRunReportsFalseAsExitCodeOne(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := newUnixExecutor(&stdout, &stderr)

	res, err := e.Run(context.Background(), "false", Options{})

	require.NoError(t, err)
	assert.Equal(t, 1, res.ExitCode)
}

func TestRunCapturesStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := newUnixExecutor(&stdout, &stderr)

	res, err := e.Run(context.Background(), "echo boom 1>&2", Options{})

	require.NoError(t, err)
	assert.Contains(t, res.Stderr, "boom")
}

func TestRunCapCapturesOnlyTailOfLongOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := newUnixExecutor(&stdout, &stderr)

	// Print far more than maxCaptureBytes so the tail-truncation actually
	// engages; the last marker must survive, an early one must not.
	cmd := "for i in $(seq 1 5000); do printf 'line-%05d-XXXXXXXXXXXXXXXXXXXX\\n' \"$i\"; done"
	res, err := e.Run(context.Background(), cmd, Options{})

	require.NoError(t, err)
	assert.LessOrEqual(t, len(res.Stdout), maxCaptureBytes)
	assert.Contains(t, res.Stdout, "line-05000", "the tail of the output must be preserved")
	assert.NotContains(t, res.Stdout, "line-00001-", "an early line must have been truncated away")
}

func TestRunTimeoutKillsProcessAndReportsTimedOut(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// Uses the real host's Executor (New, not newUnixExecutor) so this
	// exercises the actual sh-vs-powershell + kill path that ships on
	// whichever OS the test binary is running on — see
	// sleepThenTouchCmd/killedExitCode above.
	e := New(&stdout, &stderr)

	markerDir := t.TempDir()
	marker := filepath.Join(markerDir, "finished")
	cmd := sleepThenTouchCmd(5, marker)

	start := time.Now()
	res, err := e.Run(context.Background(), cmd, Options{Timeout: 200 * time.Millisecond})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.True(t, res.TimedOut)
	assert.False(t, res.Canceled)
	assert.Equal(t, killedExitCode(), res.ExitCode)
	assert.Less(t, elapsed, 3*time.Second, "Run must return promptly after the timeout, not after the full sleep")

	// Give the (already-killed) process a moment it would need if it were
	// somehow still alive, then confirm the marker file the `sleep`
	// disjunct would have created was never written — proof the process
	// was actually killed, not merely abandoned to finish in the
	// background after Run returned.
	time.Sleep(300 * time.Millisecond)
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "the process must have been killed before it could run past the timeout")
}

func TestRunCancelKillsProcessAndReportsCanceled(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// Uses the real host's Executor (New, not newUnixExecutor) — see
	// TestRunTimeoutKillsProcessAndReportsTimedOut's comment.
	e := New(&stdout, &stderr)

	markerDir := t.TempDir()
	marker := filepath.Join(markerDir, "finished")
	cmd := sleepThenTouchCmd(5, marker)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	res, err := e.Run(ctx, cmd, Options{})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.True(t, res.Canceled)
	assert.False(t, res.TimedOut)
	assert.Equal(t, killedExitCode(), res.ExitCode)
	assert.Less(t, elapsed, 3*time.Second, "Run must return promptly after ctx is canceled")

	time.Sleep(300 * time.Millisecond)
	_, statErr := os.Stat(marker)
	assert.True(t, os.IsNotExist(statErr), "the process must have been killed before it could run past the cancellation")
}

func TestRunNeverPrependsSudoOrElevation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := newUnixExecutor(&stdout, &stderr)

	// "echo $0 $@" run through `sh -c` prints the shell's own name and no
	// extra leading argument — proving the command was executed verbatim
	// with nothing prepended.
	res, err := e.Run(context.Background(), "echo verbatim-check", Options{})

	require.NoError(t, err)
	assert.NotContains(t, res.Stdout, "sudo")
	assert.Equal(t, "verbatim-check\n", res.Stdout)
}

func TestBuildCommandUsesShOnUnix(t *testing.T) {
	e := newForGOOS("linux", nil, nil)
	name, args := e.buildCommand("echo hi")
	assert.Equal(t, "sh", name)
	assert.Equal(t, []string{"-c", "echo hi"}, args)
}

func TestBuildCommandUsesPowerShellOnWindows(t *testing.T) {
	e := newForGOOS("windows", nil, nil)
	name, args := e.buildCommand("Get-Date")
	assert.Equal(t, "powershell", name)
	assert.Equal(t, []string{"-NoProfile", "-Command", "Get-Date"}, args)
}

// TestRunOnWindowsRunsPowerShellForReal is skipped on any non-Windows
// runner (this project's actual CI matrix does include windows — see
// docs/history/UYGULAMA_PLANI.md FAZ 11 — but this WSL2/Linux development environment
// does not) or when a powershell binary genuinely is not reachable, per
// the task's "guard tests with runtime.GOOS or LookPath(powershell)"
// instruction.
func TestRunOnWindowsRunsPowerShellForReal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires an actual Windows host")
	}
	if _, err := exec.LookPath("powershell"); err != nil {
		t.Skip("powershell not found on PATH")
	}

	var stdout, stderr bytes.Buffer
	e := New(&stdout, &stderr)
	res, runErr := e.Run(context.Background(), "Write-Output hi", Options{})

	require.NoError(t, runErr)
	assert.Equal(t, 0, res.ExitCode)
	assert.Contains(t, res.Stdout, "hi")
}

func TestCapWriterTruncatesToTail(t *testing.T) {
	w := newCapWriter(5)
	_, err := w.Write([]byte("abcdefgh"))
	require.NoError(t, err)
	assert.Equal(t, "defgh", w.String())
}

func TestOSStdoutStderrReturnsProcessStreams(t *testing.T) {
	stdout, stderr := osStdoutStderr()
	assert.Same(t, os.Stdout, stdout)
	assert.Same(t, os.Stderr, stderr)
}
