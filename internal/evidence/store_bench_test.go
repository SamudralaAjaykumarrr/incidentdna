package evidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// benchObjectSize is a representative evidence object size for the store's
// own content-hashing path (docs/phase-8-plan.md §13's "e.g. 1 MiB").
const benchObjectSize = 1 << 20 // 1 MiB

// benchFixtureContent returns deterministic, non-zero content of
// benchObjectSize bytes — non-zero so the benchmark cannot be short-circuited
// by a sparse-file/all-zero fast path in any underlying I/O layer.
func benchFixtureContent() []byte {
	buf := make([]byte, benchObjectSize)
	for i := range buf {
		buf[i] = byte(i % 251) // 251 is prime, avoids a short repeating cycle
	}
	return buf
}

// BenchmarkStore measures Store.Put's content-hashing-and-copy path over a
// representative 1 MiB object (docs/phase-8-plan.md §13). Informational
// only — not wired into any gating path (see scripts/run-benchmarks.sh).
func BenchmarkStore(b *testing.B) {
	content := benchFixtureContent()
	ctx := context.Background()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		parent := b.TempDir()
		s, err := Open(filepath.Join(parent, "store"))
		if err != nil {
			b.Fatalf("Open: %v", err)
		}
		src := filepath.Join(parent, "src.bin")
		if err := os.WriteFile(src, content, 0o644); err != nil {
			b.Fatalf("write fixture: %v", err)
		}
		b.StartTimer()

		if _, err := s.Put(ctx, src); err != nil {
			b.Fatalf("Put: %v", err)
		}
	}
}

// BenchmarkVerify measures Store.Verify's re-hash-and-compare path over a
// representative 1 MiB already-stored object (docs/phase-8-plan.md §13).
// Informational only — not wired into any gating path (see
// scripts/run-benchmarks.sh).
func BenchmarkVerify(b *testing.B) {
	content := benchFixtureContent()
	ctx := context.Background()

	parent := b.TempDir()
	s, err := Open(filepath.Join(parent, "store"))
	if err != nil {
		b.Fatalf("Open: %v", err)
	}
	src := filepath.Join(parent, "src.bin")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		b.Fatalf("write fixture: %v", err)
	}
	res, err := s.Put(ctx, src)
	if err != nil {
		b.Fatalf("Put: %v", err)
	}
	doc := &idir.Document{Evidence: []idir.Evidence{
		{ID: "ev-bench", Type: "log", Digest: res.Digest.String()},
	}}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := s.Verify(ctx, doc); err != nil {
			b.Fatalf("Verify: %v", err)
		}
	}
}
