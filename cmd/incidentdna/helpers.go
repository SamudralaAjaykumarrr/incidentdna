package main

import (
	"context"
	"fmt"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/validate"
)

// loadAndValidate loads path and runs the full semantic rule set against it.
// On a load (I/O or parse) error it prints to stderr and returns exitError.
// On a semantic validation failure it prints every issue and returns
// exitValidationError. On success it returns exitOK alongside the decoded
// document.
func loadAndValidate(ctx context.Context, path string) (*idir.Document, int) {
	doc, err := idir.LoadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return nil, exitError
	}

	res := validate.Validate(ctx, doc)
	if !res.Valid() {
		fmt.Fprintf(os.Stderr, "incidentdna: %s is not a valid IDIR document:\n", path)
		for _, issue := range res.Issues {
			fmt.Fprintf(os.Stderr, "  - %s\n", issue.String())
		}
		return nil, exitValidationError
	}

	return doc, exitOK
}
