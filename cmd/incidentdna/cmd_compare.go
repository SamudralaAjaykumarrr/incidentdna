package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/compare"
)

func runCompare(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna compare <file-a> <file-b>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 2 {
		fs.Usage()
		return exitError
	}
	pathA, pathB := fs.Arg(0), fs.Arg(1)

	docA, code := loadAndValidate(ctx, pathA)
	if code != exitOK {
		return code
	}
	docB, code := loadAndValidate(ctx, pathB)
	if code != exitOK {
		return code
	}

	res, err := compare.Documents(docA, docB)
	if err != nil {
		fmt.Fprintf(os.Stderr, "incidentdna: %v\n", err)
		return exitError
	}

	if res.Identical {
		fmt.Printf("IDENTICAL: %s and %s share the same normalized incident fingerprint\n", pathA, pathB)
		fmt.Println(res.FingerprintA)
		return exitOK
	}

	fmt.Printf("DIFFERENT: %s and %s do not represent the same normalized incident\n", pathA, pathB)
	fmt.Printf("  %s: %s\n", pathA, res.FingerprintA)
	fmt.Printf("  %s: %s\n", pathB, res.FingerprintB)
	fmt.Println("Material differences:")
	for _, d := range res.Differences {
		fmt.Printf("  - %s:\n      a: %s\n      b: %s\n", d.Dimension, d.A, d.B)
	}
	return exitOK
}
