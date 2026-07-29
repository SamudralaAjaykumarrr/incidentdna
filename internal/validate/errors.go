package validate

import (
	"fmt"
	"strings"
)

// Issue is one semantic validation failure: the document field it applies to
// and an actionable, human-readable message.
type Issue struct {
	Field   string
	Message string
}

func (i Issue) String() string {
	if i.Field == "" {
		return i.Message
	}
	return i.Field + ": " + i.Message
}

// Result collects every Issue found while validating a document. A document
// with no issues is valid. Result is never nil from Validate; check
// Result.Valid() or len(Result.Issues).
type Result struct {
	Issues []Issue
}

// Valid reports whether the document had zero validation issues.
func (r Result) Valid() bool {
	return len(r.Issues) == 0
}

// Error implements the error interface so a Result can be returned directly
// where an error is expected (e.g. from a CLI command). Callers that want
// the structured Issue list should use Result.Issues directly instead of
// parsing this string.
func (r Result) Error() string {
	lines := make([]string, len(r.Issues))
	for i, issue := range r.Issues {
		lines[i] = issue.String()
	}
	return strings.Join(lines, "\n")
}

func (r *Result) add(field, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{Field: field, Message: fmt.Sprintf(format, args...)})
}
