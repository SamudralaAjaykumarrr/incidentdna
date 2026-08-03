package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/library"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/scenario"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/suite"
)

// runSuite dispatches to the two `incidentdna suite` subcommands: verify,
// run. See docs/phase-5-plan.md §13 for the approved CLI contract and
// docs/scenario-suites.md for the underlying design.
//
// Unlike every other top-level command except "scenario", ctx here does NOT
// already carry main.go's commandTimeout: suite run needs its own budget for
// the (potentially many) bounded child-process executions it performs in
// sequence, so main.go special-cases "suite" (alongside "scenario") to skip
// its usual context.WithTimeout wrapping. suite verify, which never executes
// anything, restores that same 30-second budget itself, below.
func runSuite(ctx context.Context, args []string) int {
	if len(args) == 0 {
		printSuiteUsage()
		return exitError
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "verify":
		vctx, cancel := context.WithTimeout(ctx, commandTimeout)
		defer cancel()
		return runSuiteVerify(vctx, rest)
	case "run":
		return runSuiteRun(ctx, rest)
	case "-h", "--help", "help":
		printSuiteUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "incidentdna: unknown suite subcommand %q\n\n", sub)
		printSuiteUsage()
		return exitError
	}
}

func printSuiteUsage() {
	fmt.Fprint(os.Stderr, `incidentdna suite — scenario suite manifests (ISM v0.1): verify a suite
manifest and every scenario it lists, or run them all sequentially and
report the aggregate result.

Usage:
  incidentdna suite verify [--library <dir>] <suite-file>
      Structurally/semantically validate <suite-file> against the ISM v0.1
      rules (schema_version, non-empty scenarios list, path safety,
      duplicate-path rejection, aggregate timeout bound), then load and
      validate every listed scenario file against the existing IRS v0.1
      rules. Never executes anything. If --library is given, after the
      above checks pass, look up every distinct linked_fingerprint among
      the listed scenarios (first-occurrence order) against the named (or
      default) incident library, annotate each scenario's own summary
      line, and print a one-line aggregate count — purely informational,
      never affecting the exit code below.
        exit 0 — suite manifest and every listed scenario are valid
        exit 1 — I/O/parse/usage error, or (with --library) a malformed or
                 unreadable library
        exit 2 — decodes but fails a semantic rule, or a listed scenario is invalid
  incidentdna suite run [--workspace-root <dir>] [--report <file>]
                        [--keep-workspaces] [--fail-fast] <suite-file>
      Run the same checks as suite verify. If they pass, run each listed
      scenario in declared order via scenario.Run (unchanged), aggregate the
      outcomes, and print a per-scenario and summary line.
        Aggregate PASS            exit 0
        Aggregate FAIL            exit 2
        Aggregate INVALID / INTERNAL_ERROR  exit 1

--workspace-root <dir> uses an explicit shared root instead of giving each
scenario an independent temporary workspace; it must not already exist as a
non-empty directory. Each listed scenario still gets its own subdirectory
under it, never a shared workspace.
--report <file> additionally writes a deterministic JSON aggregate report,
overwritten unconditionally on each run.
--keep-workspaces leaves every scenario's workspace directory in place after
the run instead of removing it.
--fail-fast stops running further scenarios immediately after the first
non-PASS outcome; scenarios never reached are reported SKIPPED.

incidentdna suite never discovers scenarios via directory walk or glob —
a suite manifest lists them explicitly, in the order they run. It never
executes anything itself; every scenario it lists runs via
internal/scenario.Run, unchanged, once each, in declared order, never in
parallel. It performs no network access and collects no telemetry.
`)
}

func runSuiteVerify(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("suite verify", flag.ContinueOnError)
	libDir := libraryFlag(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna suite verify [--library <dir>] <suite-file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	// libraryGiven distinguishes "--library was not passed at all" (Phase 6's
	// cross-reference lookup never runs, byte-for-byte identical output to
	// pre-Phase-6 behavior) from "--library was explicitly passed" — mirrors
	// runScenarioVerify's identical handling (docs/phase-6-plan.md §7 point
	// 4/§9).
	libraryGiven := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "library" {
			libraryGiven = true
		}
	})

	doc, err := suite.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	res := suite.Validate(ctx, doc, filepath.Dir(path))

	fmt.Printf("Suite: %s (schema %s)\n", doc.Suite.ID, doc.SchemaVersion)
	fmt.Printf("Scenarios: %d listed\n", len(doc.Scenarios))

	// The library cross-reference only ever runs "after the above checks
	// pass" (§9): an invalid suite never triggers a library lookup, so its
	// per-scenario lines are never annotated.
	var libRes suiteLibraryResult
	if libraryGiven && res.Valid() {
		var code int
		libRes, code = suiteLibraryCrossReference(ctx, *libDir, doc, filepath.Dir(path))
		if code != exitOK {
			return code
		}
	}

	for _, sc := range res.Scenarios {
		status := "OK"
		if !sc.Valid {
			status = "FAIL"
		}
		label := sc.ScenarioID
		if label == "" {
			label = sc.Path
		}
		line := fmt.Sprintf("  [%s] %s (%s)", status, label, sc.Path)
		if suffix, ok := libRes.annotations[sc.Path]; ok {
			line += " " + suffix
		}
		fmt.Println(line)
	}

	if !res.Valid() {
		fmt.Fprintf(os.Stderr, "incidentdna: %s is not a valid ISM v0.1 suite:\n", path)
		for _, issue := range res.Issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", issue.String())
		}
		for _, sc := range res.Scenarios {
			for _, iss := range sc.Issues {
				fmt.Fprintf(os.Stderr, "  - %s: %s\n", sc.Path, iss)
			}
		}
		return exitValidationError
	}

	if libraryGiven {
		fmt.Printf("Library cross-reference: %d of %d distinct linked fingerprint(s) have library occurrences\n", libRes.distinctWithOccurrences, libRes.distinctChecked)
	}

	fmt.Printf("OK: suite manifest and all %d listed scenarios are structurally valid\n", len(doc.Scenarios))
	return exitOK
}

// suiteLibraryResult is the outcome of suiteLibraryCrossReference: a
// per-listed-scenario-path annotation suffix, and the aggregate distinct-
// fingerprint counts printed on the "Library cross-reference:" summary line.
type suiteLibraryResult struct {
	annotations             map[string]string
	distinctChecked         int
	distinctWithOccurrences int
}

// suiteLibraryCrossReference implements docs/phase-6-plan.md §9's suite
// verify --library behavior: look up every *distinct* linked_fingerprint
// among doc's listed scenarios against the named (or default) incident
// library, in first-occurrence declared order, at most once per distinct
// fingerprint (§7 point 3) — never once per listed scenario. Each listed
// scenario is re-loaded via scenario.LoadFile (cheap, no execution) purely
// to read its own already-validated linked_fingerprint field; internal/suite
// itself is not modified and gains no new field or method (§3/§6).
func suiteLibraryCrossReference(ctx context.Context, libDir string, doc *suite.Document, suiteDir string) (suiteLibraryResult, int) {
	st, code := openLibraryStore(libDir)
	if code != exitOK {
		return suiteLibraryResult{}, code
	}

	annotations := make(map[string]string, len(doc.Scenarios))
	cache := make(map[string]library.CheckResult)
	order := make([]string, 0, len(doc.Scenarios))

	for _, entry := range doc.Scenarios {
		if err := ctx.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "incidentdna: library cross-reference canceled: %v\n", err)
			return suiteLibraryResult{}, exitError
		}

		resolvedPath := filepath.Join(suiteDir, entry.Path)
		sdoc, err := scenario.LoadFile(resolvedPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "incidentdna: re-loading %s for library cross-reference: %v\n", entry.Path, err)
			return suiteLibraryResult{}, exitError
		}

		fp := sdoc.LinkedFingerprint
		res, seen := cache[fp]
		if !seen {
			res, err = library.CheckFingerprint(ctx, st, fp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
				return suiteLibraryResult{}, exitError
			}
			cache[fp] = res
			order = append(order, fp)
		}

		if res.Outcome == library.CheckOutcomeMatch {
			annotations[entry.Path] = fmt.Sprintf("— library: %d occurrence(s)", res.MatchCount)
		} else {
			annotations[entry.Path] = "— library: no occurrences"
		}
	}

	withOccurrences := 0
	for _, fp := range order {
		if cache[fp].Outcome == library.CheckOutcomeMatch {
			withOccurrences++
		}
	}

	return suiteLibraryResult{
		annotations:             annotations,
		distinctChecked:         len(order),
		distinctWithOccurrences: withOccurrences,
	}, exitOK
}

func runSuiteRun(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("suite run", flag.ContinueOnError)
	workspaceRoot := fs.String("workspace-root", "", "explicit shared workspace root (default: each scenario gets an independent temporary directory)")
	report := fs.String("report", "", "write a deterministic JSON aggregate report to this path")
	keep := fs.Bool("keep-workspaces", false, "do not remove any scenario's workspace directory after the run")
	failFast := fs.Bool("fail-fast", false, "stop after the first non-PASS scenario outcome")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna suite run [--workspace-root <dir>] [--report <file>] [--keep-workspaces] [--fail-fast] <suite-file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	var res suite.SuiteRunResult
	doc, err := suite.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		res = suite.SuiteRunResult{Outcome: suite.AggregateInternalError, Error: err.Error()}
	} else {
		// A fixed backstop, not a per-suite computed budget: suite.Validate
		// (called internally by suite.Run) already rejects any suite whose
		// summed scenario timeouts exceed MaxSuiteTotalTimeoutSeconds before
		// anything executes, so this context can safely use that same fixed
		// constant as its upper bound (docs/phase-5-plan.md §9/§13).
		rctx, cancel := context.WithTimeout(ctx, suite.MaxSuiteTotalTimeoutSeconds*time.Second)
		defer cancel()
		res = suite.Run(rctx, doc, filepath.Dir(path), suite.RunOptions{
			WorkspaceRoot:  *workspaceRoot,
			KeepWorkspaces: *keep,
			FailFast:       *failFast,
		})
	}

	printSuiteRunSummary(path, res)
	code := suiteRunExitCode(res.Outcome)

	if *report != "" {
		rep := suite.BuildReport(res, path)
		if werr := suite.WriteReport(*report, rep); werr != nil {
			fmt.Fprintf(os.Stderr, "incidentdna: writing --report %s: %v\n", *report, werr)
			// The run's own result was already printed above; the process
			// exit code still reflects that the requested report could not
			// be produced (docs/phase-5-plan.md §17).
			return exitError
		}
	}

	return code
}

// suiteRunExitCode maps a suite.AggregateOutcome to the exit-code contract
// docs/phase-5-plan.md §14 defines for `suite run`.
func suiteRunExitCode(outcome suite.AggregateOutcome) int {
	switch outcome {
	case suite.AggregatePass:
		return exitOK
	case suite.AggregateFail:
		return exitValidationError
	default: // AggregateInvalid, AggregateInternalError
		return exitError
	}
}

func printSuiteRunSummary(path string, res suite.SuiteRunResult) {
	label := res.SuiteID
	if label == "" {
		label = path
	}
	fmt.Printf("Suite: %s\n", label)

	total := len(res.Scenarios)
	for i, sr := range res.Scenarios {
		scenarioLabel := sr.Path
		if sr.Result != nil && sr.Result.ScenarioID != "" {
			scenarioLabel = sr.Result.ScenarioID
		}
		fmt.Printf("[%d/%d] %s ... %s\n", i+1, total, scenarioLabel, sr.Outcome)
	}

	counts := suite.CountOutcomes(res.Scenarios)
	fmt.Printf("Result: %s (%d PASS, %d FAIL, %d TIMEOUT, %d INVALID, %d INTERNAL_ERROR",
		res.Outcome, counts.Pass, counts.Fail, counts.Timeout, counts.Invalid, counts.InternalError)
	if counts.Skipped > 0 {
		fmt.Printf(", %d SKIPPED", counts.Skipped)
	}
	fmt.Println(")")
	fmt.Printf("Duration: %s\n", time.Duration(res.DurationMs)*time.Millisecond)

	if res.Outcome == suite.AggregateInvalid || res.Outcome == suite.AggregateInternalError {
		fmt.Printf("Error: %s\n", res.Error)
	}
}
