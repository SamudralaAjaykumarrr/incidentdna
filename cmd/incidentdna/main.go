// Command incidentdna validates, fingerprints, inspects, and compares IDIR
// (Incident Deterministic Intermediate Representation) documents.
//
// incidentdna performs no network access and no telemetry collection: every
// subcommand operates purely on local files named on the command line.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"
)

// Exit codes, documented in docs/architecture.md and relied on by CI.
const (
	exitOK              = 0
	exitError           = 1
	exitValidationError = 2
)

// commandTimeout bounds total execution of a single subcommand invocation.
// It exists as a resource-exhaustion backstop (see docs/threat-model.md):
// even a pathological input should not hang the process indefinitely.
const commandTimeout = 30 * time.Second

type command struct {
	name string
	run  func(ctx context.Context, args []string) int
}

var commands = []command{
	{"init", runInit},
	{"validate", runValidate},
	{"fingerprint", runFingerprint},
	{"inspect", runInspect},
	{"compare", runCompare},
	{"evidence", runEvidence},
	{"library", runLibrary},
	{"scenario", runScenario},
	{"suite", runSuite},
	{"policy", runPolicy},
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printUsage()
		return exitError
	}

	for _, c := range commands {
		if args[0] == c.name {
			ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
			defer cancel()
			// "scenario" and "suite" are special-cased: scenario run needs
			// its own, larger, document-declared timeout budget for the
			// child process it executes, up to MaxScenarioTimeoutSeconds,
			// and suite run needs its own budget for the (potentially many)
			// bounded child-process executions it performs in sequence, up
			// to suite.MaxSuiteTotalTimeoutSeconds — not the fixed
			// commandTimeout every other subcommand uses, which was sized
			// for parsing/hashing a document, a categorically smaller
			// workload (docs/phase-4-plan.md §10, docs/phase-5-plan.md
			// §13). runScenario/runSuite each apply commandTimeout
			// themselves for their own "verify" subcommand, which never
			// executes anything and keeps that budget unchanged.
			if c.name != "scenario" && c.name != "suite" {
				var cancelTimeout context.CancelFunc
				ctx, cancelTimeout = context.WithTimeout(ctx, commandTimeout)
				defer cancelTimeout()
			}
			return c.run(ctx, args[1:])
		}
	}

	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		printUsage()
		return exitOK
	}

	fmt.Fprintf(os.Stderr, "incidentdna: unknown command %q\n\n", args[0])
	printUsage()
	return exitError
}

func printUsage() {
	fmt.Fprint(os.Stderr, `incidentdna — production incident regression scenarios (IDIR)

Usage:
  incidentdna init [--force]
      Create a starter .incidentdna/incident.yaml in the current directory.
  incidentdna validate <file>
      Validate an IDIR document against the supported schema version and
      semantic rules.
  incidentdna fingerprint <file>
      Print the deterministic sha256: incident fingerprint of an IDIR
      document.
  incidentdna inspect <file>
      Print a human-readable summary of an IDIR document.
  incidentdna compare <file-a> <file-b>
      Report whether two IDIR documents share the same normalized incident
      fingerprint, and explain material differences if they do not.
  incidentdna evidence <store|verify|list|inspect> ...
      Local content-addressed evidence storage and digest verification.
      Run "incidentdna evidence help" for subcommand details.
  incidentdna library <add|check|list> ...
      Local incident library: persist validated incident occurrences and
      look up whether a candidate document's fingerprint already exists.
      Run "incidentdna library help" for subcommand details.
  incidentdna scenario <verify|run> ...
      Deterministic executable regression scenarios (IRS v0.1): verify a
      scenario document, or run its declared command in a bounded,
      offline, local workspace and compare the result against an
      expected outcome. Run "incidentdna scenario help" for subcommand
      details.
  incidentdna suite <verify|run> ...
      Scenario suite manifests (ISM v0.1): verify a suite manifest and
      every scenario it lists, or run them all sequentially, in declared
      order, and report the aggregate PASS/FAIL result. Run
      "incidentdna suite help" for subcommand details.
  incidentdna policy <verify|evaluate> ...
      Local policy evaluation (IGP v0.1): verify a policy document, or
      evaluate an already-produced scenario/suite report against one and
      report a PASS/FAIL verdict. Run "incidentdna policy help" for
      subcommand details.

incidentdna performs no network access and collects no telemetry.
`)
}
