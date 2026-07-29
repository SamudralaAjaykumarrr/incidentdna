package validate

import "github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"

// validDoc returns a minimal, fully valid IDIR document that every test
// mutates a single field of, so each test isolates exactly one rule.
func validDoc() *idir.Document {
	return &idir.Document{
		SchemaVersion: idir.SupportedSchemaVersion,
		Incident: idir.Incident{
			ID:         "INC-TEST-0001",
			Title:      "Test incident",
			Summary:    "A test incident.",
			OccurredAt: "2026-01-01T00:00:00Z",
		},
		Application: idir.Application{
			Name:           "example-app",
			Service:        "example-service",
			Environment:    "production",
			ReleaseVersion: "1.0.0",
		},
		Trigger: idir.Trigger{
			Type:        "message-redelivery",
			Description: "A message was redelivered after ack timeout.",
		},
		Events: []idir.Event{
			{ID: "evt-1", Type: "event-consumed", Description: "Event consumed."},
			{ID: "evt-2", Type: "charge-committed", Description: "Charge committed.", CausedBy: []string{"evt-1"}},
		},
		Services: []idir.Service{
			{Name: "payment-worker", Role: "consumer", TechnologyCategory: "message-queue-consumer"},
		},
		SideEffects: []idir.SideEffect{
			{Type: "duplicate_charge", Description: "Second charge attempted.", AffectedEntity: "order", Reversible: true},
		},
		BusinessInvariants: []idir.Invariant{
			{ID: "inv-1", Statement: "At most one completed charge per order."},
		},
		ExpectedCorrectedBehavior: []string{"Idempotent charge processing keyed on order id."},
		Evidence: []idir.Evidence{
			{ID: "ev-1", Type: "log", Location: "s3://bucket/key", Digest: "sha256:" + repeatHex(64)},
		},
		ReproductionSequence: []idir.ReproStep{
			{Step: 1, Description: "Consume order-created event.", RefEventID: "evt-1"},
		},
		Privacy: idir.Privacy{
			Sensitivity: idir.SensitivityInternal,
			Redacted:    false,
		},
	}
}

func repeatHex(n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = "0123456789abcdef"[i%16]
	}
	return string(out)
}
