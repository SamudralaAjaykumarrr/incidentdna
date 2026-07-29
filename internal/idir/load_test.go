package idir

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalYAML = `
schema_version: "0.1"
incident:
  id: INC-1
  title: t
trigger:
  type: message-redelivery
  description: d
`

const minimalJSON = `{"schema_version":"0.1","incident":{"id":"INC-1","title":"t"},"trigger":{"type":"message-redelivery","description":"d"}}`

func TestLoadFile_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "incident.yaml")
	if err := os.WriteFile(path, []byte(minimalYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Incident.ID != "INC-1" {
		t.Fatalf("unexpected incident id: %q", doc.Incident.ID)
	}
}

func TestLoadFile_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "incident.json")
	if err := os.WriteFile(path, []byte(minimalJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Incident.ID != "INC-1" {
		t.Fatalf("unexpected incident id: %q", doc.Incident.ID)
	}
}

func TestLoadFile_SniffsJSONWithUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "incident.txt")
	if err := os.WriteFile(path, []byte(minimalJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Incident.ID != "INC-1" {
		t.Fatalf("unexpected incident id: %q", doc.Incident.ID)
	}
}

func TestLoadFile_RejectsOversizedDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.yaml")
	huge := strings.Repeat("a", MaxDocumentSize+1)
	if err := os.WriteFile(path, []byte(huge), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected an error for an oversized document, got nil")
	}
	if !strings.Contains(err.Error(), "maximum document size") {
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
