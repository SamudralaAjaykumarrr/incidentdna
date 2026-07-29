package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the incidentdna CLI once per test run into a temp
// directory and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "incidentdna")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = wd
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// cmd/incidentdna -> repo root is two levels up.
	return filepath.Join(wd, "..", "..")
}

func runCLI(t *testing.T, bin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func examplePath(t *testing.T) string {
	return filepath.Join(repoRoot(t), "examples", "duplicate-payment", "incident.yaml")
}

func invalidFixture(t *testing.T, name string) string {
	return filepath.Join(repoRoot(t), "testdata", "golden", "invalid", name)
}

func TestCLI_ValidateExample(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "validate", examplePath(t))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "valid IDIR v0.1 document") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestCLI_ValidateRejectsCyclicFixture(t *testing.T) {
	bin := buildBinary(t)
	_, stderr, code := runCLI(t, bin, "validate", invalidFixture(t, "cyclic-causality.yaml"))
	if code != 2 {
		t.Fatalf("expected exit 2 (validation failure), got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stderr, "cyclic causal relationship") {
		t.Fatalf("expected cyclic causal relationship error, got: %s", stderr)
	}
}

func TestCLI_ValidateRejectsAllInvalidFixtures(t *testing.T) {
	bin := buildBinary(t)
	dir := filepath.Join(repoRoot(t), "testdata", "golden", "invalid")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one invalid fixture")
	}
	for _, e := range entries {
		e := e
		t.Run(e.Name(), func(t *testing.T) {
			_, stderr, code := runCLI(t, bin, "validate", filepath.Join(dir, e.Name()))
			if code != 2 {
				t.Fatalf("expected exit 2 for %s, got %d; stderr: %s", e.Name(), code, stderr)
			}
		})
	}
}

func TestCLI_FingerprintExample(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "fingerprint", examplePath(t))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	fp := strings.TrimSpace(stdout)
	if !strings.HasPrefix(fp, "sha256:") || len(fp) != len("sha256:")+64 {
		t.Fatalf("unexpected fingerprint output: %q", fp)
	}
}

func TestCLI_FingerprintIsStableAcrossRuns(t *testing.T) {
	bin := buildBinary(t)
	out1, _, _ := runCLI(t, bin, "fingerprint", examplePath(t))
	out2, _, _ := runCLI(t, bin, "fingerprint", examplePath(t))
	if out1 != out2 {
		t.Fatalf("fingerprint not stable across runs: %q != %q", out1, out2)
	}
}

func TestCLI_InspectExample(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "inspect", examplePath(t))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	for _, want := range []string{"INC-2026-0417", "Fingerprint:", "Event timeline"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("expected inspect output to contain %q, got:\n%s", want, stdout)
		}
	}
}

func TestCLI_CompareSameFileIsIdentical(t *testing.T) {
	bin := buildBinary(t)
	stdout, stderr, code := runCLI(t, bin, "compare", examplePath(t), examplePath(t))
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "IDENTICAL") {
		t.Fatalf("expected IDENTICAL, got: %s", stdout)
	}
}

func TestCLI_CompareDifferentCausalStructureReportsDifference(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	original, err := os.ReadFile(examplePath(t))
	if err != nil {
		t.Fatal(err)
	}
	// Break the causal link between evt-worker-crash and evt-charge-committed.
	modified := strings.Replace(string(original),
		"    caused_by:\n      - evt-charge-committed\n",
		"    caused_by: []\n",
		1)
	if modified == string(original) {
		t.Fatal("test setup error: replacement did not match anything in the example")
	}
	modPath := filepath.Join(dir, "modified.yaml")
	if err := os.WriteFile(modPath, []byte(modified), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, bin, "compare", examplePath(t), modPath)
	if code != 0 {
		t.Fatalf("expected exit 0 (compare succeeds even when different), got %d; stderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "DIFFERENT") {
		t.Fatalf("expected DIFFERENT, got: %s", stdout)
	}
	if !strings.Contains(stdout, "causal event structure") {
		t.Fatalf("expected a causal event structure difference to be reported, got: %s", stdout)
	}
}

func TestCLI_InitRefusesOverwriteWithoutForce(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()

	cmd := exec.Command(bin, "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("first init failed: %v\n%s", err, out)
	}

	stdout, stderr, code := runCLIIn(t, dir, bin, "init")
	if code == 0 {
		t.Fatalf("expected second init without --force to fail, stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Fatalf("expected an 'already exists' error, got: %s", stderr)
	}

	// --force should succeed.
	_, stderr, code = runCLIIn(t, dir, bin, "init", "--force")
	if code != 0 {
		t.Fatalf("expected --force init to succeed, got %d; stderr: %s", code, stderr)
	}
}

func TestCLI_InitTemplateIsValid(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	if _, stderr, code := runCLIIn(t, dir, bin, "init"); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}
	_, stderr, code := runCLIIn(t, dir, bin, "validate", filepath.Join(dir, ".incidentdna", "incident.yaml"))
	if code != 0 {
		t.Fatalf("expected the init template to validate cleanly, got %d; stderr: %s", code, stderr)
	}
}

func runCLIIn(t *testing.T, dir, bin string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}
