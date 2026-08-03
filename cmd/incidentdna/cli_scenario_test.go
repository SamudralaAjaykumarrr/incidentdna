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

// --- shared fixtures/helpers for the `incidentdna scenario` CLI surface ---
//
// Every test below writes its own scenario file (and, where needed, its own
// fixtures directory) under t.TempDir(), and every --workspace/--report path
// used is likewise rooted under t.TempDir() — none of these tests ever touch
// a real .incidentdna directory or leave generated output in the repository.

// writeScenarioYAML writes a minimal but complete IRS v0.1 scenario document
// referencing an absolute command[0] (so bare-command rejection never
// applies unless the test deliberately wants it), and returns its path.
func writeScenarioYAML(t *testing.T, dir, name string, command []string, exitCode int, timeoutSeconds int) string {
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

	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeScenarioYAMLWithFingerprint writes a minimal but complete IRS v0.1
// scenario document declaring linked_fingerprint exactly as given (used by
// the --library cross-reference tests, which need a scenario's declared
// fingerprint to match a fingerprint already seeded into a test library),
// and returns its path.
func writeScenarioYAMLWithFingerprint(t *testing.T, dir, name, linkedFingerprint string) string {
	t.Helper()
	yaml := fmt.Sprintf(`schema_version: irs/v0.1
scenario:
  id: %s
linked_fingerprint: %q
execution:
  command: ["/bin/true"]
  timeout_seconds: 5
expected:
  exit_code: 0
`, name, linkedFingerprint)

	path := filepath.Join(dir, name+".yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ============================================================================
// 1. incidentdna scenario verify
// ============================================================================

func TestCLI_ScenarioVerify_ValidDocument(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "valid-scenario", []string{"/bin/true"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "OK: scenario is structurally valid") {
		t.Fatalf("expected stdout to confirm validity, got: %s", stdout)
	}
}

func TestCLI_ScenarioVerify_LoadFailureReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "scenario", "verify", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if code != 1 {
		t.Fatalf("expected exit 1 for a missing scenario file, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_ScenarioVerify_SemanticFailureReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-schema.yaml")
	bad := `schema_version: irs/v0.2
scenario:
  id: bad
linked_fingerprint: "sha256:` + strings.Repeat("a", 64) + `"
execution:
  command: ["/bin/true"]
expected:
  exit_code: 0
`
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", path)
	if code != 2 {
		t.Fatalf("expected exit 2 for a semantically invalid scenario, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "schema_version") {
		t.Fatalf("expected stderr to name the failing field, got: %s", stderr)
	}
}

func TestCLI_ScenarioVerify_InvalidUsage(t *testing.T) {
	bin := buildBinary(t)
	cases := [][]string{
		{"scenario", "verify"},
		{"scenario", "verify", "a", "b"},
	}
	for _, args := range cases {
		_, stderr, code := runCLI(t, bin, args...)
		if code != 1 {
			t.Fatalf("args %v: expected exit 1, got %d; stderr: %s", args, code, stderr)
		}
	}
}

func TestCLI_ScenarioVerify_SourceMatch(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	goldenPath := filepath.Join(repoRoot(t), "testdata", "golden", "duplicate-payment.fingerprint")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	fp := strings.TrimSpace(string(golden))

	path := filepath.Join(dir, "source-match.yaml")
	yaml := `schema_version: irs/v0.1
scenario:
  id: source-match
linked_fingerprint: "` + fp + `"
execution:
  command: ["/bin/true"]
expected:
  exit_code: 0
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", "--source", examplePath(t), path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "linked_fingerprint matches --source") {
		t.Fatalf("expected stdout to confirm the fingerprint match, got: %s", stdout)
	}
}

func TestCLI_ScenarioVerify_SourceMismatchReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "source-mismatch", []string{"/bin/true"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", "--source", examplePath(t), path)
	if code != 2 {
		t.Fatalf("expected exit 2 for a fingerprint mismatch, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "does not match") {
		t.Fatalf("expected stderr to explain the mismatch, got: %s", stderr)
	}
}

// ============================================================================
// 1b. incidentdna scenario verify --library (docs/phase-6-plan.md)
// ============================================================================

func TestCLI_ScenarioVerify_LibraryMatch(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	st, err := library.Open(libDir)
	if err != nil {
		t.Fatal(err)
	}
	doc := minimalRedactedLibraryDoc("INC-SCEN-LIB-MATCH", "Scenario library cross-reference match incident")
	addRes, err := library.Add(context.Background(), st, doc, library.AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	scenarioPath := filepath.Join(dir, "scenario-match.yaml")
	yaml := fmt.Sprintf(`schema_version: irs/v0.1
scenario:
  id: scenario-match
linked_fingerprint: %q
execution:
  command: ["/bin/true"]
expected:
  exit_code: 0
`, addRes.Fingerprint)
	if err := os.WriteFile(scenarioPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", "--library", libDir, scenarioPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Library: 1 occurrence(s) found for this fingerprint") {
		t.Fatalf("expected stdout to report a library match, got: %s", stdout)
	}
}

func TestCLI_ScenarioVerify_LibraryNoMatchOnExistingLibrary(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	// Seed the library so its root exists on disk, but under a fingerprint
	// unrelated to the scenario below.
	st, err := library.Open(libDir)
	if err != nil {
		t.Fatal(err)
	}
	unrelated := minimalRedactedLibraryDoc("INC-SCEN-LIB-UNRELATED", "Unrelated incident")
	if _, err := library.Add(context.Background(), st, unrelated, library.AddOptions{}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	path := writeScenarioYAML(t, dir, "scenario-no-match", []string{"/bin/true"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", "--library", libDir, path)
	if code != 0 {
		t.Fatalf("expected exit 0 for a no-match library lookup (never exit 2), got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Library: no occurrences found for this fingerprint (not yet recorded in the library)") {
		t.Fatalf("expected stdout to report no library occurrences, got: %s", stdout)
	}
}

func TestCLI_ScenarioVerify_LibraryNotYetCreatedTreatedAsNoMatch(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	// libDir intentionally never created on disk.
	libDir := filepath.Join(dir, "never-created-library")

	path := writeScenarioYAML(t, dir, "scenario-no-lib", []string{"/bin/true"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", "--library", libDir, path)
	if code != 0 {
		t.Fatalf("expected exit 0 for a not-yet-created library, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Library: no occurrences found for this fingerprint (not yet recorded in the library)") {
		t.Fatalf("expected stdout to treat a not-yet-created library as no occurrences, got: %s", stdout)
	}
}

func TestCLI_ScenarioVerify_LibraryMalformedReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	st, err := library.Open(libDir)
	if err != nil {
		t.Fatal(err)
	}
	doc := minimalRedactedLibraryDoc("INC-SCEN-LIB-MALFORMED", "Malformed library incident")
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

	scenarioPath := filepath.Join(dir, "scenario-malformed.yaml")
	yaml := fmt.Sprintf(`schema_version: irs/v0.1
scenario:
  id: scenario-malformed
linked_fingerprint: %q
execution:
  command: ["/bin/true"]
expected:
  exit_code: 0
`, addRes.Fingerprint)
	if err := os.WriteFile(scenarioPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", "--library", libDir, scenarioPath)
	if code != 1 {
		t.Fatalf("expected exit 1 for a malformed library, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "malformed") {
		t.Fatalf("expected stderr to describe the malformed library, got: %s", stderr)
	}
}

func TestCLI_ScenarioVerify_LibraryCombinedWithSource(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	goldenPath := filepath.Join(repoRoot(t), "testdata", "golden", "duplicate-payment.fingerprint")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	fp := strings.TrimSpace(string(golden))

	// Seed the library with the duplicate-payment example itself, via the
	// CLI, so the seeded fingerprint is exactly the golden fingerprint
	// --source will also compute.
	if _, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, "--allow-unredacted", examplePath(t)); code != 0 {
		t.Fatalf("library add: expected exit 0, got %d; stderr: %s", code, stderr)
	}

	scenarioPath := filepath.Join(dir, "combined.yaml")
	yaml := `schema_version: irs/v0.1
scenario:
  id: combined
linked_fingerprint: "` + fp + `"
execution:
  command: ["/bin/true"]
expected:
  exit_code: 0
`
	if err := os.WriteFile(scenarioPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", "--source", examplePath(t), "--library", libDir, scenarioPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "linked_fingerprint matches --source") {
		t.Fatalf("expected stdout to confirm the --source match, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Library: 1 occurrence(s) found for this fingerprint") {
		t.Fatalf("expected stdout to confirm the --library match, got: %s", stdout)
	}
}

func TestCLI_ScenarioVerify_LibraryOmittedProducesUnchangedOutput(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "no-library-flag", []string{"/bin/true"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "verify", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	want := fmt.Sprintf("Scenario: no-library-flag (schema irs/v0.1)\nLinked fingerprint: sha256:%s\nOK: scenario is structurally valid\n", strings.Repeat("a", 64))
	if stdout != want {
		t.Fatalf("expected --library-omitted output to be byte-identical to the pre-Phase-6 baseline, got:\n%q\nwant:\n%q", stdout, want)
	}
	if strings.Contains(stdout, "Library:") {
		t.Fatalf("expected no Library: line when --library is omitted, got: %s", stdout)
	}
}

// ============================================================================
// 2. incidentdna scenario run
// ============================================================================

func TestCLI_ScenarioRun_Pass(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "run-pass", []string{"/bin/true"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: PASS") {
		t.Fatalf("expected stdout to report PASS, got: %s", stdout)
	}
}

func TestCLI_ScenarioRun_FailReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	// /bin/false exits 1; declare an expected exit code of 0, forcing FAIL.
	path := writeScenarioYAML(t, dir, "run-fail", []string{"/bin/false"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", path)
	if code != 2 {
		t.Fatalf("expected exit 2 for FAIL, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: FAIL") {
		t.Fatalf("expected stdout to report FAIL, got: %s", stdout)
	}
}

func TestCLI_ScenarioRun_TimeoutReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "run-timeout", []string{"/bin/sleep", "30"}, 0, 1)

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", path)
	if code != 2 {
		t.Fatalf("expected exit 2 for TIMEOUT, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: TIMEOUT") {
		t.Fatalf("expected stdout to report TIMEOUT, got: %s", stdout)
	}
}

func TestCLI_ScenarioRun_InvalidReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "run-invalid.yaml")
	// Bare command[0] ("echo", no path separator): rejected at the
	// pre-execution check, nothing ever runs.
	yaml := `schema_version: irs/v0.1
scenario:
  id: run-invalid
linked_fingerprint: "sha256:` + strings.Repeat("a", 64) + `"
execution:
  command: ["echo", "hello"]
expected:
  exit_code: 0
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", path)
	if code != 1 {
		t.Fatalf("expected exit 1 for INVALID, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: INVALID") {
		t.Fatalf("expected stdout to report INVALID, got: %s", stdout)
	}
}

func TestCLI_ScenarioRun_InternalErrorReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "run-internal-error", []string{"/no/such/executable-xyz"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", path)
	if code != 1 {
		t.Fatalf("expected exit 1 for INTERNAL_ERROR, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: INTERNAL_ERROR") {
		t.Fatalf("expected stdout to report INTERNAL_ERROR, got: %s", stdout)
	}
}

func TestCLI_ScenarioRun_LoadFailureStillWritesReport(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")

	_, stderr, code := runCLI(t, bin, "scenario", "run", "--report", reportPath, filepath.Join(dir, "does-not-exist.yaml"))
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; stderr: %s", code, stderr)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("expected a report to be written even for a load failure: %v", err)
	}
	var rep map[string]any
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("expected valid JSON, got: %v", err)
	}
	if rep["result"] != "INTERNAL_ERROR" {
		t.Fatalf("expected result INTERNAL_ERROR, got: %v", rep["result"])
	}
}

func TestCLI_ScenarioRun_ReportMatchesSummary(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "run-report", []string{"/bin/true"}, 0, 5)
	reportPath := filepath.Join(dir, "report.json")

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", "--report", reportPath, path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}

	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	var rep map[string]any
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("expected valid JSON, got: %v; content: %s", err, data)
	}
	if rep["result"] != "PASS" {
		t.Fatalf("expected report result PASS, got: %v", rep["result"])
	}
	if rep["scenario_id"] != "run-report" {
		t.Fatalf("expected report scenario_id run-report, got: %v", rep["scenario_id"])
	}
	if rep["schema_version"] != "irs-report/v0.1" {
		t.Fatalf("expected report schema_version irs-report/v0.1, got: %v", rep["schema_version"])
	}
}

func TestCLI_ScenarioRun_KeepWorkspaceLeavesInspectableDirectory(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "run-keep", []string{"/bin/true"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", "--keep-workspace", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	var wsPath string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "Workspace: kept at ") {
			wsPath = strings.TrimPrefix(line, "Workspace: kept at ")
		}
	}
	if wsPath == "" {
		t.Fatalf("expected stdout to report a kept workspace path, got: %s", stdout)
	}
	defer os.RemoveAll(wsPath)
	if info, err := os.Stat(wsPath); err != nil || !info.IsDir() {
		t.Fatalf("expected the kept workspace directory to exist, got err=%v", err)
	}
}

func TestCLI_ScenarioRun_WithoutKeepWorkspaceReportsRemoved(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "run-no-keep", []string{"/bin/true"}, 0, 5)

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Workspace: removed") {
		t.Fatalf("expected stdout to report the workspace was removed, got: %s", stdout)
	}
}

func TestCLI_ScenarioRun_ExplicitWorkspaceMustNotPreExistNonEmpty(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := writeScenarioYAML(t, dir, "run-explicit-ws", []string{"/bin/true"}, 0, 5)

	workspace := filepath.Join(dir, "ws")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "pre-existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", "--workspace", workspace, path)
	if code != 1 {
		t.Fatalf("expected exit 1 for a pre-existing non-empty --workspace, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
}

func TestCLI_ScenarioRun_WorkspaceRelativeCommandAgainstRealBinary(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	fixturesDir := filepath.Join(dir, "fixtures")
	if err := os.MkdirAll(fixturesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	incidentData, err := os.ReadFile(examplePath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixturesDir, "incident.yaml"), incidentData, 0o644); err != nil {
		t.Fatal(err)
	}

	goldenPath := filepath.Join(repoRoot(t), "testdata", "golden", "duplicate-payment.fingerprint")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	fp := strings.TrimSpace(string(golden))

	scenarioPath := filepath.Join(dir, "self-referential.yaml")
	yaml := fmt.Sprintf(`schema_version: irs/v0.1
scenario:
  id: self-referential
linked_fingerprint: %q
execution:
  workspace_files:
    - source: fixtures/incident.yaml
      destination: incident.yaml
  command: [%q, "fingerprint", "incident.yaml"]
  timeout_seconds: 10
expected:
  exit_code: 0
  stdout:
    mode: contains
    value: %q
`, fp, bin, fp)
	if err := os.WriteFile(scenarioPath, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "scenario", "run", scenarioPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: PASS") {
		t.Fatalf("expected stdout to report PASS, got: %s", stdout)
	}
}

func TestCLI_ScenarioRun_InvalidUsage(t *testing.T) {
	bin := buildBinary(t)
	cases := [][]string{
		{"scenario", "run"},
		{"scenario", "run", "a", "b"},
	}
	for _, args := range cases {
		_, stderr, code := runCLI(t, bin, args...)
		if code != 1 {
			t.Fatalf("args %v: expected exit 1, got %d; stderr: %s", args, code, stderr)
		}
	}
}

// ============================================================================
// 3. General CLI behavior
// ============================================================================

func TestCLI_TopLevelHelpIncludesScenarioCommand(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout+stderr, "scenario") {
		t.Fatalf("expected top-level --help to mention the scenario command, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestCLI_ScenarioHelpIncludesBothSubcommands(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "scenario", "--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	combined := stdout + stderr
	for _, want := range []string{"scenario verify", "scenario run"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected scenario --help to mention %q, got:\n%s", want, combined)
		}
	}
}

func TestCLI_ScenarioUnknownSubcommandReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "scenario", "bogus")
	if code != 1 {
		t.Fatalf("expected exit 1 for an unknown scenario subcommand, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_ScenarioNoSubcommandReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "scenario")
	if code != 1 {
		t.Fatalf("expected exit 1 for `scenario` with no subcommand, got %d; stderr: %s", code, stderr)
	}
}
