package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// buildAndRunWithVersion compiles the CLI with the given version injected
// via -ldflags, the same flag scripts/build-release.sh uses, then runs
// `version` against the freshly built binary.
func buildAndRunWithVersion(t *testing.T, bin, injected string) (stdout, stderr string, exitCode int) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-buildvcs=false",
		"-ldflags", "-X main.version="+injected, "-o", bin, ".")
	build.Dir = wd
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	cmd := exec.Command(bin, "version")
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	exitCode = 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", runErr)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// --- incidentdna version / --version / -v ---
//
// version is never overridden by these tests (that only happens via
// -ldflags at release-build time, scripts/build-release.sh), so every
// assertion here checks for the built-in "dev" default plus exit 0, never
// failing regardless of args.

func TestCLI_Version_PrintsDevAndExitsZero(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, exitCode := runCLI(t, bin, "version")
	if exitCode != exitOK {
		t.Fatalf("exit code = %d, want %d", exitCode, exitOK)
	}
	want := "incidentdna dev\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}

func TestCLI_VersionFlag_LongForm(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, exitCode := runCLI(t, bin, "--version")
	if exitCode != exitOK {
		t.Fatalf("exit code = %d, want %d", exitCode, exitOK)
	}
	if !strings.HasPrefix(stdout, "incidentdna ") {
		t.Fatalf("stdout = %q, want prefix %q", stdout, "incidentdna ")
	}
}

func TestCLI_VersionFlag_ShortForm(t *testing.T) {
	bin := buildBinary(t)
	stdout, _, exitCode := runCLI(t, bin, "-v")
	if exitCode != exitOK {
		t.Fatalf("exit code = %d, want %d", exitCode, exitOK)
	}
	if !strings.HasPrefix(stdout, "incidentdna ") {
		t.Fatalf("stdout = %q, want prefix %q", stdout, "incidentdna ")
	}
}

func TestCLI_VersionFlag_WorksStandaloneWithNoOtherArgs(t *testing.T) {
	bin := buildBinary(t)
	// Unlike every other subcommand, --version/-v must work even though
	// it is the *only* argument, exercised here explicitly since run()
	// special-cases it before the normal args[0]-dispatch loop.
	for _, args := range [][]string{{"--version"}, {"-v"}, {"version"}} {
		stdout, stderr, exitCode := runCLI(t, bin, args...)
		if exitCode != exitOK {
			t.Fatalf("args=%v: exit code = %d, want %d (stderr=%q)", args, exitCode, exitOK, stderr)
		}
		if stdout == "" {
			t.Fatalf("args=%v: stdout empty, want version line", args)
		}
	}
}

func TestCLI_Version_LdflagsInjection(t *testing.T) {
	// Proves the -ldflags "-X main.version=..." injection path §7/§8
	// depend on actually works, independent of the "dev" default tested
	// above — this is the same mechanism scripts/build-release.sh uses.
	dir := t.TempDir()
	bin := dir + "/incidentdna"
	injected := "v9.9.9-test"
	stdout, stderr, exitCode := buildAndRunWithVersion(t, bin, injected)
	if exitCode != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr=%q)", exitCode, exitOK, stderr)
	}
	want := "incidentdna " + injected + "\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
	}
}
