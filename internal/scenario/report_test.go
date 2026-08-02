package scenario

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildReport_PassFields(t *testing.T) {
	res := RunResult{
		Outcome:           OutcomePass,
		ScenarioID:        "s1",
		LinkedFingerprint: "sha256:" + strings.Repeat("a", 64),
		ExitCodeExpected:  0,
		ExitCodeActual:    0,
		DurationMs:        42,
		Stdout:            StreamCapture{Excerpt: "ok", Truncated: false},
		Stderr:            StreamCapture{Excerpt: "", Truncated: false},
	}
	r := BuildReport(res, "examples/regression-scenario-demo/scenario-pass.yaml")

	if r.SchemaVersion != ReportSchemaVersion {
		t.Fatalf("unexpected schema_version: %s", r.SchemaVersion)
	}
	if r.Result != "PASS" {
		t.Fatalf("unexpected result: %s", r.Result)
	}
	if r.WorkspacePath != nil {
		t.Fatalf("expected nil WorkspacePath when not kept, got %v", *r.WorkspacePath)
	}
	if r.Error != nil {
		t.Fatalf("expected nil Error for PASS, got %v", *r.Error)
	}
}

func TestBuildReport_PopulatesWorkspaceAndError(t *testing.T) {
	res := RunResult{
		Outcome:       OutcomeInternalError,
		ScenarioID:    "s2",
		Error:         "something went wrong",
		WorkspacePath: "/tmp/some-workspace",
	}
	r := BuildReport(res, "scenario.yaml")
	if r.WorkspacePath == nil || *r.WorkspacePath != "/tmp/some-workspace" {
		t.Fatalf("expected WorkspacePath to be populated, got %v", r.WorkspacePath)
	}
	if r.Error == nil || *r.Error != "something went wrong" {
		t.Fatalf("expected Error to be populated, got %v", r.Error)
	}
}

func TestReport_DeterministicFieldOrderAcrossMarshals(t *testing.T) {
	res := RunResult{Outcome: OutcomePass, ScenarioID: "s3", ExitCodeExpected: 0, ExitCodeActual: 0}
	r := BuildReport(res, "scenario.yaml")

	var first, second []byte
	var err error
	first, err = json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	second, err = json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("expected identical marshaled bytes across repeated marshals, got:\n%s\nvs\n%s", first, second)
	}

	// Field order in the JSON encoding must match struct declaration order
	// (encoding/json's documented guarantee for a struct, no map involved),
	// matching docs/phase-4-plan.md §15's example field order.
	wantOrder := []string{
		"schema_version", "scenario_id", "scenario_file", "linked_fingerprint",
		"result", "exit_code_expected", "exit_code_actual", "duration_ms",
		"stdout", "stderr", "workspace_path", "error",
	}
	s := string(first)
	lastIdx := -1
	for _, key := range wantOrder {
		idx := strings.Index(s, `"`+key+`"`)
		if idx < 0 {
			t.Fatalf("expected key %q in report JSON, got: %s", key, s)
		}
		if idx < lastIdx {
			t.Fatalf("key %q appeared out of order in report JSON: %s", key, s)
		}
		lastIdx = idx
	}
}

func TestWriteReport_OverwritesOnRepeatedRuns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.json")

	first := BuildReport(RunResult{Outcome: OutcomePass, ScenarioID: "first"}, "scenario.yaml")
	if err := WriteReport(path, first); err != nil {
		t.Fatal(err)
	}

	second := BuildReport(RunResult{Outcome: OutcomeFail, ScenarioID: "second"}, "scenario.yaml")
	if err := WriteReport(path, second); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "first") {
		t.Fatalf("expected the first report's content to be fully overwritten, got: %s", data)
	}
	if !strings.Contains(string(data), "second") {
		t.Fatalf("expected the second report's content to be present, got: %s", data)
	}

	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("expected valid JSON on disk: %v", err)
	}
	if decoded.Result != "FAIL" {
		t.Fatalf("unexpected result: %s", decoded.Result)
	}
}

func TestWriteReport_FailsForUnwritablePath(t *testing.T) {
	err := WriteReport(filepath.Join(t.TempDir(), "no-such-parent-dir", "report.json"), Report{})
	if err == nil {
		t.Fatal("expected an error when the report's parent directory does not exist")
	}
}
