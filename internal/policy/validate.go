// validate.go implements every IGP v0.1 semantic rule from
// docs/phase-7-plan.md §5/§13: schema_version, non-empty rules, known rule
// type, required/forbidden value per rule type, and MaxRulesPerPolicy.
// Validate never evaluates anything against a report — it is the
// structural/semantic gate both policy verify and the pre-evaluation phase
// of policy evaluate call (docs/phase-7-plan.md §10).
//
// Validate's result depends only on the policy document's own decoded
// fields — never on any report file, library state, or wall-clock time
// (docs/phase-7-plan.md §7 point 1).
package policy

import (
	"fmt"
	"strings"
)

// Issue is one semantic validation failure: the document field it applies
// to and an actionable, human-readable message. Mirrors
// internal/scenario.Issue and internal/suite.Issue.
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

// RuleValidation is one declared rule's own validation outcome, in the same
// order as Document.Rules — used by the CLI to print a per-rule [OK]/[FAIL]
// line the same way suite verify prints one per listed scenario.
type RuleValidation struct {
	Type   string
	Value  string
	Valid  bool
	Issues []Issue
}

// Result collects every Issue found while validating a policy document,
// both document-level (schema_version, empty rules, rule count) and
// per-rule. A document with no issues anywhere is valid.
type Result struct {
	Issues []Issue
	Rules  []RuleValidation
}

// Valid reports whether the document had zero validation issues anywhere,
// document-level or per-rule.
func (r Result) Valid() bool {
	if len(r.Issues) > 0 {
		return false
	}
	for _, rule := range r.Rules {
		if !rule.Valid {
			return false
		}
	}
	return true
}

// Error implements the error interface so a Result can be returned directly
// where an error is expected.
func (r Result) Error() string {
	var lines []string
	for _, i := range r.Issues {
		lines = append(lines, i.String())
	}
	for _, rule := range r.Rules {
		for _, i := range rule.Issues {
			lines = append(lines, i.String())
		}
	}
	return strings.Join(lines, "\n")
}

func (r *Result) add(field, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{Field: field, Message: fmt.Sprintf(format, args...)})
}

// Validate runs every IGP v0.1 semantic rule against doc.
func Validate(doc *Document) Result {
	var res Result
	if doc == nil {
		res.add("", "policy document is nil")
		return res
	}

	if doc.SchemaVersion != SupportedSchemaVersion {
		res.add("schema_version", "must be %q, got %q", SupportedSchemaVersion, doc.SchemaVersion)
	}

	if len(doc.Rules) == 0 {
		res.add("rules", "must not be empty")
	}
	if err := checkRuleCount(len(doc.Rules)); err != nil {
		res.add("rules", "%v", err)
	}

	for i, rule := range doc.Rules {
		res.Rules = append(res.Rules, validateRule(i, rule))
	}

	return res
}

func validateRule(index int, rule Rule) RuleValidation {
	rv := RuleValidation{Type: rule.Type, Value: rule.Value, Valid: true}

	switch rule.Type {
	case RuleTypeRequireResult:
		if strings.TrimSpace(rule.Value) == "" {
			rv.Valid = false
			rv.Issues = append(rv.Issues, Issue{
				Field:   fmt.Sprintf("rules[%d].value", index),
				Message: fmt.Sprintf("must be declared and non-empty for %s", RuleTypeRequireResult),
			})
		}
	case RuleTypeRequireLibraryOccurrence:
		if rule.Value != "" {
			rv.Valid = false
			rv.Issues = append(rv.Issues, Issue{
				Field:   fmt.Sprintf("rules[%d].value", index),
				Message: fmt.Sprintf("must not be declared for %s, which takes no parameters", RuleTypeRequireLibraryOccurrence),
			})
		}
	default:
		rv.Valid = false
		rv.Issues = append(rv.Issues, Issue{
			Field:   fmt.Sprintf("rules[%d].type", index),
			Message: fmt.Sprintf("must be one of %q, %q, got %q", RuleTypeRequireResult, RuleTypeRequireLibraryOccurrence, rule.Type),
		})
	}

	return rv
}
