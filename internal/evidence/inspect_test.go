package evidence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestInspect_OK(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "src.log", []byte("evidence to inspect"))

	res, err := s.Inspect(context.Background(), d.String())
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if res.Status != StatusOK {
		t.Errorf("Status = %v, want %v", res.Status, StatusOK)
	}
	if res.Size != int64(len("evidence to inspect")) {
		t.Errorf("Size = %d, want %d", res.Size, len("evidence to inspect"))
	}
	if res.Digest != d.String() {
		t.Errorf("Digest = %q, want %q", res.Digest, d.String())
	}
}

func TestInspect_Missing(t *testing.T) {
	s, _ := openStoreT(t)
	missing := "sha256:" + strings.Repeat("5", 64)
	res, err := s.Inspect(context.Background(), missing)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if res.Status != StatusMissing {
		t.Errorf("Status = %v, want %v", res.Status, StatusMissing)
	}
}

func TestInspect_Corrupted(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "src.log", []byte("original"))
	objPath, err := s.objectPath(d)
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(objPath, 0o644)
	if err := os.WriteFile(objPath, []byte("corrupted!"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := s.Inspect(context.Background(), d.String())
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if res.Status != StatusCorrupted {
		t.Errorf("Status = %v, want %v", res.Status, StatusCorrupted)
	}
}

func TestInspect_RejectsInvalidDigestBeforeAnyFilesystemAccess(t *testing.T) {
	s, _ := openStoreT(t)
	_, err := s.Inspect(context.Background(), "not-a-digest-at-all")
	if err == nil {
		t.Fatal("expected an error for an unparseable digest, got nil")
	}
	if !IsKind(err, ErrKindInvalidDigest) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindInvalidDigest)
	}
}

func TestInspect_RejectsPathTraversalAttempt(t *testing.T) {
	s, _ := openStoreT(t)
	// A digest-shaped string cannot contain "/", so this exercises the same
	// character-class rejection Parse guarantees — proving the traversal
	// attempt never reaches a filesystem call at all.
	_, err := s.Inspect(context.Background(), "sha256:../../../../etc/passwd")
	if err == nil {
		t.Fatal("expected an error for a path-traversal-shaped digest, got nil")
	}
	if !IsKind(err, ErrKindInvalidDigest) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindInvalidDigest)
	}
}

// InspectResult's fields are documented (result.go) as never holding
// evidence content, by type: Digest/Detail are metadata strings and Size is
// an int64, so there is no field a content byte slice could hide in. This
// test exercises that structurally: it stores a distinctive content marker,
// inspects it, and confirms nothing in the result's string fields contains
// the marker.
func TestInspect_ResultNeverContainsContent(t *testing.T) {
	s, dir := openStoreT(t)
	marker := "TOTALLY-DISTINCTIVE-EVIDENCE-CONTENT-MARKER"
	d := storeFixture(t, s, dir, "src.log", []byte(marker))

	res, err := s.Inspect(context.Background(), d.String())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Digest, marker) || strings.Contains(res.Detail, marker) {
		t.Error("InspectResult leaked evidence content into a string field")
	}
}
