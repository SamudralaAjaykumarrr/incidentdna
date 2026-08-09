// run.go implements the local, bounded, offline scenario runner
// (docs/phase-4-plan.md §8/§9/§13): workspace creation and workspace_files
// staging, exec.CommandContext-free process-group-based invocation with no
// shell interpretation and no implicit $PATH lookup, timeout enforcement,
// bounded/stream-separated stdout/stderr capture, and classification into
// one of PASS/FAIL/TIMEOUT/INVALID/INTERNAL_ERROR.
//
// Run performs the same pre-execution checks scenario verify performs
// (Validate) before ever creating a workspace or starting a process — a
// scenario that fails those checks is classified INVALID and nothing is
// executed (docs/phase-4-plan.md §14).
package scenario

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Outcome is one of the five classifications scenario run reports
// (docs/phase-4-plan.md §14).
type Outcome string

const (
	OutcomePass          Outcome = "PASS"
	OutcomeFail          Outcome = "FAIL"
	OutcomeTimeout       Outcome = "TIMEOUT"
	OutcomeInvalid       Outcome = "INVALID"
	OutcomeInternalError Outcome = "INTERNAL_ERROR"
)

// StreamCapture is one captured, bounded output stream.
type StreamCapture struct {
	Excerpt   string
	Truncated bool
}

// RunOptions configures Run's optional, non-default behavior. The zero
// value is the safe default: a fresh os.MkdirTemp workspace, removed after
// the run completes.
type RunOptions struct {
	// WorkspaceDir, if non-empty, is used as the run's workspace instead of
	// a freshly created temporary directory. It must not already exist as a
	// non-empty directory (docs/phase-4-plan.md §9/§17).
	WorkspaceDir string
	// KeepWorkspace, if true, leaves the workspace directory in place after
	// the run completes (any outcome) instead of removing it.
	KeepWorkspace bool
}

// RunResult is the outcome of one Run call.
type RunResult struct {
	Outcome           Outcome
	ScenarioID        string
	LinkedFingerprint string
	ExitCodeExpected  int
	ExitCodeActual    int
	DurationMs        int64
	Stdout            StreamCapture
	Stderr            StreamCapture
	// WorkspacePath is non-empty only when RunOptions.KeepWorkspace was set.
	WorkspacePath string
	// Error is non-empty only for OutcomeInvalid/OutcomeInternalError.
	Error string
}

// Run executes doc's declared command exactly once and classifies the
// result. scenarioDir is the directory containing the scenario file itself,
// used both by the pre-execution Validate call and to resolve
// workspace_files sources.
func Run(ctx context.Context, doc *Document, scenarioDir string, opts RunOptions) RunResult {
	result := RunResult{ScenarioID: doc.Scenario.ID, LinkedFingerprint: doc.LinkedFingerprint}
	if doc.Expected.ExitCode != nil {
		result.ExitCodeExpected = *doc.Expected.ExitCode
	}

	verify := Validate(ctx, doc, scenarioDir)
	if !verify.Valid() {
		result.Outcome = OutcomeInvalid
		result.Error = verify.Error()
		return result
	}

	workspaceRoot, cleanup, err := prepareWorkspace(opts.WorkspaceDir)
	if err != nil {
		result.Outcome = OutcomeInternalError
		result.Error = err.Error()
		return result
	}
	keptWorkspace := false
	defer func() {
		if !keptWorkspace {
			cleanup()
		}
	}()
	finish := func() {
		if opts.KeepWorkspace {
			keptWorkspace = true
			result.WorkspacePath = workspaceRoot
		}
	}

	if err := stageWorkspaceFiles(doc.Execution.WorkspaceFiles, scenarioDir, workspaceRoot); err != nil {
		result.Outcome = OutcomeInternalError
		result.Error = err.Error()
		finish()
		return result
	}

	cmd0, err := resolveCommandPath(doc.Execution.Command[0], workspaceRoot)
	if err != nil {
		result.Outcome = OutcomeInternalError
		result.Error = err.Error()
		finish()
		return result
	}

	timeout := time.Duration(EffectiveTimeoutSeconds(doc)) * time.Second
	execRes := execCommand(ctx, cmd0, doc.Execution.Command[1:], doc.Execution.Env, workspaceRoot, timeout)

	result.DurationMs = execRes.duration.Milliseconds()
	result.Stdout = execRes.stdout
	result.Stderr = execRes.stderr
	finish()

	switch execRes.kind {
	case execOutcomeInternalError:
		result.Outcome = OutcomeInternalError
		result.Error = execRes.err.Error()
		return result
	case execOutcomeTimeout:
		result.Outcome = OutcomeTimeout
		result.ExitCodeActual = execRes.exitCode
		return result
	}

	result.ExitCodeActual = execRes.exitCode
	if matchesExpected(doc.Expected, execRes.exitCode, execRes.stdout, execRes.stderr) {
		result.Outcome = OutcomePass
	} else {
		result.Outcome = OutcomeFail
	}
	return result
}

// prepareWorkspace resolves the run's workspace root and returns a cleanup
// function that removes it. workspaceDir == "" creates a fresh
// os.MkdirTemp directory. A non-empty workspaceDir must either not yet
// exist (it is created) or exist as an empty, non-symlinked directory —
// otherwise ErrWorkspaceNotEmpty is returned and nothing is written
// (docs/phase-4-plan.md §9/§17).
func prepareWorkspace(workspaceDir string) (root string, cleanup func(), err error) {
	if workspaceDir == "" {
		root, err = os.MkdirTemp("", "incidentdna-scenario-*")
		if err != nil {
			return "", nil, fmt.Errorf("create temporary workspace: %w", err)
		}
		return root, func() { os.RemoveAll(root) }, nil
	}

	abs, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", nil, fmt.Errorf("resolve --workspace %q: %w", workspaceDir, err)
	}
	abs = filepath.Clean(abs)

	info, statErr := os.Lstat(abs)
	switch {
	case statErr == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("--workspace %q must not be a symbolic link", workspaceDir)
		}
		if !info.IsDir() {
			return "", nil, fmt.Errorf("--workspace %q exists and is not a directory", workspaceDir)
		}
		entries, rerr := os.ReadDir(abs)
		if rerr != nil {
			return "", nil, fmt.Errorf("read --workspace %q: %w", workspaceDir, rerr)
		}
		if len(entries) > 0 {
			return "", nil, fmt.Errorf("%w: %q", ErrWorkspaceNotEmpty, workspaceDir)
		}
	case errors.Is(statErr, fs.ErrNotExist):
		if merr := os.MkdirAll(abs, 0o755); merr != nil {
			return "", nil, fmt.Errorf("create --workspace %q: %w", workspaceDir, merr)
		}
	default:
		return "", nil, fmt.Errorf("stat --workspace %q: %w", workspaceDir, statErr)
	}

	return abs, func() { os.RemoveAll(abs) }, nil
}

// stageWorkspaceFiles copies every declared workspace_files entry's source
// bytes (resolved relative to scenarioDir) verbatim to its destination
// (resolved relative to workspaceRoot), re-checking path containment,
// source existence/regularity, and the §10 size limits immediately before
// each read/write — a TOCTOU re-check of what Validate already confirmed
// moments earlier (docs/phase-4-plan.md §17, §19).
func stageWorkspaceFiles(files []WorkspaceFile, scenarioDir, workspaceRoot string) error {
	var total int64
	for _, wf := range files {
		srcPath := filepath.Clean(filepath.Join(scenarioDir, filepath.Clean(wf.Source)))
		destPath := filepath.Clean(filepath.Join(workspaceRoot, filepath.Clean(wf.Destination)))

		rootWithSep := workspaceRoot + string(filepath.Separator)
		if destPath != workspaceRoot && !strings.HasPrefix(destPath, rootWithSep) {
			return fmt.Errorf("workspace_files destination %q escaped the resolved workspace root", wf.Destination)
		}

		info, err := os.Lstat(srcPath)
		if err != nil {
			return fmt.Errorf("%w: %s: %v", ErrWorkspaceFileMissing, wf.Source, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: %s is not a regular file", ErrWorkspaceFileMissing, wf.Source)
		}
		if err := checkWorkspaceFileSize(wf.Source, info.Size()); err != nil {
			return err
		}
		total += info.Size()
		if err := checkWorkspaceTotalBytes(total); err != nil {
			return err
		}

		data, err := os.ReadFile(srcPath)
		if err != nil {
			return fmt.Errorf("read workspace_files source %s: %w", wf.Source, err)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create workspace directory for %s: %w", wf.Destination, err)
		}
		if destInfo, lerr := os.Lstat(destPath); lerr == nil && destInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace_files destination %q already exists as a symbolic link", wf.Destination)
		}
		if err := os.WriteFile(destPath, data, 0o644); err != nil {
			return fmt.Errorf("write workspace_files destination %s: %w", wf.Destination, err)
		}
	}
	return nil
}

// resolveCommandPath resolves cmd0 (already confirmed by Validate to be
// either absolute or containing a path separator) to an absolute path —
// used as-is if already absolute, otherwise joined against workspaceRoot —
// and confirms it exists, is not a directory, and has at least one
// executable bit set, as early as possible, before any process is started
// (docs/phase-4-plan.md §17).
func resolveCommandPath(cmd0, workspaceRoot string) (string, error) {
	resolved := cmd0
	if !filepath.IsAbs(cmd0) {
		resolved = filepath.Join(workspaceRoot, cmd0)
	}
	resolved = filepath.Clean(resolved)

	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %v", ErrCommandNotExecutable, resolved, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%w: %s is a directory", ErrCommandNotExecutable, resolved)
	}
	if info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%w: %s has no executable permission bit set", ErrCommandNotExecutable, resolved)
	}
	return resolved, nil
}

// execOutcomeKind classifies how execCommand's underlying exec.Cmd
// finished.
type execOutcomeKind int

const (
	execOutcomeCompleted execOutcomeKind = iota
	execOutcomeTimeout
	execOutcomeInternalError
)

type execResult struct {
	kind     execOutcomeKind
	exitCode int
	stdout   StreamCapture
	stderr   StreamCapture
	duration time.Duration
	err      error
}

// execCommand launches path/args as a child process with no shell
// interpretation, a fixed cwd (workspaceRoot), a fixed minimal environment
// plus declaredEnv, and bounded, stream-separated stdout/stderr capture. It
// runs the child in its own process group (via newProcessGroup, OS-specific
// — see run_unix.go/run_windows.go) so a timeout expiry kills the whole
// process tree the child spawned, not just the direct child
// (docs/phase-4-plan.md §9's "process tree" guarantee, with the documented
// residual gap for a fully detached grandchild).
func execCommand(ctx context.Context, path string, args []string, declaredEnv map[string]string, workspaceRoot string, timeout time.Duration) execResult {
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.Command(path, args...)
	cmd.Dir = workspaceRoot
	cmd.Env = buildEnv(workspaceRoot, declaredEnv)
	stdoutBuf := newBoundedWriter(MaxScenarioOutputBytes)
	stderrBuf := newBoundedWriter(MaxScenarioOutputBytes)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf
	pg := newProcessGroup(cmd)

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return execResult{kind: execOutcomeInternalError, err: fmt.Errorf("start command: %w", err), duration: time.Since(start)}
	}
	pg.started(cmd)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var waitErr error
	canceled := false
	select {
	case waitErr = <-done:
	case <-execCtx.Done():
		canceled = true
		// Kill the whole process group/job, not just the direct child, so
		// children the command itself spawned are also terminated at the
		// timeout boundary.
		pg.kill(cmd)
		waitErr = <-done
	}
	duration := time.Since(start)
	stdout := stdoutBuf.capture()
	stderr := stderrBuf.capture()

	if canceled {
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return execResult{kind: execOutcomeTimeout, exitCode: -1, stdout: stdout, stderr: stderr, duration: duration}
		}
		return execResult{kind: execOutcomeInternalError, err: fmt.Errorf("execution canceled: %w", execCtx.Err()), stdout: stdout, stderr: stderr, duration: duration}
	}

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			return execResult{kind: execOutcomeInternalError, err: fmt.Errorf("wait for command: %w", waitErr), stdout: stdout, stderr: stderr, duration: duration}
		}
	}
	return execResult{kind: execOutcomeCompleted, exitCode: exitCode, stdout: stdout, stderr: stderr, duration: duration}
}

// buildEnv constructs the child process's environment: a fixed minimal base
// (PATH unset, HOME pointed at the workspace, TZ=UTC) plus declared, in
// deterministic (key-sorted) order. The invoking process's own os.Environ()
// is never consulted (docs/phase-4-plan.md §5/§7).
func buildEnv(workspaceRoot string, declared map[string]string) []string {
	base := map[string]string{
		"HOME": workspaceRoot,
		"TZ":   "UTC",
	}
	for k, v := range declared {
		base[k] = v
	}
	keys := make([]string, 0, len(base))
	for k := range base {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, k+"="+base[k])
	}
	return env
}

// boundedWriter is an io.Writer that caps total accumulated bytes at limit:
// once the cap is reached, further writes are discarded (not buffered) and
// Truncated is set. Memory use never exceeds limit, not merely the final
// reported buffer size (docs/phase-4-plan.md §9, "Output volume").
type boundedWriter struct {
	limit     int
	buf       bytes.Buffer
	truncated bool
}

func newBoundedWriter(limit int) *boundedWriter {
	return &boundedWriter{limit: limit}
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	if w.buf.Len() >= w.limit {
		w.truncated = true
		return len(p), nil
	}
	remaining := w.limit - w.buf.Len()
	if len(p) > remaining {
		w.buf.Write(p[:remaining])
		w.truncated = true
		return len(p), nil
	}
	w.buf.Write(p)
	return len(p), nil
}

func (w *boundedWriter) capture() StreamCapture {
	return StreamCapture{Excerpt: w.buf.String(), Truncated: w.truncated}
}

// matchesExpected reports whether an actual execution outcome matches every
// declared expected field: exit code always, stdout/stderr only if
// declared.
func matchesExpected(expected Expected, exitCode int, stdout, stderr StreamCapture) bool {
	if expected.ExitCode == nil || exitCode != *expected.ExitCode {
		return false
	}
	if !matchesStream(expected.Stdout, stdout.Excerpt) {
		return false
	}
	if !matchesStream(expected.Stderr, stderr.Excerpt) {
		return false
	}
	return true
}

func matchesStream(assertion *StreamAssertion, captured string) bool {
	if assertion == nil {
		return true
	}
	switch assertion.Mode {
	case StreamModeExact:
		return captured == assertion.Value
	case StreamModeContains:
		return strings.Contains(captured, assertion.Value)
	case StreamModeRegex:
		re, err := regexp.Compile(assertion.Value)
		if err != nil {
			return false
		}
		return re.MatchString(captured)
	default:
		return false
	}
}
