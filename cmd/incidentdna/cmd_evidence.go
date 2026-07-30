package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/evidence"
)

// runEvidence dispatches to the four `incidentdna evidence` subcommands:
// store, verify, list, inspect. See docs/phase-2-plan.md §11 for the
// approved CLI contract and docs/evidence-storage.md for the underlying
// design.
func runEvidence(ctx context.Context, args []string) int {
	if len(args) == 0 {
		printEvidenceUsage()
		return exitError
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "store":
		return runEvidenceStore(ctx, rest)
	case "verify":
		return runEvidenceVerify(ctx, rest)
	case "list":
		return runEvidenceList(ctx, rest)
	case "inspect":
		return runEvidenceInspect(ctx, rest)
	case "-h", "--help", "help":
		printEvidenceUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "incidentdna: unknown evidence subcommand %q\n\n", sub)
		printEvidenceUsage()
		return exitError
	}
}

func printEvidenceUsage() {
	fmt.Fprint(os.Stderr, `incidentdna evidence — local content-addressed evidence storage and verification

Usage:
  incidentdna evidence store [--store <directory>] <evidence-file>
      Compute the SHA-256 digest of <evidence-file> and store its bytes in
      the local evidence store. Deduplicates identical content. Never
      modifies any incident document.
  incidentdna evidence verify [--store <directory>] <incident-file>
      Check every evidence[].digest declared in <incident-file> against the
      local evidence store: presence and full content-integrity re-hash.
  incidentdna evidence list [--store <directory>] <incident-file>
      Show each evidence[] entry declared in <incident-file> and whether an
      object is present in the local evidence store (a presence check only —
      not a content-integrity guarantee; use verify for that).
  incidentdna evidence inspect [--store <directory>] <digest>
      Report metadata about one stored object addressed directly by digest:
      presence, size, and integrity. Never prints evidence content.

--store <directory> overrides the evidence store root; the default is
.incidentdna/evidence/objects relative to the current working directory.

incidentdna evidence performs no network access and collects no telemetry.
`)
}

// evidenceStoreFlag registers the --store flag shared by all four evidence
// subcommands.
func evidenceStoreFlag(fs *flag.FlagSet) *string {
	return fs.String("store", "", "evidence store root directory (default: .incidentdna/evidence/objects)")
}

// openEvidenceStore resolves and opens the store, printing a consistent
// error and returning exitError on failure. The store root is resolved
// exactly once per command invocation (see docs/phase-2-plan.md §6) — every
// subcommand below calls this exactly once.
func openEvidenceStore(storeDir string) (*evidence.Store, int) {
	st, err := evidence.Open(storeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return nil, exitError
	}
	return st, exitOK
}

func runEvidenceStore(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("evidence store", flag.ContinueOnError)
	storeDir := evidenceStoreFlag(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna evidence store [--store <directory>] <evidence-file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	source := fs.Arg(0)

	st, code := openEvidenceStore(*storeDir)
	if code != exitOK {
		return code
	}

	res, err := st.Put(ctx, source)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	if res.AlreadyPresent {
		fmt.Printf("Already present: %s (%d bytes; existing stored content re-verified, not overwritten)\n\n", res.Digest, res.Size)
	} else {
		fmt.Printf("Stored: %s (%d bytes)\n\n", res.Digest, res.Size)
	}
	fmt.Println("To reference this evidence in an IDIR document, add an entry under evidence[]:")
	fmt.Println("  - id: <choose-an-id>")
	fmt.Println("    type: <choose-a-type, e.g. log>")
	fmt.Println("    location: <optional free-text description>")
	fmt.Printf("    digest: %q\n\n", res.Digest)
	fmt.Println("incidentdna evidence store never modifies incident documents automatically.")
	return exitOK
}

func runEvidenceVerify(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("evidence verify", flag.ContinueOnError)
	storeDir := evidenceStoreFlag(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna evidence verify [--store <directory>] <incident-file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	doc, code := loadAndValidate(ctx, path)
	if code != exitOK {
		return code
	}

	st, code := openEvidenceStore(*storeDir)
	if code != exitOK {
		return code
	}

	res, err := st.Verify(ctx, doc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	fmt.Printf("Evidence verification for %s (store: %s)\n", path, st.Root())
	for _, e := range res.Entries {
		fmt.Printf("  [%s] %s: %s\n", e.ID, humanEvidenceStatus(e.Status), e.Digest)
		if e.Detail != "" {
			fmt.Printf("      %s\n", e.Detail)
		}
	}

	if res.AllOK() {
		fmt.Printf("OK: all %d evidence entries verified\n", len(res.Entries))
		return exitOK
	}

	fmt.Fprintf(os.Stderr, "incidentdna: evidence verification failed for %s\n", path)
	return exitValidationError
}

func runEvidenceList(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("evidence list", flag.ContinueOnError)
	storeDir := evidenceStoreFlag(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna evidence list [--store <directory>] <incident-file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	doc, code := loadAndValidate(ctx, path)
	if code != exitOK {
		return code
	}

	st, code := openEvidenceStore(*storeDir)
	if code != exitOK {
		return code
	}

	res, err := st.List(ctx, doc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	fmt.Printf("Evidence for %s (%d entries, store: %s)\n", path, len(res.Entries), st.Root())
	fmt.Printf("Document sensitivity: %s (declared, not independently verified)\n\n", nonEmpty(doc.Privacy.Sensitivity))
	for _, e := range res.Entries {
		fmt.Printf("  [%s] %s — %s\n", e.ID, e.Type, humanEvidenceStatus(e.Status))
		fmt.Printf("      digest: %s\n", e.Digest)
		if e.Status == evidence.StatusPresent {
			fmt.Printf("      size:   %d bytes\n", e.Size)
		}
		if e.Detail != "" {
			fmt.Printf("      %s\n", e.Detail)
		}
	}
	fmt.Println()
	fmt.Println(`List shows local presence only, not content integrity. A "present" entry` +
		` has not been re-hashed — run "incidentdna evidence verify" to confirm stored` +
		` evidence has not been modified, corrupted, or truncated.`)

	// list is purely informational once the document itself is valid: an
	// evidence object being absent locally is an expected, non-error state
	// during authoring, not a command failure. See docs/phase-2-plan.md §11.
	return exitOK
}

func runEvidenceInspect(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("evidence inspect", flag.ContinueOnError)
	storeDir := evidenceStoreFlag(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna evidence inspect [--store <directory>] <digest>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	digest := fs.Arg(0)

	st, code := openEvidenceStore(*storeDir)
	if code != exitOK {
		return code
	}

	res, err := st.Inspect(ctx, digest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	fmt.Printf("Digest:    %s\n", res.Digest)
	fmt.Printf("Algorithm: %s\n", evidence.Algorithm)
	fmt.Printf("Status:    %s\n", humanEvidenceStatus(res.Status))
	if res.Size > 0 || res.Status == evidence.StatusOK {
		fmt.Printf("Size:      %d bytes\n", res.Size)
	}
	if res.Detail != "" {
		fmt.Printf("Detail:    %s\n", res.Detail)
	}

	if res.Status == evidence.StatusOK {
		return exitOK
	}
	return exitValidationError
}

// humanEvidenceStatus renders an evidence.EntryStatus for CLI output,
// deliberately worded so a "present" (List) result is never confused with a
// re-hashed, integrity-verified "OK" (Verify/Inspect) result.
func humanEvidenceStatus(s evidence.EntryStatus) string {
	switch s {
	case evidence.StatusOK:
		return "OK (integrity verified)"
	case evidence.StatusPresent:
		return "present (not verified)"
	case evidence.StatusMissing:
		return "MISSING"
	case evidence.StatusCorrupted:
		return "CORRUPTED"
	case evidence.StatusInvalid:
		return "INVALID DIGEST"
	case evidence.StatusError:
		return "ERROR"
	default:
		return string(s)
	}
}
