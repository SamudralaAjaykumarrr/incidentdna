package fingerprint

import (
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

func baseDoc() *idir.Document {
	return &idir.Document{
		SchemaVersion: "0.1",
		Incident: idir.Incident{
			ID:      "INC-A",
			Title:   "Title A",
			Summary: "Summary A",
		},
		Application: idir.Application{
			Name:           "app-a",
			ReleaseVersion: "1.0.0",
		},
		Trigger: idir.Trigger{
			Type:        "message-redelivery",
			Description: "A message was redelivered.",
		},
		Events: []idir.Event{
			{ID: "evt-1", Type: "event-consumed", Description: "d1"},
			{ID: "evt-2", Type: "charge-committed", Description: "d2", CausedBy: []string{"evt-1"}},
		},
		Services: []idir.Service{
			{Name: "svc", TechnologyCategory: "message-queue-consumer"},
		},
		SideEffects: []idir.SideEffect{
			{Type: "duplicate_charge"},
		},
		BusinessInvariants: []idir.Invariant{
			{ID: "inv-1", Statement: "At most one completed charge per order."},
		},
		ExpectedCorrectedBehavior: []string{"Idempotent charge processing."},
		Evidence: []idir.Evidence{
			{ID: "ev-1", Location: "s3://somewhere/else", Digest: "sha256:" + strings.Repeat("a", 64)},
		},
		Privacy: idir.Privacy{Sensitivity: "internal"},
	}
}

func TestCompute_Deterministic(t *testing.T) {
	doc := baseDoc()
	fp1, err := Compute(doc)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	fp2, err := Compute(doc)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if fp1 != fp2 {
		t.Fatalf("Compute is not deterministic: %s != %s", fp1, fp2)
	}
	if !strings.HasPrefix(fp1, "sha256:") {
		t.Fatalf("fingerprint missing sha256: prefix: %s", fp1)
	}
	if len(fp1) != len("sha256:")+64 {
		t.Fatalf("unexpected fingerprint length: %s", fp1)
	}
}

func TestCompute_IgnoresIdentityMetadata(t *testing.T) {
	a := baseDoc()
	b := baseDoc()
	b.Incident.ID = "INC-DIFFERENT"
	b.Incident.Title = "A completely different title"
	b.Incident.OccurredAt = "2030-05-05T00:00:00Z"
	b.Application.Name = "totally-different-app"
	b.Application.ReleaseVersion = "9.9.9"
	b.Evidence[0].Location = "gs://a-different-bucket/object"

	fpA, err := Compute(a)
	if err != nil {
		t.Fatal(err)
	}
	fpB, err := Compute(b)
	if err != nil {
		t.Fatal(err)
	}
	if fpA != fpB {
		t.Fatalf("fingerprint changed despite only identity metadata differing: %s != %s", fpA, fpB)
	}
}

func TestCompute_InvariantOrderIndependent(t *testing.T) {
	a := baseDoc()
	a.BusinessInvariants = []idir.Invariant{
		{ID: "1", Statement: "invariant one"},
		{ID: "2", Statement: "invariant two"},
	}
	b := baseDoc()
	b.BusinessInvariants = []idir.Invariant{
		{ID: "2", Statement: "invariant two"},
		{ID: "1", Statement: "invariant one"},
	}

	fpA, _ := Compute(a)
	fpB, _ := Compute(b)
	if fpA != fpB {
		t.Fatalf("reordering business invariants changed the fingerprint: %s != %s", fpA, fpB)
	}
}

func TestCompute_CausalStructureChangeChangesFingerprint(t *testing.T) {
	a := baseDoc()
	b := baseDoc()
	// In b, evt-2 is no longer caused by evt-1: a materially different
	// causal structure even though the set of event types is unchanged.
	b.Events[1].CausedBy = nil

	fpA, err := Compute(a)
	if err != nil {
		t.Fatal(err)
	}
	fpB, err := Compute(b)
	if err != nil {
		t.Fatal(err)
	}
	if fpA == fpB {
		t.Fatalf("expected a causal structure change to change the fingerprint, both were %s", fpA)
	}
}

func TestCompute_InvariantContentChangeChangesFingerprint(t *testing.T) {
	a := baseDoc()
	b := baseDoc()
	b.BusinessInvariants[0].Statement = "A different business invariant entirely."

	fpA, _ := Compute(a)
	fpB, _ := Compute(b)
	if fpA == fpB {
		t.Fatal("expected a changed business invariant to change the fingerprint")
	}
}

func TestCompute_SideEffectTypeChangeChangesFingerprint(t *testing.T) {
	a := baseDoc()
	b := baseDoc()
	b.SideEffects[0].Type = "lost_order"

	fpA, _ := Compute(a)
	fpB, _ := Compute(b)
	if fpA == fpB {
		t.Fatal("expected a changed side effect type to change the fingerprint")
	}
}

func TestCompute_TechnologyCategoryChangeChangesFingerprint(t *testing.T) {
	a := baseDoc()
	b := baseDoc()
	b.Services[0].TechnologyCategory = "relational-database"

	fpA, _ := Compute(a)
	fpB, _ := Compute(b)
	if fpA == fpB {
		t.Fatal("expected a changed technology category to change the fingerprint")
	}
}

func TestCompute_RecoveryBehaviorChangeChangesFingerprint(t *testing.T) {
	a := baseDoc()
	b := baseDoc()
	b.ExpectedCorrectedBehavior = []string{"A completely different remediation."}

	fpA, _ := Compute(a)
	fpB, _ := Compute(b)
	if fpA == fpB {
		t.Fatal("expected a changed expected-corrected-behavior to change the fingerprint")
	}
}
