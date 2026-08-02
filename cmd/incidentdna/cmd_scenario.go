package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/scenario"
)

// runScenario dispatches to the two `incidentdna scenario` subcommands:
// verify, run. See docs/phase-4-plan.md §13 for the approved CLI contract
// and docs/regression-scenarios.md for the underlying design.
//
// Unlike every other top-level command, ctx here does NOT already carry
// main.go's commandTimeout: scenario run needs its own, larger,
// document-declared timeout budget for the child process it executes
// (docs/phase-4-plan.md §10), so main.go special-cases "scenario" to skip
// its usual context.WithTimeout wrapping. scenario verify, which never
// executes anything, restores that same 30-second budget itself, below.
func runScenario(ctx context.Context, args []string) int {
	if len(args) == 0 {
		printScenarioUsage()
		return exitError
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "verify":
		vctx, cancel := context.WithTimeout(ctx, commandTimeout)
		defer cancel()
		return runScenarioVerify(vctx, rest)
	case "run":
		return runScenarioRun(ctx, rest)
	case "-h", "--help", "help":
		printScenarioUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "incidentdna: unknown scenario subcommand %q\n\n", sub)
		printScenarioUsage()
		return exitError
	}
}

func printScenarioUsage() {
	fmt.Fprint(os.Stderr, `incidentdna scenario — deterministic executable regression scenarios (IRS v0.1):
verify a scenario document, or run its declared command in a bounded,
offline, local workspace.

Usage:
  incidentdna scenario verify [--source <incident-file>] <scenario-file>
      Structurally/semantically validate <scenario-file> against the IRS
      v0.1 rules. Never executes anything. If --source is given,
      additionally load, validate, and fingerprint <incident-file> through
      the unchanged Phase 1 pipeline and assert the result equals the
      scenario's declared linked_fingerprint.
        exit 0 — valid (and, with --source, linked_fingerprint matches)
        exit 1 — I/O/parse/usage error
        exit 2 — decodes but fails a semantic rule, or a --source mismatch
  incidentdna scenario run [--workspace <dir>] [--report <file>]
                            [--keep-workspace] <scenario-file>
      Run the same checks as scenario verify (without --source). If they
      pass, create a workspace, stage execution.workspace_files, execute
      execution.command under its declared/default timeout with output
      capped per stream, and compare the result against expected.
        PASS            exit 0
        FAIL / TIMEOUT  exit 2
        INVALID / INTERNAL_ERROR  exit 1

--workspace <dir> uses an explicit workspace directory instead of a fresh
temporary one; it must not already exist as a non-empty directory.
--report <file> additionally writes a deterministic JSON execution report,
overwritten unconditionally on each run.
--keep-workspace leaves the workspace directory in place after the run
instead of removing it.

incidentdna scenario never invokes a shell, never performs an implicit
$PATH lookup, never writes outside its resolved workspace and the
user-named --report path, performs no network access, and collects no
telemetry. It does not sandbox the process it launches — see
docs/regression-scenarios.md, "Safety model".
`)
}

func runScenarioVerify(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("scenario verify", flag.ContinueOnError)
	source := fs.String("source", "", "incident file to cross-check the scenario's linked_fingerprint against")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna scenario verify [--source <incident-file>] <scenario-file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	doc, err := scenario.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	res := scenario.Validate(ctx, doc, filepath.Dir(path))
	if !res.Valid() {
		fmt.Fprintf(os.Stderr, "incidentdna: %s is not a valid IRS v0.1 scenario:\n", path)
		for _, issue := range res.Issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", issue.String())
		}
		return exitValidationError
	}

	fmt.Printf("Scenario: %s (schema %s)\n", doc.Scenario.ID, doc.SchemaVersion)
	fmt.Printf("Linked fingerprint: %s\n", doc.LinkedFingerprint)

	if *source == "" {
		fmt.Println("OK: scenario is structurally valid")
		return exitOK
	}

	srcRes, err := scenario.CheckSourceFingerprint(ctx, *source, doc.LinkedFingerprint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}
	fmt.Printf("Source fingerprint: %s\n", srcRes.SourceFingerprint)
	if !srcRes.Match {
		fmt.Fprintf(os.Stderr, "incidentdna: linked_fingerprint does not match the fingerprint computed from --source %s\n", *source)
		return exitValidationError
	}
	fmt.Println("OK: scenario is structurally valid and its linked_fingerprint matches --source")
	return exitOK
}

func runScenarioRun(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("scenario run", flag.ContinueOnError)
	workspace := fs.String("workspace", "", "explicit workspace directory (default: a fresh temporary directory)")
	report := fs.String("report", "", "write a deterministic JSON execution report to this path")
	keep := fs.Bool("keep-workspace", false, "do not remove the workspace directory after the run")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna scenario run [--workspace <dir>] [--report <file>] [--keep-workspace] <scenario-file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	var res scenario.RunResult
	doc, err := scenario.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		res = scenario.RunResult{Outcome: scenario.OutcomeInternalError, Error: err.Error()}
	} else {
		res = scenario.Run(ctx, doc, filepath.Dir(path), scenario.RunOptions{
			WorkspaceDir:  *workspace,
			KeepWorkspace: *keep,
		})
	}

	printScenarioRunSummary(path, res)
	code := scenarioRunExitCode(res.Outcome)

	if *report != "" {
		rep := scenario.BuildReport(res, path)
		if werr := scenario.WriteReport(*report, rep); werr != nil {
			fmt.Fprintf(os.Stderr, "incidentdna: writing --report %s: %v\n", *report, werr)
			// The run's own result was already printed above; the process
			// exit code still reflects that the requested report could not
			// be produced (docs/phase-4-plan.md §17).
			return exitError
		}
	}

	return code
}

// scenarioRunExitCode maps a scenario.Outcome to the exit-code contract
// docs/phase-4-plan.md §14 defines for `scenario run`.
func scenarioRunExitCode(outcome scenario.Outcome) int {
	switch outcome {
	case scenario.OutcomePass:
		return exitOK
	case scenario.OutcomeFail, scenario.OutcomeTimeout:
		return exitValidationError
	default: // OutcomeInvalid, OutcomeInternalError
		return exitError
	}
}

func printScenarioRunSummary(path string, res scenario.RunResult) {
	label := res.ScenarioID
	if label == "" {
		label = path
	}
	fmt.Printf("Scenario: %s\n", label)
	fmt.Printf("Result: %s\n", res.Outcome)

	switch res.Outcome {
	case scenario.OutcomeInvalid, scenario.OutcomeInternalError:
		fmt.Printf("Error: %s\n", res.Error)
	case scenario.OutcomeTimeout:
		fmt.Printf("Command timed out (expected exit %d)\n", res.ExitCodeExpected)
		fmt.Printf("Duration: %s\n", time.Duration(res.DurationMs)*time.Millisecond)
	default: // PASS, FAIL
		fmt.Printf("Command exited %d (expected %d)\n", res.ExitCodeActual, res.ExitCodeExpected)
		fmt.Printf("Duration: %s\n", time.Duration(res.DurationMs)*time.Millisecond)
	}

	if res.Outcome == scenario.OutcomeFail || res.Outcome == scenario.OutcomeTimeout {
		fmt.Printf("Captured stdout (truncated=%t): %s\n", res.Stdout.Truncated, res.Stdout.Excerpt)
		fmt.Printf("Captured stderr (truncated=%t): %s\n", res.Stderr.Truncated, res.Stderr.Excerpt)
	}

	switch {
	case res.WorkspacePath != "":
		fmt.Printf("Workspace: kept at %s\n", res.WorkspacePath)
	case res.Outcome != scenario.OutcomeInvalid:
		fmt.Println("Workspace: removed (pass --keep-workspace to retain it)")
	}
}
