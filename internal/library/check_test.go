package library

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint"
)

func TestCheck_NoMatchOnEmptyLibrary(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")

	res, err := Check(context.Background(), s, doc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Outcome != CheckOutcomeNoMatch {
		t.Errorf("Outcome = %q, want %q", res.Outcome, CheckOutcomeNoMatch)
	}
	if res.MatchCount != 0 {
		t.Errorf("MatchCount = %d, want 0", res.MatchCount)
	}
	if res.Fingerprint == "" {
		t.Error("expected Fingerprint to be populated even on no-match")
	}
}

func TestCheck_FindsOneMatchingOccurrence(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")

	addRes, err := Add(context.Background(), s, doc, AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	res, err := Check(context.Background(), s, doc)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Outcome != CheckOutcomeMatch {
		t.Errorf("Outcome = %q, want %q", res.Outcome, CheckOutcomeMatch)
	}
	if res.MatchCount != 1 {
		t.Errorf("MatchCount = %d, want 1", res.MatchCount)
	}
	if res.Fingerprint != addRes.Fingerprint {
		t.Errorf("Fingerprint = %q, want %q", res.Fingerprint, addRes.Fingerprint)
	}
}

func TestCheck_FindsMultipleSameFingerprintOccurrences(t *testing.T) {
	s, _ := openStoreT(t)
	docA := validDoc("INC-A")
	docB := validDoc("INC-B") // identical fingerprint-relevant fields, different incident.id

	if _, err := Add(context.Background(), s, docA, AddOptions{}); err != nil {
		t.Fatalf("Add(A): %v", err)
	}
	if _, err := Add(context.Background(), s, docB, AddOptions{}); err != nil {
		t.Fatalf("Add(B): %v", err)
	}

	// A third, brand-new incident id, never added, but sharing the same
	// fingerprint-relevant fields as docA/docB.
	candidate := validDoc("INC-CANDIDATE")

	res, err := Check(context.Background(), s, candidate)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Outcome != CheckOutcomeMatch {
		t.Errorf("Outcome = %q, want %q", res.Outcome, CheckOutcomeMatch)
	}
	if res.MatchCount != 2 {
		t.Errorf("MatchCount = %d, want 2", res.MatchCount)
	}
}

func TestCheck_DoesNotRequireMatchingIncidentID(t *testing.T) {
	s, _ := openStoreT(t)
	stored := validDoc("INC-STORED")
	if _, err := Add(context.Background(), s, stored, AddOptions{}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	candidate := validDoc("INC-NEVER-SEEN-BEFORE")
	res, err := Check(context.Background(), s, candidate)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Outcome != CheckOutcomeMatch {
		t.Errorf("Outcome = %q, want %q (matching must not require equal incident.id)", res.Outcome, CheckOutcomeMatch)
	}
}

func TestCheck_ReportsNoMatchForUnrelatedFingerprint(t *testing.T) {
	s, _ := openStoreT(t)
	stored := validDoc("INC-STORED")
	if _, err := Add(context.Background(), s, stored, AddOptions{}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Changing Trigger.Type/Description and the invariant statement changes
	// the fingerprint (internal/fingerprint's identity payload), unlike
	// changing only incident.id.
	unrelated := validDoc("INC-UNRELATED")
	unrelated.Trigger.Type = "a-completely-different-trigger-type"
	unrelated.Trigger.Description = "A different trigger, changing the fingerprint."
	unrelated.BusinessInvariants[0].Statement = "A completely different invariant statement."

	res, err := Check(context.Background(), s, unrelated)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.Outcome != CheckOutcomeNoMatch {
		t.Errorf("Outcome = %q, want %q", res.Outcome, CheckOutcomeNoMatch)
	}
}

func TestCheck_RejectsDocumentFailingValidation(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("") // empty incident.id fails validate.Validate

	_, err := Check(context.Background(), s, doc)
	if err == nil {
		t.Fatal("Check: expected validation error, got nil")
	}
	if !errors.Is(err, ErrInvalidDocument) {
		t.Errorf("expected ErrInvalidDocument, got %v", err)
	}
}

func TestCheck_MalformedIndexIsDetected(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	res, err := Add(context.Background(), s, doc, AddOptions{})
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

	_, err = Check(context.Background(), s, doc)
	if err == nil {
		t.Fatal("Check against a malformed index: expected error, got nil")
	}
	if !errors.Is(err, ErrMalformedIndex) {
		t.Errorf("expected ErrMalformedIndex, got %v", err)
	}
}

func TestCheck_MissingOccurrenceReferencedByIndexIsCorrupted(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	res, err := Add(context.Background(), s, doc, AddOptions{})
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

	_, err = Check(context.Background(), s, doc)
	if err == nil {
		t.Fatal("Check with a missing referenced occurrence: expected error, got nil")
	}
	if !errors.Is(err, ErrCorruptedOccurrence) {
		t.Errorf("expected ErrCorruptedOccurrence, got %v", err)
	}
}

func TestCheck_CorruptedOccurrenceContentIsDetected(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	res, err := Add(context.Background(), s, doc, AddOptions{})
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

	_, err = Check(context.Background(), s, doc)
	if err == nil {
		t.Fatal("Check against a corrupted occurrence: expected error, got nil")
	}
	if !errors.Is(err, ErrCorruptedOccurrence) {
		t.Errorf("expected ErrCorruptedOccurrence, got %v", err)
	}
}

func TestCheck_NoRawDocumentContentInErrorMessages(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	doc.BusinessInvariants[0].Statement = "very-distinctive-invariant-statement-marker"
	doc.Trigger.Description = "very-distinctive-trigger-description-marker"

	res, err := Add(context.Background(), s, doc, AddOptions{})
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

	_, err = Check(context.Background(), s, doc)
	if err == nil {
		t.Fatal("Check against a corrupted occurrence: expected error, got nil")
	}
	msg := err.Error()
	if strings.Contains(msg, "very-distinctive-invariant-statement-marker") || strings.Contains(msg, "very-distinctive-trigger-description-marker") {
		t.Errorf("error message must never contain raw document content, got: %s", msg)
	}
}

// --- Phase 6: CheckFingerprint (docs/phase-6-plan.md §5/§15 slice 1) ---

func TestCheckFingerprint_RejectsMalformedFingerprint(t *testing.T) {
	s, _ := openStoreT(t)

	cases := map[string]string{
		"empty":          "",
		"wrong prefix":   "md5:" + strings.Repeat("a", 64),
		"missing prefix": strings.Repeat("a", 64),
		"too short":      "sha256:" + strings.Repeat("a", 63),
		"too long":       "sha256:" + strings.Repeat("a", 65),
		"uppercase hex":  "sha256:" + strings.Repeat("A", 64),
		"non-hex":        "sha256:" + strings.Repeat("z", 64),
	}
	for name, fp := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := CheckFingerprint(context.Background(), s, fp)
			if err == nil {
				t.Fatalf("CheckFingerprint(%q): expected error, got nil", fp)
			}
			if !errors.Is(err, ErrInvalidFingerprint) {
				t.Errorf("CheckFingerprint(%q): expected ErrInvalidFingerprint, got %v", fp, err)
			}
		})
	}
}

func TestCheckFingerprint_MatchAgainstSeededLibrary(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	addRes, err := Add(context.Background(), s, doc, AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	res, err := CheckFingerprint(context.Background(), s, addRes.Fingerprint)
	if err != nil {
		t.Fatalf("CheckFingerprint: %v", err)
	}
	if res.Outcome != CheckOutcomeMatch {
		t.Errorf("Outcome = %q, want %q", res.Outcome, CheckOutcomeMatch)
	}
	if res.MatchCount != 1 {
		t.Errorf("MatchCount = %d, want 1", res.MatchCount)
	}
	if res.Fingerprint != addRes.Fingerprint {
		t.Errorf("Fingerprint = %q, want %q", res.Fingerprint, addRes.Fingerprint)
	}
}

func TestCheckFingerprint_MatchWithMultipleOccurrences(t *testing.T) {
	s, _ := openStoreT(t)
	docA := validDoc("INC-A")
	docB := validDoc("INC-B")

	addRes, err := Add(context.Background(), s, docA, AddOptions{})
	if err != nil {
		t.Fatalf("Add(A): %v", err)
	}
	if _, err := Add(context.Background(), s, docB, AddOptions{}); err != nil {
		t.Fatalf("Add(B): %v", err)
	}

	res, err := CheckFingerprint(context.Background(), s, addRes.Fingerprint)
	if err != nil {
		t.Fatalf("CheckFingerprint: %v", err)
	}
	if res.Outcome != CheckOutcomeMatch {
		t.Errorf("Outcome = %q, want %q", res.Outcome, CheckOutcomeMatch)
	}
	if res.MatchCount != 2 {
		t.Errorf("MatchCount = %d, want 2", res.MatchCount)
	}
}

func TestCheckFingerprint_NoMatchOnEmptyButExistingLibrary(t *testing.T) {
	s, _ := openStoreT(t)
	// Seed the library so its root exists, but under an unrelated
	// fingerprint.
	stored := validDoc("INC-STORED")
	if _, err := Add(context.Background(), s, stored, AddOptions{}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	unrelatedDoc := validDoc("INC-UNRELATED")
	unrelatedDoc.Trigger.Type = "a-completely-different-trigger-type"
	unrelatedDoc.Trigger.Description = "A different trigger, changing the fingerprint."
	unrelatedDoc.BusinessInvariants[0].Statement = "A completely different invariant statement."
	fp, err := fingerprint.Compute(unrelatedDoc)
	if err != nil {
		t.Fatal(err)
	}

	res, err := CheckFingerprint(context.Background(), s, fp)
	if err != nil {
		t.Fatalf("CheckFingerprint: %v", err)
	}
	if res.Outcome != CheckOutcomeNoMatch {
		t.Errorf("Outcome = %q, want %q", res.Outcome, CheckOutcomeNoMatch)
	}
	if res.MatchCount != 0 {
		t.Errorf("MatchCount = %d, want 0", res.MatchCount)
	}
}

func TestCheckFingerprint_NoMatchOnNotYetCreatedLibrary(t *testing.T) {
	s, _ := openStoreT(t) // never Add()ed to; library root does not exist on disk
	fp, err := fingerprint.Compute(validDoc("INC-NEVER-STORED"))
	if err != nil {
		t.Fatal(err)
	}

	res, err := CheckFingerprint(context.Background(), s, fp)
	if err != nil {
		t.Fatalf("CheckFingerprint against a not-yet-created library: %v", err)
	}
	if res.Outcome != CheckOutcomeNoMatch {
		t.Errorf("Outcome = %q, want %q", res.Outcome, CheckOutcomeNoMatch)
	}
}

func TestCheckFingerprint_MissingOccurrenceIsCorrupted(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	res, err := Add(context.Background(), s, doc, AddOptions{})
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

	_, err = CheckFingerprint(context.Background(), s, res.Fingerprint)
	if err == nil {
		t.Fatal("CheckFingerprint with a missing referenced occurrence: expected error, got nil")
	}
	if !errors.Is(err, ErrCorruptedOccurrence) {
		t.Errorf("expected ErrCorruptedOccurrence, got %v", err)
	}
}

func TestCheckFingerprint_MalformedIndexIsDetected(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	res, err := Add(context.Background(), s, doc, AddOptions{})
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

	_, err = CheckFingerprint(context.Background(), s, res.Fingerprint)
	if err == nil {
		t.Fatal("CheckFingerprint against a malformed index: expected error, got nil")
	}
	if !errors.Is(err, ErrMalformedIndex) {
		t.Errorf("expected ErrMalformedIndex, got %v", err)
	}
}

// TestCheckFingerprint_ParityWithCheck asserts Check(doc) and
// CheckFingerprint(fingerprint.Compute(doc)) return identical CheckResult
// values for the same document and library state (docs/phase-6-plan.md §15
// slice 1), for both a match and a no-match library state.
func TestCheckFingerprint_ParityWithCheck(t *testing.T) {
	t.Run("match", func(t *testing.T) {
		s, _ := openStoreT(t)
		doc := validDoc("INC-1")
		if _, err := Add(context.Background(), s, doc, AddOptions{}); err != nil {
			t.Fatalf("Add: %v", err)
		}

		fp, err := fingerprint.Compute(doc)
		if err != nil {
			t.Fatal(err)
		}

		checkRes, err := Check(context.Background(), s, doc)
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		fpRes, err := CheckFingerprint(context.Background(), s, fp)
		if err != nil {
			t.Fatalf("CheckFingerprint: %v", err)
		}
		if checkRes != fpRes {
			t.Errorf("Check() = %+v, CheckFingerprint() = %+v; want identical results", checkRes, fpRes)
		}
	})

	t.Run("no_match", func(t *testing.T) {
		s, _ := openStoreT(t)
		doc := validDoc("INC-NEVER-STORED")

		fp, err := fingerprint.Compute(doc)
		if err != nil {
			t.Fatal(err)
		}

		checkRes, err := Check(context.Background(), s, doc)
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		fpRes, err := CheckFingerprint(context.Background(), s, fp)
		if err != nil {
			t.Fatalf("CheckFingerprint: %v", err)
		}
		if checkRes != fpRes {
			t.Errorf("Check() = %+v, CheckFingerprint() = %+v; want identical results", checkRes, fpRes)
		}
	})
}

func TestCheckFingerprint_ContextCanceledBeforeAnyIO(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	res, err := Add(context.Background(), s, doc, AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = CheckFingerprint(ctx, s, res.Fingerprint)
	if err == nil {
		t.Fatal("CheckFingerprint with a canceled context: expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected wrapped context.Canceled, got %v", err)
	}
}
