package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

func runValidate(ctx context.Context, args []string) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: incidentdna validate <file>")
	}
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitError
	}
	path := fs.Arg(0)

	_, code := loadAndValidate(ctx, path)
	if code != exitOK {
		return code
	}
	fmt.Printf("OK: %s is a valid IDIR v0.1 document\n", path)
	return exitOK
}
