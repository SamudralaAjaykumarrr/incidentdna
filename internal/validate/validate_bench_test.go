package validate

import (
	"context"
	"testing"
)

// BenchmarkValidate measures Validate (the full semantic rule engine) over
// a minimal valid document (docs/phase-8-plan.md §13). Informational
// only — not wired into any gating path (see scripts/run-benchmarks.sh).
// Reuses validDoc(), already defined in testdata_test.go (same package).
func BenchmarkValidate(b *testing.B) {
	doc := validDoc()
	ctx := context.Background()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		res := Validate(ctx, doc)
		if !res.Valid() {
			b.Fatalf("unexpected validation issues: %v", res.Issues)
		}
	}
}
