package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/library"
)

// runLibrary dispatches to the three `incidentdna library` subcommands: add,
// check, list. See docs/phase-3-plan.md §7/§18 (Slice 6) for the approved
// CLI contract and docs/phase-3-plan.md §5 for the underlying store design.
func runLibrary(ctx context.Context, args []string) int {
	if len(args) == 0 {
		printLibraryUsage()
		return exitError
	}

	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		return runLibraryAdd(ctx, rest)
	case "check":
		return runLibraryCheck(ctx, rest)
	case "list":
		return runLibraryList(ctx, rest)
	case "-h", "--help", "help":
		printLibraryUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "incidentdna: unknown library subcommand %q\n\n", sub)
		printLibraryUsage()
		return exitError
	}
}

func printLibraryUsage() {
	fmt.Fprint(os.Stderr, `incidentdna library — local incident library: persist validated incident
occurrences and look up whether a candidate document's fingerprint already
exists

Usage:
  incidentdna library add [--library <dir>] [--allow-unredacted] <file>
      Validate and fingerprint <file>, enforce the privacy policy (require
      privacy.redacted == true unless --allow-unredacted is given, which
      always warns), and store it as an occurrence of its failure-class
      fingerprint. A new incident id under an existing fingerprint is stored
      as another occurrence; a repeated, formatting-only copy of an
      already-stored incident id is idempotent (nothing written); a
      materially changed copy of an already-stored incident id is a
      conflict and is refused, never silently overwritten.
  incidentdna library check [--library <dir>] <file>
      Validate and fingerprint <file>, and report whether the library
      already holds one or more occurrences with that fingerprint.
        exit 0 — one or more matching occurrences found
        exit 1 — invalid usage, invalid document, malformed library,
                 permission, corruption, resource-limit, or internal failure
        exit 2 — valid document, but no matching fingerprint exists
  incidentdna library list [--library <dir>]
      Enumerate the fingerprints currently in the library, bounded to:
      fingerprint, occurrence count, incident id, title, application/service,
      and occurred timestamp per occurrence. An empty library is a
      successful, empty result.

--library <path> overrides the library root; the default is
.incidentdna/library/objects relative to the current working directory.

incidentdna library never prints raw stored incident document contents. It
performs no network access and collects no telemetry.
`)
}

// libraryFlag registers the --library flag shared by all three library
// subcommands.
func libraryFlag(fs *flag.FlagSet) *string {
	return fs.String("library", "", "library root directory (default: .incidentdna/library/objects)")
}

// openLibraryStore resolves and opens the store, printing a consistent error
// and returning exitError on failure.
func openLibraryStore(libraryDir string) (*library.Store, int) {
	st, err := library.Open(libraryDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return nil, exitError
	}
	return st, exitOK
}

// loadDocumentForLibrary loads path through the existing IDIR file-loading
// boundary — the same idir.LoadFile every other command already uses,
// preserving the 5 MiB raw input-file limit and Phase 1 loading semantics
// unchanged (docs/phase-3-plan.md §11: "reused from internal/idir, not
// redefined"). It does not itself run validate.Validate: library.Add and
// library.Check each run that internally and report an invalid document as
// a wrapped library.ErrInvalidDocument, which the callers below map to exit
// 1 — the exit-2 code this binary uses elsewhere for "semantic validation
// failure" has a different meaning for `library check` (§7: "valid document,
// but no matching fingerprint exists"), so it must not be reused for an
// invalid document here.
func loadDocumentForLibrary(path string) (*idir.Document, int) {
	doc, err := idir.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return nil, exitError
	}
	return doc, exitOK
}

func runLibraryAdd(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("library add", flag.ContinueOnError)
	libDir := libraryFlag(fs)
	allowUnredacted := fs.Bool("allow-unredacted", false, "store the document even if it does not declare privacy.redacted == true (always warns)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna library add [--library <dir>] [--allow-unredacted] <file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	doc, code := loadDocumentForLibrary(path)
	if code != exitOK {
		return code
	}

	st, code := openLibraryStore(*libDir)
	if code != exitOK {
		return code
	}

	res, err := library.Add(ctx, st, doc, library.AddOptions{AllowUnredacted: *allowUnredacted})
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	// Warn only when the override was actually required to store this
	// document (res.PrivacyOverridden), never when the document already
	// declared privacy.redacted == true — docs/phase-3-plan.md §10/§12:
	// "always produces a visible warning — never a silent bypass," and never
	// a warning when there was nothing to bypass. The warning never includes
	// any part of the document's content.
	if res.PrivacyOverridden {
		fmt.Fprintln(os.Stderr, "warning: --allow-unredacted override was used: this document does not declare privacy.redacted == true; it was stored anyway")
	}

	switch res.Outcome {
	case library.AddOutcomeStored:
		fmt.Printf("Stored new occurrence: fingerprint %s (incident: %s)\n", res.Fingerprint, doc.Incident.ID)
	case library.AddOutcomeIdempotent:
		fmt.Printf("Already present (idempotent, nothing written): fingerprint %s (incident: %s)\n", res.Fingerprint, doc.Incident.ID)
	}
	return exitOK
}

func runLibraryCheck(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("library check", flag.ContinueOnError)
	libDir := libraryFlag(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna library check [--library <dir>] <file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	doc, code := loadDocumentForLibrary(path)
	if code != exitOK {
		return code
	}

	st, code := openLibraryStore(*libDir)
	if code != exitOK {
		return code
	}

	res, err := library.Check(ctx, st, doc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	fmt.Printf("Fingerprint: %s\n", res.Fingerprint)
	if res.Outcome == library.CheckOutcomeMatch {
		fmt.Printf("Match: %d occurrence(s) found for this fingerprint\n", res.MatchCount)
		return exitOK
	}
	fmt.Println("No match: no occurrence with this fingerprint exists in the library")
	// exitValidationError is numerically 2, the exit code docs/phase-3-plan.md
	// §7/§20.4 assigns to "valid document, but no matching fingerprint
	// exists" for `library check` specifically — a different meaning from
	// the same numeric code's use elsewhere in this binary, not the same
	// condition reused.
	return exitValidationError
}

func runLibraryList(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("library list", flag.ContinueOnError)
	libDir := libraryFlag(fs)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna library list [--library <dir>]")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return exitError
	}

	st, code := openLibraryStore(*libDir)
	if code != exitOK {
		return code
	}

	res, err := library.List(ctx, st)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	if len(res.Fingerprints) == 0 {
		fmt.Println("Library is empty: no incident occurrences stored yet.")
		return exitOK
	}

	for _, fp := range res.Fingerprints {
		fmt.Printf("Fingerprint: %s (%d occurrence(s))\n", fp.Fingerprint, fp.OccurrenceCount)
		for _, occ := range fp.Occurrences {
			fmt.Printf("  - incident: %s\n", occ.IncidentID)
			fmt.Printf("    title:    %s\n", occ.Title)
			fmt.Printf("    service:  %s\n", occ.Service)
			fmt.Printf("    occurred: %s\n", occ.OccurredAt)
		}
	}
	return exitOK
}
