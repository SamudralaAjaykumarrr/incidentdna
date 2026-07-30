// check.go implements Phase 3 Slice 3's read-side Check operation: does the
// library already hold one or more trustworthy occurrences under a
// candidate document's fingerprint? docs/phase-3-plan.md §3/§5/§7/§12.
package library

import (
	"context"
	"fmt"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/validate"
)

// CheckOutcome categorizes a successful Check call.
type CheckOutcome string

const (
	// CheckOutcomeMatch means the library holds one or more intact
	// occurrences under doc's fingerprint. Matching never requires any
	// stored occurrence's incident.id to equal doc's own (docs/phase-3-plan.md
	// §5: the fingerprint identifies a failure class, not a specific
	// record).
	CheckOutcomeMatch CheckOutcome = "match"

	// CheckOutcomeNoMatch means doc is a valid document but the library
	// holds no occurrence under its fingerprint.
	CheckOutcomeNoMatch CheckOutcome = "no_match"
)

// CheckResult is the outcome of a successful Check call.
type CheckResult struct {
	Outcome CheckOutcome
	// Fingerprint is doc's own "sha256:"-prefixed failure-class fingerprint
	// (internal/fingerprint.Compute's output), populated regardless of
	// Outcome.
	Fingerprint string
	// MatchCount is the number of intact occurrences found under
	// Fingerprint. Zero when Outcome is CheckOutcomeNoMatch.
	MatchCount int
}

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
