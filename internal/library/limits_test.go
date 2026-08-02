package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// TestCheckDocumentSize_ExactBoundarySucceeds and
// TestCheckDocumentSize_OverBoundaryFails test checkDocumentSize directly, as
// a pure function of an explicit byte count — the same measurement
// idir.LoadFile already applies to a file's on-disk size. Add and Check do
// not call this helper (see their doc comments): a parsed *idir.Document has
// no single well-defined "original file size" to check it against, so
// exercising it through Add/Check would only test a synthetic
// re-serialization, not the real MaxDocumentSize semantics.
func TestCheckDocumentSize_ExactBoundarySucceeds(t *testing.T) {
	if err := checkDocumentSize(int64(MaxDocumentSize)); err != nil {
		t.Errorf("checkDocumentSize(MaxDocumentSize) = %v, want nil (at limit is allowed)", err)
	}
}

func TestCheckDocumentSize_OverBoundaryFails(t *testing.T) {
	err := checkDocumentSize(int64(MaxDocumentSize) + 1)
	if err == nil {
		t.Fatal("checkDocumentSize(MaxDocumentSize+1): expected error, got nil")
	}
	if !IsKind(err, ErrKindDocumentTooLarge) {
		t.Errorf("expected ErrKindDocumentTooLarge, got %v", err)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%d", MaxDocumentSize)) {
		t.Errorf("error message %q should name the MaxDocumentSize limit", err.Error())
	}
}

// seedFingerprintEntries directly creates n distinct, intact fingerprint
// groups, each holding exactly one occurrence, without going through Add —
// so tests exercising MaxLibraryEntries/MaxListResults at four- and
// five-digit counts run in a reasonable time. Every occurrence is a real,
// digest-addressed file whose content re-hashes correctly, and every index
// is valid, so List/Check/Add treat these exactly like Add-created entries.
func seedFingerprintEntries(t *testing.T, s *Store, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		fp := fmt.Sprintf("sha256:%064x", i+1)
		body := []byte(fmt.Sprintf(
			`{"incident":{"id":"INC-SEED-%d","title":"seed","occurred_at":"2026-01-01T00:00:00Z"},"application":{"service":"seed-service"}}`,
			i,
		))
		sum := sha256.Sum256(body)
		digestHex := hex.EncodeToString(sum[:])

		occPath, err := s.OccurrencePath(fp, digestHex)
		if err != nil {
			t.Fatalf("seedFingerprintEntries: OccurrencePath: %v", err)
		}
		if err := os.MkdirAll(filepath.Dir(occPath), 0o755); err != nil {
			t.Fatalf("seedFingerprintEntries: mkdir: %v", err)
		}
		if err := os.WriteFile(occPath, body, 0o444); err != nil {
			t.Fatalf("seedFingerprintEntries: write occurrence: %v", err)
		}

		idxPath, err := s.IndexPath(fp)
		if err != nil {
			t.Fatalf("seedFingerprintEntries: IndexPath: %v", err)
		}
		idx := index{Entries: []indexEntry{{
			Digest:     digestHex,
			IncidentID: fmt.Sprintf("INC-SEED-%d", i),
			OccurredAt: "2026-01-01T00:00:00Z",
		}}}
		data, err := json.Marshal(idx)
		if err != nil {
			t.Fatalf("seedFingerprintEntries: marshal index: %v", err)
		}
		if err := os.WriteFile(idxPath, data, 0o644); err != nil {
			t.Fatalf("seedFingerprintEntries: write index: %v", err)
		}
	}
}

func TestAdd_MaxLibraryEntries_ExactBoundarySucceeds(t *testing.T) {
	s, _ := openStoreT(t)
	seedFingerprintEntries(t, s, MaxLibraryEntries-1)

	res, err := Add(context.Background(), s, validDoc("INC-NEW-ENTRY"), AddOptions{})
	if err != nil {
		t.Fatalf("Add at exact MaxLibraryEntries boundary: unexpected error: %v", err)
	}
	if res.Outcome != AddOutcomeStored {
		t.Errorf("Outcome = %q, want %q", res.Outcome, AddOutcomeStored)
	}

	count, err := countFingerprintDirs(s)
	if err != nil {
		t.Fatal(err)
	}
	if count != MaxLibraryEntries {
		t.Errorf("countFingerprintDirs = %d, want %d", count, MaxLibraryEntries)
	}
}

func TestAdd_MaxLibraryEntries_OverBoundaryFails(t *testing.T) {
	s, _ := openStoreT(t)
	seedFingerprintEntries(t, s, MaxLibraryEntries)

	_, err := Add(context.Background(), s, validDoc("INC-OVER-ENTRY"), AddOptions{})
	if err == nil {
		t.Fatal("Add over MaxLibraryEntries: expected error, got nil")
	}
	if !IsKind(err, ErrKindTooManyLibraryEntries) {
		t.Errorf("expected ErrKindTooManyLibraryEntries, got %v", err)
	}
	if !strings.Contains(err.Error(), "MaxLibraryEntries") {
		t.Errorf("error message %q should name MaxLibraryEntries", err.Error())
	}

	count, err := countFingerprintDirs(s)
	if err != nil {
		t.Fatal(err)
	}
	if count != MaxLibraryEntries {
		t.Errorf("countFingerprintDirs = %d, want %d (no new entry should have been created)", count, MaxLibraryEntries)
	}
}

func TestAdd_MaxLibraryEntries_DoesNotBlockAddingToExistingFingerprint(t *testing.T) {
	s, _ := openStoreT(t)
	// Seed one below the cap, then add a real document to occupy the last
	// slot, bringing the library to exactly MaxLibraryEntries with a real,
	// fingerprint.Compute-derived fingerprint among the seeded ones.
	seedFingerprintEntries(t, s, MaxLibraryEntries-1)
	docA := validDoc("INC-EXISTING-A")
	resA, err := Add(context.Background(), s, docA, AddOptions{})
	if err != nil {
		t.Fatalf("first Add (filling the last slot at the cap): %v", err)
	}

	// Adding another occurrence under that same, already-existing
	// fingerprint must never be blocked by MaxLibraryEntries — only a
	// genuinely new fingerprint directory is capped, and the library is now
	// at exactly the cap.
	docB := validDoc("INC-EXISTING-B")
	resB, err := Add(context.Background(), s, docB, AddOptions{})
	if err != nil {
		t.Fatalf("Add under an existing fingerprint while library is at MaxLibraryEntries: unexpected error: %v", err)
	}
	if resB.Fingerprint != resA.Fingerprint {
		t.Fatalf("expected resB to share resA's fingerprint, got %q vs %q", resB.Fingerprint, resA.Fingerprint)
	}
	if resB.Outcome != AddOutcomeStored {
		t.Errorf("Outcome = %q, want %q", resB.Outcome, AddOutcomeStored)
	}
}

func TestAdd_MaxOccurrencesPerFingerprint_ExactBoundarySucceeds(t *testing.T) {
	s, _ := openStoreT(t)
	var fp string
	for i := 0; i < MaxOccurrencesPerFingerprint-1; i++ {
		res, err := Add(context.Background(), s, validDoc(fmt.Sprintf("INC-OCC-%d", i)), AddOptions{})
		if err != nil {
			t.Fatalf("seed Add(%d): %v", i, err)
		}
		fp = res.Fingerprint
	}

	res, err := Add(context.Background(), s, validDoc("INC-OCC-BOUNDARY"), AddOptions{})
	if err != nil {
		t.Fatalf("Add at exact MaxOccurrencesPerFingerprint boundary: unexpected error: %v", err)
	}
	if res.Fingerprint != fp {
		t.Fatalf("expected the boundary occurrence to share the seeded fingerprint")
	}
	if res.Outcome != AddOutcomeStored {
		t.Errorf("Outcome = %q, want %q", res.Outcome, AddOutcomeStored)
	}
}

func TestAdd_MaxOccurrencesPerFingerprint_OverBoundaryFails(t *testing.T) {
	s, _ := openStoreT(t)
	for i := 0; i < MaxOccurrencesPerFingerprint; i++ {
		if _, err := Add(context.Background(), s, validDoc(fmt.Sprintf("INC-OCC-%d", i)), AddOptions{}); err != nil {
			t.Fatalf("seed Add(%d): %v", i, err)
		}
	}

	_, err := Add(context.Background(), s, validDoc("INC-OCC-OVER"), AddOptions{})
	if err == nil {
		t.Fatal("Add over MaxOccurrencesPerFingerprint: expected error, got nil")
	}
	if !IsKind(err, ErrKindTooManyOccurrences) {
		t.Errorf("expected ErrKindTooManyOccurrences, got %v", err)
	}
	if !strings.Contains(err.Error(), "MaxOccurrencesPerFingerprint") {
		t.Errorf("error message %q should name MaxOccurrencesPerFingerprint", err.Error())
	}
}

func TestAdd_MaxOccurrencesPerFingerprint_DoesNotBlockIdempotentReAdd(t *testing.T) {
	s, _ := openStoreT(t)
	var docs []*idir.Document
	for i := 0; i < MaxOccurrencesPerFingerprint; i++ {
		doc := validDoc(fmt.Sprintf("INC-OCC-%d", i))
		if _, err := Add(context.Background(), s, doc, AddOptions{}); err != nil {
			t.Fatalf("seed Add(%d): %v", i, err)
		}
		docs = append(docs, doc)
	}

	// The fingerprint group is now exactly at MaxOccurrencesPerFingerprint.
	// Re-adding an already-stored document (same incident.id, identical
	// content) must still succeed as an idempotent no-op — the cap only
	// refuses a genuinely new occurrence.
	res, err := Add(context.Background(), s, docs[0], AddOptions{})
	if err != nil {
		t.Fatalf("idempotent re-add at the occurrence cap: unexpected error: %v", err)
	}
	if res.Outcome != AddOutcomeIdempotent {
		t.Errorf("Outcome = %q, want %q", res.Outcome, AddOutcomeIdempotent)
	}
}

func TestList_MaxListResults_ExactBoundarySucceeds(t *testing.T) {
	s, _ := openStoreT(t)
	seedFingerprintEntries(t, s, MaxListResults)

	res, err := List(context.Background(), s)
	if err != nil {
		t.Fatalf("List at exact MaxListResults boundary: unexpected error: %v", err)
	}
	if len(res.Fingerprints) != MaxListResults {
		t.Errorf("len(Fingerprints) = %d, want %d", len(res.Fingerprints), MaxListResults)
	}
}

func TestList_MaxListResults_OverBoundaryFails(t *testing.T) {
	s, _ := openStoreT(t)
	seedFingerprintEntries(t, s, MaxListResults+1)

	_, err := List(context.Background(), s)
	if err == nil {
		t.Fatal("List over MaxListResults: expected error, got nil")
	}
	if !IsKind(err, ErrKindTooManyListResults) {
		t.Errorf("expected ErrKindTooManyListResults, got %v", err)
	}
	if !strings.Contains(err.Error(), "MaxListResults") {
		t.Errorf("error message %q should name MaxListResults", err.Error())
	}
}

func TestLimitErrors_AreDistinctKinds(t *testing.T) {
	kinds := []ErrorKind{
		ErrKindDocumentTooLarge,
		ErrKindTooManyLibraryEntries,
		ErrKindTooManyOccurrences,
		ErrKindTooManyListResults,
	}
	seen := make(map[ErrorKind]bool, len(kinds))
	for _, k := range kinds {
		if seen[k] {
			t.Errorf("duplicate ErrorKind value: %q", k)
		}
		seen[k] = true
	}
}

// TestAdd_NoPartialWritesAfterAnyLimitFailure exercises the
// MaxLibraryEntries failure path and confirms it leaves no temporary file
// behind, per docs/phase-3-plan.md §12/§14 ("Failure paths must leave no
// temporary files or partial index/object state").
func TestAdd_NoPartialWritesAfterAnyLimitFailure(t *testing.T) {
	s, parent := openStoreT(t)

	seedFingerprintEntries(t, s, MaxLibraryEntries)
	if _, err := Add(context.Background(), s, validDoc("INC-OVER-ENTRY-2"), AddOptions{}); !IsKind(err, ErrKindTooManyLibraryEntries) {
		t.Fatalf("expected ErrKindTooManyLibraryEntries, got %v", err)
	}

	var leftoverTemp []string
	_ = filepath.WalkDir(parent, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && strings.Contains(d.Name(), ".tmp") {
			leftoverTemp = append(leftoverTemp, path)
		}
		return nil
	})
	if len(leftoverTemp) != 0 {
		t.Errorf("expected no temporary files after limit failures, found %v", leftoverTemp)
	}
}
