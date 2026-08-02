package scenario

import "testing"

func TestCheckDocumentSize_Boundary(t *testing.T) {
	if err := checkDocumentSize(MaxScenarioDocumentSize); err != nil {
		t.Fatalf("expected size exactly at the limit to pass, got: %v", err)
	}
	err := checkDocumentSize(MaxScenarioDocumentSize + 1)
	if err == nil {
		t.Fatal("expected an error one byte over the limit")
	}
	if !IsKind(err, ErrKindDocumentTooLarge) {
		t.Fatalf("expected ErrKindDocumentTooLarge, got: %v", err)
	}
}

func TestCheckWorkspaceFileCount_Boundary(t *testing.T) {
	if err := checkWorkspaceFileCount(MaxWorkspaceFiles); err != nil {
		t.Fatalf("expected count exactly at the limit to pass, got: %v", err)
	}
	err := checkWorkspaceFileCount(MaxWorkspaceFiles + 1)
	if err == nil {
		t.Fatal("expected an error one over the limit")
	}
	if !IsKind(err, ErrKindTooManyWorkspaceFiles) {
		t.Fatalf("expected ErrKindTooManyWorkspaceFiles, got: %v", err)
	}
}

func TestCheckWorkspaceFileSize_Boundary(t *testing.T) {
	if err := checkWorkspaceFileSize("fixture", MaxWorkspaceFileSize); err != nil {
		t.Fatalf("expected size exactly at the limit to pass, got: %v", err)
	}
	err := checkWorkspaceFileSize("fixture", MaxWorkspaceFileSize+1)
	if err == nil {
		t.Fatal("expected an error one byte over the limit")
	}
	if !IsKind(err, ErrKindWorkspaceFileTooLarge) {
		t.Fatalf("expected ErrKindWorkspaceFileTooLarge, got: %v", err)
	}
}

func TestCheckWorkspaceTotalBytes_Boundary(t *testing.T) {
	if err := checkWorkspaceTotalBytes(MaxWorkspaceTotalBytes); err != nil {
		t.Fatalf("expected total exactly at the limit to pass, got: %v", err)
	}
	err := checkWorkspaceTotalBytes(MaxWorkspaceTotalBytes + 1)
	if err == nil {
		t.Fatal("expected an error one byte over the limit")
	}
	if !IsKind(err, ErrKindWorkspaceTotalTooLarge) {
		t.Fatalf("expected ErrKindWorkspaceTotalTooLarge, got: %v", err)
	}
}

func TestCheckTimeoutBounds_Boundary(t *testing.T) {
	if err := checkTimeoutBounds(MinScenarioTimeoutSeconds); err != nil {
		t.Fatalf("expected the minimum to pass, got: %v", err)
	}
	if err := checkTimeoutBounds(MaxScenarioTimeoutSeconds); err != nil {
		t.Fatalf("expected the maximum to pass, got: %v", err)
	}
	if err := checkTimeoutBounds(MinScenarioTimeoutSeconds - 1); err == nil {
		t.Fatal("expected an error below the minimum")
	}
	if err := checkTimeoutBounds(0); err == nil {
		t.Fatal("expected an error for zero")
	}
	if err := checkTimeoutBounds(-1); err == nil {
		t.Fatal("expected an error for a negative timeout")
	}
	if err := checkTimeoutBounds(MaxScenarioTimeoutSeconds + 1); err == nil {
		t.Fatal("expected an error above the maximum")
	}
}

func TestEffectiveTimeoutSeconds(t *testing.T) {
	doc := &Document{}
	if got := EffectiveTimeoutSeconds(doc); got != DefaultScenarioTimeoutSeconds {
		t.Fatalf("expected default %d when omitted, got %d", DefaultScenarioTimeoutSeconds, got)
	}
	explicit := 99
	doc.Execution.TimeoutSeconds = &explicit
	if got := EffectiveTimeoutSeconds(doc); got != 99 {
		t.Fatalf("expected declared value 99, got %d", got)
	}
}
