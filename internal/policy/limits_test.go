package policy

import "testing"

func TestCheckDocumentSize_Boundary(t *testing.T) {
	if err := checkDocumentSize(MaxPolicyDocumentSize); err != nil {
		t.Fatalf("expected size exactly at the limit to pass, got: %v", err)
	}
	err := checkDocumentSize(MaxPolicyDocumentSize + 1)
	if err == nil {
		t.Fatal("expected an error one byte over the limit")
	}
	if !IsKind(err, ErrKindDocumentTooLarge) {
		t.Fatalf("expected ErrKindDocumentTooLarge, got: %v", err)
	}
}

func TestCheckRuleCount_Boundary(t *testing.T) {
	if err := checkRuleCount(MaxRulesPerPolicy); err != nil {
		t.Fatalf("expected count exactly at the limit to pass, got: %v", err)
	}
	err := checkRuleCount(MaxRulesPerPolicy + 1)
	if err == nil {
		t.Fatal("expected an error one over the limit")
	}
	if !IsKind(err, ErrKindTooManyRules) {
		t.Fatalf("expected ErrKindTooManyRules, got: %v", err)
	}
}

func TestCheckReportDocumentSize_Boundary(t *testing.T) {
	if err := checkReportDocumentSize(MaxReportDocumentSize); err != nil {
		t.Fatalf("expected size exactly at the limit to pass, got: %v", err)
	}
	err := checkReportDocumentSize(MaxReportDocumentSize + 1)
	if err == nil {
		t.Fatal("expected an error one byte over the limit")
	}
	if !IsKind(err, ErrKindReportTooLarge) {
		t.Fatalf("expected ErrKindReportTooLarge, got: %v", err)
	}
}

func TestCheckDistinctFingerprintCount_Boundary(t *testing.T) {
	if err := checkDistinctFingerprintCount(MaxDistinctFingerprintsPerEvaluation); err != nil {
		t.Fatalf("expected count exactly at the limit to pass, got: %v", err)
	}
	err := checkDistinctFingerprintCount(MaxDistinctFingerprintsPerEvaluation + 1)
	if err == nil {
		t.Fatal("expected an error one over the limit")
	}
	if !IsKind(err, ErrKindTooManyDistinctFingerprints) {
		t.Fatalf("expected ErrKindTooManyDistinctFingerprints, got: %v", err)
	}
}
