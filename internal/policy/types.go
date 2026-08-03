// Package policy implements Phase 7: IGP v0.1 (Incident Gate Policy)
// documents and a local, deterministic evaluator that checks an
// already-produced scenario or suite report (Phase 4/5, unchanged) against a
// declared, reviewable policy. See docs/phase-7-plan.md and
// docs/policy-evaluation.md for the full design.
//
// A policy document is deliberately not an extension of scenario.Document or
// suite.Document and is not decoded through either package — it describes
// acceptance criteria over an already-produced report artifact, not an
// execution or a manifest. internal/policy imports internal/scenario and
// internal/suite only for their existing, unchanged Report struct
// definitions (to decode a report file into a typed value instead of
// map[string]interface{}); it does not call LoadFile, Validate, or Run from
// either package, and neither package is modified to accommodate this.
// internal/policy never imports internal/library and does not know the
// incident library exists — the require_library_occurrence rule is
// evaluated against a caller-supplied lookup result (LibraryLookups); the
// composition that opens the library and calls library.CheckFingerprint
// lives entirely in cmd/incidentdna.
package policy

// SupportedSchemaVersion is the only IGP schema version accepted in Phase 7.
// A document whose SchemaVersion does not equal this value is rejected by
// Validate before any other semantic rule runs, the same discipline
// scenario.SupportedSchemaVersion/suite.SupportedSchemaVersion already
// established.
const SupportedSchemaVersion = "policy/v0.1"

// Document is the root of an IGP v0.1 policy document: an ordered list of
// rules to evaluate against exactly one already-produced report. Field order
// here is documentation order.
type Document struct {
	SchemaVersion string `json:"schema_version" yaml:"schema_version"`
	Policy        Info   `json:"policy" yaml:"policy"`
	Rules         []Rule `json:"rules" yaml:"rules"`
}

// Info identifies the policy record itself for human/report readability.
// Policy.ID is never used as a filesystem path component — there is no
// store, so nothing to key by it.
type Info struct {
	ID          string `json:"id" yaml:"id"`
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// Rule is one declared IGP v0.1 rule. Value is required for
// RuleTypeRequireResult (a non-empty string) and forbidden for
// RuleTypeRequireLibraryOccurrence, which takes no parameters in v0.1
// (docs/phase-7-plan.md §5).
type Rule struct {
	Type  string `json:"type" yaml:"type"`
	Value string `json:"value,omitempty" yaml:"value,omitempty"`
}

// The two rule types IGP v0.1 supports (docs/phase-7-plan.md §2 goal 4).
// rules[].type must be exactly one of these; any other value is rejected at
// verify time.
const (
	// RuleTypeRequireResult requires the input report's own top-level result
	// field to equal Rule.Value.
	RuleTypeRequireResult = "require_result"
	// RuleTypeRequireLibraryOccurrence requires every distinct
	// linked_fingerprint named anywhere in the input report to have at least
	// one recorded incident library occurrence.
	RuleTypeRequireLibraryOccurrence = "require_library_occurrence"
)
