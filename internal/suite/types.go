// Package suite implements Phase 5: ISM v0.1 (Incident Scenario Manifest)
// documents and a local, sequential runner that executes every scenario a
// manifest lists (via internal/scenario.Run, unchanged) and aggregates the
// results. See docs/phase-5-plan.md and docs/scenario-suites.md for the full
// design.
//
// A suite manifest is deliberately not an extension of scenario.Document and
// is not decoded through internal/scenario at all — it describes an ordered
// *list* of scenarios, not an execution. internal/suite imports
// internal/scenario (for LoadFile, Validate, and Run, all unchanged); it does
// not import internal/idir, internal/validate, internal/fingerprint,
// internal/evidence, or internal/library directly, and nothing in internal/
// imports internal/suite.
package suite

// SupportedSchemaVersion is the only ISM schema version accepted in Phase 5.
// A document whose SchemaVersion does not equal this value is rejected by
// Validate before any other semantic rule runs, the same discipline
// scenario.SupportedSchemaVersion already established for IRS documents.
const SupportedSchemaVersion = "suite/v0.1"

// Document is the root of an ISM v0.1 suite manifest: an explicit, ordered
// list of scenario files to run. Field order here is documentation order.
type Document struct {
	SchemaVersion string          `json:"schema_version" yaml:"schema_version"`
	Suite         Info            `json:"suite" yaml:"suite"`
	Scenarios     []ScenarioEntry `json:"scenarios" yaml:"scenarios"`
}

// Info identifies the suite record itself for human/report readability. None
// of these fields participate in execution, and Suite.ID is never used as a
// filesystem path component — there is no store, so nothing to key by it.
type Info struct {
	ID          string `json:"id" yaml:"id"`
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// ScenarioEntry declares one scenario file a suite runs, in declared order.
// Path is resolved relative to the suite manifest's own directory — never a
// directory walk or glob, and never absolute, containing "..", or resolving
// outside that directory; a Path that is a symlink is rejected, not followed
// (docs/phase-5-plan.md §5).
type ScenarioEntry struct {
	Path string `json:"path" yaml:"path"`
}
