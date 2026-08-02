package suite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalYAML = `
schema_version: suite/v0.1
suite:
  id: minimal-suite
scenarios:
  - path: scenario-a.yaml
  - path: scenario-b.yaml
`

const minimalJSON = `{"schema_version":"suite/v0.1","suite":{"id":"minimal-suite"},"scenarios":[{"path":"scenario-a.yaml"},{"path":"scenario-b.yaml"}]}`

func TestLoadFile_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "suite.yaml")
	if err := os.WriteFile(path, []byte(minimalYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Suite.ID != "minimal-suite" {
		t.Fatalf("unexpected suite id: %q", doc.Suite.ID)
	}
	if len(doc.Scenarios) != 2 || doc.Scenarios[0].Path != "scenario-a.yaml" || doc.Scenarios[1].Path != "scenario-b.yaml" {
		t.Fatalf("unexpected scenarios: %v", doc.Scenarios)
	}
}

func TestLoadFile_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "suite.json")
	if err := os.WriteFile(path, []byte(minimalJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Suite.ID != "minimal-suite" {
		t.Fatalf("unexpected suite id: %q", doc.Suite.ID)
	}
}

func TestLoadFile_SniffsJSONWithUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "suite.txt")
	if err := os.WriteFile(path, []byte(minimalJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Suite.ID != "minimal-suite" {
		t.Fatalf("unexpected suite id: %q", doc.Suite.ID)
	}
}

func TestLoadFile_RejectsOversizedDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.yaml")
	huge := strings.Repeat("a", MaxSuiteDocumentSize+1)
	if err := os.WriteFile(path, []byte(huge), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected an error for an oversized document, got nil")
	}
	if !strings.Contains(err.Error(), "maximum suite document size") {
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
