package policy

import "testing"

func TestEvaluate_RequireResultMatch(t *testing.T) {
	doc := &Document{Rules: []Rule{{Type: RuleTypeRequireResult, Value: "PASS"}}}
	report := LoadedReport{Result: "PASS"}

	v, err := Evaluate(doc, report, nil)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Result != VerdictPass {
		t.Fatalf("expected PASS, got %s", v.Result)
	}
	if len(v.Rules) != 1 || v.Rules[0].Status != RuleStatusOK {
		t.Fatalf("expected one OK rule, got: %+v", v.Rules)
	}
}

func TestEvaluate_RequireResultMismatch(t *testing.T) {
	doc := &Document{Rules: []Rule{{Type: RuleTypeRequireResult, Value: "PASS"}}}
	report := LoadedReport{Result: "FAIL"}

	v, err := Evaluate(doc, report, nil)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Result != VerdictFail {
		t.Fatalf("expected FAIL, got %s", v.Result)
	}
	if len(v.Rules) != 1 || v.Rules[0].Status != RuleStatusFail {
		t.Fatalf("expected one FAIL rule, got: %+v", v.Rules)
	}
}

func TestEvaluate_RequireResultForBothReportKinds(t *testing.T) {
	for _, kind := range []ReportKind{ReportKindScenario, ReportKindSuite} {
		doc := &Document{Rules: []Rule{{Type: RuleTypeRequireResult, Value: "PASS"}}}
		report := LoadedReport{Kind: kind, Result: "PASS"}
		v, err := Evaluate(doc, report, nil)
		if err != nil {
			t.Fatalf("Evaluate(%s): %v", kind, err)
		}
		if v.Result != VerdictPass {
			t.Fatalf("Evaluate(%s): expected PASS, got %s", kind, v.Result)
		}
	}
}

func TestEvaluate_RequireLibraryOccurrenceAllMatch(t *testing.T) {
	doc := &Document{Rules: []Rule{{Type: RuleTypeRequireLibraryOccurrence}}}
	report := LoadedReport{DistinctFingerprints: []string{"fp-a", "fp-b"}}
	lookups := LibraryLookups{"fp-a": true, "fp-b": true}

	v, err := Evaluate(doc, report, lookups)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Result != VerdictPass {
		t.Fatalf("expected PASS, got %s", v.Result)
	}
	if v.Rules[0].Status != RuleStatusOK {
		t.Fatalf("expected OK, got: %+v", v.Rules[0])
	}
	if v.Rules[0].Detail != "2 of 2 distinct fingerprint(s) have library occurrences" {
		t.Fatalf("unexpected detail: %q", v.Rules[0].Detail)
	}
}

func TestEvaluate_RequireLibraryOccurrencePartialMatch(t *testing.T) {
	doc := &Document{Rules: []Rule{{Type: RuleTypeRequireLibraryOccurrence}}}
	report := LoadedReport{DistinctFingerprints: []string{"fp-a", "fp-b"}}
	lookups := LibraryLookups{"fp-a": true, "fp-b": false}

	v, err := Evaluate(doc, report, lookups)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Result != VerdictFail {
		t.Fatalf("expected FAIL, got %s", v.Result)
	}
	if v.Rules[0].Status != RuleStatusFail {
		t.Fatalf("expected FAIL rule status, got: %+v", v.Rules[0])
	}
	if v.Rules[0].Detail != "1 of 2 distinct fingerprint(s) have library occurrences" {
		t.Fatalf("unexpected detail: %q", v.Rules[0].Detail)
	}
}

func TestEvaluate_RequireLibraryOccurrenceNoMatch(t *testing.T) {
	doc := &Document{Rules: []Rule{{Type: RuleTypeRequireLibraryOccurrence}}}
	report := LoadedReport{DistinctFingerprints: []string{"fp-a"}}
	lookups := LibraryLookups{} // --library given, nothing found

	v, err := Evaluate(doc, report, lookups)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Result != VerdictFail {
		t.Fatalf("expected FAIL, got %s", v.Result)
	}
	if v.Rules[0].Detail != "0 of 1 distinct fingerprint(s) have library occurrences" {
		t.Fatalf("unexpected detail: %q", v.Rules[0].Detail)
	}
}

func TestEvaluate_RequireLibraryOccurrenceSkipWhenLookupsNil(t *testing.T) {
	doc := &Document{Rules: []Rule{{Type: RuleTypeRequireLibraryOccurrence}}}
	report := LoadedReport{DistinctFingerprints: []string{"fp-a"}}

	v, err := Evaluate(doc, report, nil)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Result != VerdictFail {
		t.Fatalf("expected FAIL (a SKIPped rule counts as not satisfied), got %s", v.Result)
	}
	if v.Rules[0].Status != RuleStatusSkip {
		t.Fatalf("expected SKIP, got: %+v", v.Rules[0])
	}
}

func TestEvaluate_RequireLibraryOccurrenceVacuouslyOKWithNoFingerprints(t *testing.T) {
	doc := &Document{Rules: []Rule{{Type: RuleTypeRequireLibraryOccurrence}}}
	report := LoadedReport{}
	lookups := LibraryLookups{}

	v, err := Evaluate(doc, report, lookups)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Rules[0].Status != RuleStatusOK {
		t.Fatalf("expected OK for zero distinct fingerprints (vacuous), got: %+v", v.Rules[0])
	}
}

func TestEvaluate_OverallVerdictPassOnlyWhenEveryRuleOK(t *testing.T) {
	doc := &Document{Rules: []Rule{
		{Type: RuleTypeRequireResult, Value: "PASS"},
		{Type: RuleTypeRequireLibraryOccurrence},
	}}
	report := LoadedReport{Result: "PASS", DistinctFingerprints: []string{"fp-a"}}
	lookups := LibraryLookups{"fp-a": true}

	v, err := Evaluate(doc, report, lookups)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Result != VerdictPass {
		t.Fatalf("expected PASS, got %s", v.Result)
	}
}

func TestEvaluate_EveryRuleEvaluatedNeverShortCircuited(t *testing.T) {
	doc := &Document{Rules: []Rule{
		{Type: RuleTypeRequireResult, Value: "PASS"}, // will FAIL
		{Type: RuleTypeRequireLibraryOccurrence},     // will also FAIL
	}}
	report := LoadedReport{Result: "FAIL", DistinctFingerprints: []string{"fp-a"}}
	lookups := LibraryLookups{}

	v, err := Evaluate(doc, report, lookups)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(v.Rules) != 2 {
		t.Fatalf("expected both rules to be evaluated and reported, got %d", len(v.Rules))
	}
	if v.Rules[0].Status != RuleStatusFail || v.Rules[1].Status != RuleStatusFail {
		t.Fatalf("expected both rules FAIL, got: %+v", v.Rules)
	}
}

func TestEvaluate_NilLookupsSafeWhenNoLibraryRule(t *testing.T) {
	doc := &Document{Rules: []Rule{{Type: RuleTypeRequireResult, Value: "PASS"}}}
	report := LoadedReport{Result: "PASS"}

	// A policy with zero library-dependent rules never requires a
	// libraryLookups argument at all.
	v, err := Evaluate(doc, report, nil)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if v.Result != VerdictPass {
		t.Fatalf("expected PASS, got %s", v.Result)
	}
}

func TestEvaluate_UnknownRuleTypeReturnsError(t *testing.T) {
	doc := &Document{Rules: []Rule{{Type: "bogus"}}}
	_, err := Evaluate(doc, LoadedReport{}, nil)
	if err == nil {
		t.Fatal("expected an error for an unknown rule type")
	}
}
