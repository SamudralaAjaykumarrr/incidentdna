// fingerprint_link.go implements the optional --source cross-check
// (docs/phase-4-plan.md §2.2/§4 Workflow A): load, validate, and fingerprint
// a real incident file through the unchanged Phase 1 pipeline
// (idir.LoadFile -> validate.Validate -> fingerprint.Compute), then compare
// the result to a scenario's declared linked_fingerprint. This is the only
// place internal/scenario imports internal/idir, internal/validate, and
// internal/fingerprint; it never looks up internal/library, and a scenario
// is fully runnable without --source ever being given
// (docs/phase-4-plan.md §6).
package scenario

import (
	"context"
	"fmt"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/validate"
)

// SourceFingerprintResult is the outcome of cross-checking a scenario's
// linked_fingerprint against a real incident file named by --source.
type SourceFingerprintResult struct {
	// SourceFingerprint is the freshly computed fingerprint of the --source
	// incident file.
	SourceFingerprint string
	// Match reports whether SourceFingerprint equals the scenario's
	// declared LinkedFingerprint.
	Match bool
}

// CheckSourceFingerprint loads and validates the IDIR document at
// sourcePath through the exact same idir.LoadFile + validate.Validate
// pipeline every other command uses, computes its fingerprint via
// fingerprint.Compute (unchanged), and compares it to linkedFingerprint.
//
// An error is returned only for an I/O/parse failure (sourcePath cannot be
// loaded) or a semantic validation failure (sourcePath loads but is not a
// valid IDIR document) — never for a fingerprint mismatch, which is reported
// via SourceFingerprintResult.Match instead, so a caller can distinguish
// "the source file itself is broken" (exit 1, docs/phase-4-plan.md §14) from
// "the source file is fine but does not match" (exit 2).
func CheckSourceFingerprint(ctx context.Context, sourcePath, linkedFingerprint string) (SourceFingerprintResult, error) {
	doc, err := idir.LoadFile(sourcePath)
	if err != nil {
		return SourceFingerprintResult{}, fmt.Errorf("load --source %s: %w", sourcePath, err)
	}

	res := validate.Validate(ctx, doc)
	if !res.Valid() {
		return SourceFingerprintResult{}, fmt.Errorf("--source %s is not a valid IDIR document:\n%s", sourcePath, res.Error())
	}

	fp, err := fingerprint.Compute(doc)
	if err != nil {
		return SourceFingerprintResult{}, fmt.Errorf("compute fingerprint for --source %s: %w", sourcePath, err)
	}

	return SourceFingerprintResult{
		SourceFingerprint: fp,
		Match:             fp == linkedFingerprint,
	}, nil
}
