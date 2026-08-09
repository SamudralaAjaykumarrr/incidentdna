//go:build !windows

// run_unix.go provides the POSIX process-group implementation of
// execCommand's timeout-kill guarantee (docs/phase-4-plan.md §9): the
// child is started in its own process group (Setpgid), and a timeout
// expiry sends SIGKILL to the whole group (negative pid), not just the
// direct child. This is the exact behavior internal/scenario has had since
// Phase 4, unchanged, just relocated out of run.go so a Windows-specific
// equivalent (run_windows.go) can live alongside it without either file
// needing an unbuildable import on the other's platform.
package scenario

import (
	"os/exec"
	"syscall"
)

// processGroup tracks the OS-level grouping needed to kill an entire
// process tree, not just the direct child, at the timeout boundary.
type processGroup struct{}

// newProcessGroup configures cmd (before Start) to run in its own process
// group.
func newProcessGroup(cmd *exec.Cmd) processGroup {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return processGroup{}
}

// started is a no-op on Unix: Setpgid takes effect at fork/exec time, with
// no separate post-Start step required.
func (processGroup) started(*exec.Cmd) {}

// kill sends SIGKILL to the whole process group (negative pid) so children
// the command itself spawned are also terminated.
func (processGroup) kill(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
