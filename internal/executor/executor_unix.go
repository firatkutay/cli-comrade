//go:build !windows

package executor

import (
	"os/exec"
	"syscall"
)

// setProcAttr puts cmd in its own process group (Setpgid) so
// killProcessGroup can later kill the whole group — the command's own
// children, not just the immediate `sh` process — in one signal.
func setProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup sends SIGKILL to the negative PID (the process group
// setProcAttr created), which the kernel delivers to every process in that
// group. A nil cmd.Process (Start never succeeded) or an already-exited
// process (the group no longer exists) both report an error from
// syscall.Kill that is deliberately ignored here — Run's caller only cares
// that the process is no longer running by the time it checks, and its
// own cmd.Wait() call (already in flight) is what actually confirms that.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
