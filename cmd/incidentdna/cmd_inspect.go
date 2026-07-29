package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/validate"
)

// runInspect prints a human-readable summary of an IDIR document. Unlike
// validate/fingerprint/compare, inspect does not require the document to be
// semantically valid — it is a diagnostic tool, so it still shows a summary
// (with a visible validity line) for a document that fails validation, to
// help a user see what's there while they fix it. It does require the
// document to structurally decode.
func runInspect(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna inspect <file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	doc, err := idir.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	res := validate.Validate(ctx, doc)

	var b strings.Builder
	fmt.Fprintf(&b, "Incident:     %s — %s\n", doc.Incident.ID, doc.Incident.Title)
	fmt.Fprintf(&b, "Schema:       %s\n", doc.SchemaVersion)
	if res.Valid() {
		fmt.Fprintf(&b, "Valid:        yes\n")
	} else {
		fmt.Fprintf(&b, "Valid:        no (%d issue(s))\n", len(res.Issues))
	}
	fmt.Fprintf(&b, "Occurred:     %s\n", nonEmpty(doc.Incident.OccurredAt))
	fmt.Fprintf(&b, "Application:  %s / %s (%s, release %s)\n", doc.Application.Name, doc.Application.Service, doc.Application.Environment, doc.Application.ReleaseVersion)
	fmt.Fprintf(&b, "\nTrigger:      [%s] %s\n", doc.Trigger.Type, strings.TrimSpace(doc.Trigger.Description))

	fmt.Fprintf(&b, "\nEvent timeline (%d):\n", len(doc.Events))
	for _, ev := range doc.Events {
		causes := "-"
		if len(ev.CausedBy) > 0 {
			causes = strings.Join(ev.CausedBy, ", ")
		}
		fmt.Fprintf(&b, "  [%s] %s (caused by: %s)\n", ev.ID, ev.Type, causes)
	}

	fmt.Fprintf(&b, "\nServices involved (%d):\n", len(doc.Services))
	for _, svc := range doc.Services {
		fmt.Fprintf(&b, "  %s — %s (%s)\n", svc.Name, svc.Role, svc.TechnologyCategory)
	}

	fmt.Fprintf(&b, "\nSide effects (%d):\n", len(doc.SideEffects))
	for _, se := range doc.SideEffects {
		fmt.Fprintf(&b, "  [%s] %s (reversible: %t)\n", se.Type, strings.TrimSpace(se.Description), se.Reversible)
	}

	fmt.Fprintf(&b, "\nViolated business invariants (%d):\n", len(doc.BusinessInvariants))
	for _, inv := range doc.BusinessInvariants {
		fmt.Fprintf(&b, "  - %s\n", strings.TrimSpace(inv.Statement))
	}

	fmt.Fprintf(&b, "\nExpected corrected behavior (%d):\n", len(doc.ExpectedCorrectedBehavior))
	for _, s := range doc.ExpectedCorrectedBehavior {
		fmt.Fprintf(&b, "  - %s\n", strings.TrimSpace(s))
	}

	fmt.Fprintf(&b, "\nEvidence:     %d item(s)\n", len(doc.Evidence))
	fmt.Fprintf(&b, "Repro steps:  %d step(s)\n", len(doc.ReproductionSequence))
	fmt.Fprintf(&b, "Privacy:      sensitivity=%s redacted=%t\n", doc.Privacy.Sensitivity, doc.Privacy.Redacted)

	if fp, err := fingerprint.Compute(doc); err == nil {
		fmt.Fprintf(&b, "\nFingerprint:  %s\n", fp)
	}

	fmt.Print(b.String())

	if !res.Valid() {
		fmt.Fprintln(os.Stderr, "\nValidation issues:")
		for _, issue := range res.Issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", issue.String())
		}
	}

	return exitOK
}

func nonEmpty(s string) string {
	if s == "" {
		return "(unspecified)"
	}
	return s
}
