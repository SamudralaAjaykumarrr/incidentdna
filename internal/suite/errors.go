package suite

import (
	"errors"
	"fmt"
)

// ErrorKind categorizes an *Error so a caller (in particular
// cmd/incidentdna) can map a failure to the right outcome/exit code without
// string matching, and so tests can assert on the specific failure category
// rather than a message substring. Mirrors internal/scenario.ErrorKind
// exactly.
type ErrorKind string

const (
	// ErrKindDocumentTooLarge means a suite manifest's on-disk size exceeds
	// MaxSuiteDocumentSize.
	ErrKindDocumentTooLarge ErrorKind = "document_too_large"
	// ErrKindTooManyScenarios means scenarios[] declares more than
	// MaxScenariosPerSuite entries.
	ErrKindTooManyScenarios ErrorKind = "too_many_scenarios"
	// ErrKindTotalTimeoutTooLarge means the sum of every listed scenario's
	// declared/default timeout_seconds exceeds MaxSuiteTotalTimeoutSeconds.
	ErrKindTotalTimeoutTooLarge ErrorKind = "total_timeout_too_large"
)

// Error is a structured suite-operation failure: a Kind for programmatic
// dispatch, a human-readable, actionable Msg, and an optional wrapped
// underlying error for diagnostics. Mirrors internal/scenario.Error exactly.
type Error struct {
	Kind ErrorKind
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Msg, e.Err)
	}
	return e.Msg
}

func (e *Error) Unwrap() error {
	return e.Err
}

// Is reports whether target is an *Error with the same Kind.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	return e.Kind == t.Kind
}

// IsKind reports whether err is (or wraps) an *Error of the given Kind.
func IsKind(err error, kind ErrorKind) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Kind == kind
}

// Sentinel errors for conditions that are not resource-limit failures.
var (
	// ErrSuiteScenarioMissing means a listed scenarios[].path does not exist,
	// is not a regular file, or is a symlink — checked at verify time
	// (relative to the suite manifest's own directory) and re-checked at run
	// time immediately before each scenario is loaded (docs/phase-5-plan.md
	// §17).
	ErrSuiteScenarioMissing = errors.New("suite: a listed scenario file is missing, not a regular file, or is a symlink")

	// ErrSuiteWorkspaceRootNotEmpty means a caller-supplied --workspace-root
	// directory already exists and is not empty. The runner never writes
	// into a directory it did not create empty (docs/phase-5-plan.md §9).
	ErrSuiteWorkspaceRootNotEmpty = errors.New("suite: --workspace-root directory already exists and is not empty")
)
