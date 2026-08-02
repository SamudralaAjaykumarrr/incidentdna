package scenario

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// validDoc returns a Document that passes Validate with no scenarioDir
// dependency (no workspace_files), suitable as a base fixture that table
// cases mutate one field at a time.
func validDoc() *Document {
	exitCode := 0
	return &Document{
		SchemaVersion:     SupportedSchemaVersion,
		Scenario:          Info{ID: "valid-scenario"},
		LinkedFingerprint: "sha256:" + strings.Repeat("a", 64),
		Execution: Execution{
			Command: []string{"/bin/true"},
		},
		Expected: Expected{ExitCode: &exitCode},
	}
}

func TestValidate_AcceptsMinimalValidDocument(t *testing.T) {
	res := Validate(context.Background(), validDoc(), "")
	if !res.Valid() {
		t.Fatalf("expected a valid document, got issues: %v", res.Issues)
	}
}

func TestValidate_RejectsMissingOrWrongSchemaVersion(t *testing.T) {
	for _, sv := range []string{"", "irs/v0.2", "0.1"} {
		doc := validDoc()
		doc.SchemaVersion = sv
		res := Validate(context.Background(), doc, "")
		if res.Valid() {
			t.Fatalf("schema_version %q: expected invalid", sv)
		}
	}
}

func TestValidate_RejectsEmptyScenarioID(t *testing.T) {
	doc := validDoc()
	doc.Scenario.ID = ""
	res := Validate(context.Background(), doc, "")
	if res.Valid() {
		t.Fatal("expected invalid for empty scenario.id")
	}
}

func TestValidate_RejectsEmptyCommand(t *testing.T) {
	doc := validDoc()
	doc.Execution.Command = nil
	res := Validate(context.Background(), doc, "")
	if res.Valid() {
		t.Fatal("expected invalid for empty command")
	}
}

func TestValidate_RejectsBareCommand(t *testing.T) {
	for _, bare := range []string{"echo", "python3", "sh"} {
		doc := validDoc()
		doc.Execution.Command = []string{bare}
		res := Validate(context.Background(), doc, "")
		if res.Valid() {
			t.Fatalf("command[0] %q: expected invalid (bare command)", bare)
		}
	}
}

func TestValidate_AcceptsAbsoluteCommand(t *testing.T) {
	doc := validDoc()
	doc.Execution.Command = []string{"/usr/bin/env"}
	res := Validate(context.Background(), doc, "")
	if !res.Valid() {
		t.Fatalf("expected an absolute command[0] to be accepted, got issues: %v", res.Issues)
	}
}

func TestValidate_AcceptsWorkspaceRelativeCommand(t *testing.T) {
	doc := validDoc()
	doc.Execution.Command = []string{"../../../bin/incidentdna"}
	res := Validate(context.Background(), doc, "")
	if !res.Valid() {
		t.Fatalf("expected a workspace-relative command[0] to be accepted, got issues: %v", res.Issues)
	}
}

func TestValidate_WorkspaceFiles_RejectsAbsoluteSource(t *testing.T) {
	doc := validDoc()
	doc.Execution.WorkspaceFiles = []WorkspaceFile{{Source: "/etc/passwd", Destination: "d"}}
	res := Validate(context.Background(), doc, t.TempDir())
	if res.Valid() {
		t.Fatal("expected invalid for an absolute workspace_files source")
	}
}

func TestValidate_WorkspaceFiles_RejectsTraversalSource(t *testing.T) {
	doc := validDoc()
	doc.Execution.WorkspaceFiles = []WorkspaceFile{{Source: "../outside.txt", Destination: "d"}}
	res := Validate(context.Background(), doc, t.TempDir())
	if res.Valid() {
		t.Fatal("expected invalid for a traversal workspace_files source")
	}
}

func TestValidate_WorkspaceFiles_RejectsAbsoluteDestination(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "f.txt", "hello")
	doc := validDoc()
	doc.Execution.WorkspaceFiles = []WorkspaceFile{{Source: "f.txt", Destination: "/tmp/evil"}}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected invalid for an absolute workspace_files destination")
	}
}

func TestValidate_WorkspaceFiles_RejectsTraversalDestination(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "f.txt", "hello")
	doc := validDoc()
	doc.Execution.WorkspaceFiles = []WorkspaceFile{{Source: "f.txt", Destination: "../escape.txt"}}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected invalid for a traversal workspace_files destination")
	}
}

func TestValidate_WorkspaceFiles_RejectsSymlinkSource(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "real.txt", "hello")
	symlink := filepath.Join(dir, "link.txt")
	if err := os.Symlink(filepath.Join(dir, "real.txt"), symlink); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	doc := validDoc()
	doc.Execution.WorkspaceFiles = []WorkspaceFile{{Source: "link.txt", Destination: "d"}}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected invalid for a symlinked workspace_files source")
	}
}

func TestValidate_WorkspaceFiles_RejectsMissingSource(t *testing.T) {
	dir := t.TempDir()
	doc := validDoc()
	doc.Execution.WorkspaceFiles = []WorkspaceFile{{Source: "does-not-exist.txt", Destination: "d"}}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected invalid for a missing workspace_files source")
	}
}

func TestValidate_WorkspaceFiles_RejectsDuplicateDestination(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.txt", "a")
	writeFixture(t, dir, "b.txt", "b")
	doc := validDoc()
	doc.Execution.WorkspaceFiles = []WorkspaceFile{
		{Source: "a.txt", Destination: "same.txt"},
		{Source: "b.txt", Destination: "same.txt"},
	}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected invalid for duplicate workspace_files destinations")
	}
}

func TestValidate_WorkspaceFiles_AcceptsValidEntries(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.txt", "a")
	writeFixture(t, dir, "b.txt", "b")
	doc := validDoc()
	doc.Execution.WorkspaceFiles = []WorkspaceFile{
		{Source: "a.txt", Destination: "one.txt"},
		{Source: "b.txt", Destination: "sub/two.txt"},
	}
	res := Validate(context.Background(), doc, dir)
	if !res.Valid() {
		t.Fatalf("expected valid, got issues: %v", res.Issues)
	}
}

func TestValidate_RejectsTimeoutBelowMinimum(t *testing.T) {
	doc := validDoc()
	zero := 0
	doc.Execution.TimeoutSeconds = &zero
	res := Validate(context.Background(), doc, "")
	if res.Valid() {
		t.Fatal("expected invalid for timeout_seconds 0")
	}
}

func TestValidate_RejectsTimeoutAboveMaximum(t *testing.T) {
	doc := validDoc()
	tooLong := MaxScenarioTimeoutSeconds + 1
	doc.Execution.TimeoutSeconds = &tooLong
	res := Validate(context.Background(), doc, "")
	if res.Valid() {
		t.Fatal("expected invalid for an over-limit timeout_seconds")
	}
}

func TestValidate_AcceptsOmittedTimeout(t *testing.T) {
	doc := validDoc()
	doc.Execution.TimeoutSeconds = nil
	res := Validate(context.Background(), doc, "")
	if !res.Valid() {
		t.Fatalf("expected valid with omitted timeout_seconds, got: %v", res.Issues)
	}
}

func TestValidate_RejectsMalformedLinkedFingerprint(t *testing.T) {
	for _, fp := range []string{"", "not-a-fingerprint", "sha256:tooshort", "md5:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("A", 64)} {
		doc := validDoc()
		doc.LinkedFingerprint = fp
		res := Validate(context.Background(), doc, "")
		if res.Valid() {
			t.Fatalf("linked_fingerprint %q: expected invalid", fp)
		}
	}
}

func TestValidate_RejectsMissingExpectedExitCode(t *testing.T) {
	doc := validDoc()
	doc.Expected.ExitCode = nil
	res := Validate(context.Background(), doc, "")
	if res.Valid() {
		t.Fatal("expected invalid for missing expected.exit_code")
	}
}

func TestValidate_AcceptsExplicitZeroExpectedExitCode(t *testing.T) {
	doc := validDoc()
	zero := 0
	doc.Expected.ExitCode = &zero
	res := Validate(context.Background(), doc, "")
	if !res.Valid() {
		t.Fatalf("expected valid for an explicit zero exit code, got: %v", res.Issues)
	}
}

func TestValidate_RejectsUnknownStreamMode(t *testing.T) {
	doc := validDoc()
	doc.Expected.Stdout = &StreamAssertion{Mode: "fuzzy", Value: "x"}
	res := Validate(context.Background(), doc, "")
	if res.Valid() {
		t.Fatal("expected invalid for an unknown stdout mode")
	}
}

func TestValidate_AcceptsKnownStreamModes(t *testing.T) {
	for _, mode := range []string{StreamModeExact, StreamModeContains, StreamModeRegex} {
		doc := validDoc()
		doc.Expected.Stdout = &StreamAssertion{Mode: mode, Value: "abc"}
		res := Validate(context.Background(), doc, "")
		if !res.Valid() {
			t.Fatalf("mode %q: expected valid, got: %v", mode, res.Issues)
		}
	}
}

func TestValidate_RejectsInvalidRegexValue(t *testing.T) {
	doc := validDoc()
	doc.Expected.Stderr = &StreamAssertion{Mode: StreamModeRegex, Value: "(unclosed"}
	res := Validate(context.Background(), doc, "")
	if res.Valid() {
		t.Fatal("expected invalid for an unparseable regex value")
	}
}

func TestValidate_RejectsTooManyWorkspaceFiles(t *testing.T) {
	dir := t.TempDir()
	doc := validDoc()
	for i := 0; i < MaxWorkspaceFiles+1; i++ {
		fname := "f" + strconv.Itoa(i) + ".txt"
		writeFixture(t, dir, fname, "x")
		doc.Execution.WorkspaceFiles = append(doc.Execution.WorkspaceFiles, WorkspaceFile{
			Source:      fname,
			Destination: fname,
		})
	}
	res := Validate(context.Background(), doc, dir)
	if res.Valid() {
		t.Fatal("expected invalid for exceeding MaxWorkspaceFiles")
	}
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
