// evaluate.go implements the core IGP v0.1 rule evaluator
// (docs/phase-7-plan.md §5/§7): a pure function of (Document, LoadedReport,
// LibraryLookups) with no internal ordering dependency beyond "declared
// order, every rule evaluated, never short-circuited" (docs/phase-7-plan.md
// §5, "rules"). Evaluate never opens a library store itself — the
// require_library_occurrence rule is evaluated against a caller-supplied
// LibraryLookups value, keeping this package unaware the incident library
// exists (docs/phase-7-plan.md §2 goal 6).
package policy

import "fmt"

// RuleStatus is one evaluated rule's own outcome.
type RuleStatus string

const (
	// RuleStatusOK means the rule was evaluated and satisfied.
	RuleStatusOK RuleStatus = "OK"
	// RuleStatusFail means the rule was evaluated and not satisfied.
	RuleStatusFail RuleStatus = "FAIL"
	// RuleStatusSkip means the rule could not be evaluated because a needed
	// input was not supplied (require_library_occurrence without
	// --library). A SKIPped rule counts as not satisfied toward the overall
	// verdict — never silently ignored, never treated as satisfied
	// (docs/phase-7-plan.md §4 Workflow C).
	RuleStatusSkip RuleStatus = "SKIP"
)

// RuleOutcome is one declared rule's evaluated result, in the same order as
// Document.Rules.
type RuleOutcome struct {
	Type   string
	Value  string
	Status RuleStatus
	Detail string
}

// The two verdict values Evaluate's overall Verdict.Result can hold.
const (
	VerdictPass = "PASS"
	VerdictFail = "FAIL"
)

// Verdict is the outcome of a complete Evaluate call: the overall
// PASS/FAIL result and every declared rule's own individual outcome, in
// declared order.
type Verdict struct {
	Result string
	Rules  []RuleOutcome
}

// LibraryLookups maps a distinct linked_fingerprint to whether the incident
// library holds one or more occurrences under it — the caller-supplied
// result of one library.CheckFingerprint call per distinct fingerprint
// (docs/phase-7-plan.md §2 goal 6). A nil map means --library was not
// given: every require_library_occurrence rule is reported SKIP. A non-nil
// (possibly empty) map means --library was given and every distinct
// fingerprint named in the report was looked up; a fingerprint absent from
// the map (or present with a false value) means the library holds no
// occurrence under it.
type LibraryLookups map[string]bool

// Evaluate runs every doc.Rules entry against report in declared order,
// never short-circuiting (docs/phase-7-plan.md §5, "rules"), and returns
// the overall verdict. doc must have already passed Validate; Evaluate
// returns an error only if it encounters a rule type Validate should
// already have rejected — defensive, not reachable through the CLI's own
// verify-then-evaluate flow.
func Evaluate(doc *Document, report LoadedReport, lookups LibraryLookups) (Verdict, error) {
	v := Verdict{Result: VerdictPass}

	for _, rule := range doc.Rules {
		var ro RuleOutcome
		switch rule.Type {
		case RuleTypeRequireResult:
			ro = evaluateRequireResult(rule, report)
		case RuleTypeRequireLibraryOccurrence:
			ro = evaluateRequireLibraryOccurrence(report, lookups)
		default:
			return Verdict{}, fmt.Errorf("policy: unknown rule type %q", rule.Type)
		}

		if ro.Status != RuleStatusOK {
			v.Result = VerdictFail
		}
		v.Rules = append(v.Rules, ro)
	}

	return v, nil
}

// evaluateRequireResult is a pure string comparison against an
// already-deterministic field a prior, already-verified
// scenario run --report/suite run --report invocation produced
// (docs/phase-7-plan.md §7 point 2) — no new hashing, canonicalization, or
// comparison logic beyond ==.
func evaluateRequireResult(rule Rule, report LoadedReport) RuleOutcome {
	if report.Result == rule.Value {
		return RuleOutcome{
			Type: rule.Type, Value: rule.Value, Status: RuleStatusOK,
			Detail: fmt.Sprintf("report result was %s", report.Result),
		}
	}
	return RuleOutcome{
		Type: rule.Type, Value: rule.Value, Status: RuleStatusFail,
		Detail: fmt.Sprintf("report result was %s, expected %s", report.Result, rule.Value),
	}
}

// evaluateRequireLibraryOccurrence requires every distinct fingerprint named
// in report to have at least one recorded library occurrence. Zero distinct
// fingerprints (an empty report) is vacuously satisfied — there is nothing
// to require an occurrence for.
func evaluateRequireLibraryOccurrence(report LoadedReport, lookups LibraryLookups) RuleOutcome {
	if lookups == nil {
		return RuleOutcome{
			Type: RuleTypeRequireLibraryOccurrence, Status: RuleStatusSkip,
			Detail: "not evaluated (--library not given)",
		}
	}

	total := len(report.DistinctFingerprints)
	matched := 0
	for _, f := range report.DistinctFingerprints {
		if lookups[f] {
			matched++
		}
	}

	detail := fmt.Sprintf("%d of %d distinct fingerprint(s) have library occurrences", matched, total)
	if matched == total {
		return RuleOutcome{Type: RuleTypeRequireLibraryOccurrence, Status: RuleStatusOK, Detail: detail}
	}
	return RuleOutcome{Type: RuleTypeRequireLibraryOccurrence, Status: RuleStatusFail, Detail: detail}
}
