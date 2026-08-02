package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/library"
)

// --- shared fixtures/helpers for the `incidentdna library` CLI surface ---
//
// Every test below either passes an explicit --library rooted under
// t.TempDir() or runs with cmd.Dir set to a t.TempDir() (via runCLIIn), so
// none of them ever touch a real .incidentdna directory (repository-local or
// user-home) or any other repository state.

// minimalRedactedLibraryDoc builds the smallest idir.Document that passes
// validate.Validate and declares privacy.redacted == true, suitable as a
// base fixture for `library add`/`check` CLI tests. incidentID and title
// control the fingerprint (title affects it) and the incident-id-conflict
// decision (incidentID does); callers vary these to exercise the different
// occurrence-decision paths.
func minimalRedactedLibraryDoc(incidentID, title string) *idir.Document {
	return &idir.Document{
		SchemaVersion: idir.SupportedSchemaVersion,
		Incident: idir.Incident{
			ID:         incidentID,
			Title:      title,
			Summary:    "Fictional incident used only to drive library CLI integration tests.",
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
			Description: "Synthetic trigger, fictional, for library CLI tests only.",
		},
		BusinessInvariants: []idir.Invariant{
			{ID: "inv-1", Statement: "at most one synthetic invariant for testing"},
		},
		ExpectedCorrectedBehavior: []string{"n/a - synthetic test fixture"},
		Privacy: idir.Privacy{
			Sensitivity: idir.SensitivityInternal,
			Redacted:    true,
		},
	}
}

// writeLibraryDocJSON marshals doc as JSON (the .json extension makes
// idir.LoadFile's format sniffing unambiguous) into dir/name and returns the
// full path.
func writeLibraryDocJSON(t *testing.T, dir, name string, doc *idir.Document) string {
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

// ============================================================================
// 1. incidentdna library add
// ============================================================================

func TestCLI_LibraryAdd_StoresRedactedIncident(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0001", "First synthetic incident")
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, docPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Stored new occurrence") {
		t.Fatalf("expected stdout to announce a new stored occurrence, got: %s", stdout)
	}
	if !strings.Contains(stdout, "sha256:") {
		t.Fatalf("expected stdout to report a fingerprint, got: %s", stdout)
	}
	if !strings.Contains(stdout, "INC-CLI-LIB-0001") {
		t.Fatalf("expected stdout to report the incident id, got: %s", stdout)
	}
}

func TestCLI_LibraryAdd_RepeatedFormattingOnlyCopyIsIdempotent(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0002", "Idempotent add test incident")
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)

	stdout1, stderr1, code1 := runCLI(t, bin, "library", "add", "--library", libDir, docPath)
	if code1 != 0 {
		t.Fatalf("first add: expected exit 0, got %d; stderr: %s", code1, stderr1)
	}
	if !strings.Contains(stdout1, "Stored new occurrence") {
		t.Fatalf("expected first add to store a new occurrence, got: %s", stdout1)
	}

	// Re-marshal to a differently-formatted (but canonically identical) copy:
	// compact JSON instead of indented. This must still collapse to the same
	// canonical bytes/digest, so the second add is idempotent, not a
	// conflict or a second occurrence.
	compact, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	compactPath := filepath.Join(dir, "incident-compact.json")
	if err := os.WriteFile(compactPath, compact, 0o644); err != nil {
		t.Fatal(err)
	}

	stdout2, stderr2, code2 := runCLI(t, bin, "library", "add", "--library", libDir, compactPath)
	if code2 != 0 {
		t.Fatalf("second add: expected exit 0, got %d; stderr: %s", code2, stderr2)
	}
	if !strings.Contains(stdout2, "Already present") || !strings.Contains(stdout2, "idempotent") {
		t.Fatalf("expected second add to report an idempotent already-present outcome, got: %s", stdout2)
	}
}

func TestCLI_LibraryAdd_SameFingerprintAnotherIncidentIDCreatesAnotherOccurrence(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	// Same title/content (so the same fingerprint) but a different
	// incident.id: this must be stored as a second occurrence, not deduped.
	docA := minimalRedactedLibraryDoc("INC-CLI-LIB-0003-A", "Shared fingerprint incident")
	docB := minimalRedactedLibraryDoc("INC-CLI-LIB-0003-B", "Shared fingerprint incident")
	pathA := writeLibraryDocJSON(t, dir, "a.json", docA)
	pathB := writeLibraryDocJSON(t, dir, "b.json", docB)

	stdoutA, stderrA, codeA := runCLI(t, bin, "library", "add", "--library", libDir, pathA)
	if codeA != 0 {
		t.Fatalf("add A: expected exit 0, got %d; stderr: %s", codeA, stderrA)
	}
	fpA := extractLibraryFingerprint(t, stdoutA)

	stdoutB, stderrB, codeB := runCLI(t, bin, "library", "add", "--library", libDir, pathB)
	if codeB != 0 {
		t.Fatalf("add B: expected exit 0, got %d; stderr: %s", codeB, stderrB)
	}
	if !strings.Contains(stdoutB, "Stored new occurrence") {
		t.Fatalf("expected add B to store a new occurrence, got: %s", stdoutB)
	}
	fpB := extractLibraryFingerprint(t, stdoutB)
	if fpA != fpB {
		t.Fatalf("expected both incidents to share a fingerprint, got %q and %q", fpA, fpB)
	}

	stdoutList, stderrList, codeList := runCLI(t, bin, "library", "list", "--library", libDir)
	if codeList != 0 {
		t.Fatalf("list: expected exit 0, got %d; stderr: %s", codeList, stderrList)
	}
	if !strings.Contains(stdoutList, "(2 occurrence(s))") {
		t.Fatalf("expected list to show 2 occurrences under the shared fingerprint, got: %s", stdoutList)
	}
}

func TestCLI_LibraryAdd_SameIncidentIDMaterialChangeReturnsConflict(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	original := minimalRedactedLibraryDoc("INC-CLI-LIB-0004", "Original title")
	origPath := writeLibraryDocJSON(t, dir, "original.json", original)
	if _, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, origPath); code != 0 {
		t.Fatalf("initial add: expected exit 0, got %d; stderr: %s", code, stderr)
	}

	changed := minimalRedactedLibraryDoc("INC-CLI-LIB-0004", "Materially different title")
	changedPath := writeLibraryDocJSON(t, dir, "changed.json", changed)

	stdout, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, changedPath)
	if code != 1 {
		t.Fatalf("expected exit 1 for a conflicting add, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Fatalf("expected stderr to explain the conflict, got: %s", stderr)
	}
}

func TestCLI_LibraryAdd_UnredactedRejectedByDefault(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0005", "Unredacted incident")
	doc.Privacy.Redacted = false
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, docPath)
	if code != 1 {
		t.Fatalf("expected exit 1 for an unredacted document without --allow-unredacted, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "privacy.redacted") {
		t.Fatalf("expected stderr to name the privacy.redacted policy, got: %s", stderr)
	}
}

func TestCLI_LibraryAdd_AllowUnredactedSucceedsAndWarns(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0006", "Allow-unredacted incident")
	doc.Privacy.Redacted = false
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, "--allow-unredacted", docPath)
	if code != 0 {
		t.Fatalf("expected exit 0 with --allow-unredacted, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "warning") || !strings.Contains(stderr, "allow-unredacted") {
		t.Fatalf("expected stderr to contain an --allow-unredacted warning, got: %s", stderr)
	}
}

func TestCLI_LibraryAdd_AlreadyRedactedWithAllowUnredactedFlagPrintsNoWarning(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	// Already declares privacy.redacted == true; --allow-unredacted is passed
	// but was not actually needed, so no override warning should be printed.
	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0007", "Already redacted incident")
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, "--allow-unredacted", docPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if strings.Contains(stderr, "warning") {
		t.Fatalf("expected no override warning when the document was already redacted, got stderr: %s", stderr)
	}
}

func TestCLI_LibraryAdd_CustomLibraryPathIsRespected(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	custom := filepath.Join(dir, "somewhere-else", "library-objects")

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0008", "Custom path incident")
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "library", "add", "--library", custom, docPath)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Stored new occurrence") {
		t.Fatalf("expected stdout to report a stored occurrence, got: %s", stdout)
	}

	// The custom root, not the default root, must actually hold the data.
	entries, err := os.ReadDir(custom)
	if err != nil {
		t.Fatalf("expected the custom library root to contain data, got: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected the custom library root to be non-empty")
	}
	if _, err := os.Stat(filepath.Join(dir, library.DefaultLibraryRoot)); !os.IsNotExist(err) {
		t.Fatalf("expected the default library root to remain untouched when --library is given")
	}
}

func TestCLI_LibraryAdd_DefaultLibraryPathIsIsolatedPerInvocation(t *testing.T) {
	bin := buildBinary(t)
	// isolatedCWD stands in for "the user's current directory" for this
	// invocation; it is a fresh temp directory, never the real repository or
	// a real user's home directory, so a default-rooted library here can
	// never touch anyone's actual .incidentdna directory.
	isolatedCWD := t.TempDir()

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0009", "Default path incident")
	docPath := writeLibraryDocJSON(t, isolatedCWD, "incident.json", doc)

	stdout, stderr, code := runCLIIn(t, isolatedCWD, bin, "library", "add", filepath.Base(docPath))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "Stored new occurrence") {
		t.Fatalf("expected stdout to report a stored occurrence, got: %s", stdout)
	}

	defaultRoot := filepath.Join(isolatedCWD, library.DefaultLibraryRoot)
	if _, err := os.Stat(defaultRoot); err != nil {
		t.Fatalf("expected the default library root %q to exist, got: %v", defaultRoot, err)
	}
}

func TestCLI_LibraryAdd_RejectsOversizedInputFile(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	src := filepath.Join(dir, "big.json")

	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(idir.MaxDocumentSize + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	_, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, src)
	if code != 1 {
		t.Fatalf("expected exit 1 for an oversized input file, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "maximum document size") {
		t.Fatalf("expected stderr to name the document-size limit, got: %s", stderr)
	}
}

func TestCLI_LibraryAdd_InvalidUsage(t *testing.T) {
	bin := buildBinary(t)
	libDir := filepath.Join(t.TempDir(), "library")

	cases := []struct {
		name string
		args []string
	}{
		{"no file argument", []string{"library", "add", "--library", libDir}},
		{"too many arguments", []string{"library", "add", "--library", libDir, "a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, code := runCLI(t, bin, c.args...)
			if code != 1 {
				t.Fatalf("expected exit 1 for invalid usage %v, got %d; stderr: %s", c.args, code, stderr)
			}
		})
	}
}

// ============================================================================
// 2. incidentdna library check
// ============================================================================

func TestCLI_LibraryCheck_MatchReturnsExitZero(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0010", "Check match incident")
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)

	if _, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, docPath); code != 0 {
		t.Fatalf("add: expected exit 0, got %d; stderr: %s", code, stderr)
	}

	stdout, stderr, code := runCLI(t, bin, "library", "check", "--library", libDir, docPath)
	if code != 0 {
		t.Fatalf("expected exit 0 for a matching check, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Match:") {
		t.Fatalf("expected stdout to report a match, got: %s", stdout)
	}
	if !strings.Contains(stdout, "1 occurrence") {
		t.Fatalf("expected stdout to report the occurrence count, got: %s", stdout)
	}
}

func TestCLI_LibraryCheck_NoMatchReturnsExitTwo(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0011", "Never added incident")
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)

	stdout, stderr, code := runCLI(t, bin, "library", "check", "--library", libDir, docPath)
	if code != 2 {
		t.Fatalf("expected exit 2 for a no-match check, got %d; stdout: %s; stderr: %s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "No match") {
		t.Fatalf("expected stdout to report no match, got: %s", stdout)
	}
}

func TestCLI_LibraryCheck_CorruptionReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0012", "Corruption check incident")
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)

	stdoutAdd, stderrAdd, codeAdd := runCLI(t, bin, "library", "add", "--library", libDir, docPath)
	if codeAdd != 0 {
		t.Fatalf("add: expected exit 0, got %d; stderr: %s", codeAdd, stderrAdd)
	}
	fp := extractLibraryFingerprint(t, stdoutAdd)

	// Corrupt the single stored occurrence file directly on disk: any
	// non-empty rewrite makes its content no longer re-hash to its own
	// digest-derived filename.
	occPath := findLibraryOccurrenceFile(t, libDir, fp)
	if err := os.Chmod(occPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(occPath, []byte(`{"tampered":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, stderr, code := runCLI(t, bin, "library", "check", "--library", libDir, docPath)
	if code != 1 {
		t.Fatalf("expected exit 1 for a corrupted stored occurrence, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "corrupted") {
		t.Fatalf("expected stderr to report corruption, got: %s", stderr)
	}
}

func TestCLI_LibraryCheck_InvalidUsage(t *testing.T) {
	bin := buildBinary(t)
	libDir := filepath.Join(t.TempDir(), "library")

	cases := []struct {
		name string
		args []string
	}{
		{"no file argument", []string{"library", "check", "--library", libDir}},
		{"too many arguments", []string{"library", "check", "--library", libDir, "a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, code := runCLI(t, bin, c.args...)
			if code != 1 {
				t.Fatalf("expected exit 1 for invalid usage %v, got %d; stderr: %s", c.args, code, stderr)
			}
		})
	}
}

// ============================================================================
// 3. incidentdna library list
// ============================================================================

func TestCLI_LibraryList_EmptyLibrarySucceeds(t *testing.T) {
	bin := buildBinary(t)
	libDir := filepath.Join(t.TempDir(), "library")

	stdout, stderr, code := runCLI(t, bin, "library", "list", "--library", libDir)
	if code != 0 {
		t.Fatalf("expected exit 0 for an empty (never-created) library, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "empty") {
		t.Fatalf("expected stdout to clearly report an empty library, got: %s", stdout)
	}
}

func TestCLI_LibraryList_DeterministicMetadataOutput(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0013", "List metadata incident")
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)
	if _, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, docPath); code != 0 {
		t.Fatalf("add: expected exit 0, got %d; stderr: %s", code, stderr)
	}

	stdout1, stderr1, code1 := runCLI(t, bin, "library", "list", "--library", libDir)
	if code1 != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code1, stderr1)
	}
	for _, want := range []string{
		"sha256:",
		"INC-CLI-LIB-0013",
		"List metadata incident",
		"cli-test-service",
		"2026-01-01T00:00:00Z",
	} {
		if !strings.Contains(stdout1, want) {
			t.Fatalf("expected list output to contain %q, got:\n%s", want, stdout1)
		}
	}

	stdout2, stderr2, code2 := runCLI(t, bin, "library", "list", "--library", libDir)
	if code2 != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code2, stderr2)
	}
	if stdout1 != stdout2 {
		t.Fatalf("expected deterministic list output across repeated runs, got:\n%q\nvs\n%q", stdout1, stdout2)
	}
}

func TestCLI_LibraryList_DoesNotExposeSensitiveMarkerContent(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	const marker = "UNIQUE-MARKER-DO-NOT-LEAK-LIBRARY-LIST-7734921"

	doc := minimalRedactedLibraryDoc("INC-CLI-LIB-0014", "Marker leak test incident")
	doc.Incident.Summary = marker
	doc.BusinessInvariants[0].Statement = marker
	doc.ExpectedCorrectedBehavior[0] = marker
	docPath := writeLibraryDocJSON(t, dir, "incident.json", doc)
	if _, stderr, code := runCLI(t, bin, "library", "add", "--library", libDir, docPath); code != 0 {
		t.Fatalf("add: expected exit 0, got %d; stderr: %s", code, stderr)
	}

	stdout, stderr, code := runCLI(t, bin, "library", "list", "--library", libDir)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if strings.Contains(stdout, marker) || strings.Contains(stderr, marker) {
		t.Fatalf("library list must never print sensitive document content, but the marker leaked; stdout: %s; stderr: %s", stdout, stderr)
	}
}

func TestCLI_LibraryList_InvalidUsage(t *testing.T) {
	bin := buildBinary(t)
	libDir := filepath.Join(t.TempDir(), "library")

	_, stderr, code := runCLI(t, bin, "library", "list", "--library", libDir, "unexpected-argument")
	if code != 1 {
		t.Fatalf("expected exit 1 for an unexpected positional argument, got %d; stderr: %s", code, stderr)
	}
}

// ============================================================================
// 4. General CLI behavior
// ============================================================================

func TestCLI_TopLevelHelpIncludesLibraryCommand(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	if !strings.Contains(stdout+stderr, "library") {
		t.Fatalf("expected top-level --help to mention the library command, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestCLI_LibraryHelpIncludesAllThreeSubcommands(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "library", "--help")
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	combined := stdout + stderr
	for _, want := range []string{"library add", "library check", "library list"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("expected library --help to mention %q, got:\n%s", want, combined)
		}
	}
}

func TestCLI_LibraryUnknownSubcommandReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "library", "bogus")
	if code != 1 {
		t.Fatalf("expected exit 1 for an unknown library subcommand, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_LibraryNoSubcommandReturnsExitOne(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "library")
	if code != 1 {
		t.Fatalf("expected exit 1 for `library` with no subcommand, got %d; stderr: %s", code, stderr)
	}
}

// --- helpers ---

// extractLibraryFingerprint pulls the first canonical "sha256:<64 hex>"
// token out of s, failing the test if none is present.
func extractLibraryFingerprint(t *testing.T, s string) string {
	t.Helper()
	return digestRE.FindString(s)
}

// findLibraryOccurrenceFile locates the single occurrence file stored under
// fingerprint fp within libDir, using the same sharding convention
// internal/library's Store uses (docs/phase-3-plan.md §5): the first two hex
// characters of the fingerprint (after "sha256:") name a shard directory,
// the rest name the fingerprint directory, and within its occurrences/
// subdirectory the same two-level sharding applies to the occurrence's own
// digest. Used only to make a direct, read-only-then-corrupting filesystem
// edit for a corruption test — never to bypass the CLI for anything it is
// expected to do itself.
func findLibraryOccurrenceFile(t *testing.T, libDir, fp string) string {
	t.Helper()
	hex := strings.TrimPrefix(fp, "sha256:")
	fpDir := filepath.Join(libDir, hex[:2], hex[2:], "occurrences")

	var found string
	shardEntries, err := os.ReadDir(fpDir)
	if err != nil {
		t.Fatalf("reading occurrences directory %q: %v", fpDir, err)
	}
	for _, shard := range shardEntries {
		shardDir := filepath.Join(fpDir, shard.Name())
		fileEntries, err := os.ReadDir(shardDir)
		if err != nil {
			t.Fatalf("reading occurrence shard %q: %v", shardDir, err)
		}
		for _, f := range fileEntries {
			found = filepath.Join(shardDir, f.Name())
		}
	}
	if found == "" {
		t.Fatalf("no occurrence file found under %q", fpDir)
	}
	return found
}
