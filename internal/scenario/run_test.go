package scenario

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// shDoc builds a Document whose command is /bin/sh -c script, an absolute
// path (so Validate's bare-command check accepts it) — the standard fixture
// shape used throughout this file, mirroring docs/phase-4-plan.md §23's
// guidance to use small, fast, self-contained fixture commands rather than
// the built incidentdna binary.
func shDoc(script string, exitCode int) *Document {
	return &Document{
		SchemaVersion:     SupportedSchemaVersion,
		Scenario:          Info{ID: "fixture-scenario"},
		LinkedFingerprint: "sha256:" + strings.Repeat("a", 64),
		Execution: Execution{
			Command: []string{"/bin/sh", "-c", script},
		},
		Expected: Expected{ExitCode: &exitCode},
	}
}

func withTimeout(doc *Document, seconds int) *Document {
	doc.Execution.TimeoutSeconds = &seconds
	return doc
}

// ============================================================================
// PASS
// ============================================================================

func TestRun_Pass_ExitCodeAndStdoutContains(t *testing.T) {
	doc := shDoc("echo hello-world", 0)
	doc.Expected.Stdout = &StreamAssertion{Mode: StreamModeContains, Value: "hello-world"}

	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	if res.Outcome != OutcomePass {
		t.Fatalf("expected PASS, got %s (error: %s)", res.Outcome, res.Error)
	}
	if res.ExitCodeActual != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCodeActual)
	}
	if !strings.Contains(res.Stdout.Excerpt, "hello-world") {
		t.Fatalf("expected captured stdout to contain hello-world, got %q", res.Stdout.Excerpt)
	}
	if res.WorkspacePath != "" {
		t.Fatalf("expected no workspace path without --keep-workspace, got %q", res.WorkspacePath)
	}
}

// ============================================================================
// FAIL
// ============================================================================

func TestRun_Fail_ExitCodeMismatch(t *testing.T) {
	doc := shDoc("exit 3", 0)
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	if res.Outcome != OutcomeFail {
		t.Fatalf("expected FAIL, got %s", res.Outcome)
	}
	if res.ExitCodeActual != 3 {
		t.Fatalf("expected actual exit code 3, got %d", res.ExitCodeActual)
	}
}

func TestRun_Fail_StdoutExactMismatch(t *testing.T) {
	doc := shDoc("echo not-what-is-expected", 0)
	doc.Expected.Stdout = &StreamAssertion{Mode: StreamModeExact, Value: "exact-expected-value\n"}
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	if res.Outcome != OutcomeFail {
		t.Fatalf("expected FAIL, got %s", res.Outcome)
	}
}

func TestRun_Fail_StderrRegexMismatch(t *testing.T) {
	doc := shDoc("echo something-else 1>&2", 0)
	doc.Expected.Stderr = &StreamAssertion{Mode: StreamModeRegex, Value: `^ERROR: [0-9]+$`}
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	if res.Outcome != OutcomeFail {
		t.Fatalf("expected FAIL, got %s", res.Outcome)
	}
}

// ============================================================================
// TIMEOUT
// ============================================================================

func TestRun_Timeout_KillsProcessWithinBoundedWallClock(t *testing.T) {
	doc := shDoc("sleep 30", 0)
	doc = withTimeout(doc, MinScenarioTimeoutSeconds)

	start := time.Now()
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	elapsed := time.Since(start)

	if res.Outcome != OutcomeTimeout {
		t.Fatalf("expected TIMEOUT, got %s (error: %s)", res.Outcome, res.Error)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("expected the run to be bounded well under the 30s sleep, took %s", elapsed)
	}
}

// ============================================================================
// INVALID
// ============================================================================

func TestRun_Invalid_NeverExecutesAndReportsNoWorkspace(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")

	doc := shDoc("touch "+marker, 0)
	doc.Execution.Command = []string{"sh", "-c", "touch " + marker} // bare command[0]: invalid at verify time

	res := Run(context.Background(), doc, dir, RunOptions{})
	if res.Outcome != OutcomeInvalid {
		t.Fatalf("expected INVALID, got %s", res.Outcome)
	}
	if res.Error == "" {
		t.Fatal("expected a non-empty Error for INVALID")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("expected the command to never have run, but the marker file exists")
	}
}

// ============================================================================
// INTERNAL_ERROR
// ============================================================================

func TestRun_InternalError_NonexistentCommand(t *testing.T) {
	doc := shDoc("true", 0)
	doc.Execution.Command = []string{"/no/such/executable-xyz"}
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	if res.Outcome != OutcomeInternalError {
		t.Fatalf("expected INTERNAL_ERROR, got %s", res.Outcome)
	}
	if res.Error == "" {
		t.Fatal("expected a non-empty Error for INTERNAL_ERROR")
	}
}

func TestRun_InternalError_PreExistingNonEmptyWorkspace(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "already-here.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc := shDoc("true", 0)
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{WorkspaceDir: workspace})
	if res.Outcome != OutcomeInternalError {
		t.Fatalf("expected INTERNAL_ERROR, got %s", res.Outcome)
	}
}

func TestStageWorkspaceFiles_MissingSourceIsReportedDistinctly(t *testing.T) {
	scenarioDir := t.TempDir()
	workspaceRoot := t.TempDir()
	src := filepath.Join(scenarioDir, "will-disappear.txt")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Simulate the TOCTOU gap documented in docs/phase-4-plan.md §17/§19:
	// Validate (verify time) sees the file present; it disappears before
	// staging (run time) actually reads it.
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}

	err := stageWorkspaceFiles([]WorkspaceFile{{Source: "will-disappear.txt", Destination: "dest.txt"}}, scenarioDir, workspaceRoot)
	if err == nil {
		t.Fatal("expected an error when a workspace_files source disappears before staging")
	}
}

// ============================================================================
// Output-cap truncation
// ============================================================================

func TestRun_OutputCapTruncatesAndBoundsMemory(t *testing.T) {
	// Write well past MaxScenarioOutputBytes on stdout.
	doc := shDoc("head -c 2097152 /dev/zero", 0)
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	if res.Outcome == OutcomeInternalError {
		t.Fatalf("unexpected INTERNAL_ERROR: %s", res.Error)
	}
	if !res.Stdout.Truncated {
		t.Fatal("expected stdout to be reported truncated")
	}
	if len(res.Stdout.Excerpt) != MaxScenarioOutputBytes {
		t.Fatalf("expected captured stdout to be capped at exactly %d bytes, got %d", MaxScenarioOutputBytes, len(res.Stdout.Excerpt))
	}
}

// ============================================================================
// Environment isolation
// ============================================================================

func TestRun_EnvironmentIsolation_ParentEnvNotInherited(t *testing.T) {
	const leakVar = "INCIDENTDNA_SCENARIO_TEST_LEAK_MARKER"
	const leakValue = "leaked-parent-value-should-never-appear"
	t.Setenv(leakVar, leakValue)

	doc := shDoc("echo [$"+leakVar+"]", 0)
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	if res.Outcome != OutcomePass {
		t.Fatalf("expected PASS, got %s (error: %s)", res.Outcome, res.Error)
	}
	if strings.Contains(res.Stdout.Excerpt, leakValue) {
		t.Fatalf("expected the parent's environment variable to not be inherited, but it leaked: %q", res.Stdout.Excerpt)
	}
	if !strings.Contains(res.Stdout.Excerpt, "[]") {
		t.Fatalf("expected an empty substitution for the unset variable, got %q", res.Stdout.Excerpt)
	}
}

func TestRun_EnvironmentIsolation_DeclaredEnvReachesChild(t *testing.T) {
	doc := shDoc("echo [$SCENARIO_DECLARED_VAR]", 0)
	doc.Execution.Env = map[string]string{"SCENARIO_DECLARED_VAR": "declared-value"}
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	if res.Outcome != OutcomePass {
		t.Fatalf("expected PASS, got %s (error: %s)", res.Outcome, res.Error)
	}
	if !strings.Contains(res.Stdout.Excerpt, "declared-value") {
		t.Fatalf("expected the declared env var to reach the child, got %q", res.Stdout.Excerpt)
	}
}

// ============================================================================
// cwd isolation
// ============================================================================

func TestRun_CwdIsolation_RelativeWritesLandInWorkspace(t *testing.T) {
	doc := shDoc("echo marker-content > written-by-child.txt", 0)
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{KeepWorkspace: true})
	if res.Outcome != OutcomePass {
		t.Fatalf("expected PASS, got %s (error: %s)", res.Outcome, res.Error)
	}
	if res.WorkspacePath == "" {
		t.Fatal("expected a non-empty WorkspacePath with --keep-workspace")
	}
	defer os.RemoveAll(res.WorkspacePath)

	written := filepath.Join(res.WorkspacePath, "written-by-child.txt")
	data, err := os.ReadFile(written)
	if err != nil {
		t.Fatalf("expected the child's relative write to land inside the workspace: %v", err)
	}
	if !strings.Contains(string(data), "marker-content") {
		t.Fatalf("unexpected file content: %q", data)
	}
}

// ============================================================================
// workspace_files staging end to end
// ============================================================================

func TestRun_StagesWorkspaceFilesBeforeExecution(t *testing.T) {
	scenarioDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(scenarioDir, "fixture.txt"), []byte("fixture-body"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc := shDoc("cat staged/fixture-copy.txt", 0)
	doc.Execution.WorkspaceFiles = []WorkspaceFile{{Source: "fixture.txt", Destination: "staged/fixture-copy.txt"}}
	doc.Expected.Stdout = &StreamAssertion{Mode: StreamModeContains, Value: "fixture-body"}

	res := Run(context.Background(), doc, scenarioDir, RunOptions{})
	if res.Outcome != OutcomePass {
		t.Fatalf("expected PASS, got %s (error: %s; stdout: %s; stderr: %s)", res.Outcome, res.Error, res.Stdout.Excerpt, res.Stderr.Excerpt)
	}
}

// ============================================================================
// Idempotency / determinism of the runner's own behavior
// ============================================================================

func TestRun_RepeatedRunsAreIndependentWorkspaces(t *testing.T) {
	doc := shDoc("pwd", 0)
	res1 := Run(context.Background(), doc, t.TempDir(), RunOptions{KeepWorkspace: true})
	res2 := Run(context.Background(), doc, t.TempDir(), RunOptions{KeepWorkspace: true})
	defer os.RemoveAll(res1.WorkspacePath)
	defer os.RemoveAll(res2.WorkspacePath)

	if res1.Outcome != OutcomePass || res2.Outcome != OutcomePass {
		t.Fatalf("expected both runs to PASS, got %s and %s", res1.Outcome, res2.Outcome)
	}
	if res1.WorkspacePath == res2.WorkspacePath {
		t.Fatal("expected two independent workspace directories, got the same path")
	}
}

func TestRun_WorkspaceRemovedByDefault(t *testing.T) {
	doc := shDoc("true", 0)
	res := Run(context.Background(), doc, t.TempDir(), RunOptions{})
	if res.Outcome != OutcomePass {
		t.Fatalf("expected PASS, got %s", res.Outcome)
	}
	if res.WorkspacePath != "" {
		t.Fatalf("expected no reported workspace path by default, got %q", res.WorkspacePath)
	}
}
