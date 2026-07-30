package library

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// validDoc returns a minimal, semantically valid IDIR document (passes
// validate.Validate) with the given incident id. Tests mutate a copy's
// fields to explore the §5 occurrence-decision cases.
func validDoc(incidentID string) *idir.Document {
	return &idir.Document{
		SchemaVersion: idir.SupportedSchemaVersion,
		Incident: idir.Incident{
			ID:         incidentID,
			Title:      "Example incident",
			Summary:    "An example incident used for library tests.",
			OccurredAt: "2026-01-01T00:00:00Z",
		},
		Application: idir.Application{
			Name:           "example-app",
			Service:        "example-service",
			Environment:    "production",
			ReleaseVersion: "1.0.0",
		},
		Trigger: idir.Trigger{
			Type:        "example-trigger",
			Description: "An example trigger.",
		},
		Events: []idir.Event{
			{ID: "evt-1", Type: "example-event", Description: "An example event."},
		},
		BusinessInvariants: []idir.Invariant{
			{ID: "inv-1", Statement: "An example invariant."},
		},
		ExpectedCorrectedBehavior: []string{"The system should behave correctly."},
		Privacy: idir.Privacy{
			Sensitivity: idir.SensitivityInternal,
			Redacted:    false,
		},
	}
}

func TestAdd_FirstOccurrenceStoredSuccessfully(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")

	res, err := Add(context.Background(), s, doc)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if res.Outcome != AddOutcomeStored {
		t.Errorf("Outcome = %q, want %q", res.Outcome, AddOutcomeStored)
	}
	if res.Fingerprint == "" || res.Digest == "" {
		t.Errorf("expected non-empty fingerprint/digest, got %+v", res)
	}

	occPath, err := s.OccurrencePath(res.Fingerprint, res.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(occPath); err != nil {
		t.Errorf("expected occurrence file at %q: %v", occPath, err)
	}
	idxPath, err := s.IndexPath(res.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := readIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Entries) != 1 || idx.Entries[0].IncidentID != "INC-1" || idx.Entries[0].Digest != res.Digest {
		t.Errorf("unexpected index content: %+v", idx.Entries)
	}
}

func TestAdd_FormattingOnlyDifferenceIsIdempotent(t *testing.T) {
	s, parent := openStoreT(t)

	yamlContent := `schema_version: "0.1"
incident:
  id: INC-FORMAT-TEST
  title: Formatting test incident
  summary: A formatting test incident used to prove canonicalization normalizes source formatting.
  occurred_at: "2026-01-02T00:00:00Z"
application:
  name: example-app
  service: example-service
  environment: production
  release_version: "1.0.0"
trigger:
  type: example-trigger
  description: An example trigger for the formatting test.
events:
  - id: evt-1
    type: example-event
    description: An example event for the formatting test.
business_invariants:
  - id: inv-1
    statement: An example invariant for the formatting test.
expected_corrected_behavior:
  - The system should behave correctly.
privacy:
  sensitivity: internal
  redacted: false
`
	jsonContent := `{
  "privacy": { "redacted": false, "sensitivity": "internal" },
  "expected_corrected_behavior": ["The system should behave correctly."],
  "business_invariants": [ { "statement": "An example invariant for the formatting test.", "id": "inv-1" } ],
  "events": [ { "description": "An example event for the formatting test.", "type": "example-event", "id": "evt-1" } ],
  "trigger": { "description": "An example trigger for the formatting test.", "type": "example-trigger" },
  "application": { "release_version": "1.0.0", "environment": "production", "service": "example-service", "name": "example-app" },
  "incident": { "occurred_at": "2026-01-02T00:00:00Z", "summary": "A formatting test incident used to prove canonicalization normalizes source formatting.", "title": "Formatting test incident", "id": "INC-FORMAT-TEST" },
  "schema_version": "0.1"
}
`
	yamlPath := filepath.Join(parent, "fixture.yaml")
	jsonPath := filepath.Join(parent, "fixture.json")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0o644); err != nil {
		t.Fatal(err)
	}

	docA, err := idir.LoadFile(yamlPath)
	if err != nil {
		t.Fatalf("LoadFile(yaml): %v", err)
	}
	docB, err := idir.LoadFile(jsonPath)
	if err != nil {
		t.Fatalf("LoadFile(json): %v", err)
	}

	res1, err := Add(context.Background(), s, docA)
	if err != nil {
		t.Fatalf("Add(yaml): %v", err)
	}
	if res1.Outcome != AddOutcomeStored {
		t.Fatalf("first Add outcome = %q, want %q", res1.Outcome, AddOutcomeStored)
	}

	res2, err := Add(context.Background(), s, docB)
	if err != nil {
		t.Fatalf("Add(json): %v", err)
	}
	if res2.Outcome != AddOutcomeIdempotent {
		t.Errorf("second Add outcome = %q, want %q", res2.Outcome, AddOutcomeIdempotent)
	}
	if res2.Fingerprint != res1.Fingerprint || res2.Digest != res1.Digest {
		t.Errorf("expected identical fingerprint/digest across reformatted copies, got %+v vs %+v", res1, res2)
	}

	idxPath, err := s.IndexPath(res1.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := readIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Entries) != 1 {
		t.Errorf("expected exactly one index entry after idempotent re-add, got %d", len(idx.Entries))
	}
}

func TestAdd_DifferentIncidentIDSameFingerprintCreatesAnotherOccurrence(t *testing.T) {
	s, _ := openStoreT(t)
	docA := validDoc("INC-A")
	docB := validDoc("INC-B") // identical fingerprint-relevant fields, different incident.id

	resA, err := Add(context.Background(), s, docA)
	if err != nil {
		t.Fatalf("Add(A): %v", err)
	}
	resB, err := Add(context.Background(), s, docB)
	if err != nil {
		t.Fatalf("Add(B): %v", err)
	}

	if resA.Fingerprint != resB.Fingerprint {
		t.Fatalf("expected same fingerprint for A and B, got %q vs %q", resA.Fingerprint, resB.Fingerprint)
	}
	if resB.Outcome != AddOutcomeStored {
		t.Errorf("Outcome for B = %q, want %q (a new occurrence, not a dedup)", resB.Outcome, AddOutcomeStored)
	}
	if resA.Digest == resB.Digest {
		t.Error("expected different occurrence digests for different incident ids")
	}

	idxPath, err := s.IndexPath(resA.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := readIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Entries) != 2 {
		t.Fatalf("expected 2 index entries, got %d: %+v", len(idx.Entries), idx.Entries)
	}
}

func TestAdd_SameIncidentIDDifferentContentReturnsConflict(t *testing.T) {
	s, _ := openStoreT(t)
	doc1 := validDoc("INC-1")
	doc2 := validDoc("INC-1")
	doc2.Incident.Summary = "A materially different summary describing a different narrative."

	res1, err := Add(context.Background(), s, doc1)
	if err != nil {
		t.Fatalf("Add(doc1): %v", err)
	}

	_, err = Add(context.Background(), s, doc2)
	if err == nil {
		t.Fatal("Add(doc2): expected conflict error, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("Add(doc2): expected ErrConflict, got %v", err)
	}

	// The original occurrence must be untouched, and no second occurrence
	// silently created.
	idxPath, err := s.IndexPath(res1.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := readIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Entries) != 1 {
		t.Fatalf("expected index to still have exactly 1 entry after a refused conflict, got %d", len(idx.Entries))
	}

	occPath, err := s.OccurrencePath(res1.Fingerprint, res1.Digest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(occPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "materially different") {
		t.Error("original occurrence content must not reflect the refused conflicting write")
	}
}

func TestAdd_RejectsDocumentFailingValidation(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("") // empty incident.id fails validate.Validate

	_, err := Add(context.Background(), s, doc)
	if err == nil {
		t.Fatal("Add: expected validation error, got nil")
	}
	if !errors.Is(err, ErrInvalidDocument) {
		t.Errorf("expected ErrInvalidDocument, got %v", err)
	}

	entries, err := os.ReadDir(s.Root())
	if err == nil && len(entries) != 0 {
		t.Errorf("expected nothing written to the library root, found %d entries", len(entries))
	}
}

func TestAdd_MalformedIndexIsDetected(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	res, err := Add(context.Background(), s, doc)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	idxPath, err := s.IndexPath(res.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(idxPath, []byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc2 := validDoc("INC-2") // same fingerprint-relevant fields, different id
	_, err = Add(context.Background(), s, doc2)
	if err == nil {
		t.Fatal("Add against a malformed index: expected error, got nil")
	}
	if !errors.Is(err, ErrMalformedIndex) {
		t.Errorf("expected ErrMalformedIndex, got %v", err)
	}
}

func TestAdd_MissingOccurrenceReferencedByIndexIsCorrupted(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	res, err := Add(context.Background(), s, doc)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	occPath, err := s.OccurrencePath(res.Fingerprint, res.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(occPath); err != nil {
		t.Fatal(err)
	}

	// Re-adding the same incident id forces Add to consult (and find missing)
	// the occurrence the index references.
	_, err = Add(context.Background(), s, doc)
	if err == nil {
		t.Fatal("Add with a missing referenced occurrence: expected error, got nil")
	}
	if !errors.Is(err, ErrCorruptedOccurrence) {
		t.Errorf("expected ErrCorruptedOccurrence, got %v", err)
	}
}

func TestAdd_CorruptedOccurrenceContentIsDetected(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	res, err := Add(context.Background(), s, doc)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	occPath, err := s.OccurrencePath(res.Fingerprint, res.Digest)
	if err != nil {
		t.Fatal(err)
	}
	// occurrence files are written read-only; restore write permission to
	// simulate on-disk tampering/corruption.
	if err := os.Chmod(occPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(occPath, []byte(`{"tampered":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Add(context.Background(), s, doc)
	if err == nil {
		t.Fatal("Add against a corrupted occurrence: expected error, got nil")
	}
	if !errors.Is(err, ErrCorruptedOccurrence) {
		t.Errorf("expected ErrCorruptedOccurrence, got %v", err)
	}
}

func TestAdd_DeterministicIndexOrdering(t *testing.T) {
	s, _ := openStoreT(t)
	ids := []string{"INC-Z", "INC-A", "INC-M"}
	var fp string
	for _, id := range ids {
		res, err := Add(context.Background(), s, validDoc(id))
		if err != nil {
			t.Fatalf("Add(%s): %v", id, err)
		}
		fp = res.Fingerprint
	}

	idxPath, err := s.IndexPath(fp)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	var idx index
	if err := json.Unmarshal(data, &idx); err != nil {
		t.Fatal(err)
	}
	if len(idx.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(idx.Entries))
	}
	for i := 1; i < len(idx.Entries); i++ {
		if idx.Entries[i-1].Digest > idx.Entries[i].Digest {
			t.Errorf("index entries not sorted by digest ascending: %+v", idx.Entries)
		}
	}

	// Re-reading and re-writing the same logical index must reproduce
	// byte-identical output (docs/phase-3-plan.md §13).
	idx2, err := readIndex(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeIndex(idxPath, idx2); err != nil {
		t.Fatal(err)
	}
	data2, err := os.ReadFile(idxPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(data2) {
		t.Error("rewriting an unchanged index produced different bytes; expected deterministic serialization")
	}
}

func TestAdd_NeverUsesIncidentIDAsPathComponent(t *testing.T) {
	s, _ := openStoreT(t)
	suspicious := "../../../etc/passwd-INC-1"
	doc := validDoc(suspicious)

	res, err := Add(context.Background(), s, doc)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	occPath, err := s.OccurrencePath(res.Fingerprint, res.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(occPath, suspicious) {
		t.Errorf("occurrence path %q must never contain the incident id", occPath)
	}
	rootWithSep := s.Root() + string(os.PathSeparator)
	if !strings.HasPrefix(occPath, rootWithSep) {
		t.Errorf("occurrence path %q escaped the library root %q", occPath, s.Root())
	}
}

func TestWriteOccurrenceAtomic_CleansUpTempFileOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "occurrence.json")
	// Pre-create a non-empty directory at the destination path so the final
	// os.Rename fails (renaming a file onto an existing directory).
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeOccurrenceAtomic(path, []byte(`{"x":1}`)); err == nil {
		t.Fatal("writeOccurrenceAtomic: expected rename failure, got nil")
	}

	leftover, err := filepath.Glob(filepath.Join(dir, ".occurrence-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftover) != 0 {
		t.Errorf("expected no leftover temp files after a failed write, found %v", leftover)
	}
}

func TestWriteIndex_CleansUpTempFileOnRenameFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.json")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	idx := &index{Entries: []indexEntry{{Digest: strings.Repeat("a", 64), IncidentID: "INC-1", OccurredAt: "2026-01-01T00:00:00Z"}}}
	if err := writeIndex(path, idx); err == nil {
		t.Fatal("writeIndex: expected rename failure, got nil")
	}

	leftover, err := filepath.Glob(filepath.Join(dir, ".index-*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftover) != 0 {
		t.Errorf("expected no leftover temp files after a failed write, found %v", leftover)
	}
}

func TestReadIndex_MissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	idx, err := readIndex(filepath.Join(dir, "does-not-exist", "index.json"))
	if err != nil {
		t.Fatalf("readIndex on a missing file: unexpected error: %v", err)
	}
	if len(idx.Entries) != 0 {
		t.Errorf("expected empty index, got %+v", idx.Entries)
	}
}

func TestReadIndex_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real-index.json")
	if err := os.WriteFile(real, []byte(`{"entries":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "index.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	if _, err := readIndex(link); !errors.Is(err, ErrMalformedIndex) {
		t.Errorf("expected ErrMalformedIndex for a symlinked index, got %v", err)
	}
}
