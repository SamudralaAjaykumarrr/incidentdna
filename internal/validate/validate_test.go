package validate

import (
	"context"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

func TestValidate_ValidDocumentHasNoIssues(t *testing.T) {
	res := Validate(context.Background(), validDoc())
	if !res.Valid() {
		t.Fatalf("expected valid document to have no issues, got: %v", res.Issues)
	}
}

func TestValidate_RejectsEachRequiredCase(t *testing.T) {
	tests := []struct {
		name          string
		mutate        func(*idir.Document)
		wantField     string
		wantSubstring string
	}{
		{
			name:          "unsupported schema version",
			mutate:        func(d *idir.Document) { d.SchemaVersion = "9.9" },
			wantField:     "schema_version",
			wantSubstring: "unsupported schema version",
		},
		{
			name:          "missing incident identifier",
			mutate:        func(d *idir.Document) { d.Incident.ID = "" },
			wantField:     "incident.id",
			wantSubstring: "missing incident identifier",
		},
		{
			name:          "missing business invariants",
			mutate:        func(d *idir.Document) { d.BusinessInvariants = nil },
			wantField:     "business_invariants",
			wantSubstring: "at least one violated business invariant",
		},
		{
			name: "empty business invariant statement",
			mutate: func(d *idir.Document) {
				d.BusinessInvariants = []idir.Invariant{{ID: "inv-1", Statement: ""}}
			},
			wantField:     "business_invariants[0].statement",
			wantSubstring: "must not be empty",
		},
		{
			name: "causal reference to nonexistent event",
			mutate: func(d *idir.Document) {
				d.Events[1].CausedBy = []string{"evt-does-not-exist"}
			},
			wantField:     "events[1].caused_by",
			wantSubstring: "nonexistent causal event",
		},
		{
			name: "duplicate event identifiers",
			mutate: func(d *idir.Document) {
				d.Events = append(d.Events, idir.Event{ID: "evt-1", Type: "duplicate"})
			},
			wantField:     "events[2].id",
			wantSubstring: "duplicate event id",
		},
		{
			name: "cyclic causal relationship",
			mutate: func(d *idir.Document) {
				d.Events[0].CausedBy = []string{"evt-2"} // evt-1 <-> evt-2 cycle
			},
			wantField:     "events",
			wantSubstring: "cyclic causal relationship",
		},
		{
			name: "evidence digest without algorithm prefix",
			mutate: func(d *idir.Document) {
				d.Evidence[0].Digest = repeatHex(64) // missing "sha256:" prefix
			},
			wantField:     "evidence[0].digest",
			wantSubstring: "sha256-prefixed",
		},
		{
			name: "evidence digest with wrong length",
			mutate: func(d *idir.Document) {
				d.Evidence[0].Digest = "sha256:abc123"
			},
			wantField:     "evidence[0].digest",
			wantSubstring: "sha256-prefixed",
		},
		{
			name:          "unknown sensitivity classification",
			mutate:        func(d *idir.Document) { d.Privacy.Sensitivity = "top-secret" },
			wantField:     "privacy.sensitivity",
			wantSubstring: "unknown sensitivity classification",
		},
		{
			name: "reproduction sequence references nonexistent event",
			mutate: func(d *idir.Document) {
				d.ReproductionSequence[0].RefEventID = "evt-does-not-exist"
			},
			wantField:     "reproduction_sequence[0].ref_event_id",
			wantSubstring: "nonexistent event",
		},
		{
			name: "redacted but sensitive field remains",
			mutate: func(d *idir.Document) {
				d.Privacy.Redacted = true
				d.Evidence[0].RawExcerpt = "customer said the charge was wrong"
			},
			wantField:     "evidence[0].raw_excerpt",
			wantSubstring: "not empty",
		},
		{
			name: "redacted but email address remains in description",
			mutate: func(d *idir.Document) {
				d.Privacy.Redacted = true
				d.Events[0].Description = "contact jane.doe@example.com for details"
			},
			wantField:     "events[0].description",
			wantSubstring: "email address",
		},
		{
			name:          "empty expected corrected behavior",
			mutate:        func(d *idir.Document) { d.ExpectedCorrectedBehavior = nil },
			wantField:     "expected_corrected_behavior",
			wantSubstring: "must not be empty",
		},
		{
			name: "expected corrected behavior with only blank entries",
			mutate: func(d *idir.Document) {
				d.ExpectedCorrectedBehavior = []string{""}
			},
			wantField:     "expected_corrected_behavior",
			wantSubstring: "must not be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := validDoc()
			tt.mutate(doc)

			res := Validate(context.Background(), doc)
			if res.Valid() {
				t.Fatalf("expected validation to reject document, but it was valid")
			}

			var found *Issue
			for i := range res.Issues {
				if res.Issues[i].Field == tt.wantField {
					found = &res.Issues[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("expected an issue on field %q, got issues: %v", tt.wantField, res.Issues)
			}
			if !strings.Contains(found.Message, tt.wantSubstring) {
				t.Fatalf("issue message %q does not contain %q", found.Message, tt.wantSubstring)
			}
		})
	}
}

func TestValidate_RedactedDocumentWithNoSensitiveFieldsIsValid(t *testing.T) {
	doc := validDoc()
	doc.Privacy.Redacted = true
	res := Validate(context.Background(), doc)
	if !res.Valid() {
		t.Fatalf("expected redacted-but-clean document to be valid, got: %v", res.Issues)
	}
}

func TestValidate_LongerCycleIsDetected(t *testing.T) {
	doc := validDoc()
	doc.Events = []idir.Event{
		{ID: "a", Type: "t", CausedBy: []string{"c"}},
		{ID: "b", Type: "t", CausedBy: []string{"a"}},
		{ID: "c", Type: "t", CausedBy: []string{"b"}},
	}
	res := Validate(context.Background(), doc)
	if res.Valid() {
		t.Fatal("expected a 3-node cycle to be rejected")
	}
	found := false
	for _, issue := range res.Issues {
		if issue.Field == "events" && strings.Contains(issue.Message, "cyclic") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a cyclic causal relationship issue, got: %v", res.Issues)
	}
}

func TestValidate_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	doc := validDoc()
	res := Validate(ctx, doc)
	if res.Valid() {
		t.Fatal("expected a cancelled context to produce an issue")
	}
}
