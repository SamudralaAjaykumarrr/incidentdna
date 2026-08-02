// add.go implements Phase 3 Slice 2's §5 occurrence-decision logic for
// adding a validated IDIR document to a library Store — new occurrence,
// idempotent re-add, conflict, or corrupted-existing-entry — per
// docs/phase-3-plan.md §5/§12/§13.
//
// Add enforces the §10 privacy gate (see privacy.go, Slice 4) immediately
// after validation and before any fingerprint/digest/storage work, and the
// §11 resource limits (see limits.go, Slice 5) before any unsafe or
// unnecessary write: MaxLibraryEntries before creating a fingerprint
// directory that does not yet exist, and MaxOccurrencesPerFingerprint
// before storing a genuinely new occurrence under an existing one.
// MaxDocumentSize is not enforced here — Add accepts an already-parsed
// *idir.Document, not a file, and that limit belongs at the file-loading
// boundary (idir.LoadFile), which the future CLI (Slice 6) will call
// unchanged (docs/phase-3-plan.md §11; see limits.go's checkDocumentSize
// doc comment).
package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/canonical"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/validate"
)

// Add validates doc, enforces the §10 privacy gate, computes doc's
// fingerprint and canonical-bytes digest, and applies the occurrence-decision
// semantics of docs/phase-3-plan.md §5:
//
//  1. No existing occurrence under doc's fingerprint shares its incident.id
//     → a new occurrence is stored (AddOutcomeStored).
//  2. An existing occurrence shares incident.id and has the same digest →
//     idempotent, nothing written (AddOutcomeIdempotent).
//  3. An existing occurrence shares incident.id but has a different digest
//     → ErrConflict; the existing occurrence is never overwritten.
//  4. An existing occurrence Add needed to consult is missing or corrupted
//     → ErrCorruptedOccurrence; never reused, repaired, or overwritten.
//
// Before any of that occurrence-decision work, and before any bytes of doc
// are written anywhere, Add checks docs/phase-3-plan.md §10's privacy gate
// (see privacy.go): doc must declare privacy.redacted == true unless
// opts.AllowUnredacted is set, in which case Add proceeds and reports
// AddResult.PrivacyOverridden so the caller can warn (docs/phase-3-plan.md
// §12, "checked after validation succeeds and before any
// fingerprint/digest/storage work happens").
//
// incident.id (or any other document-supplied field) is never used as, or
// concatenated into, a filesystem path — only fingerprint and digest values
// validated by Store's parseFingerprint/parseDigestHex ever become path
// components (docs/phase-3-plan.md §5).
//
// MaxLibraryEntries and MaxOccurrencesPerFingerprint are enforced further
// below, immediately before Add would otherwise create a new fingerprint
// directory or store a genuinely new occurrence — never for an idempotent
// re-add, and never after any unsafe or unnecessary write
// (docs/phase-3-plan.md §12).
func Add(ctx context.Context, s *Store, doc *idir.Document, opts AddOptions) (AddResult, error) {
	if err := ctx.Err(); err != nil {
		return AddResult{}, fmt.Errorf("library: add canceled: %w", err)
	}
	if doc == nil {
		return AddResult{}, fmt.Errorf("library: %w: document is nil", ErrInvalidDocument)
	}

	if res := validate.Validate(ctx, doc); !res.Valid() {
		return AddResult{}, fmt.Errorf("library: %w:\n%s", ErrInvalidDocument, res.Error())
	}

	overridden, err := checkPrivacyGate(doc, opts.AllowUnredacted)
	if err != nil {
		return AddResult{}, err
	}

	fp, err := fingerprint.Compute(doc)
	if err != nil {
		return AddResult{}, fmt.Errorf("library: compute fingerprint: %w", err)
	}

	canonicalBytes, err := canonical.Marshal(doc)
	if err != nil {
		return AddResult{}, fmt.Errorf("library: canonicalize document: %w", err)
	}
	sum := sha256.Sum256(canonicalBytes)
	digestHex := hex.EncodeToString(sum[:])

	fpDir, err := s.FingerprintDir(fp)
	if err != nil {
		return AddResult{}, fmt.Errorf("library: resolve fingerprint directory: %w", err)
	}
	fpDirExisted := true
	if _, statErr := os.Stat(fpDir); statErr != nil {
		if !errors.Is(statErr, fs.ErrNotExist) {
			return AddResult{}, fmt.Errorf("library: stat fingerprint directory %q: %w", fpDir, statErr)
		}
		fpDirExisted = false
	}

	idxPath, err := s.IndexPath(fp)
	if err != nil {
		return AddResult{}, fmt.Errorf("library: resolve index path: %w", err)
	}
	idx, err := readIndex(idxPath)
	if err != nil {
		return AddResult{}, err
	}

	existing, err := findByIncidentID(idx, doc.Incident.ID)
	if err != nil {
		return AddResult{}, err
	}

	if existing != nil {
		occPath, err := s.OccurrencePath(fp, existing.Digest)
		if err != nil {
			return AddResult{}, fmt.Errorf("library: %w: index entry for incident id %q has an unusable digest %q: %v", ErrMalformedIndex, doc.Incident.ID, existing.Digest, err)
		}
		if _, err := verifyOccurrenceBytes(occPath, existing.Digest); err != nil {
			return AddResult{}, err
		}

		if existing.Digest == digestHex {
			return AddResult{Outcome: AddOutcomeIdempotent, Fingerprint: fp, Digest: digestHex, PrivacyOverridden: overridden}, nil
		}
		return AddResult{}, fmt.Errorf("library: %w (incident id %q, fingerprint %s)", ErrConflict, doc.Incident.ID, fp)
	}

	// From here, Add is about to store a genuinely new occurrence — the
	// only path that can grow either the library's fingerprint count or a
	// fingerprint group's occurrence count, so both resource-limit checks
	// run here, before any write.
	if !fpDirExisted {
		if err := checkLibraryEntryCap(s); err != nil {
			return AddResult{}, err
		}
	}
	if err := checkOccurrenceCap(len(idx.Entries)); err != nil {
		return AddResult{}, err
	}

	occPath, err := s.OccurrencePath(fp, digestHex)
	if err != nil {
		return AddResult{}, fmt.Errorf("library: resolve occurrence path: %w", err)
	}
	if err := ensureOccurrenceStored(occPath, digestHex, canonicalBytes); err != nil {
		return AddResult{}, err
	}

	idx.Entries = append(idx.Entries, indexEntry{
		Digest:     digestHex,
		IncidentID: doc.Incident.ID,
		OccurredAt: doc.Incident.OccurredAt,
	})
	if err := writeIndex(idxPath, idx); err != nil {
		return AddResult{}, err
	}

	return AddResult{Outcome: AddOutcomeStored, Fingerprint: fp, Digest: digestHex, PrivacyOverridden: overridden}, nil
}

// findByIncidentID returns the single index entry sharing incidentID, or nil
// if none does. More than one entry sharing incidentID is itself a
// malformed-index condition: Add's own semantics never produce that state
// (a conflicting incident.id is refused before a second entry is ever
// written), so seeing it means the index was corrupted or hand-edited
// out-of-band.
func findByIncidentID(idx *index, incidentID string) (*indexEntry, error) {
	var found *indexEntry
	matches := 0
	for i := range idx.Entries {
		if idx.Entries[i].IncidentID == incidentID {
			found = &idx.Entries[i]
			matches++
		}
	}
	if matches > 1 {
		return nil, fmt.Errorf("library: %w: index has %d entries for incident id %q, expected at most one", ErrMalformedIndex, matches, incidentID)
	}
	return found, nil
}

// verifyOccurrenceBytes reads the file at path — expected to be a canonical
// JSON occurrence object addressed by digestHex, the hex SHA-256 digest of
// its own bytes — and confirms it is a regular (non-symlink) file whose
// content re-hashes to digestHex exactly. A missing, non-regular, unreadable,
// or hash-mismatched occurrence is reported via a wrapped
// ErrCorruptedOccurrence; it is never trusted, reused, or silently repaired
// (docs/phase-3-plan.md §5 case 4).
func verifyOccurrenceBytes(path, digestHex string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("library: %w: occurrence referenced by index at digest %s does not exist on disk", ErrCorruptedOccurrence, digestHex)
		}
		return nil, fmt.Errorf("library: stat occurrence at digest %s: %w", digestHex, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("library: %w: occurrence at digest %s is not a regular file", ErrCorruptedOccurrence, digestHex)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("library: read occurrence at digest %s: %w", digestHex, err)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != digestHex {
		return nil, fmt.Errorf("library: %w: occurrence at digest %s re-hashes to %s; stored bytes were modified or truncated", ErrCorruptedOccurrence, digestHex, got)
	}
	return data, nil
}

// ensureOccurrenceStored makes sure canonicalBytes is durably present at
// path, addressed by digestHex. If nothing exists at path yet, it is written
// via a temp-file-then-rename (see writeOccurrenceAtomic). If something
// already exists there — an orphaned object from a prior Add that wrote the
// occurrence but was interrupted before its index.json update — its content
// is verified against digestHex rather than blindly rewritten or trusted by
// filename alone; a mismatch is reported as ErrCorruptedOccurrence rather
// than silently overwritten (docs/phase-3-plan.md §5 case 4, §10 "no silent
// overwrite").
func ensureOccurrenceStored(path, digestHex string, canonicalBytes []byte) error {
	info, lerr := os.Lstat(path)
	switch {
	case lerr == nil:
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("library: %w: an existing entry at digest %s is not a regular file; refusing to trust or replace it", ErrCorruptedOccurrence, digestHex)
		}
		if _, err := verifyOccurrenceBytes(path, digestHex); err != nil {
			return err
		}
		return nil
	case errors.Is(lerr, fs.ErrNotExist):
		return writeOccurrenceAtomic(path, canonicalBytes)
	default:
		return fmt.Errorf("library: stat occurrence at digest %s: %w", digestHex, lerr)
	}
}

// writeOccurrenceAtomic writes data (canonical document bytes) to path via a
// temporary file created in the same directory as path — so the finalizing
// rename is same-filesystem and atomic — fsync'd and closed, then made
// read-only and renamed into place. The temporary file is removed on any
// failure before the rename (docs/phase-3-plan.md §12/§14, "atomic-write
// cleanup after a simulated failure").
func writeOccurrenceAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("library: create occurrence directory %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".occurrence-*.tmp")
	if err != nil {
		return fmt.Errorf("library: create temporary occurrence file: %w", err)
	}
	tmpPath := tmp.Name()
	needsCleanup := true
	defer func() {
		if needsCleanup {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("library: write temporary occurrence file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("library: sync temporary occurrence file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("library: close temporary occurrence file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o444); err != nil {
		return fmt.Errorf("library: set read-only permissions on occurrence file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("library: finalize occurrence file %q: %w", path, err)
	}
	needsCleanup = false
	return nil
}
