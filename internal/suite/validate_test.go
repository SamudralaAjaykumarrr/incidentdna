package suite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeValidScenario writes a minimal, semantically valid IRS v0.1 scenario
// file (referencing /bin/true, exit code 0) into dir, and returns its bare
// filename (suitable for a scenarios[].path entry).
func writeValidScenario(t *testing.T, dir, name string, timeoutSeconds int) string {
	t.Helper()
	yaml := fmt.Sprintf(`schema_version: irs/v0.1
scenario:
  id: %s
linked_fingerprint: "sha256:%s"
execution:
  command: ["/bin/true"]
  timeout_seconds: %d
expected:
  exit_code: 0
`, name, strings.Repeat("a", 64), timeoutSeconds)
	filename := name + ".yaml"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return filename
}

func writeInvalidScenario(t *testing.T, dir, name string) string {
	t.Helper()
	// Bare command[0] ("echo"): fails scenario.Validate.
	yaml := fmt.Sprintf(`schema_version: irs/v0.1
scenario:
  id: %s
linked_fingerprint: "sha256:%s"
execution:
  command: ["echo"]
expected:
  exit_code: 0
`, name, strings.Repeat("a", 64))
	filename := name + ".yaml"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestValidate_AcceptsWellFormedSuite(t *testing.T) {
	dir := t.TempDir()
	a := writeValidScenario(t, dir, "scenario-a", 5)
	b := writeValidScenario(t, dir, "scenario-b", 5)

	doc := &Document{
		SchemaVersion: SupportedSchemaVersion,
		Suite:         Info{ID: "well-formed-suite"},
		Scenarios:     []ScenarioEntry{{Path: a}, {Path: b}},
	}
	res := Validate(context.Background(), doc, dir)
	if !res.Valid() {
		t.Fatalf("expected a valid suite, got issues: %v", res.Issues)
	}
	if len(res.Scenarios) != 2 {
		t.Fatalf("expected 2 scenario checks, got %d", len(res.Scenarios))
	}
	for _, sc := range res.Scenarios {
		if !sc.Valid {
			t.Fatalf("expected scenario %q valid, issues: %v", sc.Path, sc.Issues)
		}
		if sc.ScenarioID == "" {
			t.Fatalf("expected scenario id to be populated for %q", sc.Path)
		}
	}
}

func TestValidate_RejectsWrongSchemaVersion(t *testing.T) {
	dir := t.TempDir()
	a := writeValidScenario(t, dir, "scenario-a", 5)
	doc := &Document{
		SchemaVersion: "suite/v0.2",
		Suite:         Info{ID: "bad-schema"},
		Scenarios:     []ScenarioEntry{{Path: a}},
	}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected schema_version rejection")
	}
	assertIssueField(t, res, "schema_version")
}

func TestValidate_RejectsEmptySuiteID(t *testing.T) {
	dir := t.TempDir()
	a := writeValidScenario(t, dir, "scenario-a", 5)
	doc := &Document{
		SchemaVersion: SupportedSchemaVersion,
		Suite:         Info{ID: ""},
		Scenarios:     []ScenarioEntry{{Path: a}},
	}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected suite.id rejection")
	}
	assertIssueField(t, res, "suite.id")
}

func TestValidate_RejectsEmptyScenarios(t *testing.T) {
	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "empty"}, Scenarios: nil}
	res := Validate(context.Background(), doc, t.TempDir())
	if res.Valid() {
		t.Fatal("expected empty scenarios rejection")
	}
	assertIssueField(t, res, "scenarios")
}

func TestValidate_RejectsTooManyScenarios(t *testing.T) {
	entries := make([]ScenarioEntry, MaxScenariosPerSuite+1)
	for i := range entries {
		entries[i] = ScenarioEntry{Path: fmt.Sprintf("scenario-%d.yaml", i)}
	}
	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "too-many"}, Scenarios: entries}
	res := Validate(context.Background(), doc, t.TempDir())
	if res.Valid() {
		t.Fatal("expected too-many-scenarios rejection")
	}
	assertIssueField(t, res, "scenarios")
}

func TestValidate_RejectsPathTraversal(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"absolute", "/etc/passwd"},
		{"traversal", "../outside.yaml"},
		{"empty", ""},
		{"dot", "."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			doc := &Document{
				SchemaVersion: SupportedSchemaVersion,
				Suite:         Info{ID: "traversal"},
				Scenarios:     []ScenarioEntry{{Path: tc.path}},
			}
			res := Validate(context.Background(), doc, dir)
			if res.Valid() {
				t.Fatalf("expected path %q to be rejected", tc.path)
			}
		})
	}
}

func TestValidate_RejectsSymlinkedScenarioPath(t *testing.T) {
	dir := t.TempDir()
	real := writeValidScenario(t, dir, "real-scenario", 5)
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(filepath.Join(dir, real), link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := &Document{
		SchemaVersion: SupportedSchemaVersion,
		Suite:         Info{ID: "symlinked"},
		Scenarios:     []ScenarioEntry{{Path: "link.yaml"}},
	}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected a symlinked scenario path to be rejected")
	}
}

func TestValidate_RejectsMissingScenarioFile(t *testing.T) {
	dir := t.TempDir()
	doc := &Document{
		SchemaVersion: SupportedSchemaVersion,
		Suite:         Info{ID: "missing"},
		Scenarios:     []ScenarioEntry{{Path: "does-not-exist.yaml"}},
	}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected a missing scenario file to be rejected")
	}
}

func TestValidate_RejectsDuplicateScenarioPath(t *testing.T) {
	dir := t.TempDir()
	a := writeValidScenario(t, dir, "scenario-a", 5)
	doc := &Document{
		SchemaVersion: SupportedSchemaVersion,
		Suite:         Info{ID: "duplicate"},
		Scenarios:     []ScenarioEntry{{Path: a}, {Path: a}},
	}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected a duplicate scenario path to be rejected")
	}
	assertIssueField(t, res, "scenarios[1].path")
}

func TestValidate_RejectsSuiteListingAnInvalidScenario(t *testing.T) {
	dir := t.TempDir()
	bad := writeInvalidScenario(t, dir, "bad-scenario")
	doc := &Document{
		SchemaVersion: SupportedSchemaVersion,
		Suite:         Info{ID: "invalid-listed"},
		Scenarios:     []ScenarioEntry{{Path: bad}},
	}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected the suite to be invalid because a listed scenario is invalid")
	}
	if len(res.Scenarios) != 1 || res.Scenarios[0].Valid {
		t.Fatalf("expected the listed scenario check to be invalid, got: %+v", res.Scenarios)
	}
}

func TestValidate_RejectsOverBudgetAggregateTimeout(t *testing.T) {
	dir := t.TempDir()
	var entries []ScenarioEntry
	// 7 scenarios * 300s (the max per-scenario timeout) = 2100s > 1800s.
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("scenario-%d", i)
		p := writeValidScenario(t, dir, name, 300)
		entries = append(entries, ScenarioEntry{Path: p})
	}
	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "over-budget"}, Scenarios: entries}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected an over-budget aggregate timeout to be rejected")
	}
	assertIssueField(t, res, "scenarios")
}

func TestValidate_AcceptsAggregateTimeoutAtExactBoundary(t *testing.T) {
	dir := t.TempDir()
	var entries []ScenarioEntry
	// 6 scenarios * 300s = 1800s, exactly MaxSuiteTotalTimeoutSeconds.
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("scenario-%d", i)
		p := writeValidScenario(t, dir, name, 300)
		entries = append(entries, ScenarioEntry{Path: p})
	}
	doc := &Document{SchemaVersion: SupportedSchemaVersion, Suite: Info{ID: "at-boundary"}, Scenarios: entries}
	res := Validate(context.Background(), doc, dir)
	if !res.Valid() {
		t.Fatalf("expected the exact-boundary aggregate timeout to pass, got issues: %v", res.Issues)
	}
}

func assertIssueField(t *testing.T, res Result, wantField string) {
	t.Helper()
	for _, issue := range res.Issues {
		if strings.HasPrefix(issue.Field, wantField) {
			return
		}
	}
	t.Fatalf("expected an issue with field prefix %q, got: %v", wantField, res.Issues)
}
