//go:build windows

package executor

import "os/exec"

// setProcAttr is a no-op on Windows: os/exec's Windows SysProcAttr has no
// direct Unix-style process-group equivalent that a same-shaped
// Setpgid+negative-PID-kill translates to, and PowerShell already runs the
// command line as its own child process tree under cmd.Process. See
// killProcessGroup and docs/phases/FAZ-06.md's "Windows deferred-runtime
// note" — real Windows process-tree-kill hardening (e.g.
// CREATE_NEW_PROCESS_GROUP + a job object) is a manual/future item; the
// Windows branch is otherwise unit-guarded (t.Skip when powershell is
// absent) rather than exercised in this Linux/WSL2 development
// environment.
func setProcAttr(cmd *exec.Cmd) {}

// killProcessGroup kills the direct child process. It does not guarantee
// killing further-nested grandchildren PowerShell itself spawned — see
// setProcAttr's doc comment.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
