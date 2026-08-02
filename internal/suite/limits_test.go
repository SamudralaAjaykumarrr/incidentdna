package suite

import "testing"

func TestCheckDocumentSize_Boundary(t *testing.T) {
	if err := checkDocumentSize(MaxSuiteDocumentSize); err != nil {
		t.Fatalf("expected size exactly at the limit to pass, got: %v", err)
	}
	err := checkDocumentSize(MaxSuiteDocumentSize + 1)
	if err == nil {
		t.Fatal("expected an error one byte over the limit")
	}
	if !IsKind(err, ErrKindDocumentTooLarge) {
		t.Fatalf("expected ErrKindDocumentTooLarge, got: %v", err)
	}
}

func TestCheckScenarioCount_Boundary(t *testing.T) {
	if err := checkScenarioCount(MaxScenariosPerSuite); err != nil {
		t.Fatalf("expected count exactly at the limit to pass, got: %v", err)
	}
	err := checkScenarioCount(MaxScenariosPerSuite + 1)
	if err == nil {
		t.Fatal("expected an error one over the limit")
	}
	if !IsKind(err, ErrKindTooManyScenarios) {
		t.Fatalf("expected ErrKindTooManyScenarios, got: %v", err)
	}
}

func TestCheckTotalTimeoutSeconds_Boundary(t *testing.T) {
	if err := checkTotalTimeoutSeconds(MaxSuiteTotalTimeoutSeconds); err != nil {
		t.Fatalf("expected total exactly at the limit to pass, got: %v", err)
	}
	err := checkTotalTimeoutSeconds(MaxSuiteTotalTimeoutSeconds + 1)
	if err == nil {
		t.Fatal("expected an error one second over the limit")
	}
	if !IsKind(err, ErrKindTotalTimeoutTooLarge) {
		t.Fatalf("expected ErrKindTotalTimeoutTooLarge, got: %v", err)
	}
}
