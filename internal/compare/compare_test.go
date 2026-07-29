package compare

import (
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

func doc() *idir.Document {
	return &idir.Document{
		SchemaVersion: "0.1",
		Incident:      idir.Incident{ID: "INC-1", Title: "t"},
		Trigger:       idir.Trigger{Type: "message-redelivery", Description: "d"},
		Events: []idir.Event{
			{ID: "e1", Type: "consumed"},
			{ID: "e2", Type: "charged", CausedBy: []string{"e1"}},
		},
		Services: []idir.Service{
			{Name: "svc", TechnologyCategory: "message-queue-consumer"},
		},
		SideEffects: []idir.SideEffect{
			{Type: "duplicate_charge"},
		},
		BusinessInvariants: []idir.Invariant{
			{ID: "i1", Statement: "at most one charge"},
		},
		ExpectedCorrectedBehavior: []string{"idempotent charging"},
		Privacy:                   idir.Privacy{Sensitivity: "internal"},
	}
}

func TestDocuments_IdenticalWhenSameIdentity(t *testing.T) {
	a := doc()
	b := doc()
	b.Incident.ID = "INC-DIFFERENT-BUT-SAME-PATTERN"

	res, err := Documents(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Identical {
		t.Fatalf("expected identical fingerprints, got a=%s b=%s diffs=%v", res.FingerprintA, res.FingerprintB, res.Differences)
	}
	if len(res.Differences) != 0 {
		t.Fatalf("expected no differences reported for identical fingerprints, got %v", res.Differences)
	}
}

func TestDocuments_ReportsInvariantDifference(t *testing.T) {
	a := doc()
	b := doc()
	b.BusinessInvariants[0].Statement = "a totally different invariant"

	res, err := Documents(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Identical {
		t.Fatal("expected fingerprints to differ")
	}
	found := false
	for _, d := range res.Differences {
		if d.Dimension == "business invariants" {
			found = true
			if !strings.Contains(d.A, "at most one charge") || !strings.Contains(d.B, "a totally different invariant") {
				t.Fatalf("unexpected diff content: %+v", d)
			}
		}
	}
	if !found {
		t.Fatalf("expected a business invariants difference, got %v", res.Differences)
	}
}

func TestDocuments_ReportsCausalStructureDifference(t *testing.T) {
	a := doc()
	b := doc()
	b.Events[1].CausedBy = nil

	res, err := Documents(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if res.Identical {
		t.Fatal("expected fingerprints to differ")
	}
	found := false
	for _, d := range res.Differences {
		if d.Dimension == "causal event structure" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a causal event structure difference, got %v", res.Differences)
	}
}
