package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/library"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/policy"
)

// runPolicy dispatches to the two `incidentdna policy` subcommands: verify,
// evaluate. See docs/phase-7-plan.md §10 for the approved CLI contract and
// docs/policy-evaluation.md for the underlying design.
//
// Unlike "scenario"/"suite", policy evaluate performs no process execution
// and no unbounded-duration operation of its own, so it needs no special
// case in main.go's command dispatch — both subcommands run under the
// existing fixed 30-second command context every other subcommand already
// uses (docs/phase-7-plan.md §10).
func runPolicy(ctx context.Context, args []string) int {
	if len(args) == 0 {
		printPolicyUsage()
		return exitError
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "verify":
		return runPolicyVerify(rest)
	case "evaluate":
		return runPolicyEvaluate(ctx, rest)
	case "-h", "--help", "help":
		printPolicyUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "incidentdna: unknown policy subcommand %q\n\n", sub)
		printPolicyUsage()
		return exitError
	}
}

func printPolicyUsage() {
	fmt.Fprint(os.Stderr, `incidentdna policy — local policy evaluation (IGP v0.1): verify a policy
document, or evaluate an already-produced scenario/suite report against one.

Usage:
  incidentdna policy verify <policy-file>
      Load and structurally/semantically validate <policy-file> against the
      IGP v0.1 rules (schema_version, non-empty rules, known rule type,
      required/forbidden value per rule type, MaxRulesPerPolicy). Never
      evaluates anything.
        exit 0 — policy document is structurally/semantically valid
        exit 1 — I/O/parse/usage error
        exit 2 — policy document decodes but fails a semantic IGP v0.1 rule
  incidentdna policy evaluate --policy <policy-file>
                               (--scenario-report <file> | --suite-report <file>)
                               [--library <dir>] [--report <file>]
      Run the same checks as policy verify. If they pass, load the named
      report file (a scenario run --report or suite run --report JSON file,
      unchanged Phase 4/5 shape), evaluate every declared rule against it —
      for require_library_occurrence, only if --library is given; otherwise
      that rule is reported SKIP and counted as not satisfied — print a
      per-rule and overall verdict line, and (if --report is given) write
      the same outcome as one deterministic JSON verdict report.
        Verdict PASS         exit 0
        Verdict FAIL         exit 2
        INVALID               exit 1 — the policy or report file failed
                               pre-evaluation checks; nothing was evaluated
        INTERNAL_ERROR         exit 1 — an environmental failure (e.g. a
                               malformed --library) prevented determining
                               the verdict

--library <path> reuses the same --library flag scenario verify/suite
verify already have; a require_library_occurrence rule is evaluated only
when this flag is given, otherwise it is reported SKIP (never silently
treated as satisfied).
--report <file> additionally writes a deterministic JSON verdict report,
overwritten unconditionally on each run.

incidentdna policy never calls exec.Command, never runs scenario.Run or
suite.Run, never mutates the incident library, performs no network access,
and collects no telemetry. It only reads local files named on the command
line.
`)
}

func runPolicyVerify(args []string) int {
	fs := flag.NewFlagSet("policy verify", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna policy verify <policy-file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	doc, err := policy.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	res := policy.Validate(doc)

	fmt.Printf("Policy: %s (schema %s)\n", doc.Policy.ID, doc.SchemaVersion)
	fmt.Printf("Rules: %d declared\n", len(doc.Rules))
	for _, rv := range res.Rules {
		printPolicyRuleValidationLine(rv)
	}

	if !res.Valid() {
		fmt.Fprintf(os.Stderr, "incidentdna: %s is not a valid IGP v0.1 policy:\n", path)
		for _, issue := range res.Issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", issue.String())
		}
		for _, rv := range res.Rules {
			for _, issue := range rv.Issues {
				fmt.Fprintf(os.Stderr, "  - %s\n", issue.String())
			}
		}
		return exitValidationError
	}

	fmt.Println("OK: policy is structurally valid")
	return exitOK
}

func printPolicyRuleValidationLine(rv policy.RuleValidation) {
	status := "OK"
	if !rv.Valid {
		status = "FAIL"
	}
	line := fmt.Sprintf("  [%s] %s", status, rv.Type)
	if rv.Value != "" {
		line += ": " + rv.Value
	}
	fmt.Println(line)
}

func runPolicyEvaluate(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("policy evaluate", flag.ContinueOnError)
	policyPath := fs.String("policy", "", "policy file to evaluate against")
	scenarioReportPath := fs.String("scenario-report", "", "scenario run --report JSON file to evaluate")
	suiteReportPath := fs.String("suite-report", "", "suite run --report JSON file to evaluate")
	libDir := libraryFlag(fs)
	reportOut := fs.String("report", "", "write a deterministic JSON verdict report to this path")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna policy evaluate --policy <policy-file> (--scenario-report <file> | --suite-report <file>) [--library <dir>] [--report <file>]")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return exitError
	}

	if *policyPath == "" {
		fmt.Fprintln(os.Stderr, "incidentdna: --policy is required")
		fs.Usage()
		return exitError
	}
	if (*scenarioReportPath == "") == (*suiteReportPath == "") {
		fmt.Fprintln(os.Stderr, "incidentdna: exactly one of --scenario-report or --suite-report is required")
		fs.Usage()
		return exitError
	}

	// libraryGiven distinguishes "--library was not passed at all" (every
	// require_library_occurrence rule is reported SKIP) from "--library was
	// explicitly passed" (even with an empty value, which resolves to
	// library.DefaultLibraryRoot) — mirrors runScenarioVerify's/
	// runSuiteVerify's identical Phase 6 handling.
	libraryGiven := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "library" {
			libraryGiven = true
		}
	})

	doc, err := policy.LoadFile(*policyPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return writePolicyOutcomeReport(*reportOut, "INVALID", "", *policyPath, "", "", err.Error())
	}

	res := policy.Validate(doc)
	if !res.Valid() {
		fmt.Fprintf(os.Stderr, "incidentdna: %s is not a valid IGP v0.1 policy:\n", *policyPath)
		for _, issue := range res.Issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", issue.String())
		}
		for _, rv := range res.Rules {
			for _, issue := range rv.Issues {
				fmt.Fprintf(os.Stderr, "  - %s\n", issue.String())
			}
		}
		return writePolicyOutcomeReport(*reportOut, "INVALID", doc.Policy.ID, *policyPath, "", "", "policy document is not a valid IGP v0.1 policy")
	}

	var loaded policy.LoadedReport
	var reportKind, reportFile string
	if *scenarioReportPath != "" {
		reportKind, reportFile = "scenario", *scenarioReportPath
		loaded, err = policy.LoadScenarioReport(reportFile)
	} else {
		reportKind, reportFile = "suite", *suiteReportPath
		loaded, err = policy.LoadSuiteReport(reportFile)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return writePolicyOutcomeReport(*reportOut, "INVALID", doc.Policy.ID, *policyPath, reportKind, reportFile, err.Error())
	}

	fmt.Printf("Policy: %s (schema %s)\n", doc.Policy.ID, doc.SchemaVersion)
	fmt.Printf("Input: %s report (result: %s), %d distinct linked fingerprint(s)\n", reportKind, loaded.Result, len(loaded.DistinctFingerprints))

	var lookups policy.LibraryLookups
	if libraryGiven {
		lookups, err = buildPolicyLibraryLookups(ctx, *libDir, loaded.DistinctFingerprints)
		if err != nil {
			fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
			return writePolicyOutcomeReport(*reportOut, "INTERNAL_ERROR", doc.Policy.ID, *policyPath, reportKind, reportFile, err.Error())
		}
	}

	verdict, err := policy.Evaluate(doc, loaded, lookups)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return writePolicyOutcomeReport(*reportOut, "INTERNAL_ERROR", doc.Policy.ID, *policyPath, reportKind, reportFile, err.Error())
	}

	for _, ro := range verdict.Rules {
		printPolicyRuleOutcomeLine(ro)
	}
	fmt.Printf("Verdict: %s\n", verdict.Result)

	code := exitOK
	if verdict.Result == policy.VerdictFail {
		code = exitValidationError
	}

	if *reportOut != "" {
		rep := policy.BuildReport(doc.Policy.ID, *policyPath, reportKind, reportFile, verdict.Result, verdict.Rules, "")
		if werr := policy.WriteReport(*reportOut, rep); werr != nil {
			fmt.Fprintf(os.Stderr, "incidentdna: writing --report %s: %v\n", *reportOut, werr)
			// The verdict was already printed above; the process exit code
			// still reflects that the requested report could not be
			// produced (mirrors scenario run's/suite run's --report
			// failure handling exactly).
			return exitError
		}
	}

	return code
}

func printPolicyRuleOutcomeLine(ro policy.RuleOutcome) {
	if ro.Value != "" {
		fmt.Printf("  [%s] %s: %s (%s)\n", ro.Status, ro.Type, ro.Value, ro.Detail)
	} else {
		fmt.Printf("  [%s] %s: %s\n", ro.Status, ro.Type, ro.Detail)
	}
}

// writePolicyOutcomeReport writes a best-effort --report (if reportPath is
// non-empty) for the INVALID/INTERNAL_ERROR pre-evaluation-failure paths —
// so a caller scripting against --report output always finds a report at
// the path it named, never only sometimes, mirroring scenario run's/suite
// run's identical best-effort --report behavior on their own INVALID/
// INTERNAL_ERROR paths — then returns exitError (1), the exit code §11
// assigns to both INVALID and INTERNAL_ERROR alike.
func writePolicyOutcomeReport(reportPath, outcome, policyID, policyFile, reportKind, reportFile, errMsg string) int {
	if reportPath != "" {
		rep := policy.BuildReport(policyID, policyFile, reportKind, reportFile, outcome, nil, errMsg)
		if werr := policy.WriteReport(reportPath, rep); werr != nil {
			fmt.Fprintf(os.Stderr, "incidentdna: writing --report %s: %v\n", reportPath, werr)
		}
	}
	return exitError
}

// buildPolicyLibraryLookups looks up every distinct fingerprint in the
// named (or default) incident library, once each, and returns the result as
// a policy.LibraryLookups map. This is the cmd/incidentdna-layer
// composition docs/phase-7-plan.md §2 goal 6 requires: internal/policy never
// opens a library store itself, mirroring cmd_scenario.go's/cmd_suite.go's
// existing Phase 6 library.CheckFingerprint composition exactly.
func buildPolicyLibraryLookups(ctx context.Context, libDir string, fingerprints []string) (policy.LibraryLookups, error) {
	st, err := library.Open(libDir)
	if err != nil {
		return nil, err
	}

	lookups := make(policy.LibraryLookups, len(fingerprints))
	for _, f := range fingerprints {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("library cross-reference canceled: %w", err)
		}
		res, err := library.CheckFingerprint(ctx, st, f)
		if err != nil {
			return nil, err
		}
		lookups[f] = res.Outcome == library.CheckOutcomeMatch
	}
	return lookups, nil
}
