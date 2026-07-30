package evidence

import (
	"errors"
	"fmt"
	"io/fs"
)

// ErrorKind categorizes an *Error so a caller (in particular cmd/incidentdna)
// can map failures to the right exit code and message without string
// matching, and so tests can assert on the specific failure category rather
// than a message substring.
type ErrorKind string

// The full set of structured evidence-related error categories this package
// distinguishes, per docs/phase-2-plan.md's error-handling requirements.
const (
	ErrKindInvalidDigest        ErrorKind = "invalid_digest"
	ErrKindUnsupportedAlgorithm ErrorKind = "unsupported_algorithm"
	ErrKindSourceMissing        ErrorKind = "source_missing"
	ErrKindSourceIsDirectory    ErrorKind = "source_is_directory"
	ErrKindUnsafeSource         ErrorKind = "unsafe_source"
	ErrKindObjectTooLarge       ErrorKind = "object_too_large"
	ErrKindTooManyEntries       ErrorKind = "too_many_entries"
	ErrKindTotalBytesExceeded   ErrorKind = "total_bytes_exceeded"
	ErrKindObjectMissing        ErrorKind = "object_missing"
	ErrKindObjectCorrupted      ErrorKind = "object_corrupted"
	ErrKindPermission           ErrorKind = "permission_failure"
	ErrKindStoreRoot            ErrorKind = "store_root_failure"
	ErrKindTempFile             ErrorKind = "temp_file_failure"
	ErrKindReadFailure          ErrorKind = "read_failure"
	ErrKindWriteFailure         ErrorKind = "write_failure"
	ErrKindFinalize             ErrorKind = "finalize_failure"
	ErrKindCanceled             ErrorKind = "canceled"
)

// Error is a structured evidence-operation failure: a Kind for programmatic
// dispatch, a human-readable, actionable Msg that never includes evidence
// content, and an optional wrapped underlying error for diagnostics.
//
// Msg is deliberately built to avoid leaking sensitive material: it may
// name a digest (digests are not sensitive — they're already meant to be
// shared, e.g. pasted into a document) or a store-relative object path, but
// callers constructing an *Error must not put evidence file contents, or an
// unnecessary absolute source path, into Msg (see docs/evidence-storage.md,
// "Evidence sensitivity").
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

// Is reports whether target is an *Error with the same Kind, so callers can
// write errors.Is(err, &evidence.Error{Kind: evidence.ErrKindObjectMissing})
// or, more idiomatically, use IsKind below.
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

// wrapIOError classifies a raw filesystem error (from os.Stat, os.Open, ...)
// into the most specific ErrorKind that applies — permission failure or
// missing-object, falling back to a generic read failure — and wraps it with
// msg. Callers that need a different fallback Kind (e.g. ErrKindSourceMissing
// instead of ErrKindObjectMissing) classify inline instead of using this
// helper.
func wrapIOError(err error, msg string) *Error {
	kind := ErrKindReadFailure
	switch {
	case errors.Is(err, fs.ErrPermission):
		kind = ErrKindPermission
	case errors.Is(err, fs.ErrNotExist):
		kind = ErrKindObjectMissing
	}
	return &Error{Kind: kind, Msg: msg, Err: err}
}
