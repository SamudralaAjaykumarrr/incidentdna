package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalPolicyYAML = `
schema_version: policy/v0.1
policy:
  id: minimal-policy
rules:
  - type: require_result
    value: PASS
`

const minimalPolicyJSON = `{"schema_version":"policy/v0.1","policy":{"id":"minimal-policy"},"rules":[{"type":"require_result","value":"PASS"}]}`

func TestLoadFile_YAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")
	if err := os.WriteFile(path, []byte(minimalPolicyYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Policy.ID != "minimal-policy" {
		t.Fatalf("unexpected policy id: %q", doc.Policy.ID)
	}
	if len(doc.Rules) != 1 || doc.Rules[0].Type != RuleTypeRequireResult || doc.Rules[0].Value != "PASS" {
		t.Fatalf("unexpected rules: %+v", doc.Rules)
	}
}

func TestLoadFile_JSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(path, []byte(minimalPolicyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Policy.ID != "minimal-policy" {
		t.Fatalf("unexpected policy id: %q", doc.Policy.ID)
	}
}

func TestLoadFile_SniffsJSONWithUnknownExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.txt")
	if err := os.WriteFile(path, []byte(minimalPolicyJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if doc.Policy.ID != "minimal-policy" {
		t.Fatalf("unexpected policy id: %q", doc.Policy.ID)
	}
}

func TestLoadFile_RejectsOversizedDocument(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.yaml")
	huge := strings.Repeat("a", MaxPolicyDocumentSize+1)
	if err := os.WriteFile(path, []byte(huge), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected an error for an oversized document, got nil")
	}
	if !strings.Contains(err.Error(), "maximum policy document size") {
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

func TestLoadFile_RejectsMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFile(path)
	if err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
}

func TestLoadFile_RejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadFile(dir)
	if err == nil {
		t.Fatal("expected an error when path is a directory, got nil")
	}
}
