package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdd_RedactedDocumentAcceptedByDefault(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1") // validDoc declares Redacted: true

	res, err := Add(context.Background(), s, doc, AddOptions{})
	if err != nil {
		t.Fatalf("Add: unexpected error for a redacted document: %v", err)
	}
	if res.Outcome != AddOutcomeStored {
		t.Errorf("Outcome = %q, want %q", res.Outcome, AddOutcomeStored)
	}
	if res.PrivacyOverridden {
		t.Error("PrivacyOverridden = true for an already-redacted document, want false")
	}
}

func TestAdd_UnredactedDocumentRejectedByDefault(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	doc.Privacy.Redacted = false

	_, err := Add(context.Background(), s, doc, AddOptions{})
	if err == nil {
		t.Fatal("Add: expected privacy-policy error for an unredacted document, got nil")
	}
	if !errors.Is(err, ErrPrivacyNotRedacted) {
		t.Errorf("expected ErrPrivacyNotRedacted, got %v", err)
	}
}

func TestAdd_ExplicitOverrideAcceptsUnredactedDocument(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	doc.Privacy.Redacted = false

	res, err := Add(context.Background(), s, doc, AddOptions{AllowUnredacted: true})
	if err != nil {
		t.Fatalf("Add with AllowUnredacted: unexpected error: %v", err)
	}
	if res.Outcome != AddOutcomeStored {
		t.Errorf("Outcome = %q, want %q", res.Outcome, AddOutcomeStored)
	}
}

func TestAdd_OverrideResultIndicatesFutureCLIMustWarn(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	doc.Privacy.Redacted = false

	res, err := Add(context.Background(), s, doc, AddOptions{AllowUnredacted: true})
	if err != nil {
		t.Fatalf("Add with AllowUnredacted: unexpected error: %v", err)
	}
	if !res.PrivacyOverridden {
		t.Error("PrivacyOverridden = false after an unredacted document was stored via AllowUnredacted, want true")
	}
}

func TestAdd_RedactedDocumentDoesNotProduceOverrideWarning(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1") // Redacted: true

	// Even with AllowUnredacted set, an already-redacted document must never
	// be reported as having triggered the override warning path.
	res, err := Add(context.Background(), s, doc, AddOptions{AllowUnredacted: true})
	if err != nil {
		t.Fatalf("Add: unexpected error: %v", err)
	}
	if res.PrivacyOverridden {
		t.Error("PrivacyOverridden = true for an already-redacted document even though AllowUnredacted was set, want false")
	}
}

func TestAdd_PrivacyRejectionWritesNoOccurrenceObjectOrIndex(t *testing.T) {
	s, parent := openStoreT(t)
	doc := validDoc("INC-1")
	doc.Privacy.Redacted = false

	_, err := Add(context.Background(), s, doc, AddOptions{})
	if err == nil {
		t.Fatal("Add: expected privacy-policy error, got nil")
	}
	if !errors.Is(err, ErrPrivacyNotRedacted) {
		t.Fatalf("expected ErrPrivacyNotRedacted, got %v", err)
	}

	// The library root must not have been created at all: the gate runs
	// before any fingerprint/digest/storage work (docs/phase-3-plan.md §12),
	// so no fingerprint directory, occurrence object, or index.json can
	// exist.
	if _, statErr := os.Stat(s.Root()); statErr == nil {
		entries, readErr := os.ReadDir(s.Root())
		if readErr != nil {
			t.Fatal(readErr)
		}
		if len(entries) != 0 {
			t.Errorf("expected nothing written under the library root after a privacy rejection, found %d entries: %v", len(entries), entries)
		}
	} else if !os.IsNotExist(statErr) {
		t.Fatal(statErr)
	}

	// No temporary files left anywhere under parent either.
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
		t.Errorf("expected no temporary files after a privacy rejection, found %v", leftoverTemp)
	}
}

func TestAdd_PrivacyErrorContainsNoRawIncidentContent(t *testing.T) {
	s, _ := openStoreT(t)
	doc := validDoc("INC-1")
	doc.Privacy.Redacted = false
	doc.Incident.Title = "SENSITIVE-TITLE-MARKER"
	doc.Incident.Summary = "SENSITIVE-SUMMARY-MARKER a customer's private data was exposed"
	doc.Trigger.Description = "SENSITIVE-TRIGGER-MARKER"

	_, err := Add(context.Background(), s, doc, AddOptions{})
	if err == nil {
		t.Fatal("Add: expected privacy-policy error, got nil")
	}

	msg := err.Error()
	for _, marker := range []string{"SENSITIVE-TITLE-MARKER", "SENSITIVE-SUMMARY-MARKER", "SENSITIVE-TRIGGER-MARKER", doc.Incident.ID} {
		if strings.Contains(msg, marker) {
			t.Errorf("privacy error %q must not contain document content %q", msg, marker)
		}
	}
}

func TestCheckPrivacyGate_DirectCases(t *testing.T) {
	tests := []struct {
		name            string
		redacted        bool
		allowUnredacted bool
		wantOverridden  bool
		wantErr         bool
	}{
		{name: "redacted, no override needed", redacted: true, allowUnredacted: false, wantOverridden: false, wantErr: false},
		{name: "redacted, override also set", redacted: true, allowUnredacted: true, wantOverridden: false, wantErr: false},
		{name: "unredacted, no override", redacted: false, allowUnredacted: false, wantOverridden: false, wantErr: true},
		{name: "unredacted, override set", redacted: false, allowUnredacted: true, wantOverridden: true, wantErr: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := validDoc("INC-1")
			doc.Privacy.Redacted = tc.redacted

			overridden, err := checkPrivacyGate(doc, tc.allowUnredacted)
			if tc.wantErr && !errors.Is(err, ErrPrivacyNotRedacted) {
				t.Errorf("expected ErrPrivacyNotRedacted, got %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if overridden != tc.wantOverridden {
				t.Errorf("overridden = %v, want %v", overridden, tc.wantOverridden)
			}
		})
	}
}
