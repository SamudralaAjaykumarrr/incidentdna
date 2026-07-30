// add.go implements Phase 3 Slice 2: the §5 occurrence-decision logic for
// adding a validated IDIR document to a library Store — new occurrence,
// idempotent re-add, conflict, or corrupted-existing-entry — per
// docs/phase-3-plan.md §5/§12/§13.
//
// Add enforces the §10 privacy gate (see privacy.go, added in Slice 4)
// immediately after validation and before any fingerprint/digest/storage
// work. Add does not enforce any of the §11 resource limits; those are a
// later slice (docs/phase-3-plan.md §18, slice 5) layered on top of this
// one.
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

// Sentinel errors identifying the distinct Add outcomes/failures required by
// docs/phase-3-plan.md §5/§12. Add distinguishes its failure modes via these
// wrapped sentinels and errors.Is, following store.go's existing plain-error
// style (Slice 1) rather than introducing a structured *Error/ErrorKind type
// here — docs/phase-3-plan.md §18 assigns that fuller type (errors.go) to
// Slice 5, alongside the resource-limit constants this file deliberately
// does not enforce yet.
var (
	// ErrInvalidDocument means doc failed validate.Validate.
	ErrInvalidDocument = errors.New("library: document failed validation")

	// ErrConflict means an occurrence with the same incident.id already
	// exists under this fingerprint with materially different content
	// (docs/phase-3-plan.md §5 case 3).
	ErrConflict = errors.New("library: an occurrence with this incident id already exists under this fingerprint with different content")

	// ErrCorruptedOccurrence means an existing occurrence that Add needed to
	// consult (because its index entry shares the incoming document's
	// incident.id) is missing, unreadable, or does not re-hash to its own
	// declared digest (docs/phase-3-plan.md §5 case 4). Add never reuses,
	// repairs, or overwrites such an entry.
	ErrCorruptedOccurrence = errors.New("library: an existing stored occurrence is corrupted")

	// ErrMalformedIndex means a fingerprint directory's index.json exists
	// but is not well-formed index data.
	ErrMalformedIndex = errors.New("library: index is malformed")
)

// AddOutcome categorizes a successful Add call.
type AddOutcome string

const (
	// AddOutcomeStored means a new occurrence was written: either no
	// existing occurrence under this fingerprint shared the document's
	// incident.id, or one did but at an orphaned, unindexed, byte-identical
	// digest path recovered from a prior incomplete write.
	AddOutcomeStored AddOutcome = "stored"

	// AddOutcomeIdempotent means an occurrence with the same incident.id and
	// the same canonical-bytes digest already existed; nothing was written.
	AddOutcomeIdempotent AddOutcome = "idempotent"
)

// AddResult is the outcome of a successful Add call.
type AddResult struct {
	Outcome AddOutcome
	// Fingerprint is the "sha256:"-prefixed failure-class fingerprint
	// (internal/fingerprint.Compute's output) doc was stored or matched
	// under.
	Fingerprint string
	// Digest is the bare (unprefixed) 64-character lowercase hex SHA-256
	// digest of the occurrence's own canonical document bytes.
	Digest string
	// PrivacyOverridden is true when doc's privacy.redacted field was false
	// (or unset) and Add proceeded only because AddOptions.AllowUnredacted
	// was set (docs/phase-3-plan.md §10). When true, the caller (the future
	// CLI) MUST display a warning to the user; internal/library itself never
	// prints one. Always false when doc already declared privacy.redacted
	// == true, regardless of AllowUnredacted.
	PrivacyOverridden bool
}

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
