package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint"
)

func runFingerprint(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("fingerprint", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna fingerprint <file>")
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

	fp, err := fingerprint.Compute(doc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}
	fmt.Println(fp)
	return exitOK
}
