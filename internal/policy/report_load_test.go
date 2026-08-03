package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/scenario"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/suite"
)

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestLoadScenarioReport_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario-report.json")
	writeJSON(t, path, scenario.Report{
		SchemaVersion:     scenario.ReportSchemaVersion,
		ScenarioID:        "demo",
		ScenarioFile:      "demo.yaml",
		LinkedFingerprint: "sha256:" + strings.Repeat("a", 64),
		Result:            "PASS",
		ExitCodeExpected:  0,
		ExitCodeActual:    0,
	})

	got, err := LoadScenarioReport(path)
	if err != nil {
		t.Fatalf("LoadScenarioReport: %v", err)
	}
	if got.Kind != ReportKindScenario {
		t.Fatalf("expected ReportKindScenario, got %v", got.Kind)
	}
	if got.Result != "PASS" {
		t.Fatalf("expected result PASS, got %q", got.Result)
	}
	if len(got.DistinctFingerprints) != 1 || got.DistinctFingerprints[0] != "sha256:"+strings.Repeat("a", 64) {
		t.Fatalf("unexpected distinct fingerprints: %v", got.DistinctFingerprints)
	}
}

func TestLoadScenarioReport_EmptyLinkedFingerprintContributesNone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario-report.json")
	writeJSON(t, path, scenario.Report{
		SchemaVersion: scenario.ReportSchemaVersion,
		Result:        "INTERNAL_ERROR",
	})

	got, err := LoadScenarioReport(path)
	if err != nil {
		t.Fatalf("LoadScenarioReport: %v", err)
	}
	if len(got.DistinctFingerprints) != 0 {
		t.Fatalf("expected no distinct fingerprints, got %v", got.DistinctFingerprints)
	}
}

func TestLoadScenarioReport_RejectsWrongSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario-report.json")
	writeJSON(t, path, suite.Report{SchemaVersion: suite.ReportSchemaVersion, Result: "PASS"})

	_, err := LoadScenarioReport(path)
	if err == nil {
		t.Fatal("expected an error for a suite report loaded as a scenario report")
	}
}

func TestLoadScenarioReport_RejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario-report.json")
	if err := os.WriteFile(path, []byte("{not valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadScenarioReport(path)
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestLoadScenarioReport_RejectsOversizedDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.json")
	huge := strings.Repeat("a", MaxReportDocumentSize+1)
	if err := os.WriteFile(path, []byte(huge), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadScenarioReport(path)
	if err == nil {
		t.Fatal("expected an error for an oversized report")
	}
	if !strings.Contains(err.Error(), "maximum report document size") {
		t.Fatalf("expected a max-size error, got: %v", err)
	}
}

func fp(n byte) string {
	return "sha256:" + strings.Repeat(string(rune('a'+n)), 64)
}

func TestLoadSuiteReport_DedupsInFirstOccurrenceOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "suite-report.json")
	writeJSON(t, path, suite.Report{
		SchemaVersion: suite.ReportSchemaVersion,
		Result:        "FAIL",
		Scenarios: []suite.ScenarioReportEntry{
			{Path: "a.yaml", Report: &scenario.Report{LinkedFingerprint: fp(0), Result: "PASS"}},
			{Path: "b.yaml", Report: &scenario.Report{LinkedFingerprint: fp(1), Result: "FAIL"}},
			{Path: "c.yaml", Report: &scenario.Report{LinkedFingerprint: fp(0), Result: "PASS"}}, // duplicate of a.yaml's fingerprint
			{Path: "d.yaml", Report: nil}, // SKIPPED, contributes nothing
		},
	})

	got, err := LoadSuiteReport(path)
	if err != nil {
		t.Fatalf("LoadSuiteReport: %v", err)
	}
	if got.Kind != ReportKindSuite {
		t.Fatalf("expected ReportKindSuite, got %v", got.Kind)
	}
	if got.Result != "FAIL" {
		t.Fatalf("expected result FAIL, got %q", got.Result)
	}
	want := []string{fp(0), fp(1)}
	if len(got.DistinctFingerprints) != len(want) || got.DistinctFingerprints[0] != want[0] || got.DistinctFingerprints[1] != want[1] {
		t.Fatalf("expected deduplicated, first-occurrence-order fingerprints %v, got %v", want, got.DistinctFingerprints)
	}
}

func TestLoadSuiteReport_RejectsWrongSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "suite-report.json")
	writeJSON(t, path, scenario.Report{SchemaVersion: scenario.ReportSchemaVersion, Result: "PASS"})

	_, err := LoadSuiteReport(path)
	if err == nil {
		t.Fatal("expected an error for a scenario report loaded as a suite report")
	}
}

func TestLoadSuiteReport_RejectsTooManyDistinctFingerprints(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "suite-report.json")

	var entries []suite.ScenarioReportEntry
	for i := 0; i < MaxDistinctFingerprintsPerEvaluation+1; i++ {
		// 64-hex-char fingerprints, each distinct via a zero-padded suffix.
		suffix := fmt.Sprintf("%064x", i)
		entries = append(entries, suite.ScenarioReportEntry{
			Path:   "s.yaml",
			Report: &scenario.Report{LinkedFingerprint: "sha256:" + suffix, Result: "PASS"},
		})
	}
	writeJSON(t, path, suite.Report{SchemaVersion: suite.ReportSchemaVersion, Result: "PASS", Scenarios: entries})

	_, err := LoadSuiteReport(path)
	if err == nil {
		t.Fatal("expected an error for a report naming more than MaxDistinctFingerprintsPerEvaluation distinct fingerprints")
	}
	if !IsKind(err, ErrKindTooManyDistinctFingerprints) {
		t.Fatalf("expected ErrKindTooManyDistinctFingerprints, got: %v", err)
	}
}

func TestLoadSuiteReport_RejectsMissingFile(t *testing.T) {
	_, err := LoadSuiteReport(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
}
