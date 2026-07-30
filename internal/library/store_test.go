package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// openStoreT returns a Store rooted at a fresh "library" subdirectory under
// a temporary directory, mirroring internal/evidence's openStoreT helper.
func openStoreT(t *testing.T) (*Store, string) {
	t.Helper()
	parent := t.TempDir()
	s, err := Open(filepath.Join(parent, "library"))
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
		t.Fatalf(`Open(""): %v`, err)
	}
	if !strings.HasSuffix(s.Root(), DefaultLibraryRoot) {
		t.Errorf("Root() = %q, want it to end with %q", s.Root(), DefaultLibraryRoot)
	}
	if !filepath.IsAbs(s.Root()) {
		t.Errorf("Root() = %q, want an absolute path", s.Root())
	}
}

func TestOpen_CustomLibraryRoot(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "custom-library-root")
	s, err := Open(custom)
	if err != nil {
		t.Fatalf("Open(%q): %v", custom, err)
	}
	if s.Root() != custom {
		t.Errorf("Root() = %q, want %q", s.Root(), custom)
	}
}

func TestOpen_NonExistentRootIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	notYetCreated := filepath.Join(dir, "does-not-exist-yet")
	s, err := Open(notYetCreated)
	if err != nil {
		t.Fatalf("Open on a not-yet-created root: unexpected error: %v", err)
	}
	if s.Root() != notYetCreated {
		t.Errorf("Root() = %q, want %q", s.Root(), notYetCreated)
	}
	if _, err := os.Stat(notYetCreated); err == nil {
		t.Error("Open should not create the root directory itself; that is a later slice's write path")
	}
}

func TestOpen_RejectsNonDirectoryRoot(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Open(filePath)
	if err == nil {
		t.Fatal("Open on a regular file: expected error, got nil")
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

func TestOpen_RejectsSymlinkRootToNonDirectory(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real-file")
	if err := os.WriteFile(real, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link-to-file")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	_, err := Open(link)
	if err == nil {
		t.Fatal("Open(symlink to a non-directory): expected error, got nil")
	}
}

var exampleFingerprint = "sha256:" + repeatHex("a1", 32)
var exampleFingerprint2 = "sha256:" + repeatHex("b2", 32)
var exampleDigestHex = repeatHex("c3", 32)
var exampleDigestHex2 = repeatHex("d4", 32)

func repeatHex(pair string, n int) string {
	out := make([]byte, 0, len(pair)*n)
	for i := 0; i < n; i++ {
		out = append(out, pair...)
	}
	return string(out)
}

func TestFingerprintDir_Shape(t *testing.T) {
	s, _ := openStoreT(t)
	dir, err := s.FingerprintDir(exampleFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	rootWithSep := s.Root() + string(os.PathSeparator)
	if !strings.HasPrefix(dir, rootWithSep) {
		t.Errorf("fingerprint dir %q is not under library root %q", dir, s.Root())
	}
	rel, err := filepath.Rel(s.Root(), dir)
	if err != nil {
		t.Fatal(err)
	}
	hexPart := strings.TrimPrefix(exampleFingerprint, "sha256:")
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 2 || parts[0] != hexPart[:2] || parts[1] != hexPart[2:] {
		t.Errorf("unexpected fingerprint dir shape: %v", parts)
	}
}

func TestFingerprintDir_DifferentFingerprintsNeverCollide(t *testing.T) {
	s, _ := openStoreT(t)
	dirA, err := s.FingerprintDir(exampleFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	dirB, err := s.FingerprintDir(exampleFingerprint2)
	if err != nil {
		t.Fatal(err)
	}
	if dirA == dirB {
		t.Error("different fingerprints produced the same directory path")
	}
}

func TestFingerprintDir_RejectsInvalidFingerprint(t *testing.T) {
	s, _ := openStoreT(t)
	cases := []string{
		"",
		"not-a-fingerprint",
		"sha256:" + strings.Repeat("g", 64), // non-hex
		"sha256:" + strings.Repeat("a", 63), // too short
		"sha256:" + strings.Repeat("a", 65), // too long
		"sha1:" + strings.Repeat("a", 64),   // wrong algorithm
		"sha256:" + strings.ToUpper(strings.Repeat("a", 64)), // uppercase
		"sha256:" + strings.Repeat("a", 64) + "/../../etc/passwd",
	}
	for _, fp := range cases {
		if _, err := s.FingerprintDir(fp); err == nil {
			t.Errorf("FingerprintDir(%q): expected error, got nil", fp)
		}
	}
}

func TestOccurrencePath_Shape(t *testing.T) {
	s, _ := openStoreT(t)
	path, err := s.OccurrencePath(exampleFingerprint, exampleDigestHex)
	if err != nil {
		t.Fatal(err)
	}
	fpDir, err := s.FingerprintDir(exampleFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	fpDirWithSep := fpDir + string(os.PathSeparator)
	if !strings.HasPrefix(path, fpDirWithSep) {
		t.Errorf("occurrence path %q is not under its fingerprint dir %q", path, fpDir)
	}
	rel, err := filepath.Rel(fpDir, path)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(occurrencesDirName, exampleDigestHex[:2], exampleDigestHex[2:]+".json")
	if rel != want {
		t.Errorf("occurrence path relative shape = %q, want %q", rel, want)
	}
}

func TestOccurrencePath_DifferentDigestsNeverCollide(t *testing.T) {
	s, _ := openStoreT(t)
	pathA, err := s.OccurrencePath(exampleFingerprint, exampleDigestHex)
	if err != nil {
		t.Fatal(err)
	}
	pathB, err := s.OccurrencePath(exampleFingerprint, exampleDigestHex2)
	if err != nil {
		t.Fatal(err)
	}
	if pathA == pathB {
		t.Error("different digests produced the same occurrence path")
	}
}

func TestOccurrencePath_DoesNotContainIncidentIDOrOtherUntrustedString(t *testing.T) {
	s, _ := openStoreT(t)
	path, err := s.OccurrencePath(exampleFingerprint, exampleDigestHex)
	if err != nil {
		t.Fatal(err)
	}
	suspicious := "very-distinctive-incident-id-INC-1234"
	if strings.Contains(path, suspicious) {
		t.Errorf("occurrence path %q must never contain an untrusted string like an incident id", path)
	}
}

func TestOccurrencePath_RejectsInvalidDigest(t *testing.T) {
	s, _ := openStoreT(t)
	cases := []string{
		"",
		"not-a-digest",
		strings.Repeat("g", 64),
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.ToUpper(strings.Repeat("a", 64)),
		"../../../etc/passwd",
		strings.Repeat("a", 64) + "/../../etc/passwd",
	}
	for _, d := range cases {
		if _, err := s.OccurrencePath(exampleFingerprint, d); err == nil {
			t.Errorf("OccurrencePath(fp, %q): expected error, got nil", d)
		}
	}
}

func TestOccurrencePath_RejectsInvalidFingerprint(t *testing.T) {
	s, _ := openStoreT(t)
	if _, err := s.OccurrencePath("not-a-fingerprint", exampleDigestHex); err == nil {
		t.Error("OccurrencePath with invalid fingerprint: expected error, got nil")
	}
}

func TestIndexPath_UnderFingerprintDir(t *testing.T) {
	s, _ := openStoreT(t)
	idxPath, err := s.IndexPath(exampleFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	fpDir, err := s.FingerprintDir(exampleFingerprint)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(fpDir, indexFileName)
	if idxPath != want {
		t.Errorf("IndexPath = %q, want %q", idxPath, want)
	}
}

func TestIndexPath_RejectsInvalidFingerprint(t *testing.T) {
	s, _ := openStoreT(t)
	if _, err := s.IndexPath("garbage"); err == nil {
		t.Error("IndexPath with invalid fingerprint: expected error, got nil")
	}
}

func TestCheckContained_RejectsEscapedPath(t *testing.T) {
	s, _ := openStoreT(t)
	if err := s.checkContained(filepath.Join(filepath.Dir(s.Root()), "outside")); err == nil {
		t.Error("checkContained on a path outside the root: expected error, got nil")
	}
	if err := s.checkContained(s.Root()); err != nil {
		t.Errorf("checkContained on the root itself: unexpected error: %v", err)
	}
}
