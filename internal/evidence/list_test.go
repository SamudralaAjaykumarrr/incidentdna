package evidence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

func TestList_PresentAndMissing(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "stored.log", []byte("stored evidence"))
	missing := "sha256:" + strings.Repeat("9", 64)

	doc := docWithEvidence(
		idir.Evidence{ID: "ev-present", Type: "log", Digest: d.String()},
		idir.Evidence{ID: "ev-missing", Type: "log", Digest: missing},
	)
	res, err := s.List(context.Background(), doc)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if res.Entries[0].Status != StatusPresent {
		t.Errorf("entry 0 status = %v, want %v", res.Entries[0].Status, StatusPresent)
	}
	if res.Entries[1].Status != StatusMissing {
		t.Errorf("entry 1 status = %v, want %v", res.Entries[1].Status, StatusMissing)
	}
}

// TestList_DoesNotDetectContentCorruption is the key test distinguishing
// List from Verify: List reports StatusPresent for an object whose content
// has been tampered with, because it never reads or re-hashes the object —
// only Verify's re-hash would catch this. This is the behavioral proof
// behind "list presence does not prove integrity."
func TestList_DoesNotDetectContentCorruption(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "src.log", []byte("original content"))
	objPath, err := s.objectPath(d)
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(objPath, 0o644)
	if err := os.WriteFile(objPath, []byte("TAMPERED, wrong content entirely!"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: d.String()})
	res, err := s.List(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries[0].Status != StatusPresent {
		t.Errorf("List on tampered-but-present object: status = %v, want %v (List must not claim integrity)", res.Entries[0].Status, StatusPresent)
	}
}

func TestList_EnforcesMaxEntriesPerCommand(t *testing.T) {
	s, _ := openStoreT(t)
	entries := make([]idir.Evidence, MaxEntriesPerCommand+1)
	for i := range entries {
		entries[i] = idir.Evidence{ID: "ev", Type: "log", Digest: "sha256:" + strings.Repeat("a", 64)}
	}
	_, err := s.List(context.Background(), docWithEvidence(entries...))
	if err == nil {
		t.Fatal("expected an error for exceeding MaxEntriesPerCommand, got nil")
	}
	if !IsKind(err, ErrKindTooManyEntries) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindTooManyEntries)
	}
}

func TestList_InvalidDigestDeclaration(t *testing.T) {
	s, _ := openStoreT(t)
	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: "garbage"})
	res, err := s.List(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries[0].Status != StatusInvalid {
		t.Errorf("status = %v, want %v", res.Entries[0].Status, StatusInvalid)
	}
}

func TestList_NeverReadsObjectContent(t *testing.T) {
	// Indirect proof: an object far larger than MaxObjectSize would fail if
	// List ever tried to read/hash it (hashLimitedCopy would reject it).
	// List must report it Present anyway, since it only stats.
	s, dir := openStoreT(t)
	d, err := Parse("sha256:" + strings.Repeat("7", 64))
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.objectPath(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxObjectSize + 1); err != nil { // deliberately over the object cap
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	_ = dir

	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: d.String()})
	res, err := s.List(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries[0].Status != StatusPresent {
		t.Errorf("status = %v, want %v (List must not reject an oversized object, since it never reads content)", res.Entries[0].Status, StatusPresent)
	}
}
