package policy

import "testing"

func validDocument() *Document {
	return &Document{
		SchemaVersion: SupportedSchemaVersion,
		Policy:        Info{ID: "test-policy"},
		Rules: []Rule{
			{Type: RuleTypeRequireResult, Value: "PASS"},
			{Type: RuleTypeRequireLibraryOccurrence},
		},
	}
}

func TestValidate_AcceptsValidDocument(t *testing.T) {
	res := Validate(validDocument())
	if !res.Valid() {
		t.Fatalf("expected a valid document, got issues: %s", res.Error())
	}
}

func TestValidate_RejectsNilDocument(t *testing.T) {
	res := Validate(nil)
	if res.Valid() {
		t.Fatal("expected a nil document to be invalid")
	}
}

func TestValidate_RejectsWrongSchemaVersion(t *testing.T) {
	doc := validDocument()
	doc.SchemaVersion = "policy/v99"
	res := Validate(doc)
	if res.Valid() {
		t.Fatal("expected an invalid schema_version to be rejected")
	}
}

func TestValidate_RejectsMissingSchemaVersion(t *testing.T) {
	doc := validDocument()
	doc.SchemaVersion = ""
	res := Validate(doc)
	if res.Valid() {
		t.Fatal("expected an empty schema_version to be rejected")
	}
}

func TestValidate_RejectsEmptyRules(t *testing.T) {
	doc := validDocument()
	doc.Rules = nil
	res := Validate(doc)
	if res.Valid() {
		t.Fatal("expected an empty rules list to be rejected")
	}
}

func TestValidate_RejectsUnknownRuleType(t *testing.T) {
	doc := validDocument()
	doc.Rules = []Rule{{Type: "require_something_else"}}
	res := Validate(doc)
	if res.Valid() {
		t.Fatal("expected an unknown rule type to be rejected")
	}
	if len(res.Rules) != 1 || res.Rules[0].Valid {
		t.Fatalf("expected the single rule to be marked invalid, got: %+v", res.Rules)
	}
}

func TestValidate_RejectsMissingValueOnRequireResult(t *testing.T) {
	doc := validDocument()
	doc.Rules = []Rule{{Type: RuleTypeRequireResult}}
	res := Validate(doc)
	if res.Valid() {
		t.Fatal("expected require_result without a value to be rejected")
	}
}

func TestValidate_RejectsWhitespaceOnlyValueOnRequireResult(t *testing.T) {
	doc := validDocument()
	doc.Rules = []Rule{{Type: RuleTypeRequireResult, Value: "   "}}
	res := Validate(doc)
	if res.Valid() {
		t.Fatal("expected require_result with a whitespace-only value to be rejected")
	}
}

func TestValidate_RejectsValueOnRequireLibraryOccurrence(t *testing.T) {
	doc := validDocument()
	doc.Rules = []Rule{{Type: RuleTypeRequireLibraryOccurrence, Value: "PASS"}}
	res := Validate(doc)
	if res.Valid() {
		t.Fatal("expected require_library_occurrence with a declared value to be rejected")
	}
}

func TestValidate_RejectsTooManyRules(t *testing.T) {
	doc := validDocument()
	doc.Rules = nil
	for i := 0; i < MaxRulesPerPolicy+1; i++ {
		doc.Rules = append(doc.Rules, Rule{Type: RuleTypeRequireLibraryOccurrence})
	}
	res := Validate(doc)
	if res.Valid() {
		t.Fatal("expected a rule count over MaxRulesPerPolicy to be rejected")
	}
}

func TestValidate_AcceptsExactlyMaxRules(t *testing.T) {
	doc := validDocument()
	doc.Rules = nil
	for i := 0; i < MaxRulesPerPolicy; i++ {
		doc.Rules = append(doc.Rules, Rule{Type: RuleTypeRequireLibraryOccurrence})
	}
	res := Validate(doc)
	if !res.Valid() {
		t.Fatalf("expected exactly MaxRulesPerPolicy rules to pass, got issues: %s", res.Error())
	}
}

func TestValidate_PerRuleValidationIsIndependent(t *testing.T) {
	doc := validDocument()
	doc.Rules = []Rule{
		{Type: RuleTypeRequireResult, Value: "PASS"},
		{Type: "bogus"},
	}
	res := Validate(doc)
	if res.Valid() {
		t.Fatal("expected the document to be invalid due to the second rule")
	}
	if len(res.Rules) != 2 {
		t.Fatalf("expected 2 per-rule results, got %d", len(res.Rules))
	}
	if !res.Rules[0].Valid {
		t.Fatalf("expected the first rule to remain individually valid, got: %+v", res.Rules[0])
	}
	if res.Rules[1].Valid {
		t.Fatalf("expected the second rule to be individually invalid, got: %+v", res.Rules[1])
	}
}
