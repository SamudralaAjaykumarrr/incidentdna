package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/evidence"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// --- shared fixtures/helpers for the `incidentdna evidence` CLI surface ---
//
// Every test below either passes an explicit --store rooted under t.TempDir()
// or runs with cmd.Dir set to a t.TempDir() (via runCLIIn), so none of them
// ever touch a real .incidentdna directory or any other repository state.
// examples/duplicate-payment/incident.yaml is read (via invalidFixture-style
// helpers below it is not even read) but never written to by any test here.

var digestRE = regexp.MustCompile(`sha256:[0-9a-f]{64}`)

// extractDigest pulls the first canonical "sha256:<64 hex>" token out of s,
// failing the test if none is present.
func extractDigest(t *testing.T, s string) string {
	t.Helper()
	m := digestRE.FindString(s)
	if m == "" {
		t.Fatalf("no sha256 digest found in output: %q", s)
	}
	return m
}

// objectPathFor mirrors the store's on-disk sharding convention
// (docs/phase-2-plan.md §4): the first two hex characters of the digest name
// a subdirectory, the remaining 62 name the file. Used here only to make
// direct, read-only filesystem assertions in black-box CLI tests — never to
// bypass the CLI itself for anything the CLI is expected to do.
func objectPathFor(storeRoot, digest string) string {
	hex := strings.TrimPrefix(digest, "sha256:")
	return filepath.Join(storeRoot, hex[:2], hex[2:])
}

// storeFixtureCLI runs `evidence store --store storeDir srcPath` and returns
// the printed canonical digest, failing the test on any non-zero exit.
func storeFixtureCLI(t *testing.T, bin, storeDir, srcPath string) string {
	t.Helper()
	stdout, stderr, code := runCLI(t, bin, "evidence", "store", "--store", storeDir, srcPath)
	if code != 0 {
		t.Fatalf("evidence store %s failed: exit %d\nstdout: %s\nstderr: %s", srcPath, code, stdout, stderr)
	}
	return extractDigest(t, stdout)
}

// placeSparseObject writes a sparse (fast, mostly-zero-filled) file of
// exactly size bytes directly at the store path for hexDigest, bypassing
// `evidence store` entirely. This is the same technique
// internal/evidence/verify_test.go uses for its MaxTotalVerifyBytes test:
// checkTotalBytes only needs accurate os.Stat sizes, not real content, so
// there's no need to actually write hundreds of megabytes to disk.
func placeSparseObject(t *testing.T, storeDir, hexDigest string, size int64) {
	t.Helper()
	shardDir := filepath.Join(storeDir, hexDigest[:2])
	if err := os.MkdirAll(shardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(shardDir, hexDigest[2:]))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		t.Fatal(err)
	}
}

// minimalValidDoc builds the smallest idir.Document that passes
// validate.Validate, carrying only the evidence entries under test. See
// internal/validate/validate.go: schema_version, incident.id,
// business_invariants (>=1, non-empty statement), expected_corrected_behavior
// (>=1 non-empty string), and privacy.sensitivity (one of the four known
// values) are the only hard requirements; events/reproduction_sequence may be
// empty.
func minimalValidDoc(evidenceEntries []idir.Evidence) *idir.Document {
	return &idir.Document{
		SchemaVersion: idir.SupportedSchemaVersion,
		Incident: idir.Incident{
			ID:         "INC-CLI-TEST-0001",
			Title:      "Synthetic CLI test incident",
			Summary:    "Fictional incident used only to drive evidence CLI integration tests.",
			OccurredAt: "2026-01-01T00:00:00Z",
		},
		Application: idir.Application{
			Name:           "cli-test-app",
			Service:        "cli-test-service",
			Environment:    "test",
			ReleaseVersion: "0.0.0",
		},
		Trigger: idir.Trigger{
			Type:        "synthetic",
			Description: "Synthetic trigger, fictional, for evidence CLI tests only.",
		},
		BusinessInvariants: []idir.Invariant{
			{ID: "inv-1", Statement: "at most one synthetic invariant for testing"},
		},
		ExpectedCorrectedBehavior: []string{"n/a - synthetic test fixture"},
		Evidence:                  evidenceEntries,
		Privacy: idir.Privacy{
			Sensitivity: idir.SensitivityInternal,
		},
	}
}

// writeDocJSON marshals doc as JSON (the .json extension makes
// idir.LoadFile's format sniffing unambiguous) into dir/name and returns the
// full path.
func writeDocJSON(t *testing.T, dir, name string, doc *idir.Document) string {
	t.Helper()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// fakeDigest returns a well-formed-but-never-stored "sha256:" digest built by
// repeating c 64 times, so distinct calls with distinct c produce distinct
// digests without needing to hash anything.
func fakeDigest(c byte) string {
	return "sha256:" + strings.Repeat(string(c), 64)
}

// ============================================================================
// 1. incidentdna evidence store
// ============================================================================

func TestCLI_EvidenceStore_SuccessAndCanonicalDigest(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "note.log")
	content := []byte("fictional evidence content for the CLI store test\n")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "evidence", "store", "--store", storeDir, src)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Stored:") {
		t.Fatalf("expected stdout to announce a new store, got: %s", stdout)
	}

	digest := extractDigest(t, stdout)
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("digest %q is not in canonical sha256:<64 hex> form", digest)
	}

	// Stored bytes must match the source exactly.
	got, err := os.ReadFile(objectPathFor(storeDir, digest))
	if err != nil {
		t.Fatalf("reading stored object: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("stored bytes = %q, want %q", got, content)
	}
}

func TestCLI_EvidenceStore_DeduplicatesIdenticalContent(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "note.log")
	if err := os.WriteFile(src, []byte("identical content stored twice"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout1, stderr1, code1 := runCLI(t, bin, "evidence", "store", "--store", storeDir, src)
	if code1 != 0 {
		t.Fatalf("first store: expected exit 0, got %d; stderr: %s", code1, stderr1)
	}
	digest1 := extractDigest(t, stdout1)

	stdout2, stderr2, code2 := runCLI(t, bin, "evidence", "store", "--store", storeDir, src)
	if code2 != 0 {
		t.Fatalf("second store: expected exit 0, got %d; stderr: %s", code2, stderr2)
	}
	if !strings.Contains(stdout2, "Already present") {
		t.Fatalf("expected second store of identical content to report deduplication, got: %s", stdout2)
	}
	digest2 := extractDigest(t, stdout2)
	if digest1 != digest2 {
		t.Fatalf("digest changed across dedup: %q != %q", digest1, digest2)
	}
}

func TestCLI_EvidenceStore_SourceFileUnchanged(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "note.log")
	original := []byte("source bytes that must survive evidence store untouched")
	if err := os.WriteFile(src, original, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, stderr, code := runCLI(t, bin, "evidence", "store", "--store", storeDir, src); code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}

	after, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("source file was modified by evidence store: got %q, want %q", after, original)
	}
}

func TestCLI_EvidenceStore_RejectsOversizedSource(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "big.log")

	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(evidence.MaxObjectSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	_, stderr, code := runCLI(t, bin, "evidence", "store", "--store", storeDir, src)
	if code == 0 {
		t.Fatalf("expected a non-zero exit for an oversized evidence file, got 0")
	}
	if !strings.Contains(stderr, "maximum object size") {
		t.Fatalf("expected stderr to name the object-size limit, got: %s", stderr)
	}
}

func TestCLI_EvidenceStore_RejectsSymlinkSource(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	real := filepath.Join(dir, "real.log")
	if err := os.WriteFile(real, []byte("real content behind a symlink"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.log")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks not supported on this platform/filesystem: %v", err)
	}

	_, stderr, code := runCLI(t, bin, "evidence", "store", "--store", storeDir, link)
	if code == 0 {
		t.Fatalf("expected a non-zero exit for a symlink evidence source, got 0")
	}
	if !strings.Contains(stderr, "symbolic link") {
		t.Fatalf("expected stderr to explain the symlink rejection, got: %s", stderr)
	}
}

func TestCLI_EvidenceStore_InvalidUsage(t *testing.T) {
	bin := buildBinary(t)
	storeDir := filepath.Join(t.TempDir(), "store")

	cases := []struct {
		name string
		args []string
	}{
		{"no file argument", []string{"evidence", "store", "--store", storeDir}},
		{"too many arguments", []string{"evidence", "store", "--store", storeDir, "a", "b"}},
		{"unknown evidence subcommand", []string{"evidence", "bogus"}},
		{"evidence with no subcommand", []string{"evidence"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, code := runCLI(t, bin, c.args...)
			if code == 0 {
				t.Fatalf("expected a non-zero exit for invalid usage %v, got 0; stderr: %s", c.args, stderr)
			}
		})
	}
}

// ============================================================================
// 2. incidentdna evidence verify
// ============================================================================

func TestCLI_EvidenceVerify_Success(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")

	srcA := filepath.Join(dir, "a.log")
	srcB := filepath.Join(dir, "b.log")
	os.WriteFile(srcA, []byte("evidence file A"), 0o644)
	os.WriteFile(srcB, []byte("evidence file B"), 0o644)
	digestA := storeFixtureCLI(t, bin, storeDir, srcA)
	digestB := storeFixtureCLI(t, bin, storeDir, srcB)

	doc := minimalValidDoc([]idir.Evidence{
		{ID: "ev-a", Type: "log", Digest: digestA},
		{ID: "ev-b", Type: "log", Digest: digestB},
	})
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "evidence", "verify", "--store", storeDir, docPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "OK: all 2 evidence entries verified") {
		t.Fatalf("expected an all-OK summary, got: %s", stdout)
	}
}

func TestCLI_EvidenceVerify_MissingObject(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")

	doc := minimalValidDoc([]idir.Evidence{
		{ID: "ev-missing", Type: "log", Digest: fakeDigest('9')},
	})
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "evidence", "verify", "--store", storeDir, docPath)
	if code != 2 {
		t.Fatalf("expected exit 2 for a missing evidence object, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "MISSING") {
		t.Fatalf("expected MISSING to be reported, got: %s", stdout)
	}
}

func TestCLI_EvidenceVerify_DetectsCorruptedObject(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "note.log")
	os.WriteFile(src, []byte("original evidence bytes, unmodified"), 0o644)
	digest := storeFixtureCLI(t, bin, storeDir, src)

	objPath := objectPathFor(storeDir, digest)
	os.Chmod(objPath, 0o644)
	if err := os.WriteFile(objPath, []byte("tampered evidence bytes!!, same length"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc := minimalValidDoc([]idir.Evidence{{ID: "ev", Type: "log", Digest: digest}})
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	stdout, _, code := runCLI(t, bin, "evidence", "verify", "--store", storeDir, docPath)
	if code != 2 {
		t.Fatalf("expected exit 2 for a corrupted object, got %d; stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "CORRUPTED") {
		t.Fatalf("expected CORRUPTED to be reported, got: %s", stdout)
	}
}

func TestCLI_EvidenceVerify_DetectsTruncatedObject(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "note.log")
	os.WriteFile(src, []byte("this content will be truncated after storage"), 0o644)
	digest := storeFixtureCLI(t, bin, storeDir, src)

	objPath := objectPathFor(storeDir, digest)
	os.Chmod(objPath, 0o644)
	if err := os.Truncate(objPath, 5); err != nil {
		t.Fatal(err)
	}

	doc := minimalValidDoc([]idir.Evidence{{ID: "ev", Type: "log", Digest: digest}})
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	stdout, _, code := runCLI(t, bin, "evidence", "verify", "--store", storeDir, docPath)
	if code != 2 {
		t.Fatalf("expected exit 2 for a truncated object, got %d; stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "CORRUPTED") {
		t.Fatalf("expected CORRUPTED to be reported, got: %s", stdout)
	}
}

func TestCLI_EvidenceVerify_DetectsTrailingBytes(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "note.log")
	os.WriteFile(src, []byte("this content will gain trailing bytes"), 0o644)
	digest := storeFixtureCLI(t, bin, storeDir, src)

	objPath := objectPathFor(storeDir, digest)
	os.Chmod(objPath, 0o644)
	f, err := os.OpenFile(objPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("...and some unexpected trailing data")); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	doc := minimalValidDoc([]idir.Evidence{{ID: "ev", Type: "log", Digest: digest}})
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	stdout, _, code := runCLI(t, bin, "evidence", "verify", "--store", storeDir, docPath)
	if code != 2 {
		t.Fatalf("expected exit 2 for an object with trailing bytes, got %d; stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "CORRUPTED") {
		t.Fatalf("expected CORRUPTED to be reported, got: %s", stdout)
	}
}

func TestCLI_EvidenceVerify_EnforcesMaxEntriesPerCommand(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")

	entries := make([]idir.Evidence, evidence.MaxEntriesPerCommand+1)
	for i := range entries {
		entries[i] = idir.Evidence{ID: fakeIDFor(i), Type: "log", Digest: fakeDigest('a')}
	}
	doc := minimalValidDoc(entries)
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	_, stderr, code := runCLI(t, bin, "evidence", "verify", "--store", storeDir, docPath)
	if code != 1 {
		t.Fatalf("expected exit 1 (resource-limit violation) for too many evidence entries, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "100") {
		t.Fatalf("expected stderr to name the MaxEntriesPerCommand limit (100), got: %s", stderr)
	}
}

func fakeIDFor(i int) string {
	return "ev-" + itoa(i)
}

// itoa is a tiny local decimal formatter so this file needs no strconv import
// just for evidence IDs whose exact text is never asserted on.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

func TestCLI_EvidenceVerify_EnforcesMaxTotalVerifyBytes(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	if err := os.MkdirAll(storeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 11 objects of exactly MaxObjectSize each = 550 MiB, over the 500 MiB
	// aggregate cap, while each individual object stays at (not over) the
	// per-object cap. Placed directly at their shard paths as sparse files —
	// see placeSparseObject's doc comment for why this stays fast.
	const n = 11
	const hexDigits = "0123456789a"
	var entries []idir.Evidence
	for i := 0; i < n; i++ {
		hex := strings.Repeat(string(hexDigits[i]), 64)
		placeSparseObject(t, storeDir, hex, evidence.MaxObjectSize)
		entries = append(entries, idir.Evidence{ID: fakeIDFor(i), Type: "log", Digest: "sha256:" + hex})
	}
	doc := minimalValidDoc(entries)
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	_, stderr, code := runCLI(t, bin, "evidence", "verify", "--store", storeDir, docPath)
	if code != 1 {
		t.Fatalf("expected exit 1 (resource-limit violation) for exceeding MaxTotalVerifyBytes, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "MiB") {
		t.Fatalf("expected stderr to name the MaxTotalVerifyBytes limit, got: %s", stderr)
	}
}

func TestCLI_EvidenceVerify_RejectsInvalidIncidentDocument(t *testing.T) {
	bin := buildBinary(t)
	storeDir := filepath.Join(t.TempDir(), "store")

	stdout, stderr, code := runCLI(t, bin, "evidence", "verify", "--store", storeDir, invalidFixture(t, "missing-incident-id.yaml"))
	if code != 2 {
		t.Fatalf("expected exit 2 for an invalid IDIR document, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "not a valid IDIR document") {
		t.Fatalf("expected the standard validation-failure message, got: %s", stderr)
	}
}

// ============================================================================
// 3. incidentdna evidence list
// ============================================================================

func TestCLI_EvidenceList_PresentAndMissing(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "note.log")
	os.WriteFile(src, []byte("evidence that is actually stored"), 0o644)
	presentDigest := storeFixtureCLI(t, bin, storeDir, src)

	doc := minimalValidDoc([]idir.Evidence{
		{ID: "ev-present", Type: "log", Digest: presentDigest},
		{ID: "ev-missing", Type: "log", Digest: fakeDigest('7')},
	})
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "evidence", "list", "--store", storeDir, docPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "present (not verified)") {
		t.Fatalf("expected the present entry to be reported, got: %s", stdout)
	}
	if !strings.Contains(stdout, "MISSING") {
		t.Fatalf("expected the missing entry to be reported, got: %s", stdout)
	}
}

func TestCLI_EvidenceList_AllMissingStillExitsZero(t *testing.T) {
	bin := buildBinary(t)
	storeDir := filepath.Join(t.TempDir(), "store")
	dir := t.TempDir()

	doc := minimalValidDoc([]idir.Evidence{
		{ID: "ev-a", Type: "log", Digest: fakeDigest('1')},
		{ID: "ev-b", Type: "log", Digest: fakeDigest('2')},
	})
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "evidence", "list", "--store", storeDir, docPath)
	if code != 0 {
		t.Fatalf("expected exit 0 even when every evidence entry is missing (informational, not a failure), got %d; stderr: %s", code, stderr)
	}
	if strings.Count(stdout, "MISSING") != 2 {
		t.Fatalf("expected both entries reported MISSING, got: %s", stdout)
	}
}

func TestCLI_EvidenceList_NotesPresenceIsNotIntegrity(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "note.log")
	os.WriteFile(src, []byte("evidence for the presence-vs-integrity wording check"), 0o644)
	digest := storeFixtureCLI(t, bin, storeDir, src)

	doc := minimalValidDoc([]idir.Evidence{{ID: "ev", Type: "log", Digest: digest}})
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	stdout, _, _ := runCLI(t, bin, "evidence", "list", "--store", storeDir, docPath)
	if !strings.Contains(stdout, "presence only, not content integrity") {
		t.Fatalf("expected list's output to clarify presence is not integrity, got: %s", stdout)
	}
}

func TestCLI_EvidenceList_NeverPrintsEvidenceContent(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	const marker = "UNIQUE-MARKER-DO-NOT-LEAK-LIST-4471829"
	src := filepath.Join(dir, "secretish.log")
	os.WriteFile(src, []byte(marker), 0o644)
	digest := storeFixtureCLI(t, bin, storeDir, src)

	doc := minimalValidDoc([]idir.Evidence{{ID: "ev", Type: "log", Digest: digest}})
	docPath := writeDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, _ := runCLI(t, bin, "evidence", "list", "--store", storeDir, docPath)
	if strings.Contains(stdout, marker) || strings.Contains(stderr, marker) {
		t.Fatalf("evidence list must never print stored content, but the marker leaked; stdout: %s; stderr: %s", stdout, stderr)
	}
}

// ============================================================================
// 4. incidentdna evidence inspect
// ============================================================================

func TestCLI_EvidenceInspect_ValidStoredObject(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	src := filepath.Join(dir, "note.log")
	content := []byte("evidence for a direct inspect test")
	os.WriteFile(src, content, 0o644)
	digest := storeFixtureCLI(t, bin, storeDir, src)

	stdout, stderr, code := runCLI(t, bin, "evidence", "inspect", "--store", storeDir, digest)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	for _, want := range []string{
		"Digest:    " + digest,
		"Status:    OK (integrity verified)",
		"Size:      " + itoa(len(content)) + " bytes",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected inspect output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestCLI_EvidenceInspect_MissingObject(t *testing.T) {
	bin := buildBinary(t)
	storeDir := filepath.Join(t.TempDir(), "store")

	stdout, _, code := runCLI(t, bin, "evidence", "inspect", "--store", storeDir, fakeDigest('3'))
	if code != 2 {
		t.Fatalf("expected exit 2 for a never-stored digest, got %d; stdout: %s", code, stdout)
	}
	if !strings.Contains(stdout, "MISSING") {
		t.Fatalf("expected MISSING status, got: %s", stdout)
	}
}

func TestCLI_EvidenceInspect_RejectsMalformedDigests(t *testing.T) {
	bin := buildBinary(t)
	storeDir := filepath.Join(t.TempDir(), "store")

	cases := []struct {
		name   string
		digest string
	}{
		{"no algorithm prefix", "not-a-digest"},
		{"too few hex characters", "sha256:abc"},
		{"unsupported algorithm", "md5:" + strings.Repeat("a", 64)},
		{"uppercase hex", "sha256:" + strings.Repeat("A", 64)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, code := runCLI(t, bin, "evidence", "inspect", "--store", storeDir, c.digest)
			if code != 1 {
				t.Fatalf("expected exit 1 (usage error) for %q, got %d; stderr: %s", c.digest, code, stderr)
			}
		})
	}
}

func TestCLI_EvidenceInspect_NeverPrintsEvidenceContent(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	storeDir := filepath.Join(dir, "store")
	const marker = "UNIQUE-MARKER-DO-NOT-LEAK-INSPECT-2298104"
	src := filepath.Join(dir, "secretish.log")
	os.WriteFile(src, []byte(marker), 0o644)
	digest := storeFixtureCLI(t, bin, storeDir, src)

	stdout, stderr, _ := runCLI(t, bin, "evidence", "inspect", "--store", storeDir, digest)
	if strings.Contains(stdout, marker) || strings.Contains(stderr, marker) {
		t.Fatalf("evidence inspect must never print stored content, but the marker leaked; stdout: %s; stderr: %s", stdout, stderr)
	}
}

// ============================================================================
// 5. General CLI behavior
// ============================================================================

func TestCLI_TopLevelHelpIncludesEvidenceCommand(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout+stderr, "evidence") {
		t.Fatalf("expected top-level --help to mention the evidence command, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestCLI_EvidenceHelpIncludesAllFourSubcommands(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "evidence", "--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	combined := stdout + stderr
	for _, want := range []string{"evidence store", "evidence verify", "evidence list", "evidence inspect"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected evidence --help to mention %q, got:\n%s", want, combined)
		}
	}
}

func TestCLI_EvidenceStore_CustomStorePathIsRespected(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	custom := filepath.Join(dir, "somewhere-else", "evidence-objects")
	src := filepath.Join(dir, "note.log")
	content := []byte("evidence routed to a custom store root")
	os.WriteFile(src, content, 0o644)

	digest := storeFixtureCLI(t, bin, custom, src)
	got, err := os.ReadFile(objectPathFor(custom, digest))
	if err != nil {
		t.Fatalf("expected the object under the custom --store root, got: %v", err)
	}
	if string(got) != string(content) {
		t.Fatalf("stored bytes under custom store = %q, want %q", got, content)
	}
}

func TestCLI_EvidenceStore_DefaultStoreRootIsIsolatedPerInvocation(t *testing.T) {
	bin := buildBinary(t)
	// isolatedCWD stands in for "the user's current directory" for this
	// invocation; it is a fresh temp directory, never the real repository or
	// a real user's home directory, so a default-rooted store here can never
	// touch anyone's actual .incidentdna directory.
	isolatedCWD := t.TempDir()
	src := filepath.Join(isolatedCWD, "note.log")
	content := []byte("evidence stored under the default store root")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLIIn(t, isolatedCWD, bin, "evidence", "store", "note.log")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	digest := extractDigest(t, stdout)

	defaultRoot := filepath.Join(isolatedCWD, evidence.DefaultStoreRoot)
	got, err := os.ReadFile(objectPathFor(defaultRoot, digest))
	if err != nil {
		t.Fatalf("expected the object under the default store root %q, got: %v", defaultRoot, err)
	}
	if string(got) != string(content) {
		t.Fatalf("stored bytes under default store = %q, want %q", got, content)
	}
}
