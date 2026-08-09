package canonical

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// repoRoot resolves the repository root from internal/canonical's package
// directory (two levels up), the same convention every other package's
// fixture-loading test/benchmark in this module already uses.
func repoRoot(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..")
}

// BenchmarkMarshal measures canonical.Marshal over the duplicate-payment
// example document — the operation every `fingerprint`, `compare`,
// `scenario verify --source`, and `policy evaluate` invocation performs at
// least once (docs/phase-8-plan.md §13). This benchmark is informational
// only: it is not wired into any gating path (see scripts/run-benchmarks.sh).
func BenchmarkMarshal(b *testing.B) {
	docPath := filepath.Join(repoRoot(b), "examples", "duplicate-payment", "incident.yaml")
	doc, err := idir.LoadFile(docPath)
	if err != nil {
		b.Fatalf("LoadFile(%s): %v", docPath, err)
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(doc); err != nil {
			b.Fatalf("Marshal: %v", err)
		}
	}
}
