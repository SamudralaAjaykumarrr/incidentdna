package evidence

import (
	"context"
	"fmt"
	"io"
)

// Resource limits enforced by this package. Each is a fixed constant, not a
// configurable flag — see docs/phase-2-plan.md §9/§14 for why: this mirrors
// idir.MaxDocumentSize's existing precedent of a hardcoded constant rather
// than a configuration surface.
const (
	// MaxObjectSize is the maximum number of bytes any single evidence
	// object may occupy, enforced while streaming (both when ingesting a
	// new object in Store and when re-hashing an existing one in Verify or
	// Inspect) — never only via a preliminary os.Stat, since a file can
	// grow between a stat and a read.
	MaxObjectSize = 50 * 1024 * 1024 // 50 MiB

	// MaxEntriesPerCommand is the maximum number of evidence[] entries List
	// or Verify will process from one IDIR document in one invocation.
	MaxEntriesPerCommand = 100

	// MaxTotalVerifyBytes is the maximum aggregate number of stored-object
	// bytes Verify will read across all evidence entries in one invocation,
	// checked via a cheap Stat-only pre-pass before any object's content is
	// read (see verify.go).
	MaxTotalVerifyBytes = 500 * 1024 * 1024 // 500 MiB
)

// copyBufferSize is the chunk size used by the context-aware limited copy
// below. It is small and fixed regardless of MaxObjectSize, so streaming a
// 50 MiB object never allocates a 50 MiB buffer.
const copyBufferSize = 32 * 1024

// ctxReader wraps an io.Reader with a context check performed before every
// Read call, so a long copy (io.Copy loops calling Read repeatedly) responds
// to cancellation at chunk granularity rather than only at its start or end.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c *ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// hashLimitedCopy copies from src into dst (typically an io.MultiWriter of a
// hash.Hash and, for Store, a destination file) up to limit+1 bytes, using a
// fixed-size buffer regardless of limit, and checking ctx for cancellation
// before every chunk. It reports ErrKindObjectTooLarge if more than limit
// bytes were available, ErrKindCanceled if ctx was canceled mid-copy, and
// wraps any underlying read/write error as ErrKindReadFailure/
// ErrKindWriteFailure via the caller (hashLimitedCopy itself returns the
// raw error for the caller to classify, since only the caller knows whether
// the failing side was the source or the destination).
func hashLimitedCopy(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (int64, error) {
	guarded := &ctxReader{ctx: ctx, r: src}
	limited := io.LimitReader(guarded, limit+1)
	n, err := io.CopyBuffer(dst, limited, make([]byte, copyBufferSize))
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, &Error{
			Kind: ErrKindObjectTooLarge,
			Msg:  fmt.Sprintf("evidence object exceeds the maximum object size of %d bytes (%d MiB)", MaxObjectSize, MaxObjectSize/(1024*1024)),
		}
	}
	return n, nil
}

// checkEntryCount enforces MaxEntriesPerCommand against n (typically
// len(doc.Evidence)) before any per-entry filesystem work begins.
func checkEntryCount(n int) error {
	if n > MaxEntriesPerCommand {
		return &Error{
			Kind: ErrKindTooManyEntries,
			Msg:  fmt.Sprintf("document has %d evidence entries, which exceeds the maximum of %d entries processed per command", n, MaxEntriesPerCommand),
		}
	}
	return nil
}

// checkTotalBytes enforces MaxTotalVerifyBytes against a precomputed
// aggregate total (see verify.go's Stat-only pre-pass).
func checkTotalBytes(total int64) error {
	if total > MaxTotalVerifyBytes {
		return &Error{
			Kind: ErrKindTotalBytesExceeded,
			Msg:  fmt.Sprintf("evidence to verify totals %d bytes, which exceeds the maximum of %d bytes (%d MiB) verified per command", total, MaxTotalVerifyBytes, MaxTotalVerifyBytes/(1024*1024)),
		}
	}
	return nil
}
