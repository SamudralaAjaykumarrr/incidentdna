// result.go consolidates the structured operation-result types introduced
// by Slices 2-3 (add.go's AddOutcome/AddResult, check.go's
// CheckOutcome/CheckResult, list.go's OccurrenceSummary/FingerprintSummary/
// ListResult) into a single file, per Phase 3 Slice 5
// (docs/phase-3-plan.md §6/§18). Their public names and semantics are
// unchanged from Slices 2-3 — only their physical location moves.
package library

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

// OccurrenceSummary is one occurrence's approved display metadata
// (docs/phase-3-plan.md §7/§20.7) — never the full stored document,
// business invariants, event timeline, evidence contents, or remediation
// text.
type OccurrenceSummary struct {
	IncidentID string
	Title      string
	// Service is application.service, falling back to application.name if
	// service is empty (docs/phase-3-plan.md §7: "Application/service
	// (application.service, possibly application.name)").
	Service    string
	OccurredAt string
}

// FingerprintSummary is one fingerprint (failure class) group's approved
// summary: the fingerprint itself, how many occurrences are stored under it,
// and each occurrence's bounded display metadata, in deterministic
// (occurrence-digest-ascending) order.
type FingerprintSummary struct {
	Fingerprint     string
	OccurrenceCount int
	Occurrences     []OccurrenceSummary
}

// ListResult is the outcome of a successful List call: every fingerprint
// group currently in the library, ordered deterministically by fingerprint
// hex string ascending (docs/phase-3-plan.md §13), regardless of filesystem
// enumeration order.
type ListResult struct {
	Fingerprints []FingerprintSummary
}
