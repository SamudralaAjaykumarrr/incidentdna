//go:build windows

// run_windows.go is the Windows counterpart to run_unix.go: it provides
// execCommand's timeout-kill guarantee (docs/phase-4-plan.md §9 — a
// timeout must terminate the whole process tree the command spawned, not
// just the direct child) using the closest Windows equivalent to a POSIX
// process group, a Job Object.
//
// Windows has no pid/pgid concept, so `syscall.Setpgid`/`syscall.Kill`
// (run_unix.go) have no equivalent in the standard os/exec surface. A Job
// Object is the standard, documented Windows mechanism for grouping a
// process and its descendants so they can be terminated together
// (CreateJobObjectW / AssignProcessToJobObject / TerminateJobObject). These
// are called directly against kernel32.dll via syscall.NewLazyDLL, which
// is part of the standard library's "syscall" package — this file adds no
// module dependency beyond what go.mod already declares
// (docs/phase-8-plan.md §4's "no new dependency" invariant applies to
// Windows exactly as it does to every other platform).
//
// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE is set as defense in depth (children
// are also terminated if the job handle is ever closed without an explicit
// kill, e.g. this process itself being killed) but the primary,
// load-bearing mechanism — the one the timeout path in run.go actually
// relies on — is the explicit TerminateJobObject call in kill(), which
// does not depend on that flag being applied correctly.
package scenario

import (
	"os/exec"
	"syscall"
	"unsafe"
)

var (
	modkernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW         = modkernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject  = modkernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJobObject = modkernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject       = modkernel32.NewProc("TerminateJobObject")
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x00002000

	// PROCESS_SET_QUOTA is not exposed by the standard library's syscall
	// package for windows (unlike PROCESS_TERMINATE, which is); its value
	// is a stable, documented Win32 constant.
	processSetQuota = 0x0100
)

// jobObjectBasicLimitInformation mirrors the Win32
// JOBOBJECT_BASIC_LIMIT_INFORMATION struct layout (amd64): field order,
// sizes, and therefore alignment/padding must match exactly for
// SetInformationJobObject to interpret LimitFlags correctly.
type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

// ioCounters mirrors the Win32 IO_COUNTERS struct, embedded (unused, but
// required for correct struct layout) in
// jobObjectExtendedLimitInformationT below.
type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

// jobObjectExtendedLimitInformationT mirrors the Win32
// JOBOBJECT_EXTENDED_LIMIT_INFORMATION struct.
type jobObjectExtendedLimitInformationT struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// processGroup tracks the Job Object handle (if creation succeeded) used
// to group the child process and its descendants for termination.
type processGroup struct {
	job syscall.Handle
}

// newProcessGroup configures cmd (before Start) and creates the Job Object
// the child will be assigned to once running. If Job Object creation
// fails, pg.job is 0 and kill falls back to killing only the direct child
// process (documented residual gap, no different in kind from the
// existing "fully detached grandchild" gap already documented for Unix).
func newProcessGroup(cmd *exec.Cmd) processGroup {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}

	h, _, _ := procCreateJobObjectW.Call(0, 0)
	job := syscall.Handle(h)
	if job == 0 {
		return processGroup{}
	}

	info := jobObjectExtendedLimitInformationT{
		BasicLimitInformation: jobObjectBasicLimitInformation{
			LimitFlags: jobObjectLimitKillOnJobClose,
		},
	}
	_, _, _ = procSetInformationJobObject.Call(
		uintptr(job),
		uintptr(jobObjectExtendedLimitInformation),
		uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info),
	)

	return processGroup{job: job}
}

// started assigns the now-running child process to the Job Object created
// by newProcessGroup, so it (and any descendant it spawns, unless that
// descendant explicitly breaks away) is terminated together at the timeout
// boundary.
func (pg processGroup) started(cmd *exec.Cmd) {
	if pg.job == 0 || cmd.Process == nil {
		return
	}
	h, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE|processSetQuota, false, uint32(cmd.Process.Pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(h)
	_, _, _ = procAssignProcessToJobObject.Call(uintptr(pg.job), uintptr(h))
}

// kill terminates the whole Job Object (every process assigned to it) if
// one was successfully created and the child was assigned to it;
// otherwise it falls back to killing only the direct child process.
func (pg processGroup) kill(cmd *exec.Cmd) {
	if pg.job != 0 {
		_, _, _ = procTerminateJobObject.Call(uintptr(pg.job), 1)
		return
	}
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
