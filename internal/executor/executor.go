// Package executor runs a single generated shell command on the host
// platform, streaming its stdout/stderr live to the caller while also
// capturing a tail-truncated copy of each, and reports its exit code,
// duration, and whether it was killed by a per-step timeout or context
// cancellation (Ctrl-C).
//
// Per CLAUDE.md's "Platform dallanmaları build tag ile DEĞİL, runtime
// runtime.GOOS + internal/executor soyutlamasıyla" rule, every branch that
// *can* be decided at runtime is (see buildCommand). The one narrow
// exception is process-group setup/teardown (setProcAttr/killProcessGroup,
// in executor_unix.go/executor_windows.go): syscall.SysProcAttr is a
// different Go type per GOOS (its Setpgid field does not exist on
// windows), which is a compile-time distinction the Go type system forces,
// not a design choice — the same reason the standard library itself splits
// this exact concern into GOOS-suffixed files.
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// maxCaptureBytes is the tail-truncation cap applied independently to the
// captured (not live-streamed) copy of stdout and stderr: 8KB each, per
// UYGULAMA_PLANI.md FAZ 6 item 1.
const maxCaptureBytes = 8 * 1024

// Result is the outcome of one Executor.Run call.
type Result struct {
	// ExitCode is the child process's exit code, or -1 when it never
	// exited normally (killed by TimedOut/Canceled, or failed to start —
	// Run itself returns a non-nil error in the failed-to-start case, so
	// callers should treat a returned error as authoritative over a -1
	// ExitCode).
	ExitCode int
	Duration time.Duration
	// Stdout/Stderr are the tail-truncated (maxCaptureBytes each)
	// captured output — independent of whatever live streaming Run also
	// performed to the Executor's configured live writers.
	Stdout string
	Stderr string
	// TimedOut is true when opts.Timeout elapsed before the command
	// exited, and Run killed its process group as a result.
	TimedOut bool
	// Canceled is true when ctx was canceled (e.g. Ctrl-C) before the
	// command exited, and Run killed its process group as a result.
	Canceled bool
}

// Options configures a single Run call.
type Options struct {
	// Timeout is the maximum time the command may run before Run kills
	// its process group and returns with Result.TimedOut set. Zero means
	// no timeout.
	Timeout time.Duration
}

// Executor runs commands via a POSIX shell on Unix (runtime.GOOS !=
// "windows") or PowerShell on Windows. It holds no global state: the live
// stdout/stderr writers are injected at construction (New), and goos is
// fixed at construction so tests can exercise the Windows command-building
// branch on a non-Windows CI runner without an actual Windows host — see
// newForGOOS in executor_test.go.
type Executor struct {
	liveStdout io.Writer
	liveStderr io.Writer
	goos       string
}

// New builds an Executor for the host platform (runtime.GOOS). liveStdout
// and liveStderr receive the child's output in real time as it runs, in
// addition to (never instead of) the tail-truncated copy Result carries;
// either may be nil, in which case that stream is not live-streamed
// anywhere (os.Stdout/os.Stderr are not assumed implicitly — the caller
// decides, e.g. cmd.OutOrStdout() in tests).
func New(liveStdout, liveStderr io.Writer) *Executor {
	return newForGOOS(runtime.GOOS, liveStdout, liveStderr)
}

// newForGOOS is New's OS-injectable constructor, used directly by this
// package's own tests to exercise the Windows command-building branch
// (buildCommand) without requiring an actual Windows host.
func newForGOOS(goos string, liveStdout, liveStderr io.Writer) *Executor {
	if liveStdout == nil {
		liveStdout = io.Discard
	}
	if liveStderr == nil {
		liveStderr = io.Discard
	}
	return &Executor{liveStdout: liveStdout, liveStderr: liveStderr, goos: goos}
}

// buildCommand returns the shell binary and argv used to run command:
// `sh -c <command>` on Unix, `powershell -NoProfile -Command <command>` on
// Windows. It never prepends sudo, runas, or any other elevation verb —
// per UYGULAMA_PLANI.md FAZ 6 item 1 ("Elevated adımlar: komutu SUDO İLE
// ÇALIŞTIRMA"), an elevated step runs exactly as the plan wrote it. If that
// plan step needs sudo, the model put `sudo` in the command text itself;
// if it doesn't, and the underlying operation truly requires elevation, the
// command fails with the OS's own permission-denied error and the user
// sees that failure — this package never auto-escalates on their behalf.
func (e *Executor) buildCommand(command string) (name string, args []string) {
	if e.goos == "windows" {
		return "powershell", []string{"-NoProfile", "-Command", command}
	}
	return "sh", []string{"-c", command}
}

// Run executes command and blocks until it exits, ctx is canceled, or
// opts.Timeout elapses (whichever happens first). On timeout or
// cancellation, Run kills the command's entire process group (not just the
// direct child) so that any children it spawned die too, then waits for
// the process to actually be reaped before returning — Result.TimedOut/
// Canceled is never reported without the kill having already completed.
//
// Run's own returned error is non-nil only when the command could not even
// be started (e.g. the shell binary itself is missing) — a nonzero exit
// code, a timeout, or a cancellation are all reported through Result, not
// through this return value, so callers can always inspect Result even
// when err is nil.
func (e *Executor) Run(ctx context.Context, command string, opts Options) (Result, error) {
	runCtx := ctx
	if opts.Timeout > 0 {
		var cancelTimeout context.CancelFunc
		runCtx, cancelTimeout = context.WithTimeout(ctx, opts.Timeout)
		defer cancelTimeout()
	}

	name, args := e.buildCommand(command)
	cmd := exec.Command(name, args...) //nolint:gosec,noctx // gosec: command text comes from a plan step the mode loop has already run through internal/safety; this package's job is only to execute it, per its doc comment. noctx: exec.CommandContext's automatic-SIGKILL-on-cancel is deliberately NOT used here — Run manages runCtx cancellation itself (see the select below) so it can kill the whole process GROUP via killProcessGroup, not just the direct child.
	setProcAttr(cmd)

	outCap := newCapWriter(maxCaptureBytes)
	errCap := newCapWriter(maxCaptureBytes)
	cmd.Stdout = io.MultiWriter(e.liveStdout, outCap)
	cmd.Stderr = io.MultiWriter(e.liveStderr, errCap)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return Result{ExitCode: -1, Duration: time.Since(start)}, fmt.Errorf("executor: start command: %w", err)
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	var timedOut, canceled bool
	var waitErr error
	select {
	case waitErr = <-waitDone:
	case <-runCtx.Done():
		killProcessGroup(cmd)
		waitErr = <-waitDone // block until the killed process is actually reaped
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			timedOut = true
		} else {
			canceled = true
		}
	}

	return Result{
		ExitCode: extractExitCode(waitErr, timedOut || canceled),
		Duration: time.Since(start),
		Stdout:   outCap.String(),
		Stderr:   errCap.String(),
		TimedOut: timedOut,
		Canceled: canceled,
	}, nil
}

// extractExitCode derives Result.ExitCode from cmd.Wait's error: nil means
// a clean exit 0 (unless killed is true, in which case -1 — the process
// never got to report its own code); a *exec.ExitError reports its own
// ExitCode() (which is itself -1 when the process was terminated by a
// signal rather than exiting normally, e.g. our own SIGKILL); any other
// error (a genuinely unexpected exec failure) also reports -1.
func extractExitCode(waitErr error, killed bool) int {
	if waitErr == nil {
		if killed {
			return -1
		}
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// capWriter is an io.Writer that keeps only the trailing limit bytes ever
// written to it — a tail-truncating ring buffer, used to cap Result's
// captured Stdout/Stderr independently of whatever else the command
// writes. Safe for concurrent use: cmd.Stdout/cmd.Stderr may be written by
// the child process's own separate OS-level pipe-reader goroutines inside
// os/exec.
type capWriter struct {
	mu    sync.Mutex
	buf   []byte
	limit int
}

func newCapWriter(limit int) *capWriter {
	return &capWriter{limit: limit}
}

func (w *capWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if len(w.buf) > w.limit {
		w.buf = w.buf[len(w.buf)-w.limit:]
	}
	return len(p), nil
}

func (w *capWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.buf)
}

// osStdoutStderr is a small convenience New callers outside this package's
// own tests are expected to use: the live-streaming writers wired to the
// process's own inherited stdout/stderr, for the common "actually run this
// on the user's terminal" case.
func osStdoutStderr() (stdout, stderr io.Writer) {
	return os.Stdout, os.Stderr
}
