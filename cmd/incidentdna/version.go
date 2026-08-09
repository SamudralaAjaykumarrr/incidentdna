package main

import (
	"context"
	"fmt"
)

// version is the build's version string. It is "dev" for every ordinary
// build (including `make build`) and is overridden only via
// `-ldflags "-X main.version=$VERSION"` at release-build time
// (scripts/build-release.sh). It is never derived from git, the working
// tree, or any other VCS-sourced signal at build or run time — the same
// determinism discipline `-buildvcs=false` already establishes for the rest
// of the build (see docs/release-process.md).
var version = "dev"

// runVersion implements `incidentdna version`. It never fails: it prints
// the build's version string to stdout and always returns exitOK.
func runVersion(_ context.Context, _ []string) int {
	fmt.Printf("incidentdna %s\n", version)
	return exitOK
}
