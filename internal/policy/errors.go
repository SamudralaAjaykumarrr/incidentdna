package policy

import (
	"errors"
	"fmt"
)

// ErrorKind categorizes an *Error so a caller (in particular
// cmd/incidentdna) can map a failure to the right outcome/exit code without
// string matching, and so tests can assert on the specific failure category
// rather than a message substring. Mirrors internal/scenario.ErrorKind and
// internal/suite.ErrorKind exactly.
type ErrorKind string

const (
	// ErrKindDocumentTooLarge means a policy file's on-disk size exceeds
	// MaxPolicyDocumentSize.
	ErrKindDocumentTooLarge ErrorKind = "document_too_large"
	// ErrKindTooManyRules means rules[] declares more than
	// MaxRulesPerPolicy entries.
	ErrKindTooManyRules ErrorKind = "too_many_rules"
	// ErrKindReportTooLarge means an input report file's on-disk size
	// exceeds MaxReportDocumentSize.
	ErrKindReportTooLarge ErrorKind = "report_too_large"
	// ErrKindTooManyDistinctFingerprints means an input report names more
	// than MaxDistinctFingerprintsPerEvaluation distinct linked
	// fingerprints.
	ErrKindTooManyDistinctFingerprints ErrorKind = "too_many_distinct_fingerprints"
)

// Error is a structured policy-operation failure: a Kind for programmatic
// dispatch, a human-readable, actionable Msg, and an optional wrapped
// underlying error for diagnostics. Mirrors internal/scenario.Error and
// internal/suite.Error exactly.
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
