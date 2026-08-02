package suite

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/scenario"
)

func TestBuildReport_CountsAndEmbeddedScenarioReports(t *testing.T) {
	res := SuiteRunResult{
		SuiteID:    "s1",
		Outcome:    AggregateFail,
		DurationMs: 1234,
		Scenarios: []ScenarioResult{
			{Path: "a.yaml", Outcome: ScenarioOutcomePass, Result: &scenario.RunResult{Outcome: scenario.OutcomePass, ScenarioID: "a"}},
			{Path: "b.yaml", Outcome: ScenarioOutcomeFail, Result: &scenario.RunResult{Outcome: scenario.OutcomeFail, ScenarioID: "b"}},
			{Path: "c.yaml", Outcome: ScenarioOutcomeTimeout, Result: &scenario.RunResult{Outcome: scenario.OutcomeTimeout, ScenarioID: "c"}},
			{Path: "d.yaml", Outcome: ScenarioOutcomeInvalid, Result: &scenario.RunResult{Outcome: scenario.OutcomeInvalid, ScenarioID: "d"}},
			{Path: "e.yaml", Outcome: ScenarioOutcomeInternalError, Result: &scenario.RunResult{Outcome: scenario.OutcomeInternalError, ScenarioID: "e"}},
			{Path: "f.yaml", Outcome: ScenarioOutcomeSkipped},
		},
	}
	r := BuildReport(res, "suite.yaml")

	if r.SchemaVersion != ReportSchemaVersion {
		t.Fatalf("unexpected schema_version: %s", r.SchemaVersion)
	}
	if r.Result != "FAIL" {
		t.Fatalf("unexpected result: %s", r.Result)
	}
	wantCounts := Counts{Pass: 1, Fail: 1, Timeout: 1, Invalid: 1, InternalError: 1, Skipped: 1}
	if r.Counts != wantCounts {
		t.Fatalf("unexpected counts: %+v, want %+v", r.Counts, wantCounts)
	}
	if len(r.Scenarios) != 6 {
		t.Fatalf("expected 6 scenario report entries, got %d", len(r.Scenarios))
	}
	if r.Scenarios[5].Report != nil {
		t.Fatalf("expected a nil report for a SKIPPED scenario, got %+v", r.Scenarios[5].Report)
	}
	if r.Scenarios[0].Report == nil || r.Scenarios[0].Report.ScenarioID != "a" {
		t.Fatalf("expected scenario a's embedded report to carry its own scenario_id, got %+v", r.Scenarios[0].Report)
	}
}

func TestBuildReport_PopulatesErrorForInvalidAndInternalError(t *testing.T) {
	res := SuiteRunResult{SuiteID: "s2", Outcome: AggregateInvalid, Error: "suite manifest is invalid"}
	r := BuildReport(res, "suite.yaml")
	if r.Error == nil || *r.Error != "suite manifest is invalid" {
		t.Fatalf("expected Error to be populated, got %v", r.Error)
	}
}

func TestBuildReport_NilErrorForPass(t *testing.T) {
	res := SuiteRunResult{SuiteID: "s3", Outcome: AggregatePass}
	r := BuildReport(res, "suite.yaml")
	if r.Error != nil {
		t.Fatalf("expected nil Error for PASS, got %v", *r.Error)
	}
}

func TestReport_DeterministicFieldOrderAcrossMarshals(t *testing.T) {
	res := SuiteRunResult{
		SuiteID: "s4",
		Outcome: AggregatePass,
		Scenarios: []ScenarioResult{
			{Path: "a.yaml", Outcome: ScenarioOutcomePass, Result: &scenario.RunResult{Outcome: scenario.OutcomePass, ScenarioID: "a"}},
		},
	}
	r := BuildReport(res, "suite.yaml")

	first, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("expected identical marshaled bytes across repeated marshals, got:\n%s\nvs\n%s", first, second)
	}

	wantOrder := []string{
		"schema_version", "suite_id", "suite_file", "result",
		"counts", "duration_ms", "scenarios", "error",
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

	first := BuildReport(SuiteRunResult{SuiteID: "first", Outcome: AggregatePass}, "suite.yaml")
	if err := WriteReport(path, first); err != nil {
		t.Fatal(err)
	}

	second := BuildReport(SuiteRunResult{SuiteID: "second", Outcome: AggregateFail}, "suite.yaml")
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
