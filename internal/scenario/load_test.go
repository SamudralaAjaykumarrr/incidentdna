package scenario

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalYAML = `
schema_version: irs/v0.1
scenario:
  id: minimal-scenario
execution:
  command:
    - /bin/true
expected:
  exit_code: 0
`

const minimalJSON = `{"schema_version":"irs/v0.1","scenario":{"id":"minimal-scenario"},"execution":{"command":["/bin/true"]},"expected":{"exit_code":0}}`

func TestLoadFile_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.yaml")
	if err := os.WriteFile(path, []byte(minimalYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Scenario.ID != "minimal-scenario" {
		t.Fatalf("unexpected scenario id: %q", doc.Scenario.ID)
	}
	if len(doc.Execution.Command) != 1 || doc.Execution.Command[0] != "/bin/true" {
		t.Fatalf("unexpected command: %v", doc.Execution.Command)
	}
	if doc.Expected.ExitCode == nil || *doc.Expected.ExitCode != 0 {
		t.Fatalf("unexpected expected.exit_code: %v", doc.Expected.ExitCode)
	}
}

func TestLoadFile_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.json")
	if err := os.WriteFile(path, []byte(minimalJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Scenario.ID != "minimal-scenario" {
		t.Fatalf("unexpected scenario id: %q", doc.Scenario.ID)
	}
}

func TestLoadFile_SniffsJSONWithUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.txt")
	if err := os.WriteFile(path, []byte(minimalJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Scenario.ID != "minimal-scenario" {
		t.Fatalf("unexpected scenario id: %q", doc.Scenario.ID)
	}
}

func TestLoadFile_RejectsOversizedDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.yaml")
	huge := strings.Repeat("a", MaxScenarioDocumentSize+1)
	if err := os.WriteFile(path, []byte(huge), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected an error for an oversized document, got nil")
	}
	if !strings.Contains(err.Error(), "maximum scenario document size") {
		t.Fatalf("expected a max-size error, got: %v", err)
	}
}

func TestLoadFile_RejectsMissingFile(t *testing.T) {
	_, err := LoadFile(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestLoadFile_RejectsMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected an error for malformed YAML, got nil")
	}
}

func TestLoadFile_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadFile(dir)
	if err == nil {
		t.Fatal("expected an error when path is a directory, got nil")
	}
}

func TestLoadFile_PointerFieldsDistinguishAbsence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.yaml")
	// No timeout_seconds declared: must decode as nil, not 0, so Validate
	// can distinguish "omitted" from "explicitly zero" (invalid).
	doc := `
schema_version: irs/v0.1
scenario:
  id: no-timeout
execution:
  command:
    - /bin/true
expected:
  exit_code: 1
`
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if d.Execution.TimeoutSeconds != nil {
		t.Fatalf("expected nil TimeoutSeconds when omitted, got %v", *d.Execution.TimeoutSeconds)
	}
	if d.Expected.ExitCode == nil || *d.Expected.ExitCode != 1 {
		t.Fatalf("expected ExitCode pointer to 1, got %v", d.Expected.ExitCode)
	}
}
