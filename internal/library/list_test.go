package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestList_EmptyLibraryIsEmptyNotError(t *testing.T) {
	s, _ := openStoreT(t)

	res, err := List(context.Background(), s)
	if err != nil {
		t.Fatalf("List on an empty/not-yet-created library: unexpected error: %v", err)
	}
	if len(res.Fingerprints) != 0 {
		t.Errorf("expected no fingerprint groups, got %d", len(res.Fingerprints))
	}
}

func TestList_DeterministicOrderingAndApprovedFields(t *testing.T) {
	s, _ := openStoreT(t)

	classA1 := validDoc("INC-A1")
	classA2 := validDoc("INC-A2") // same fingerprint as classA1, different incident.id

	classB := validDoc("INC-B1")
	classB.Trigger.Type = "a-completely-different-trigger-type"
	classB.Trigger.Description = "A different trigger, changing the fingerprint."
	classB.BusinessInvariants[0].Statement = "A completely different invariant statement."
	classB.Application.Service = "other-service"
	classB.Incident.Title = "A different incident title"
	classB.Incident.OccurredAt = "2026-02-02T00:00:00Z"

	resA1, err := Add(context.Background(), s, classA1)
	if err != nil {
		t.Fatalf("Add(A1): %v", err)
	}
	resA2, err := Add(context.Background(), s, classA2)
	if err != nil {
		t.Fatalf("Add(A2): %v", err)
	}
	resB, err := Add(context.Background(), s, classB)
	if err != nil {
		t.Fatalf("Add(B): %v", err)
	}
	if resA1.Fingerprint == resB.Fingerprint {
		t.Fatal("test setup error: expected classA and classB to have different fingerprints")
	}

	result, err := List(context.Background(), s)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(result.Fingerprints) != 2 {
		t.Fatalf("expected 2 fingerprint groups, got %d: %+v", len(result.Fingerprints), result.Fingerprints)
	}

	// Deterministic ordering: fingerprint hex string ascending.
	if !sort.SliceIsSorted(result.Fingerprints, func(i, j int) bool {
		return result.Fingerprints[i].Fingerprint < result.Fingerprints[j].Fingerprint
	}) {
		t.Errorf("fingerprint groups are not sorted ascending by fingerprint: %+v", result.Fingerprints)
	}

	var groupA, groupB *FingerprintSummary
	for i := range result.Fingerprints {
		switch result.Fingerprints[i].Fingerprint {
		case resA1.Fingerprint:
			groupA = &result.Fingerprints[i]
		case resB.Fingerprint:
			groupB = &result.Fingerprints[i]
		}
	}
	if groupA == nil || groupB == nil {
		t.Fatalf("expected both fingerprint groups present, got %+v", result.Fingerprints)
	}

	if groupA.OccurrenceCount != 2 {
		t.Errorf("groupA.OccurrenceCount = %d, want 2", groupA.OccurrenceCount)
	}
	if len(groupA.Occurrences) != 2 {
		t.Fatalf("groupA.Occurrences len = %d, want 2", len(groupA.Occurrences))
	}
	// Occurrences within a group are ordered by occurrence digest ascending.
	wantDigestsAsc := []string{resA1.Digest, resA2.Digest}
	sort.Strings(wantDigestsAsc)
	gotIDs := []string{groupA.Occurrences[0].IncidentID, groupA.Occurrences[1].IncidentID}
	wantIDsByDigest := map[string]string{resA1.Digest: "INC-A1", resA2.Digest: "INC-A2"}
	wantIDs := []string{wantIDsByDigest[wantDigestsAsc[0]], wantIDsByDigest[wantDigestsAsc[1]]}
	if gotIDs[0] != wantIDs[0] || gotIDs[1] != wantIDs[1] {
		t.Errorf("groupA occurrence order/content = %+v, want incident ids in digest order %v", groupA.Occurrences, wantIDs)
	}

	if groupB.OccurrenceCount != 1 {
		t.Errorf("groupB.OccurrenceCount = %d, want 1", groupB.OccurrenceCount)
	}
	if len(groupB.Occurrences) != 1 {
		t.Fatalf("groupB.Occurrences len = %d, want 1", len(groupB.Occurrences))
	}
	occB := groupB.Occurrences[0]
	if occB.IncidentID != "INC-B1" {
		t.Errorf("occB.IncidentID = %q, want %q", occB.IncidentID, "INC-B1")
	}
	if occB.Title != "A different incident title" {
		t.Errorf("occB.Title = %q, want the approved title field", occB.Title)
	}
	if occB.Service != "other-service" {
		t.Errorf("occB.Service = %q, want %q", occB.Service, "other-service")
	}
	if occB.OccurredAt != "2026-02-02T00:00:00Z" {
		t.Errorf("occB.OccurredAt = %q, want %q", occB.OccurredAt, "2026-02-02T00:00:00Z")
	}
}

func TestList_MalformedIndexIsDetected(t *testing.T) {
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

	_, err = List(context.Background(), s)
	if err == nil {
		t.Fatal("List against a malformed index: expected error, got nil")
	}
	if !errors.Is(err, ErrMalformedIndex) {
		t.Errorf("expected ErrMalformedIndex, got %v", err)
	}
}

func TestList_MissingIndexedOccurrenceIsDetected(t *testing.T) {
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

	_, err = List(context.Background(), s)
	if err == nil {
		t.Fatal("List with a missing referenced occurrence: expected error, got nil")
	}
	if !errors.Is(err, ErrCorruptedOccurrence) {
		t.Errorf("expected ErrCorruptedOccurrence, got %v", err)
	}
}

func TestList_CorruptedOccurrenceContentIsDetected(t *testing.T) {
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
	if err := os.Chmod(occPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(occPath, []byte(`{"tampered":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = List(context.Background(), s)
	if err == nil {
		t.Fatal("List against a corrupted occurrence: expected error, got nil")
	}
	if !errors.Is(err, ErrCorruptedOccurrence) {
		t.Errorf("expected ErrCorruptedOccurrence, got %v", err)
	}
}

func TestList_RejectsSymlinkedFingerprintDirectory(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	if _, err := Add(context.Background(), s, doc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Introduce a symlinked "fingerprint directory" under an existing shard,
	// pointing outside the library root entirely.
	shardEntries, err := os.ReadDir(s.Root())
	if err != nil {
		t.Fatal(err)
	}
	if len(shardEntries) == 0 {
		t.Fatal("expected at least one shard directory after Add")
	}
	shardDir := filepath.Join(s.Root(), shardEntries[0].Name())

	outsideTarget := t.TempDir()
	linkPath := filepath.Join(shardDir, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbe")
	if err := os.Symlink(outsideTarget, linkPath); err != nil {
		t.Skipf("symlinks not supported on this platform/filesystem: %v", err)
	}

	_, err = List(context.Background(), s)
	if err == nil {
		t.Fatal("List with a symlinked fingerprint directory: expected error, got nil")
	}
	if !errors.Is(err, ErrMalformedLibrary) {
		t.Errorf("expected ErrMalformedLibrary, got %v", err)
	}
}

func TestList_RejectsNonHexShardOrFingerprintDirNames(t *testing.T) {
	tests := []struct {
		name        string
		shardName   string
		fpName      string
		makeAsShard bool // true: place the bad name at the shard level; false: at the fingerprint level under a good shard
	}{
		{name: "shard wrong length", shardName: "abc", makeAsShard: true},
		{name: "shard non-hex chars", shardName: "zz", makeAsShard: true},
		{name: "fingerprint dir wrong length", shardName: "aa", fpName: "tooshort"},
		{name: "fingerprint dir non-hex chars", shardName: "aa", fpName: strings.Repeat("z", 62)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := openStoreT(t)
			if err := os.MkdirAll(s.Root(), 0o755); err != nil {
				t.Fatal(err)
			}

			if tc.makeAsShard {
				if err := os.MkdirAll(filepath.Join(s.Root(), tc.shardName), 0o755); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.MkdirAll(filepath.Join(s.Root(), tc.shardName, tc.fpName), 0o755); err != nil {
					t.Fatal(err)
				}
			}

			_, err := List(context.Background(), s)
			if err == nil {
				t.Fatal("List against a malformed directory name: expected error, got nil")
			}
			if !errors.Is(err, ErrMalformedLibrary) {
				t.Errorf("expected ErrMalformedLibrary, got %v", err)
			}
		})
	}
}

func TestList_NoRawDocumentContentExposed(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	doc.BusinessInvariants[0].Statement = "very-distinctive-invariant-statement-marker"
	doc.Trigger.Description = "very-distinctive-trigger-description-marker"
	if _, err := Add(context.Background(), s, doc); err != nil {
		t.Fatalf("Add: %v", err)
	}

	res, err := List(context.Background(), s)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, group := range res.Fingerprints {
		for _, occ := range group.Occurrences {
			if strings.Contains(occ.Title, "very-distinctive") || strings.Contains(occ.Service, "very-distinctive") {
				t.Errorf("List exposed unexpected content in approved fields: %+v", occ)
			}
		}
	}
}
