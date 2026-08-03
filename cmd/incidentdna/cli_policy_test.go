package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/library"
)

// --- shared fixtures/helpers for the `incidentdna policy` CLI surface ---
//
// Every test below writes its own policy/scenario/suite files (and, where
// needed, its own --library directory) under t.TempDir() — none of these
// tests ever touch a real .incidentdna directory or leave generated output
// in the repository.

// writePolicyYAML writes a minimal but complete IGP v0.1 policy document
// declaring rulesYAML (already-indented "  - type: ..." lines) and returns
// its path.
func writePolicyYAML(t *testing.T, dir, name, rulesYAML string) string {
	t.Helper()
	yaml := fmt.Sprintf(`schema_version: policy/v0.1
policy:
  id: %s
rules:
%s`, name, rulesYAML)
	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const requireResultPassRule = "  - type: require_result\n    value: PASS\n"
const requireLibraryOccurrenceRule = "  - type: require_library_occurrence\n"
const bothRulesRequirePass = requireResultPassRule + requireLibraryOccurrenceRule

// writeSuiteYAML (shared with cli_suite_test.go) writes a minimal ISM v0.1
// suite manifest listing the given scenario filenames (relative to dir).

// ============================================================================
// 1. incidentdna policy verify
// ============================================================================

func TestCLI_PolicyVerify_ValidDocument(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writePolicyYAML(t, dir, "valid-policy", bothRulesRequirePass)

	stdout, stderr, code := runCLI(t, bin, "policy", "verify", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{"Policy: valid-policy", "Rules: 2 declared", "[OK] require_result: PASS", "[OK] require_library_occurrence", "OK: policy is structurally valid"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected stdout to contain %q, got: %s", want, stdout)
		}
	}
}

func TestCLI_PolicyVerify_LoadFailureReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	_, _, code := runCLI(t, bin, "policy", "verify", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if code != 1 {
		t.Fatalf("expected exit 1 for a missing file, got %d", code)
	}
}

func TestCLI_PolicyVerify_SemanticFailureReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writePolicyYAML(t, dir, "invalid-policy", "  - type: bogus_rule_type\n")

	stdout, stderr, code := runCLI(t, bin, "policy", "verify", path)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "[FAIL] bogus_rule_type") {
		t.Fatalf("expected a [FAIL] line for the unknown rule type, got: %s", stdout)
	}
}

func TestCLI_PolicyVerify_EmptyRulesReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writePolicyYAML(t, dir, "empty-rules-policy", "")
	// writePolicyYAML always emits "rules:\n" with no entries when rulesYAML
	// is empty, i.e. rules: [] shape.

	_, stderr, code := runCLI(t, bin, "policy", "verify", path)
	if code != 2 {
		t.Fatalf("expected exit 2 for empty rules, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_PolicyVerify_InvalidUsage(t *testing.T) {
	bin := buildBinary(t)
	_, _, code := runCLI(t, bin, "policy", "verify")
	if code != 1 {
		t.Fatalf("expected exit 1 for missing argument, got %d", code)
	}
}

// ============================================================================
// 2. incidentdna policy evaluate
// ============================================================================

// runScenarioReport builds a real scenario file (PASS or FAIL depending on
// command) and runs `scenario run --report` against it via the real
// compiled binary, returning the report file's path.
func runScenarioReport(t *testing.T, bin, dir, name string, command []string) string {
	t.Helper()
	scenarioPath := writeScenarioYAML(t, dir, name, command, 0, 5)
	reportPath := filepath.Join(dir, name+"-report.json")
	runCLI(t, bin, "scenario", "run", "--report", reportPath, scenarioPath)
	return reportPath
}

// runSuiteReport builds a real suite manifest over the given scenario
// specs (name -> command) and runs `suite run --report` against it via the
// real compiled binary, returning the report file's path.
func runSuiteReport(t *testing.T, bin, dir, suiteName string, scenarios map[string][]string) string {
	t.Helper()
	var relPaths []string
	for name, command := range scenarios {
		writeScenarioYAML(t, dir, name, command, 0, 5)
		relPaths = append(relPaths, name+".yaml")
	}
	suitePath := writeSuiteYAML(t, dir, suiteName, relPaths)
	reportPath := filepath.Join(dir, suiteName+"-report.json")
	runCLI(t, bin, "suite", "run", "--report", reportPath, suitePath)
	return reportPath
}

func TestCLI_PolicyEvaluate_ScenarioReportPass(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	reportPath := runScenarioReport(t, bin, dir, "eval-scenario-pass", []string{"/bin/true"})
	policyPath := writePolicyYAML(t, dir, "scenario-gate", requireResultPassRule)

	stdout, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", reportPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{"Input: scenario report (result: PASS)", "[OK] require_result: PASS", "Verdict: PASS"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected stdout to contain %q, got: %s", want, stdout)
		}
	}
}

func TestCLI_PolicyEvaluate_SuiteReportFailViaRequireResult(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	reportPath := runSuiteReport(t, bin, dir, "eval-suite-mixed", map[string][]string{
		"scen-pass": {"/bin/true"},
		"scen-fail": {"/bin/false"},
	})
	policyPath := writePolicyYAML(t, dir, "suite-gate", requireResultPassRule)

	stdout, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--suite-report", reportPath)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	for _, want := range []string{"Input: suite report (result: FAIL)", "[FAIL] require_result: PASS", "Verdict: FAIL"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected stdout to contain %q, got: %s", want, stdout)
		}
	}
}

func TestCLI_PolicyEvaluate_SuiteReportPass(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	reportPath := runSuiteReport(t, bin, dir, "eval-suite-allpass", map[string][]string{
		"scen-a": {"/bin/true"},
		"scen-b": {"/bin/true"},
	})
	policyPath := writePolicyYAML(t, dir, "suite-allpass-gate", requireResultPassRule)

	stdout, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--suite-report", reportPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Verdict: PASS") {
		t.Fatalf("expected Verdict: PASS, got: %s", stdout)
	}
}

func TestCLI_PolicyEvaluate_LibraryOccurrenceSkippedWithoutLibraryFlag(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	reportPath := runScenarioReport(t, bin, dir, "eval-skip", []string{"/bin/true"})
	policyPath := writePolicyYAML(t, dir, "skip-gate", requireLibraryOccurrenceRule)

	stdout, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", reportPath)
	if code != 2 {
		t.Fatalf("expected exit 2 (a SKIPped rule counts as not satisfied), got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "[SKIP] require_library_occurrence: not evaluated (--library not given)") {
		t.Fatalf("expected a SKIP line, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Verdict: FAIL") {
		t.Fatalf("expected Verdict: FAIL, got: %s", stdout)
	}
}

func TestCLI_PolicyEvaluate_LibraryOccurrenceMatch(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	st, err := library.Open(libDir)
	if err != nil {
		t.Fatal(err)
	}
	doc := minimalRedactedLibraryDoc("INC-POLICY-LIB-MATCH", "Policy library cross-reference match incident")
	addRes, err := library.Add(context.Background(), st, doc, library.AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	scenarioPath := writeScenarioYAMLWithFingerprint(t, dir, "eval-lib-match", addRes.Fingerprint)
	reportPath := filepath.Join(dir, "eval-lib-match-report.json")
	runCLI(t, bin, "scenario", "run", "--report", reportPath, scenarioPath)

	policyPath := writePolicyYAML(t, dir, "lib-match-gate", bothRulesRequirePass)

	stdout, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", reportPath, "--library", libDir)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "[OK] require_library_occurrence: 1 of 1 distinct fingerprint(s) have library occurrences") {
		t.Fatalf("expected an OK library occurrence line, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Verdict: PASS") {
		t.Fatalf("expected Verdict: PASS, got: %s", stdout)
	}
}

func TestCLI_PolicyEvaluate_LibraryOccurrenceNoMatch(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library") // never seeded

	reportPath := runScenarioReport(t, bin, dir, "eval-lib-nomatch", []string{"/bin/true"})
	policyPath := writePolicyYAML(t, dir, "lib-nomatch-gate", requireLibraryOccurrenceRule)

	stdout, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", reportPath, "--library", libDir)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "[FAIL] require_library_occurrence: 0 of 1 distinct fingerprint(s) have library occurrences") {
		t.Fatalf("expected a FAIL library occurrence line, got: %s", stdout)
	}
}

func TestCLI_PolicyEvaluate_MalformedPolicyReturnsInvalidExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	reportPath := runScenarioReport(t, bin, dir, "eval-badpolicy", []string{"/bin/true"})

	_, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", filepath.Join(dir, "does-not-exist.yaml"), "--scenario-report", reportPath)
	if code != 1 {
		t.Fatalf("expected exit 1 for a missing policy file, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_PolicyEvaluate_SemanticallyInvalidPolicyReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	reportPath := runScenarioReport(t, bin, dir, "eval-semanticbad", []string{"/bin/true"})
	policyPath := writePolicyYAML(t, dir, "semantic-bad-gate", "  - type: bogus\n")

	_, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", reportPath)
	if code != 1 {
		t.Fatalf("expected exit 1 (INVALID) for a semantically invalid policy, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_PolicyEvaluate_MalformedReportReturnsInvalidExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	badReportPath := filepath.Join(dir, "bad-report.json")
	if err := os.WriteFile(badReportPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
	policyPath := writePolicyYAML(t, dir, "malformed-report-gate", requireResultPassRule)

	_, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", badReportPath)
	if code != 1 {
		t.Fatalf("expected exit 1 for a malformed report file, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_PolicyEvaluate_IncompatibleReportSchemaReturnsInvalidExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	// A genuine suite report loaded as a --scenario-report is a schema
	// mismatch, not a parse failure.
	suiteReportPath := runSuiteReport(t, bin, dir, "eval-wrongkind", map[string][]string{
		"scen-a": {"/bin/true"},
	})
	policyPath := writePolicyYAML(t, dir, "wrongkind-gate", requireResultPassRule)

	_, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", suiteReportPath)
	if code != 1 {
		t.Fatalf("expected exit 1 for an incompatible report schema, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_PolicyEvaluate_MalformedLibraryReturnsInternalErrorExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	st, err := library.Open(libDir)
	if err != nil {
		t.Fatal(err)
	}
	doc := minimalRedactedLibraryDoc("INC-POLICY-LIB-MALFORMED", "Malformed library incident")
	addRes, err := library.Add(context.Background(), st, doc, library.AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	idxPath, err := st.IndexPath(addRes.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(idxPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	scenarioPath := writeScenarioYAMLWithFingerprint(t, dir, "eval-lib-malformed", addRes.Fingerprint)
	reportPath := filepath.Join(dir, "eval-lib-malformed-report.json")
	runCLI(t, bin, "scenario", "run", "--report", reportPath, scenarioPath)

	policyPath := writePolicyYAML(t, dir, "lib-malformed-gate", requireLibraryOccurrenceRule)

	_, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", reportPath, "--library", libDir)
	if code != 1 {
		t.Fatalf("expected exit 1 (INTERNAL_ERROR) for a malformed library, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "malformed") {
		t.Fatalf("expected stderr to describe the malformed library, got: %s", stderr)
	}
}

func TestCLI_PolicyEvaluate_RequiresExactlyOneReportFlag(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	policyPath := writePolicyYAML(t, dir, "no-report-flag-gate", requireResultPassRule)

	_, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath)
	if code != 1 {
		t.Fatalf("expected exit 1 when neither --scenario-report nor --suite-report is given, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "exactly one of --scenario-report or --suite-report") {
		t.Fatalf("expected an explanatory error, got: %s", stderr)
	}
}

func TestCLI_PolicyEvaluate_RejectsBothReportFlagsTogether(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	scenReportPath := runScenarioReport(t, bin, dir, "eval-both-a", []string{"/bin/true"})
	suiteReportPath := runSuiteReport(t, bin, dir, "eval-both-b", map[string][]string{"scen-x": {"/bin/true"}})
	policyPath := writePolicyYAML(t, dir, "both-flags-gate", requireResultPassRule)

	_, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", scenReportPath, "--suite-report", suiteReportPath)
	if code != 1 {
		t.Fatalf("expected exit 1 when both --scenario-report and --suite-report are given, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_PolicyEvaluate_RequiresPolicyFlag(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	reportPath := runScenarioReport(t, bin, dir, "eval-nopolicy", []string{"/bin/true"})

	_, stderr, code := runCLI(t, bin, "policy", "evaluate", "--scenario-report", reportPath)
	if code != 1 {
		t.Fatalf("expected exit 1 when --policy is missing, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "--policy is required") {
		t.Fatalf("expected an explanatory error, got: %s", stderr)
	}
}

func TestCLI_PolicyEvaluate_ReportMatchesSummary(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	reportPath := runScenarioReport(t, bin, dir, "eval-report-out", []string{"/bin/true"})
	policyPath := writePolicyYAML(t, dir, "report-out-gate", requireResultPassRule)
	verdictReportPath := filepath.Join(dir, "verdict-report.json")

	stdout, stderr, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", reportPath, "--report", verdictReportPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}

	data, err := os.ReadFile(verdictReportPath)
	if err != nil {
		t.Fatalf("expected a verdict report file to be written: %v", err)
	}
	var decoded struct {
		SchemaVersion   string `json:"schema_version"`
		PolicyID        string `json:"policy_id"`
		InputReportKind string `json:"input_report_kind"`
		Verdict         string `json:"verdict"`
		Rules           []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"rules"`
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("expected valid JSON, got: %v", err)
	}
	if decoded.SchemaVersion != "policy-report/v0.1" {
		t.Fatalf("unexpected schema_version: %s", decoded.SchemaVersion)
	}
	if decoded.PolicyID != "report-out-gate" {
		t.Fatalf("unexpected policy_id: %s", decoded.PolicyID)
	}
	if decoded.InputReportKind != "scenario" {
		t.Fatalf("unexpected input_report_kind: %s", decoded.InputReportKind)
	}
	if decoded.Verdict != "PASS" {
		t.Fatalf("unexpected verdict: %s", decoded.Verdict)
	}
	if decoded.Error != nil {
		t.Fatalf("expected nil error for a PASS verdict, got %v", *decoded.Error)
	}
	if len(decoded.Rules) != 1 || decoded.Rules[0].Status != "OK" {
		t.Fatalf("unexpected rules: %+v", decoded.Rules)
	}
}

func TestCLI_PolicyEvaluate_ReportWrittenOnInvalidOutcome(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	policyPath := writePolicyYAML(t, dir, "invalid-report-out-gate", requireResultPassRule)
	verdictReportPath := filepath.Join(dir, "invalid-verdict-report.json")

	_, _, code := runCLI(t, bin, "policy", "evaluate", "--policy", policyPath, "--scenario-report", filepath.Join(dir, "does-not-exist.json"), "--report", verdictReportPath)
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}

	data, err := os.ReadFile(verdictReportPath)
	if err != nil {
		t.Fatalf("expected a best-effort verdict report to be written even on INVALID: %v", err)
	}
	var decoded struct {
		Verdict string  `json:"verdict"`
		Error   *string `json:"error"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("expected valid JSON, got: %v", err)
	}
	if decoded.Verdict != "INVALID" {
		t.Fatalf("expected verdict INVALID, got %s", decoded.Verdict)
	}
	if decoded.Error == nil || *decoded.Error == "" {
		t.Fatal("expected a non-empty error message")
	}
}

// ============================================================================
// 3. incidentdna policy help/usage
// ============================================================================

func TestCLI_Policy_UnknownSubcommand(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "policy", "bogus")
	if code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if !strings.Contains(stderr, "unknown policy subcommand") {
		t.Fatalf("expected an unknown-subcommand error, got: %s", stderr)
	}
}

func TestCLI_Policy_HelpExitsZero(t *testing.T) {
	bin := buildBinary(t)
	_, _, code := runCLI(t, bin, "policy", "help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
}
