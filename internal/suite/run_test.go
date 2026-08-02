package suite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeCommandScenario writes a semantically valid IRS v0.1 scenario file
// declaring command and expected exit code exitCode, and returns its bare
// filename (suitable for a scenarios[].path entry, resolved relative to
// dir).
func writeCommandScenario(t *testing.T, dir, name string, command []string, exitCode int, timeoutSeconds int) string {
	t.Helper()
	quoted := make([]string, len(command))
	for i, c := range command {
		quoted[i] = fmt.Sprintf("%q", c)
	}
	yaml := fmt.Sprintf(`schema_version: irs/v0.1
scenario:
  id: %s
linked_fingerprint: "sha256:%s"
execution:
  command: [%s]
  timeout_seconds: %d
expected:
  exit_code: %d
`, name, strings.Repeat("a", 64), strings.Join(quoted, ", "), timeoutSeconds, exitCode)

	filename := name + ".yaml"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return filename
}

// writeShellScenario writes a scenario running /bin/sh -c script, expecting
// exit code 0, with a 5 second timeout.
func writeShellScenario(t *testing.T, dir, name, script string) string {
	t.Helper()
	return writeCommandScenario(t, dir, name, []string{"/bin/sh", "-c", script}, 0, 5)
}

func TestRun_AggregatePass_AllScenariosPass(t *testing.T) {
	dir := t.TempDir()
	a := writeCommandScenario(t, dir, "pass-a", []string{"/bin/true"}, 0, 5)
	b := writeCommandScenario(t, dir, "pass-b", []string{"/bin/true"}, 0, 5)

	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "all-pass"}, Scenarios: []ScenarioEntry{{Path: a}, {Path: b}}}
	res := Run(context.Background(), doc, dir, RunOptions{})
	if res.Outcome != AggregatePass {
		t.Fatalf("expected AggregatePass, got %s (error=%s)", res.Outcome, res.Error)
	}
	for _, sr := range res.Scenarios {
		if sr.Outcome != ScenarioOutcomePass {
			t.Fatalf("expected scenario %s to PASS, got %s", sr.Path, sr.Outcome)
		}
	}
}

func TestRun_AggregateFail_MixedOutcomesIndependently(t *testing.T) {
	dir := t.TempDir()
	passPath := writeCommandScenario(t, dir, "mix-pass", []string{"/bin/true"}, 0, 5)
	failPath := writeCommandScenario(t, dir, "mix-fail", []string{"/bin/false"}, 0, 5)
	timeoutPath := writeCommandScenario(t, dir, "mix-timeout", []string{"/bin/sleep", "5"}, 0, 1)
	internalErrPath := writeCommandScenario(t, dir, "mix-internal-error", []string{"/no/such/executable-xyz"}, 0, 5)

	doc := &Document{
		SchemaVersion: SupportedSchemaVersion,
		Suite:         Info{ID: "mixed"},
		Scenarios:     []ScenarioEntry{{Path: passPath}, {Path: failPath}, {Path: timeoutPath}, {Path: internalErrPath}},
	}
	res := Run(context.Background(), doc, dir, RunOptions{})
	if res.Outcome != AggregateFail {
		t.Fatalf("expected AggregateFail, got %s", res.Outcome)
	}
	wantOutcomes := []ScenarioOutcome{ScenarioOutcomePass, ScenarioOutcomeFail, ScenarioOutcomeTimeout, ScenarioOutcomeInternalError}
	for i, want := range wantOutcomes {
		if res.Scenarios[i].Outcome != want {
			t.Fatalf("scenario %d: expected %s, got %s", i, want, res.Scenarios[i].Outcome)
		}
	}
}

func TestRun_DeclaredOrderExecution(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "order.log")
	a := writeShellScenario(t, dir, "order-first", fmt.Sprintf("echo first >> %s", marker))
	b := writeShellScenario(t, dir, "order-second", fmt.Sprintf("echo second >> %s", marker))
	c := writeShellScenario(t, dir, "order-third", fmt.Sprintf("echo third >> %s", marker))

	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "ordered"}, Scenarios: []ScenarioEntry{{Path: a}, {Path: b}, {Path: c}}}
	res := Run(context.Background(), doc, dir, RunOptions{})
	if res.Outcome != AggregatePass {
		t.Fatalf("expected AggregatePass, got %s (%s)", res.Outcome, res.Error)
	}

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(data))
	want := "first\nsecond\nthird"
	if got != want {
		t.Fatalf("expected append order %q, got %q", want, got)
	}
}

func TestRun_FailFast_SkipsLaterScenariosWithoutExecutingThem(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "never-created.marker")
	failing := writeCommandScenario(t, dir, "ff-failing", []string{"/bin/false"}, 0, 5)
	never := writeShellScenario(t, dir, "ff-never", fmt.Sprintf("touch %s", marker))

	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "fail-fast"}, Scenarios: []ScenarioEntry{{Path: failing}, {Path: never}}}
	res := Run(context.Background(), doc, dir, RunOptions{FailFast: true})
	if res.Outcome != AggregateFail {
		t.Fatalf("expected AggregateFail, got %s", res.Outcome)
	}
	if res.Scenarios[1].Outcome != ScenarioOutcomeSkipped {
		t.Fatalf("expected the second scenario to be SKIPPED, got %s", res.Scenarios[1].Outcome)
	}
	if res.Scenarios[1].Result != nil {
		t.Fatal("expected a nil Result for a SKIPPED scenario")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("expected the marker file to never be created, since the scenario was skipped")
	}
}

func TestRun_WithoutFailFast_ContinuesPastNonPassScenario(t *testing.T) {
	dir := t.TempDir()
	failing := writeCommandScenario(t, dir, "cont-failing", []string{"/bin/false"}, 0, 5)
	after := writeCommandScenario(t, dir, "cont-after", []string{"/bin/true"}, 0, 5)

	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "no-fail-fast"}, Scenarios: []ScenarioEntry{{Path: failing}, {Path: after}}}
	res := Run(context.Background(), doc, dir, RunOptions{FailFast: false})
	if res.Scenarios[1].Outcome != ScenarioOutcomePass {
		t.Fatalf("expected the second scenario to still run and PASS, got %s", res.Scenarios[1].Outcome)
	}
}

func TestRun_WorkspaceIsolationAcrossScenariosInSameSuiteRun(t *testing.T) {
	dir := t.TempDir()
	a := writeShellScenario(t, dir, "iso-writer-a", "echo A > output.txt")
	b := writeShellScenario(t, dir, "iso-writer-b", "echo B > output.txt")
	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "isolated"}, Scenarios: []ScenarioEntry{{Path: a}, {Path: b}}}

	workspaceRoot := filepath.Join(dir, "workspaces")
	res := Run(context.Background(), doc, dir, RunOptions{WorkspaceRoot: workspaceRoot, KeepWorkspaces: true})
	if res.Outcome != AggregatePass {
		t.Fatalf("expected AggregatePass, got %s (%s)", res.Outcome, res.Error)
	}

	want := []string{"A\n", "B\n"}
	for i, sr := range res.Scenarios {
		if sr.Result == nil || sr.Result.WorkspacePath == "" {
			t.Fatalf("scenario %d: expected a kept workspace path", i)
		}
		data, err := os.ReadFile(filepath.Join(sr.Result.WorkspacePath, "output.txt"))
		if err != nil {
			t.Fatalf("scenario %d: expected output.txt in its own workspace: %v", i, err)
		}
		if string(data) != want[i] {
			t.Fatalf("scenario %d: expected %q, got %q", i, want[i], data)
		}
	}
}

func TestRun_AggregateInvalid_NothingExecutes(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "never.marker")
	good := writeShellScenario(t, dir, "invalid-suite-good", fmt.Sprintf("touch %s", marker))

	doc := &Document{
		SchemaVersion: SupportedSchemaVersion,
		Suite:         Info{ID: "invalid"},
		Scenarios:     []ScenarioEntry{{Path: good}, {Path: "does-not-exist.yaml"}},
	}
	res := Run(context.Background(), doc, dir, RunOptions{})
	if res.Outcome != AggregateInvalid {
		t.Fatalf("expected AggregateInvalid, got %s", res.Outcome)
	}
	if res.Error == "" {
		t.Fatal("expected a non-empty Error for AggregateInvalid")
	}
	if len(res.Scenarios) != 0 {
		t.Fatalf("expected no per-scenario results for an INVALID suite, got %v", res.Scenarios)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("expected nothing to have executed for an invalid suite")
	}
}

func TestRun_AggregateInternalError_PreexistingNonEmptyWorkspaceRoot(t *testing.T) {
	dir := t.TempDir()
	a := writeCommandScenario(t, dir, "bad-root-a", []string{"/bin/true"}, 0, 5)
	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "bad-root"}, Scenarios: []ScenarioEntry{{Path: a}}}

	root := filepath.Join(dir, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "pre-existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := Run(context.Background(), doc, dir, RunOptions{WorkspaceRoot: root})
	if res.Outcome != AggregateInternalError {
		t.Fatalf("expected AggregateInternalError, got %s", res.Outcome)
	}
	if res.Error == "" {
		t.Fatal("expected a non-empty Error for AggregateInternalError")
	}
}

func TestRun_WorkspaceRootRemovedByDefault(t *testing.T) {
	dir := t.TempDir()
	a := writeCommandScenario(t, dir, "cleanup-a", []string{"/bin/true"}, 0, 5)
	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "cleanup"}, Scenarios: []ScenarioEntry{{Path: a}}}

	root := filepath.Join(dir, "root")
	res := Run(context.Background(), doc, dir, RunOptions{WorkspaceRoot: root})
	if res.Outcome != AggregatePass {
		t.Fatalf("expected AggregatePass, got %s", res.Outcome)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("expected workspace root to be removed by default, err=%v", err)
	}
}

func TestRun_WorkspaceRootKeptWithKeepWorkspaces(t *testing.T) {
	dir := t.TempDir()
	a := writeCommandScenario(t, dir, "keep-a", []string{"/bin/true"}, 0, 5)
	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "keep"}, Scenarios: []ScenarioEntry{{Path: a}}}

	root := filepath.Join(dir, "root")
	res := Run(context.Background(), doc, dir, RunOptions{WorkspaceRoot: root, KeepWorkspaces: true})
	if res.Outcome != AggregatePass {
		t.Fatalf("expected AggregatePass, got %s", res.Outcome)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("expected workspace root to be kept, err=%v", err)
	}
	if res.Scenarios[0].Result.WorkspacePath == "" {
		t.Fatal("expected the scenario's own workspace path to be reported")
	}
}
