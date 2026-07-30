package evidence

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWriteFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// openStoreT returns a Store rooted at a fresh "store" subdirectory, plus a
// sibling scratch directory for tests to write source evidence files into.
// The two are kept separate deliberately: several tests below list/count
// everything under the store root, and a source file written directly into
// the store root would be miscounted as a stored object.
func openStoreT(t *testing.T) (*Store, string) {
	t.Helper()
	parent := t.TempDir()
	s, err := Open(filepath.Join(parent, "store"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return s, parent
}

func TestOpen_DefaultRootIsRelativeToCWD(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	s, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\"): %v", err)
	}
	want := filepath.Join(dir, DefaultStoreRoot)
	// Resolve both sides through EvalSymlinks-safe comparison: on some
	// platforms t.TempDir() may itself live under a symlinked path (e.g.
	// macOS /tmp -> /private/tmp); compare the cleaned suffix instead of
	// requiring exact equality of the leading temp-dir component.
	if !strings.HasSuffix(s.Root(), DefaultStoreRoot) {
		t.Errorf("Root() = %q, want it to end with %q", s.Root(), DefaultStoreRoot)
	}
	if !filepath.IsAbs(s.Root()) {
		t.Errorf("Root() = %q, want an absolute path", s.Root())
	}
	_ = want
}

func TestOpen_CustomStoreRoot(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom-evidence-root")
	s, err := Open(custom)
	if err != nil {
		t.Fatalf("Open(%q): %v", custom, err)
	}
	if s.Root() != custom {
		t.Errorf("Root() = %q, want %q", s.Root(), custom)
	}
}

func TestOpen_RejectsNonDirectoryRoot(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-directory")
	mustWriteFile(t, filePath, []byte("x"))

	_, err := Open(filePath)
	if err == nil {
		t.Fatal("Open on a regular file: expected error, got nil")
	}
	if !IsKind(err, ErrKindStoreRoot) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindStoreRoot)
	}
}

func TestOpen_ResolvesSymlinkRoot(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real-root")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link-root")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported on this platform/filesystem: %v", err)
	}

	s, err := Open(link)
	if err != nil {
		t.Fatalf("Open(symlink root): %v", err)
	}
	realResolved, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	if s.Root() != realResolved {
		t.Errorf("Root() = %q, want resolved real path %q", s.Root(), realResolved)
	}
}

func TestPut_RoundTrip(t *testing.T) {
	s, dir := openStoreT(t)
	content := []byte("fictional evidence content for phase 2 store test")
	src := filepath.Join(dir, "src.log")
	mustWriteFile(t, src, content)

	res, err := s.Put(context.Background(), src)
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if res.AlreadyPresent {
		t.Error("first Put reported AlreadyPresent = true, want false")
	}
	if res.Size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", res.Size, len(content))
	}

	wantHasher := NewHasher()
	wantHasher.Write(content)
	wantDigest := digestFromSum(wantHasher.Sum(nil))
	if !res.Digest.Equal(wantDigest) {
		t.Errorf("Digest = %s, want %s", res.Digest, wantDigest)
	}

	// Object must actually be on disk at the expected sharded path.
	objPath, err := s.objectPath(res.Digest)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(objPath)
	if err != nil {
		t.Fatalf("read stored object: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Error("stored object content does not match source content")
	}

	// Source file must be untouched.
	srcAfter, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(srcAfter, content) {
		t.Error("source file was modified by Put")
	}
}

func TestPut_OriginalFilenameNotInObjectPath(t *testing.T) {
	s, dir := openStoreT(t)
	content := []byte("evidence with a distinctive filename")
	src := filepath.Join(dir, "very-distinctive-original-name.log")
	mustWriteFile(t, src, content)

	res, err := s.Put(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	objPath, err := s.objectPath(res.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(objPath, "very-distinctive-original-name") {
		t.Errorf("object path %q contains the original filename", objPath)
	}
}

func TestPut_DeduplicatesIdenticalContent(t *testing.T) {
	s, dir := openStoreT(t)
	content := []byte("duplicate content stored twice")
	src1 := filepath.Join(dir, "a.log")
	src2 := filepath.Join(dir, "b.log") // different name, same bytes
	mustWriteFile(t, src1, content)
	mustWriteFile(t, src2, content)

	res1, err := s.Put(context.Background(), src1)
	if err != nil {
		t.Fatal(err)
	}
	if res1.AlreadyPresent {
		t.Fatal("first store of new content reported AlreadyPresent")
	}

	res2, err := s.Put(context.Background(), src2)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.AlreadyPresent {
		t.Error("storing identical content again should report AlreadyPresent = true")
	}
	if !res1.Digest.Equal(res2.Digest) {
		t.Error("identical content produced different digests")
	}

	// Only one object on disk for this digest — confirm exactly one shard
	// file exists under the store root (excluding .tmp).
	count := 0
	filepath.WalkDir(s.Root(), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(path, tmpDirName) {
			return nil
		}
		count++
		return nil
	})
	if count != 1 {
		t.Errorf("expected exactly 1 stored object, found %d", count)
	}
}

func TestPut_DifferentContentNeverCollides(t *testing.T) {
	s, dir := openStoreT(t)
	srcA := filepath.Join(dir, "a.log")
	srcB := filepath.Join(dir, "b.log")
	mustWriteFile(t, srcA, []byte("content A"))
	mustWriteFile(t, srcB, []byte("content B"))

	resA, err := s.Put(context.Background(), srcA)
	if err != nil {
		t.Fatal(err)
	}
	resB, err := s.Put(context.Background(), srcB)
	if err != nil {
		t.Fatal(err)
	}
	if resA.Digest.Equal(resB.Digest) {
		t.Fatal("different content produced the same digest")
	}
	pathA, _ := s.objectPath(resA.Digest)
	pathB, _ := s.objectPath(resB.Digest)
	if pathA == pathB {
		t.Error("different digests produced the same object path")
	}
}

func TestPut_RejectsExistingCorruptedObject(t *testing.T) {
	s, dir := openStoreT(t)
	content := []byte("evidence that will be corrupted after storage")
	src := filepath.Join(dir, "src.log")
	mustWriteFile(t, src, content)

	res, err := s.Put(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	objPath, err := s.objectPath(res.Digest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(objPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(objPath, []byte("tampered bytes, wrong content entirely"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Re-storing the same original content should now fail: the existing
	// object at that digest's path no longer matches the digest.
	_, err = s.Put(context.Background(), src)
	if err == nil {
		t.Fatal("Put over a corrupted existing object: expected error, got nil")
	}
	if !IsKind(err, ErrKindObjectCorrupted) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindObjectCorrupted)
	}
}

func TestPut_RejectsSourceSymlink(t *testing.T) {
	s, dir := openStoreT(t)
	real := filepath.Join(dir, "real.log")
	mustWriteFile(t, real, []byte("real content"))
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	_, err := s.Put(context.Background(), link)
	if err == nil {
		t.Fatal("Put on a symlink source: expected error, got nil")
	}
	if !IsKind(err, ErrKindUnsafeSource) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindUnsafeSource)
	}
}

func TestPut_RejectsDirectorySource(t *testing.T) {
	s, dir := openStoreT(t)
	sub := filepath.Join(dir, "a-directory")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := s.Put(context.Background(), sub)
	if err == nil {
		t.Fatal("Put on a directory: expected error, got nil")
	}
	if !IsKind(err, ErrKindSourceIsDirectory) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindSourceIsDirectory)
	}
}

func TestPut_RejectsMissingSource(t *testing.T) {
	s, dir := openStoreT(t)
	_, err := s.Put(context.Background(), filepath.Join(dir, "does-not-exist.log"))
	if err == nil {
		t.Fatal("Put on a missing file: expected error, got nil")
	}
	if !IsKind(err, ErrKindSourceMissing) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindSourceMissing)
	}
}

func TestPut_RejectsOversizedSource(t *testing.T) {
	s, dir := openStoreT(t)
	src := filepath.Join(dir, "big.log")
	// Sparse-ish large file: MaxObjectSize+1 bytes.
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxObjectSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	_, err = s.Put(context.Background(), src)
	if err == nil {
		t.Fatal("Put on an oversized file: expected error, got nil")
	}
	if !IsKind(err, ErrKindObjectTooLarge) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindObjectTooLarge)
	}
}

func TestPut_EmptyFile(t *testing.T) {
	s, dir := openStoreT(t)
	src := filepath.Join(dir, "empty.log")
	mustWriteFile(t, src, nil)

	res, err := s.Put(context.Background(), src)
	if err != nil {
		t.Fatalf("Put on an empty file: unexpected error: %v", err)
	}
	if res.Size != 0 {
		t.Errorf("Size = %d, want 0", res.Size)
	}
	wantHasher := NewHasher()
	want := digestFromSum(wantHasher.Sum(nil))
	if !res.Digest.Equal(want) {
		t.Errorf("empty-file digest = %s, want %s (sha256 of empty input)", res.Digest, want)
	}
}

func TestPut_NoTempFilesLeakedAfterSuccess(t *testing.T) {
	s, dir := openStoreT(t)
	src := filepath.Join(dir, "src.log")
	mustWriteFile(t, src, []byte("content"))
	if _, err := s.Put(context.Background(), src); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(s.tmpDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf(".tmp directory has %d leftover entries after a successful Put, want 0", len(entries))
	}
}

func TestPut_NoTempFilesLeakedAfterFailure(t *testing.T) {
	s, dir := openStoreT(t)
	src := filepath.Join(dir, "big.log")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxObjectSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	if _, err := s.Put(context.Background(), src); err == nil {
		t.Fatal("expected Put to fail on oversized source")
	}
	entries, err := os.ReadDir(s.tmpDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf(".tmp directory has %d leftover entries after a failed Put, want 0", len(entries))
	}
}

func TestPut_NoPartialFinalObjectAfterFailure(t *testing.T) {
	s, dir := openStoreT(t)
	src := filepath.Join(dir, "big.log")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxObjectSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	if _, err := s.Put(context.Background(), src); err == nil {
		t.Fatal("expected Put to fail on oversized source")
	}

	// No shard directories should have been created under the root at all,
	// since finalization never runs on a failed ingest.
	entries, err := os.ReadDir(s.Root())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() == tmpDirName {
			continue
		}
		t.Errorf("unexpected entry %q under store root after a failed Put", e.Name())
	}
}

func TestObjectPath_ContainedUnderRoot(t *testing.T) {
	s, _ := openStoreT(t)
	d, err := Parse("sha256:" + strings.Repeat("c3", 32))
	if err != nil {
		t.Fatal(err)
	}
	path, err := s.objectPath(d)
	if err != nil {
		t.Fatal(err)
	}
	rootWithSep := s.Root() + string(os.PathSeparator)
	if !strings.HasPrefix(path, rootWithSep) {
		t.Errorf("object path %q is not under store root %q", path, s.Root())
	}
	// Shard directory is exactly the first two hex chars.
	rel, err := filepath.Rel(s.Root(), path)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 2 || parts[0] != d.Hex()[:2] || parts[1] != d.Hex()[2:] {
		t.Errorf("unexpected object path shape: %v", parts)
	}
}

func TestObjectPath_RejectsZeroDigest(t *testing.T) {
	s, _ := openStoreT(t)
	_, err := s.objectPath(Digest{})
	if err == nil {
		t.Fatal("objectPath on zero Digest: expected error, got nil")
	}
	if !IsKind(err, ErrKindInvalidDigest) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindInvalidDigest)
	}
}

func TestPut_ConcurrentIdenticalContent(t *testing.T) {
	s, dir := openStoreT(t)
	content := []byte("concurrently stored identical content")
	const n = 8
	srcs := make([]string, n)
	for i := 0; i < n; i++ {
		srcs[i] = filepath.Join(dir, "src.log")
	}
	// All goroutines store the *same* source file concurrently.
	mustWriteFile(t, srcs[0], content)

	errs := make(chan error, n)
	digests := make(chan Digest, n)
	for i := 0; i < n; i++ {
		go func() {
			res, err := s.Put(context.Background(), srcs[0])
			errs <- err
			digests <- res.Digest
		}()
	}
	var first Digest
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Put %d: %v", i, err)
		}
		d := <-digests
		if i == 0 {
			first = d
		} else if !d.Equal(first) {
			t.Errorf("concurrent Put %d produced a different digest", i)
		}
	}
}

func TestCheckObject_DetectsSymlinkStoredObject(t *testing.T) {
	s, dir := openStoreT(t)
	src := filepath.Join(dir, "src.log")
	mustWriteFile(t, src, []byte("real evidence"))
	res, err := s.Put(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	objPath, err := s.objectPath(res.Digest)
	if err != nil {
		t.Fatal(err)
	}

	// Replace the real object with a symlink to itself (via a copy) to
	// simulate a store tampered with a symlink at a valid object path,
	// without following an actual external file.
	decoy := filepath.Join(dir, "decoy-target.log")
	mustWriteFile(t, decoy, []byte("real evidence")) // same content, would hash-match if read directly
	if err := os.Remove(objPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(decoy, objPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	status, _, detail := s.checkObject(context.Background(), res.Digest)
	if status != objectCorrupted {
		t.Errorf("checkObject on symlinked object = %v, want objectCorrupted (detail: %q)", status, detail)
	}
}

func TestOpen_ErrorsWrapConsistently(t *testing.T) {
	// Sanity check that *Error satisfies the error interface and Unwrap
	// works, since callers use errors.As/errors.Is against it.
	var target *Error
	baseErr := os.ErrPermission
	wrapped := &Error{Kind: ErrKindPermission, Msg: "wrapped", Err: baseErr}
	if !errors.As(error(wrapped), &target) {
		t.Fatal("errors.As should find the *Error")
	}
	if !errors.Is(wrapped, baseErr) {
		t.Fatal("errors.Is should see through Unwrap to the base error")
	}
}
