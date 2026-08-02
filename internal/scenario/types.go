// Package scenario implements Phase 4: IRS v0.1 (Incident Regression
// Scenario) documents and a local, bounded, offline runner that executes the
// one declared command they describe and classifies the result. See
// docs/phase-4-plan.md and docs/regression-scenarios.md for the full design.
//
// A scenario document is deliberately not an extension of idir.Document and
// is not decoded through internal/idir — it describes an *execution*, not an
// *incident*. internal/scenario imports internal/idir, internal/validate,
// and internal/fingerprint only for the optional --source fingerprint
// cross-check (fingerprint_link.go); it does not import internal/evidence or
// internal/library, and nothing in internal/ imports internal/scenario.
package scenario

// SupportedSchemaVersion is the only IRS schema version accepted in Phase 4.
// A document whose SchemaVersion does not equal this value is rejected by
// Validate before any other semantic rule runs, the same discipline
// idir.SupportedSchemaVersion already established for IDIR documents.
const SupportedSchemaVersion = "irs/v0.1"

// Document is the root of an IRS v0.1 scenario document: one bounded local
// command to execute, the fixed inputs it runs against, and the expected
// outcome to compare against. Field order here is documentation order.
type Document struct {
	SchemaVersion     string    `json:"schema_version" yaml:"schema_version"`
	Scenario          Info      `json:"scenario" yaml:"scenario"`
	LinkedFingerprint string    `json:"linked_fingerprint" yaml:"linked_fingerprint"`
	Execution         Execution `json:"execution" yaml:"execution"`
	Expected          Expected  `json:"expected" yaml:"expected"`
}

// Info identifies the scenario record itself for human/report readability.
// None of these fields participate in execution or the linked-fingerprint
// check.
type Info struct {
	ID          string `json:"id" yaml:"id"`
	Title       string `json:"title,omitempty" yaml:"title,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// WorkspaceFile declares one file to stage into the run's workspace before
// the command executes. Source is resolved relative to the scenario file's
// own directory; Destination is resolved relative to the workspace root.
// Both are rejected if absolute, containing "..", or resolving outside their
// respective root; a Source that is a symlink is rejected, not followed
// (docs/phase-4-plan.md §5).
type WorkspaceFile struct {
	Source      string `json:"source" yaml:"source"`
	Destination string `json:"destination" yaml:"destination"`
}

// StreamAssertion compares a captured output stream (stdout or stderr)
// against Value using Mode, one of "exact", "contains", or "regex".
type StreamAssertion struct {
	Mode  string `json:"mode" yaml:"mode"`
	Value string `json:"value" yaml:"value"`
}

// Stream assertion modes accepted for expected.stdout/expected.stderr.
const (
	StreamModeExact    = "exact"
	StreamModeContains = "contains"
	StreamModeRegex    = "regex"
)

// Execution describes the one bounded local command a scenario runs, and the
// fixed inputs it runs against.
type Execution struct {
	WorkspaceFiles []WorkspaceFile `json:"workspace_files,omitempty" yaml:"workspace_files,omitempty"`
	// Command is a required, non-empty argv list. Command[0] must be either
	// an absolute path or a path relative to the workspace root; a bare
	// command name (no path separator) is rejected at verify time, since
	// resolving it would require an implicit, host-dependent $PATH lookup
	// (docs/phase-4-plan.md §5/§11). The runner never wraps this argv in a
	// shell.
	Command []string `json:"command" yaml:"command"`
	// Env is an optional, fixed map of extra environment variables set for
	// the child process. The child's environment is not inherited from the
	// invoking incidentdna process — only a minimal fixed base (PATH unset,
	// HOME pointed at the workspace, TZ=UTC) plus Env reaches the child
	// (docs/phase-4-plan.md §5/§7).
	Env map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	// TimeoutSeconds is optional; nil means "not declared", which defaults
	// to DefaultScenarioTimeoutSeconds. A pointer distinguishes "not
	// declared" from an explicit 0, which is invalid (must be >=
	// MinScenarioTimeoutSeconds).
	TimeoutSeconds *int `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
}

// Expected declares the outcome scenario run compares the actual execution
// result against.
type Expected struct {
	// ExitCode is required. A pointer distinguishes "not declared" (nil,
	// invalid) from an explicitly declared 0 (a fully valid expectation).
	ExitCode *int             `json:"exit_code" yaml:"exit_code"`
	Stdout   *StreamAssertion `json:"stdout,omitempty" yaml:"stdout,omitempty"`
	Stderr   *StreamAssertion `json:"stderr,omitempty" yaml:"stderr,omitempty"`
}
