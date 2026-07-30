package evidence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// docWithEvidence builds a minimal idir.Document carrying only the evidence
// entries under test — Verify/List only look at doc.Evidence.
func docWithEvidence(entries ...idir.Evidence) *idir.Document {
	return &idir.Document{Evidence: entries}
}

func storeFixture(t *testing.T, s *Store, srcDir, name string, content []byte) Digest {
	t.Helper()
	src := filepath.Join(srcDir, name)
	mustWriteFile(t, src, content)
	res, err := s.Put(context.Background(), src)
	if err != nil {
		t.Fatalf("Put fixture %s: %v", name, err)
	}
	return res.Digest
}

func TestVerify_AllOKAndMissing(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "stored.log", []byte("stored evidence"))
	missingDigest := "sha256:" + strings.Repeat("9", 64)

	doc := docWithEvidence(
		idir.Evidence{ID: "ev-stored", Type: "log", Digest: d.String()},
		idir.Evidence{ID: "ev-missing", Type: "log", Digest: missingDigest},
	)

	res, err := s.Verify(context.Background(), doc)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(res.Entries))
	}
	if res.Entries[0].Status != StatusOK {
		t.Errorf("entry 0 status = %v, want %v", res.Entries[0].Status, StatusOK)
	}
	if res.Entries[1].Status != StatusMissing {
		t.Errorf("entry 1 status = %v, want %v", res.Entries[1].Status, StatusMissing)
	}
	if res.AllOK() {
		t.Error("AllOK() = true, want false (one entry is missing)")
	}
}

func TestVerify_EmptyEvidenceIsVacuouslyOK(t *testing.T) {
	s, _ := openStoreT(t)
	res, err := s.Verify(context.Background(), docWithEvidence())
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !res.AllOK() {
		t.Error("AllOK() = false for zero entries, want true")
	}
}

func TestVerify_DetectsModifiedObject(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "src.log", []byte("original content"))
	objPath, err := s.objectPath(d)
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(objPath, 0o644)
	if err := os.WriteFile(objPath, []byte("modified content, same length!!"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: d.String()})
	res, err := s.Verify(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries[0].Status != StatusCorrupted {
		t.Errorf("status = %v, want %v", res.Entries[0].Status, StatusCorrupted)
	}
}

func TestVerify_DetectsTruncatedObject(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "src.log", []byte("this content will be truncated"))
	objPath, err := s.objectPath(d)
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(objPath, 0o644)
	if err := os.Truncate(objPath, 5); err != nil {
		t.Fatal(err)
	}

	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: d.String()})
	res, err := s.Verify(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries[0].Status != StatusCorrupted {
		t.Errorf("status = %v, want %v", res.Entries[0].Status, StatusCorrupted)
	}
}

func TestVerify_DetectsTrailingBytes(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "src.log", []byte("original"))
	objPath, err := s.objectPath(d)
	if err != nil {
		t.Fatal(err)
	}
	os.Chmod(objPath, 0o644)
	f, err := os.OpenFile(objPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("EXTRA TRAILING BYTES"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: d.String()})
	res, err := s.Verify(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries[0].Status != StatusCorrupted {
		t.Errorf("status = %v, want %v", res.Entries[0].Status, StatusCorrupted)
	}
}

func TestVerify_DetectsSymlinkStoredObject(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "src.log", []byte("real evidence"))
	objPath, err := s.objectPath(d)
	if err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(dir, "decoy.log")
	mustWriteFile(t, decoy, []byte("real evidence"))
	os.Remove(objPath)
	if err := os.Symlink(decoy, objPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: d.String()})
	res, err := s.Verify(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries[0].Status != StatusCorrupted {
		t.Errorf("status = %v, want %v", res.Entries[0].Status, StatusCorrupted)
	}
}

func TestVerify_InvalidDigestDeclaration(t *testing.T) {
	s, _ := openStoreT(t)
	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: "not-a-valid-digest"})
	res, err := s.Verify(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries[0].Status != StatusInvalid {
		t.Errorf("status = %v, want %v", res.Entries[0].Status, StatusInvalid)
	}
}

func TestVerify_UnsupportedAlgorithm(t *testing.T) {
	s, _ := openStoreT(t)
	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: "md5:" + strings.Repeat("a", 32)})
	res, err := s.Verify(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if res.Entries[0].Status != StatusInvalid {
		t.Errorf("status = %v, want %v", res.Entries[0].Status, StatusInvalid)
	}
}

func TestVerify_EnforcesMaxEntriesPerCommand(t *testing.T) {
	s, _ := openStoreT(t)
	entries := make([]idir.Evidence, MaxEntriesPerCommand+1)
	for i := range entries {
		entries[i] = idir.Evidence{ID: "ev", Type: "log", Digest: "sha256:" + strings.Repeat("a", 64)}
	}
	_, err := s.Verify(context.Background(), docWithEvidence(entries...))
	if err == nil {
		t.Fatal("expected an error for exceeding MaxEntriesPerCommand, got nil")
	}
	if !IsKind(err, ErrKindTooManyEntries) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindTooManyEntries)
	}
}

func TestVerify_EnforcesMaxTotalVerifyBytes(t *testing.T) {
	s, _ := openStoreT(t)

	// The aggregate pre-pass (checkTotalBytes) runs purely off os.Lstat
	// sizes, before any object's content is read or hashed — so this test
	// places sparse files (fast to create, no real disk writes for the
	// zero-filled interior) of exactly MaxObjectSize each, directly at
	// valid shard paths for distinct made-up-but-well-formed digests. 11 *
	// 50 MiB = 550 MiB, over the 500 MiB aggregate cap, while each
	// individual object is exactly at (not over) the per-object cap.
	const n = 11
	const hexDigits = "0123456789abcdef"
	var entries []idir.Evidence
	for i := 0; i < n; i++ {
		hex := strings.Repeat(string(hexDigits[i]), 64)
		d, err := Parse("sha256:" + hex)
		if err != nil {
			t.Fatalf("Parse fixture digest %d: %v", i, err)
		}
		path, err := s.objectPath(d)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(MaxObjectSize); err != nil {
			f.Close()
			t.Fatal(err)
		}
		f.Close()
		entries = append(entries, idir.Evidence{ID: d.Hex()[:8], Type: "log", Digest: d.String()})
	}

	_, err := s.Verify(context.Background(), docWithEvidence(entries...))
	if err == nil {
		t.Fatal("expected an error for exceeding MaxTotalVerifyBytes, got nil")
	}
	if !IsKind(err, ErrKindTotalBytesExceeded) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindTotalBytesExceeded)
	}
}

func TestVerify_SingleAtCapObjectDoesNotTripAggregateLimit(t *testing.T) {
	s, dir := openStoreT(t)
	big := make([]byte, MaxObjectSize)
	src := filepath.Join(dir, "big.log")
	if err := os.WriteFile(src, big, 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := s.Put(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: res.Digest.String()})
	vres, err := s.Verify(context.Background(), doc)
	if err != nil {
		t.Fatalf("Verify of a single at-cap object should not trip the aggregate limit: %v", err)
	}
	if vres.AggregateBytes != MaxObjectSize {
		t.Errorf("AggregateBytes = %d, want %d", vres.AggregateBytes, MaxObjectSize)
	}
}

func TestVerify_DedupesRepeatedDigest(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "src.log", []byte("shared evidence"))
	doc := docWithEvidence(
		idir.Evidence{ID: "ev-a", Type: "log", Digest: d.String()},
		idir.Evidence{ID: "ev-b", Type: "log", Digest: d.String()},
	)
	res, err := s.Verify(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range res.Entries {
		if e.Status != StatusOK {
			t.Errorf("entry %d status = %v, want %v", i, e.Status, StatusOK)
		}
	}
	// AggregateBytes should count the shared digest once, not twice.
	if res.AggregateBytes != int64(len("shared evidence")) {
		t.Errorf("AggregateBytes = %d, want %d (deduplicated)", res.AggregateBytes, len("shared evidence"))
	}
}

func TestVerify_DoesNotMutateIncidentOrEvidence(t *testing.T) {
	s, dir := openStoreT(t)
	content := []byte("evidence untouched by verify")
	d := storeFixture(t, s, dir, "src.log", content)
	objPath, err := s.objectPath(d)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(objPath)
	if err != nil {
		t.Fatal(err)
	}

	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: d.String()})
	if _, err := s.Verify(context.Background(), doc); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(objPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("Verify modified the stored evidence object")
	}
}

func TestVerify_RespectsContextCancellation(t *testing.T) {
	s, dir := openStoreT(t)
	d := storeFixture(t, s, dir, "src.log", []byte("content"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	doc := docWithEvidence(idir.Evidence{ID: "ev", Type: "log", Digest: d.String()})
	_, err := s.Verify(ctx, doc)
	if err == nil {
		t.Fatal("expected an error from a canceled context, got nil")
	}
	if !IsKind(err, ErrKindCanceled) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindCanceled)
	}
}
