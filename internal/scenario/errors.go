package scenario

import (
	"errors"
	"fmt"
)

// ErrorKind categorizes an *Error so a caller (in particular
// cmd/incidentdna) can map a failure to the right outcome/exit code without
// string matching, and so tests can assert on the specific failure category
// rather than a message substring. Mirrors internal/library.ErrorKind and
// internal/evidence.ErrorKind exactly.
type ErrorKind string

const (
	// ErrKindDocumentTooLarge means a scenario file's on-disk size exceeds
	// MaxScenarioDocumentSize.
	ErrKindDocumentTooLarge ErrorKind = "document_too_large"
	// ErrKindTooManyWorkspaceFiles means execution.workspace_files declares
	// more than MaxWorkspaceFiles entries.
	ErrKindTooManyWorkspaceFiles ErrorKind = "too_many_workspace_files"
	// ErrKindWorkspaceFileTooLarge means one declared workspace_files source
	// file exceeds MaxWorkspaceFileSize.
	ErrKindWorkspaceFileTooLarge ErrorKind = "workspace_file_too_large"
	// ErrKindWorkspaceTotalTooLarge means the sum of all declared
	// workspace_files source file sizes exceeds MaxWorkspaceTotalBytes.
	ErrKindWorkspaceTotalTooLarge ErrorKind = "workspace_total_too_large"
)

// Error is a structured scenario-operation failure: a Kind for programmatic
// dispatch, a human-readable, actionable Msg, and an optional wrapped
// underlying error for diagnostics. Mirrors internal/library.Error and
// internal/evidence.Error exactly.
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
	// ErrWorkspaceFileMissing means a declared workspace_files source file
	// does not exist, is not a regular file, or is a symlink — checked at
	// verify time (relative to the scenario file's own directory) and
	// re-checked at run time immediately before staging
	// (docs/phase-4-plan.md §17).
	ErrWorkspaceFileMissing = errors.New("scenario: a declared workspace_files source file is missing, not a regular file, or is a symlink")

	// ErrWorkspaceNotEmpty means a caller-supplied --workspace directory
	// already exists and is not empty. The runner never writes into a
	// directory it did not create empty (docs/phase-4-plan.md §17).
	ErrWorkspaceNotEmpty = errors.New("scenario: --workspace directory already exists and is not empty")

	// ErrCommandNotExecutable means execution.command[0], resolved to an
	// absolute path (either declared absolute, or joined against the
	// workspace root), does not exist, is a directory, or is not executable
	// (docs/phase-4-plan.md §17).
	ErrCommandNotExecutable = errors.New("scenario: execution.command[0] does not exist or is not executable")
)
