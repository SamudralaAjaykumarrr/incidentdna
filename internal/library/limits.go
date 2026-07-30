// limits.go implements Phase 3 Slice 5's four approved resource limits
// (docs/phase-3-plan.md §11/§18): MaxDocumentSize (reused from
// internal/idir, not redefined), MaxLibraryEntries, MaxOccurrencesPerFingerprint,
// and MaxListResults. Each has a dedicated constant and a dedicated check
// function returning a distinct *Error Kind, so a resource-limit failure is
// never confused with another limit's failure or with an ordinary
// validation/corruption failure.
package library

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

const (
	// MaxDocumentSize is the maximum number of bytes a document's original,
	// on-disk (YAML or JSON) representation may occupy. It is reused
	// directly from internal/idir.MaxDocumentSize, not a separately
	// hardcoded literal (docs/phase-3-plan.md §11: "reused from
	// internal/idir, not redefined") — this is the same value
	// idir.LoadFile already enforces, at the file-loading boundary, before
	// any parsing occurs.
	//
	// internal/library does not itself read files: Add and Check accept an
	// already-parsed *idir.Document. Re-deriving an approximation of the
	// original file size from that parsed document (e.g. by re-serializing
	// it) is not equivalent to the original input's byte size and must not
	// be used as a stand-in for it — a parsed document's canonical or
	// standard-library JSON encoding can be smaller or larger than the
	// original YAML/JSON bytes it was decoded from. Enforcing this limit
	// belongs at the real file-loading boundary, which is
	// cmd/incidentdna's future `library` CLI wiring (Slice 6), calling
	// idir.LoadFile exactly as every other command already does — not a
	// second, reinterpreted check inside this package.
	MaxDocumentSize = idir.MaxDocumentSize

	// MaxLibraryEntries is the maximum number of distinct fingerprint
	// (failure-class) entries the library will hold. Add refuses to create
	// a new fingerprint directory once the library already holds this many
	// (docs/phase-3-plan.md §11).
	MaxLibraryEntries = 10000

	// MaxOccurrencesPerFingerprint is the maximum number of occurrences Add
	// will store under a single fingerprint directory before refusing to
	// store a genuinely new one. An idempotent re-add (case 2, §5) is never
	// blocked by this limit, since it writes nothing (docs/phase-3-plan.md
	// §11).
	MaxOccurrencesPerFingerprint = 100

	// MaxListResults is the maximum number of fingerprint entries List will
	// enumerate in one invocation (docs/phase-3-plan.md §11).
	MaxListResults = 1000
)

// checkDocumentSize enforces MaxDocumentSize against sizeBytes, an explicit
// byte count representing a document's original, on-disk serialized size
// (e.g. os.FileInfo.Size(), exactly what idir.LoadFile already checks
// before parsing). This is a pure, reusable check: it takes a byte count
// directly rather than measuring, re-serializing, or otherwise
// approximating a parsed *idir.Document's size, since a parsed document's
// canonical or JSON re-encoding is not the same measurement as the
// original input's byte size (docs/phase-3-plan.md §11).
//
// Nothing in this package invokes this against a document's own bytes: Add
// and Check accept an already-parsed *idir.Document, not a file, so this
// limit's real enforcement point remains idir.LoadFile at the future CLI's
// file-loading boundary (Slice 6), unchanged from Phase 1. This helper is
// kept here, ready for that integration, and is covered by its own
// boundary tests.
func checkDocumentSize(sizeBytes int64) error {
	if sizeBytes > MaxDocumentSize {
		return &Error{
			Kind: ErrKindDocumentTooLarge,
			Msg: fmt.Sprintf(
				"document is %d bytes, which exceeds the maximum document size of %d bytes (%d MiB)",
				sizeBytes, MaxDocumentSize, MaxDocumentSize/(1024*1024),
			),
		}
	}
	return nil
}

// countFingerprintDirs counts the number of fingerprint (failure-class)
// directories currently stored under s, by walking the shard/fingerprint
// two-level directory scheme (docs/phase-3-plan.md §5). A library root that
// does not exist yet has zero entries, mirroring the rest of this package's
// "not-yet-created is not an error" stance (docs/phase-3-plan.md §12).
//
// This is a cheap, permissive count used only to enforce MaxLibraryEntries
// before Add creates a new fingerprint directory — it does not validate
// shard/fingerprint directory name shape the way List's own walk does
// (list.go's requireHexDir); that structural validation remains List's
// distinct responsibility (ErrMalformedLibrary), not this resource-limit
// check's.
func countFingerprintDirs(s *Store) (int, error) {
	shardEntries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("library: read library root %q: %w", s.root, err)
	}

	count := 0
	for _, shardEntry := range shardEntries {
		if !shardEntry.IsDir() {
			continue
		}
		fpEntries, err := os.ReadDir(filepath.Join(s.root, shardEntry.Name()))
		if err != nil {
			return 0, fmt.Errorf("library: read shard directory %q: %w", shardEntry.Name(), err)
		}
		for _, fpEntry := range fpEntries {
			if fpEntry.IsDir() {
				count++
			}
		}
	}
	return count, nil
}

// checkLibraryEntryCap enforces MaxLibraryEntries. It must be called only
// when Add is about to create a fingerprint directory that does not yet
// exist — an occurrence added to an already-existing fingerprint group never
// changes the total fingerprint count, so it never needs this check.
func checkLibraryEntryCap(s *Store) error {
	count, err := countFingerprintDirs(s)
	if err != nil {
		return fmt.Errorf("library: count existing library entries: %w", err)
	}
	if count >= MaxLibraryEntries {
		return &Error{
			Kind: ErrKindTooManyLibraryEntries,
			Msg: fmt.Sprintf(
				"library already holds %d fingerprint entries, which meets the maximum of %d (MaxLibraryEntries); refusing to create a new fingerprint entry",
				count, MaxLibraryEntries,
			),
		}
	}
	return nil
}

// checkOccurrenceCap enforces MaxOccurrencesPerFingerprint against
// existingCount, the number of occurrences already indexed under the
// fingerprint Add is about to store a genuinely new occurrence under.
func checkOccurrenceCap(existingCount int) error {
	if existingCount >= MaxOccurrencesPerFingerprint {
		return &Error{
			Kind: ErrKindTooManyOccurrences,
			Msg: fmt.Sprintf(
				"this fingerprint already holds %d occurrences, which meets the maximum of %d (MaxOccurrencesPerFingerprint); refusing to store a new occurrence",
				existingCount, MaxOccurrencesPerFingerprint,
			),
		}
	}
	return nil
}

// checkListResultsCap enforces MaxListResults against n, the number of
// fingerprint entries List discovered in its structural walk, before List
// does any further per-entry work (opening and verifying occurrence files).
func checkListResultsCap(n int) error {
	if n > MaxListResults {
		return &Error{
			Kind: ErrKindTooManyListResults,
			Msg: fmt.Sprintf(
				"library holds %d fingerprint entries, which exceeds the maximum of %d (MaxListResults) that list will enumerate in one invocation",
				n, MaxListResults,
			),
		}
	}
	return nil
}
