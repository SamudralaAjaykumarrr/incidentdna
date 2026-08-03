// check.go implements Phase 3 Slice 3's read-side Check operation: does the
// library already hold one or more trustworthy occurrences under a
// candidate document's fingerprint? docs/phase-3-plan.md §3/§5/§7/§12.
//
// Check does not enforce MaxDocumentSize — it accepts an already-parsed
// *idir.Document, not a file, and that limit belongs at the file-loading
// boundary (idir.LoadFile), which the future CLI (Slice 6) will call
// unchanged (docs/phase-3-plan.md §11; see limits.go's checkDocumentSize
// doc comment).
package library

import (
	"context"
	"fmt"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/validate"
)

// Check validates doc, computes its fingerprint (byte-for-byte the same
// internal/fingerprint.Compute call every other command uses — no separate
// "library-specific" fingerprinting logic, docs/phase-3-plan.md §13), and
// reports whether the library holds one or more occurrences under that
// fingerprint.
//
// Before reporting CheckOutcomeMatch, Check verifies every occurrence
// indexed under the fingerprint is intact (present, a regular file, and
// re-hashes to its own declared digest — the same verifyOccurrenceBytes
// check Add uses before trusting an existing entry): an untrustworthy
// library entry must never be reported as a match. If any indexed occurrence
// is missing or corrupted, Check fails with a wrapped ErrCorruptedOccurrence
// rather than silently reporting a match based on the remaining entries, or
// falling back to CheckOutcomeNoMatch — the approved plan (§12) treats
// "index.json references an occurrence that doesn't exist / doesn't parse"
// as a "corrupted" finding, distinct from "not found," so a corrupted
// library must never be misreported as an ordinary no-match.
func Check(ctx context.Context, s *Store, doc *idir.Document) (CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return CheckResult{}, fmt.Errorf("library: check canceled: %w", err)
	}
	if doc == nil {
		return CheckResult{}, fmt.Errorf("library: %w: document is nil", ErrInvalidDocument)
	}

	if res := validate.Validate(ctx, doc); !res.Valid() {
		return CheckResult{}, fmt.Errorf("library: %w:\n%s", ErrInvalidDocument, res.Error())
	}

	fp, err := fingerprint.Compute(doc)
	if err != nil {
		return CheckResult{}, fmt.Errorf("library: compute fingerprint: %w", err)
	}

	return checkFingerprint(ctx, s, fp)
}

// CheckFingerprint reports whether the library holds one or more intact
// occurrences under fp directly, without requiring a full idir.Document to
// compute it from. fp must already be a well-formed "sha256:"-prefixed,
// 64-lowercase-hex fingerprint string — the same format
// internal/fingerprint.Compute produces and Check's own internal validation
// already enforces once it has computed one from a document.
//
// This is the read-only lookup entry point Phase 6's scenario/suite
// cross-reference uses (docs/phase-6-plan.md §5): a scenario or suite
// manifest declares a linked_fingerprint directly, with no idir.Document
// available to recompute it from, so Check (which requires one) cannot be
// reused as-is. CheckFingerprint and Check share every line of lookup logic
// after fingerprint computation — only how the fingerprint is obtained
// differs.
func CheckFingerprint(ctx context.Context, s *Store, fp string) (CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return CheckResult{}, fmt.Errorf("library: check canceled: %w", err)
	}
	if _, err := parseFingerprint(fp); err != nil {
		return CheckResult{}, fmt.Errorf("library: %w: %v", ErrInvalidFingerprint, err)
	}
	return checkFingerprint(ctx, s, fp)
}

// checkFingerprint is the shared lookup logic behind both Check and
// CheckFingerprint: given an already-validated, well-formed fingerprint
// string, look up its index, verify every indexed occurrence is intact, and
// report the match/no-match outcome.
func checkFingerprint(ctx context.Context, s *Store, fp string) (CheckResult, error) {
	idxPath, err := s.IndexPath(fp)
	if err != nil {
		return CheckResult{}, fmt.Errorf("library: resolve index path: %w", err)
	}
	idx, err := readIndex(idxPath)
	if err != nil {
		return CheckResult{}, err
	}

	if len(idx.Entries) == 0 {
		return CheckResult{Outcome: CheckOutcomeNoMatch, Fingerprint: fp}, nil
	}

	for _, e := range idx.Entries {
		if err := ctx.Err(); err != nil {
			return CheckResult{}, fmt.Errorf("library: check canceled: %w", err)
		}
		occPath, err := s.OccurrencePath(fp, e.Digest)
		if err != nil {
			return CheckResult{}, fmt.Errorf("library: %w: index entry for incident id %q has an unusable digest %q: %v", ErrMalformedIndex, e.IncidentID, e.Digest, err)
		}
		if _, err := verifyOccurrenceBytes(occPath, e.Digest); err != nil {
			return CheckResult{}, err
		}
	}

	return CheckResult{Outcome: CheckOutcomeMatch, Fingerprint: fp, MatchCount: len(idx.Entries)}, nil
}
