package policy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildReport_PassFields(t *testing.T) {
	rules := []RuleOutcome{
		{Type: RuleTypeRequireResult, Value: "PASS", Status: RuleStatusOK, Detail: "report result was PASS"},
	}
	r := BuildReport("policy-1", "policies/release-gate.yaml", "suite", "/tmp/suite-report.json", VerdictPass, rules, "")

	if r.SchemaVersion != ReportSchemaVersion {
		t.Fatalf("unexpected schema_version: %s", r.SchemaVersion)
	}
	if r.Verdict != VerdictPass {
		t.Fatalf("unexpected verdict: %s", r.Verdict)
	}
	if len(r.Rules) != 1 || r.Rules[0].Status != string(RuleStatusOK) {
		t.Fatalf("unexpected rules: %+v", r.Rules)
	}
	if r.Error != nil {
		t.Fatalf("expected nil Error for PASS, got %v", *r.Error)
	}
}

func TestBuildReport_PopulatesErrorForInvalid(t *testing.T) {
	r := BuildReport("", "policies/release-gate.yaml", "", "", "INVALID", nil, "policy document is not valid")
	if r.Error == nil || *r.Error != "policy document is not valid" {
		t.Fatalf("expected Error to be populated, got %v", r.Error)
	}
	if len(r.Rules) != 0 {
		t.Fatalf("expected no rules for INVALID, got %+v", r.Rules)
	}
}

func TestBuildReport_OmitsValueForRuleWithoutOne(t *testing.T) {
	rules := []RuleOutcome{
		{Type: RuleTypeRequireLibraryOccurrence, Status: RuleStatusSkip, Detail: "not evaluated (--library not given)"},
	}
	r := BuildReport("policy-1", "p.yaml", "scenario", "s.json", VerdictFail, rules, "")
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"value"`) {
		t.Fatalf("expected value to be omitted for a rule with no declared value, got: %s", data)
	}
}

func TestReport_DeterministicFieldOrderAcrossMarshals(t *testing.T) {
	rules := []RuleOutcome{{Type: RuleTypeRequireResult, Value: "PASS", Status: RuleStatusOK, Detail: "ok"}}
	r := BuildReport("policy-1", "p.yaml", "suite", "s.json", VerdictPass, rules, "")

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
		"schema_version", "policy_id", "policy_file", "input_report_kind",
		"input_report_file", "verdict", "rules", "error",
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

	first := BuildReport("policy-1", "p.yaml", "suite", "s.json", VerdictPass, nil, "")
	if err := WriteReport(path, first); err != nil {
		t.Fatal(err)
	}

	second := BuildReport("policy-1", "p.yaml", "suite", "s.json", VerdictFail, nil, "")
	if err := WriteReport(path, second); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"PASS"`) {
		t.Fatalf("expected the first report's content to be fully overwritten, got: %s", data)
	}

	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("expected valid JSON on disk: %v", err)
	}
	if decoded.Verdict != VerdictFail {
		t.Fatalf("unexpected verdict: %s", decoded.Verdict)
	}
}

func TestWriteReport_FailsForUnwritablePath(t *testing.T) {
	err := WriteReport(filepath.Join(t.TempDir(), "no-such-parent-dir", "report.json"), Report{})
	if err == nil {
		t.Fatal("expected an error when the report's parent directory does not exist")
	}
}
