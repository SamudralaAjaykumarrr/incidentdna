// list.go implements Phase 3 Slice 3's read-side List operation: enumerating
// the fingerprint groups and occurrences already stored in a library Store,
// bounded to exactly the metadata fields docs/phase-3-plan.md §7/§20.7
// approve for display — never the full stored document, business
// invariants, event timelines, evidence contents, or remediation text.
//
// List enforces MaxListResults (see limits.go, Slice 5) once its structural
// walk has discovered every fingerprint entry, but before doing any of the
// per-entry work (opening and verifying occurrence files) that
// summarizeFingerprint performs — so a library over the limit fails fast
// rather than paying for unnecessary work it will refuse to return
// (docs/phase-3-plan.md §12).
package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// List enumerates every fingerprint group and occurrence currently stored in
// s, returning only the bounded metadata fields docs/phase-3-plan.md §7
// approves. A library root that does not exist yet is not an error — List
// returns an empty ListResult, mirroring evidence.Open's "not-yet-created is
// not an error" stance (docs/phase-3-plan.md §12).
//
// Before including an occurrence in the result, List verifies it is intact
// (present, a regular file, and re-hashes to its own declared digest) —
// exactly the same check Check performs — so a corrupted or missing
// occurrence is never silently omitted or misreported: List fails with a
// distinct, wrapped error instead of returning a partial listing. Likewise,
// an unexpected shard/fingerprint directory name (wrong length, not lowercase
// hex, or a symlink) is reported as ErrMalformedLibrary rather than followed
// or silently skipped (docs/phase-3-plan.md §12/§19 acceptance criterion 7;
// no "partial results with warnings" mode is approved for this phase).
func List(ctx context.Context, s *Store) (ListResult, error) {
	if err := ctx.Err(); err != nil {
		return ListResult{}, fmt.Errorf("library: list canceled: %w", err)
	}

	shardEntries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ListResult{}, nil
		}
		return ListResult{}, fmt.Errorf("library: read library root %q: %w", s.root, err)
	}

	var fps []string
	for _, shardEntry := range shardEntries {
		if err := ctx.Err(); err != nil {
			return ListResult{}, fmt.Errorf("library: list canceled: %w", err)
		}
		if err := requireHexDir(shardEntry, shardPrefixLen); err != nil {
			return ListResult{}, fmt.Errorf("library: %w: shard %q under library root: %v", ErrMalformedLibrary, shardEntry.Name(), err)
		}

		fpEntries, err := os.ReadDir(filepath.Join(s.root, shardEntry.Name()))
		if err != nil {
			return ListResult{}, fmt.Errorf("library: read shard directory %q: %w", shardEntry.Name(), err)
		}
		for _, fpEntry := range fpEntries {
			if err := ctx.Err(); err != nil {
				return ListResult{}, fmt.Errorf("library: list canceled: %w", err)
			}
			if err := requireHexDir(fpEntry, 64-shardPrefixLen); err != nil {
				return ListResult{}, fmt.Errorf("library: %w: fingerprint directory %q under shard %q: %v", ErrMalformedLibrary, fpEntry.Name(), shardEntry.Name(), err)
			}

			fps = append(fps, "sha256:"+shardEntry.Name()+fpEntry.Name())
		}
	}

	// Shard and fingerprint directory names come back from os.ReadDir sorted
	// by filename ascending already (Go stdlib guarantee), and iterating
	// shards outermost then names innermost preserves that as overall
	// fingerprint-hex-ascending order — but sort defensively so ListResult's
	// contract does not depend on that documented-but-easy-to-miss stdlib
	// detail (docs/phase-3-plan.md §13).
	sort.Strings(fps)

	if err := checkListResultsCap(len(fps)); err != nil {
		return ListResult{}, err
	}

	summaries := make([]FingerprintSummary, 0, len(fps))
	for _, fp := range fps {
		if err := ctx.Err(); err != nil {
			return ListResult{}, fmt.Errorf("library: list canceled: %w", err)
		}
		summary, err := summarizeFingerprint(s, fp)
		if err != nil {
			return ListResult{}, err
		}
		summaries = append(summaries, summary)
	}

	return ListResult{Fingerprints: summaries}, nil
}

// requireHexDir validates that entry is a real (non-symlink) directory whose
// name is exactly wantLen lowercase hex characters — the fixed shape every
// shard and fingerprint directory docs/phase-3-plan.md §5 defines must have.
// Rejecting any other shape (a symlink, a non-directory, or a name outside
// the hex alphabet, which structurally cannot contain "..", "/", or a null
// byte) is what makes this walk safe against path escapes: only names List
// itself validated are ever joined into a path.
func requireHexDir(entry os.DirEntry, wantLen int) error {
	if entry.Type()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%q is a symbolic link, not a real directory", entry.Name())
	}
	if !entry.IsDir() {
		return fmt.Errorf("%q is not a directory", entry.Name())
	}
	if len(entry.Name()) != wantLen || !isLowerHex(entry.Name()) {
		return fmt.Errorf("%q is not exactly %d lowercase hexadecimal characters", entry.Name(), wantLen)
	}
	return nil
}

func isLowerHex(s string) bool {
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

// summarizeFingerprint reads fp's index.json and, for each indexed
// occurrence (verified intact first), builds its bounded display summary.
// Title and Service are read from the occurrence's own stored body rather
// than index.json, since index.json deliberately holds only digest,
// incident_id, and occurred_at (docs/phase-3-plan.md §5) — title and
// application/service are only available in the full canonical document
// body, which is exactly why List needs to open (and integrity-check) each
// occurrence rather than trusting index.json alone.
func summarizeFingerprint(s *Store, fp string) (FingerprintSummary, error) {
	idxPath, err := s.IndexPath(fp)
	if err != nil {
		return FingerprintSummary{}, fmt.Errorf("library: resolve index path for fingerprint %s: %w", fp, err)
	}
	idx, err := readIndex(idxPath)
	if err != nil {
		return FingerprintSummary{}, err
	}
	idx.sortEntries()

	occurrences := make([]OccurrenceSummary, 0, len(idx.Entries))
	for _, e := range idx.Entries {
		occPath, err := s.OccurrencePath(fp, e.Digest)
		if err != nil {
			return FingerprintSummary{}, fmt.Errorf("library: %w: index entry for incident id %q under fingerprint %s has an unusable digest %q: %v", ErrMalformedIndex, e.IncidentID, fp, e.Digest, err)
		}
		data, err := verifyOccurrenceBytes(occPath, e.Digest)
		if err != nil {
			return FingerprintSummary{}, err
		}

		var doc idir.Document
		if err := json.Unmarshal(data, &doc); err != nil {
			return FingerprintSummary{}, fmt.Errorf("library: %w: occurrence at digest %s under fingerprint %s does not decode as a valid document: %v", ErrCorruptedOccurrence, e.Digest, fp, err)
		}

		service := doc.Application.Service
		if service == "" {
			service = doc.Application.Name
		}
		occurrences = append(occurrences, OccurrenceSummary{
			IncidentID: doc.Incident.ID,
			Title:      doc.Incident.Title,
			Service:    service,
			OccurredAt: doc.Incident.OccurredAt,
		})
	}

	return FingerprintSummary{
		Fingerprint:     fp,
		OccurrenceCount: len(occurrences),
		Occurrences:     occurrences,
	}, nil
}
