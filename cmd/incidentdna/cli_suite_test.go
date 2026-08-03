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

// writeSuiteYAML writes a minimal ISM v0.1 suite manifest listing the given
// scenario filenames (each resolved relative to dir), and returns its path.
func writeSuiteYAML(t *testing.T, dir, id string, scenarioFilenames []string) string {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "schema_version: suite/v0.1\nsuite:\n  id: %s\nscenarios:\n", id)
	for _, f := range scenarioFilenames {
		fmt.Fprintf(&b, "  - path: %s\n", f)
	}
	path := filepath.Join(dir, id+".yaml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// ============================================================================
// 1. incidentdna suite verify
// ============================================================================

func TestCLI_SuiteVerify_ValidManifest(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	a := writeScenarioYAML(t, dir, "suite-verify-a", []string{"/bin/true"}, 0, 5)
	b := writeScenarioYAML(t, dir, "suite-verify-b", []string{"/bin/true"}, 0, 5)
	suitePath := writeSuiteYAML(t, dir, "verify-suite", []string{filepath.Base(a), filepath.Base(b)})

	stdout, stderr, code := runCLI(t, bin, "suite", "verify", suitePath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "OK: suite manifest and all 2 listed scenarios are structurally valid") {
		t.Fatalf("expected stdout to confirm validity, got: %s", stdout)
	}
	if !strings.Contains(stdout, "[OK]") {
		t.Fatalf("expected per-scenario [OK] lines, got: %s", stdout)
	}
}

func TestCLI_SuiteVerify_LoadFailureReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "suite", "verify", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if code != 1 {
		t.Fatalf("expected exit 1 for a missing suite file, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_SuiteVerify_SemanticFailureReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-schema.yaml")
	bad := "schema_version: suite/v0.2\nsuite:\n  id: bad\nscenarios:\n  - path: nope.yaml\n"
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "suite", "verify", path)
	if code != 2 {
		t.Fatalf("expected exit 2 for a semantically invalid suite, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "schema_version") {
		t.Fatalf("expected stderr to name the failing field, got: %s", stderr)
	}
}

func TestCLI_SuiteVerify_ListedInvalidScenarioReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "bad-scenario.yaml")
	bad := `schema_version: irs/v0.1
scenario:
  id: bare-cmd
linked_fingerprint: "sha256:` + strings.Repeat("a", 64) + `"
execution:
  command: ["echo"]
expected:
  exit_code: 0
`
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	suitePath := writeSuiteYAML(t, dir, "invalid-listed", []string{"bad-scenario.yaml"})

	stdout, stderr, code := runCLI(t, bin, "suite", "verify", suitePath)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "[FAIL]") {
		t.Fatalf("expected a [FAIL] line for the invalid listed scenario, got: %s", stdout)
	}
}

func TestCLI_SuiteVerify_InvalidUsage(t *testing.T) {
	bin := buildBinary(t)
	cases := [][]string{
		{"suite", "verify"},
		{"suite", "verify", "a", "b"},
	}
	for _, args := range cases {
		_, stderr, code := runCLI(t, bin, args...)
		if code != 1 {
			t.Fatalf("args %v: expected exit 1, got %d; stderr: %s", args, code, stderr)
		}
	}
}

// ============================================================================
// 1b. incidentdna suite verify --library (docs/phase-6-plan.md)
// ============================================================================

func TestCLI_SuiteVerify_LibraryAllMatch(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	st, err := library.Open(libDir)
	if err != nil {
		t.Fatal(err)
	}
	doc := minimalRedactedLibraryDoc("INC-SUITE-LIB-MATCH", "Suite library cross-reference match incident")
	addRes, err := library.Add(context.Background(), st, doc, library.AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	a := writeScenarioYAMLWithFingerprint(t, dir, "suite-lib-match-a", addRes.Fingerprint)
	b := writeScenarioYAMLWithFingerprint(t, dir, "suite-lib-match-b", addRes.Fingerprint)
	suitePath := writeSuiteYAML(t, dir, "suite-lib-match", []string{filepath.Base(a), filepath.Base(b)})

	stdout, stderr, code := runCLI(t, bin, "suite", "verify", "--library", libDir, suitePath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "— library: 1 occurrence(s)") {
		t.Fatalf("expected per-scenario library annotations, got: %s", stdout)
	}
	// Two listed scenarios share one linked_fingerprint: the aggregate line
	// must reflect 1 *distinct* fingerprint, not the raw 2-scenario count.
	if !strings.Contains(stdout, "Library cross-reference: 1 of 1 distinct linked fingerprint(s) have library occurrences") {
		t.Fatalf("expected the aggregate line to reflect dedup by distinct fingerprint, got: %s", stdout)
	}
}

func TestCLI_SuiteVerify_LibraryMixedMatchAndNoMatch(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	st, err := library.Open(libDir)
	if err != nil {
		t.Fatal(err)
	}
	doc := minimalRedactedLibraryDoc("INC-SUITE-LIB-MIXED", "Suite library cross-reference mixed incident")
	addRes, err := library.Add(context.Background(), st, doc, library.AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	matched := writeScenarioYAMLWithFingerprint(t, dir, "suite-lib-mixed-match", addRes.Fingerprint)
	unmatched := writeScenarioYAML(t, dir, "suite-lib-mixed-nomatch", []string{"/bin/true"}, 0, 5)
	suitePath := writeSuiteYAML(t, dir, "suite-lib-mixed", []string{filepath.Base(matched), filepath.Base(unmatched)})

	stdout, stderr, code := runCLI(t, bin, "suite", "verify", "--library", libDir, suitePath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "— library: 1 occurrence(s)") {
		t.Fatalf("expected a matched per-scenario annotation, got: %s", stdout)
	}
	if !strings.Contains(stdout, "— library: no occurrences") {
		t.Fatalf("expected a no-match per-scenario annotation, got: %s", stdout)
	}
	if !strings.Contains(stdout, "Library cross-reference: 1 of 2 distinct linked fingerprint(s) have library occurrences") {
		t.Fatalf("expected the aggregate line to report 1 of 2 distinct fingerprints, got: %s", stdout)
	}
}

func TestCLI_SuiteVerify_LibraryMalformedReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	st, err := library.Open(libDir)
	if err != nil {
		t.Fatal(err)
	}
	doc := minimalRedactedLibraryDoc("INC-SUITE-LIB-MALFORMED", "Suite malformed library incident")
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

	a := writeScenarioYAMLWithFingerprint(t, dir, "suite-lib-malformed-a", addRes.Fingerprint)
	suitePath := writeSuiteYAML(t, dir, "suite-lib-malformed", []string{filepath.Base(a)})

	stdout, stderr, code := runCLI(t, bin, "suite", "verify", "--library", libDir, suitePath)
	if code != 1 {
		t.Fatalf("expected exit 1 for a malformed library, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "malformed") {
		t.Fatalf("expected stderr to describe the malformed library, got: %s", stderr)
	}
	// The existing suite/scenario-count header is printed before the library
	// lookup ever runs, unaffected by the later library failure.
	if !strings.Contains(stdout, "Suite: suite-lib-malformed") || !strings.Contains(stdout, "Scenarios: 1 listed") {
		t.Fatalf("expected the existing suite header to have already printed, got stdout: %s", stdout)
	}
}

func TestCLI_SuiteVerify_LibraryOmittedProducesUnchangedOutput(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	a := writeScenarioYAML(t, dir, "suite-no-library-flag-a", []string{"/bin/true"}, 0, 5)
	b := writeScenarioYAML(t, dir, "suite-no-library-flag-b", []string{"/bin/true"}, 0, 5)
	suitePath := writeSuiteYAML(t, dir, "no-library-flag-suite", []string{filepath.Base(a), filepath.Base(b)})

	stdout, stderr, code := runCLI(t, bin, "suite", "verify", suitePath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "Library") {
		t.Fatalf("expected no Library-related output when --library is omitted, got: %s", stdout)
	}
	if !strings.Contains(stdout, "OK: suite manifest and all 2 listed scenarios are structurally valid") {
		t.Fatalf("expected the unchanged validity summary line, got: %s", stdout)
	}
}

// ============================================================================
// 2. incidentdna suite run
// ============================================================================

func TestCLI_SuiteRun_AggregatePass(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	a := writeScenarioYAML(t, dir, "run-pass-a", []string{"/bin/true"}, 0, 5)
	b := writeScenarioYAML(t, dir, "run-pass-b", []string{"/bin/true"}, 0, 5)
	suitePath := writeSuiteYAML(t, dir, "all-pass", []string{filepath.Base(a), filepath.Base(b)})

	stdout, stderr, code := runCLI(t, bin, "suite", "run", suitePath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: PASS") {
		t.Fatalf("expected stdout to report PASS, got: %s", stdout)
	}
	if !strings.Contains(stdout, "2 PASS") {
		t.Fatalf("expected stdout to report 2 PASS, got: %s", stdout)
	}
}

func TestCLI_SuiteRun_AggregateFailReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	passPath := writeScenarioYAML(t, dir, "mix-pass", []string{"/bin/true"}, 0, 5)
	failPath := writeScenarioYAML(t, dir, "mix-fail", []string{"/bin/false"}, 0, 5)
	suitePath := writeSuiteYAML(t, dir, "mixed", []string{filepath.Base(passPath), filepath.Base(failPath)})

	stdout, stderr, code := runCLI(t, bin, "suite", "run", suitePath)
	if code != 2 {
		t.Fatalf("expected exit 2 for aggregate FAIL, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: FAIL") {
		t.Fatalf("expected stdout to report FAIL, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 PASS, 1 FAIL") {
		t.Fatalf("expected stdout to report the per-outcome counts, got: %s", stdout)
	}
}

func TestCLI_SuiteRun_AggregateInvalidReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	suitePath := writeSuiteYAML(t, dir, "invalid-run", []string{"does-not-exist.yaml"})

	stdout, stderr, code := runCLI(t, bin, "suite", "run", suitePath)
	if code != 1 {
		t.Fatalf("expected exit 1 for aggregate INVALID, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: INVALID") {
		t.Fatalf("expected stdout to report INVALID, got: %s", stdout)
	}
}

func TestCLI_SuiteRun_LoadFailureReportsInternalErrorAndExitOne(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "suite", "run", filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if code != 1 {
		t.Fatalf("expected exit 1, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Result: INTERNAL_ERROR") {
		t.Fatalf("expected stdout to report INTERNAL_ERROR, got: %s", stdout)
	}
}

func TestCLI_SuiteRun_ReportMatchesSummary(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	a := writeScenarioYAML(t, dir, "report-a", []string{"/bin/true"}, 0, 5)
	suitePath := writeSuiteYAML(t, dir, "report-suite", []string{filepath.Base(a)})
	reportPath := filepath.Join(dir, "report.json")

	stdout, stderr, code := runCLI(t, bin, "suite", "run", "--report", reportPath, suitePath)
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
	if rep["suite_id"] != "report-suite" {
		t.Fatalf("expected report suite_id report-suite, got: %v", rep["suite_id"])
	}
	if rep["schema_version"] != "suite-report/v0.1" {
		t.Fatalf("expected report schema_version suite-report/v0.1, got: %v", rep["schema_version"])
	}
	counts, ok := rep["counts"].(map[string]any)
	if !ok || counts["pass"].(float64) != 1 {
		t.Fatalf("expected counts.pass == 1, got: %v", rep["counts"])
	}
	scenarios, ok := rep["scenarios"].([]any)
	if !ok || len(scenarios) != 1 {
		t.Fatalf("expected 1 embedded scenario report, got: %v", rep["scenarios"])
	}
	first := scenarios[0].(map[string]any)
	if first["report"] == nil {
		t.Fatalf("expected the embedded scenario report to be non-null, got: %v", first)
	}
}

func TestCLI_SuiteRun_KeepWorkspacesLeavesInspectableDirectories(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	a := writeScenarioYAML(t, dir, "keep-a", []string{"/bin/true"}, 0, 5)
	b := writeScenarioYAML(t, dir, "keep-b", []string{"/bin/true"}, 0, 5)
	suitePath := writeSuiteYAML(t, dir, "keep-suite", []string{filepath.Base(a), filepath.Base(b)})

	workspaceRoot := filepath.Join(dir, "ws-root")
	stdout, stderr, code := runCLI(t, bin, "suite", "run", "--workspace-root", workspaceRoot, "--keep-workspaces", suitePath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	entries, err := os.ReadDir(workspaceRoot)
	if err != nil {
		t.Fatalf("expected the workspace root to exist and be inspectable: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 per-scenario subdirectories under the workspace root, got %d: %v", len(entries), entries)
	}
}

func TestCLI_SuiteRun_WorkspaceRootMustNotPreExistNonEmpty(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	a := writeScenarioYAML(t, dir, "preexist-a", []string{"/bin/true"}, 0, 5)
	suitePath := writeSuiteYAML(t, dir, "preexist-suite", []string{filepath.Base(a)})

	workspaceRoot := filepath.Join(dir, "ws-root")
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceRoot, "pre-existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "suite", "run", "--workspace-root", workspaceRoot, suitePath)
	if code != 1 {
		t.Fatalf("expected exit 1 for a pre-existing non-empty --workspace-root, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: INTERNAL_ERROR") {
		t.Fatalf("expected stdout to report INTERNAL_ERROR, got: %s", stdout)
	}
}

func TestCLI_SuiteRun_FailFastSkipsLaterScenarios(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	failPath := writeScenarioYAML(t, dir, "ff-fail", []string{"/bin/false"}, 0, 5)
	neverPath := writeScenarioYAML(t, dir, "ff-never", []string{"/bin/true"}, 0, 5)
	suitePath := writeSuiteYAML(t, dir, "fail-fast-suite", []string{filepath.Base(failPath), filepath.Base(neverPath)})

	stdout, stderr, code := runCLI(t, bin, "suite", "run", "--fail-fast", suitePath)
	if code != 2 {
		t.Fatalf("expected exit 2, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "SKIPPED") {
		t.Fatalf("expected stdout to report a SKIPPED scenario, got: %s", stdout)
	}
}

func TestCLI_SuiteRun_InvalidUsage(t *testing.T) {
	bin := buildBinary(t)
	cases := [][]string{
		{"suite", "run"},
		{"suite", "run", "a", "b"},
	}
	for _, args := range cases {
		_, stderr, code := runCLI(t, bin, args...)
		if code != 1 {
			t.Fatalf("args %v: expected exit 1, got %d; stderr: %s", args, code, stderr)
		}
	}
}

func TestCLI_SuiteRun_SelfReferentialAgainstRealBinary(t *testing.T) {
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
	suitePath := writeSuiteYAML(t, dir, "self-referential-suite", []string{"self-referential.yaml"})

	stdout, stderr, code := runCLI(t, bin, "suite", "run", suitePath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Result: PASS") {
		t.Fatalf("expected stdout to report PASS, got: %s", stdout)
	}
}

// ============================================================================
// 3. General CLI behavior
// ============================================================================

func TestCLI_TopLevelHelpIncludesSuiteCommand(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout+stderr, "suite") {
		t.Fatalf("expected top-level --help to mention the suite command, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestCLI_SuiteHelpIncludesBothSubcommands(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "suite", "--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	combined := stdout + stderr
	for _, want := range []string{"suite verify", "suite run"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected suite --help to mention %q, got:\n%s", want, combined)
		}
	}
}

func TestCLI_SuiteUnknownSubcommandReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "suite", "bogus")
	if code != 1 {
		t.Fatalf("expected exit 1 for an unknown suite subcommand, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_SuiteNoSubcommandReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "suite")
	if code != 1 {
		t.Fatalf("expected exit 1 for `suite` with no subcommand, got %d; stderr: %s", code, stderr)
	}
}
