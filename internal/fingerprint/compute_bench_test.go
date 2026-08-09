package fingerprint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// benchRepoRoot mirrors golden_test.go's repoRoot but accepts *testing.B —
// golden_test.go's own repoRoot(t *testing.T) is an existing test file this
// phase must not edit (docs/phase-8-plan.md §21/§25), so this benchmark
// file defines its own equivalent under a distinct name instead.
func benchRepoRoot(b *testing.B) string {
	b.Helper()
	wd, err := os.Getwd()
	if err != nil {
		b.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

// BenchmarkCompute measures Compute (identity-payload extraction plus
// canonical.Marshal plus SHA-256) over the duplicate-payment example
// document, end to end (docs/phase-8-plan.md §13). Informational only — not
// wired into any gating path (see scripts/run-benchmarks.sh).
func BenchmarkCompute(b *testing.B) {
	docPath := filepath.Join(benchRepoRoot(b), "examples", "duplicate-payment", "incident.yaml")
	doc, err := idir.LoadFile(docPath)
	if err != nil {
		b.Fatalf("LoadFile(%s): %v", docPath, err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Compute(doc); err != nil {
			b.Fatalf("Compute: %v", err)
		}
	}
}
