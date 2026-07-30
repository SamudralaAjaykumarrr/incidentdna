// errors.go implements Phase 3 Slice 5's structured error model
// (docs/phase-3-plan.md §6/§11/§18), mirroring internal/evidence/errors.go's
// existing, already-reviewed *Error/ErrorKind design. It also consolidates
// the plain sentinel errors introduced by Slices 2-4 (add.go, list.go,
// privacy.go) into this single file, unchanged in name, value, and
// errors.Is/errors.As behavior — only their physical location moves.
package library

import (
	"errors"
	"fmt"
)

// ErrorKind categorizes an *Error so a caller (in particular the future
// cmd/incidentdna CLI, Slice 6) can map a resource-limit failure to the
// right exit code and message without string matching, and so tests can
// assert on the specific failure category rather than a message substring.
type ErrorKind string

// The structured resource-limit error categories this package distinguishes,
// per docs/phase-3-plan.md §11: each of the four approved limits gets its
// own distinct, actionable Kind rather than a single generic
// "resource limit exceeded" category.
const (
	// ErrKindDocumentTooLarge means a document's canonical byte
	// representation exceeds MaxDocumentSize (docs/phase-3-plan.md §11,
	// reusing internal/idir.MaxDocumentSize's value).
	ErrKindDocumentTooLarge ErrorKind = "document_too_large"

	// ErrKindTooManyLibraryEntries means the library already holds
	// MaxLibraryEntries distinct fingerprint groups and Add refused to
	// create another one.
	ErrKindTooManyLibraryEntries ErrorKind = "too_many_library_entries"

	// ErrKindTooManyOccurrences means a fingerprint group already holds
	// MaxOccurrencesPerFingerprint occurrences and Add refused to store a
	// genuinely new one (an idempotent re-add is never blocked by this).
	ErrKindTooManyOccurrences ErrorKind = "too_many_occurrences_per_fingerprint"

	// ErrKindTooManyListResults means the library holds more than
	// MaxListResults fingerprint entries and List refused to enumerate them
	// in a single invocation.
	ErrKindTooManyListResults ErrorKind = "too_many_list_results"
)

// Error is a structured library-operation failure: a Kind for programmatic
// dispatch, a human-readable, actionable Msg that never includes incident
// document content, and an optional wrapped underlying error for
// diagnostics. This mirrors internal/evidence.Error exactly.
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
// use errors.Is, or, more idiomatically, IsKind below.
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

// Sentinel errors identifying the distinct outcomes/failures introduced by
// Slices 2-4 (docs/phase-3-plan.md §5/§10/§12). These remain plain wrapped
// errors.New values, not *Error, so every existing errors.Is(err, ErrXxx)
// call site and test written against Slices 2-4 continues to work
// byte-for-byte identically — only their declaration moved here from
// add.go/list.go/privacy.go, consolidating this package's error
// declarations into one file the way internal/evidence already does
// (docs/phase-3-plan.md §6, "errors.go ... mirroring
// internal/evidence/errors.go").
var (
	// ErrInvalidDocument means doc failed validate.Validate.
	ErrInvalidDocument = errors.New("library: document failed validation")

	// ErrConflict means an occurrence with the same incident.id already
	// exists under this fingerprint with materially different content
	// (docs/phase-3-plan.md §5 case 3).
	ErrConflict = errors.New("library: an occurrence with this incident id already exists under this fingerprint with different content")

	// ErrCorruptedOccurrence means an existing occurrence that Add, Check,
	// or List needed to consult is missing, unreadable, or does not re-hash
	// to its own declared digest (docs/phase-3-plan.md §5 case 4). Never
	// reused, repaired, or overwritten.
	ErrCorruptedOccurrence = errors.New("library: an existing stored occurrence is corrupted")

	// ErrMalformedIndex means a fingerprint directory's index.json exists
	// but is not well-formed index data.
	ErrMalformedIndex = errors.New("library: index is malformed")

	// ErrMalformedLibrary means the library root's on-disk layout itself (a
	// shard or fingerprint directory's name, or its file type) does not
	// match the fixed content-addressed shape docs/phase-3-plan.md §5
	// defines. Distinct from ErrMalformedIndex, which is specifically about
	// one fingerprint directory's index.json content.
	ErrMalformedLibrary = errors.New("library: library layout is malformed")

	// ErrPrivacyNotRedacted means doc's privacy.redacted field is false (or
	// unset) and AddOptions.AllowUnredacted was not set (docs/phase-3-plan.md
	// §10/§12). The error carries no document content beyond the fact of the
	// missing declaration.
	ErrPrivacyNotRedacted = errors.New("library: document is not declared redacted (privacy.redacted != true); pass --allow-unredacted to store it anyway")
)
