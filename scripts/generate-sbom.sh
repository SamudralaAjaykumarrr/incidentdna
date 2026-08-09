#!/usr/bin/env sh
# scripts/generate-sbom.sh — minimal, dependency-free SBOM / dependency
# inventory (docs/phase-8-plan.md §12).
#
# Uses only `go list -m -json all` (a stdlib go command, reads the local
# go.sum-pinned module cache, no network request when the cache is already
# populated) and reshapes its output into the small, hand-specified
# "incidentdna-sbom/v1" JSON document the plan defines — not a full
# CycloneDX/SPDX document, since this module's entire dependency graph is
# two modules (gopkg.in/yaml.v3, direct; gopkg.in/check.v1, yaml.v3's own
# test-only transitive dependency).
#
# VERSION (env, default "dev") becomes the document's "generated_for"
# field, the same convention scripts/build-release.sh uses.
#
# Output: dist/incidentdna-$VERSION-sbom.json (deterministic field order,
# dependencies sorted by path — a fixed Go struct, never map iteration).
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
VERSION="${VERSION:-dev}"
OUT="$DIST/incidentdna-$VERSION-sbom.json"

mkdir -p "$DIST"

helper="$(mktemp -d)/gensbom.go"
cat > "$helper" <<'EOF'
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
)

type moduleJSON struct {
	Path     string `json:"Path"`
	Version  string `json:"Version"`
	Main     bool   `json:"Main"`
	Indirect bool   `json:"Indirect"`
}

type dependency struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Direct  bool   `json:"direct"`
}

type sbom struct {
	Format       string       `json:"format"`
	GeneratedFor string       `json:"generated_for"`
	GoVersion    string       `json:"go_version"`
	Module       string       `json:"module"`
	Dependencies []dependency `json:"dependencies"`
}

func main() {
	generatedFor := os.Args[1]

	dec := json.NewDecoder(os.Stdin)
	var module string
	var deps []dependency
	for {
		var m moduleJSON
		if err := dec.Decode(&m); err != nil {
			break
		}
		if m.Main {
			module = m.Path
			continue
		}
		deps = append(deps, dependency{
			Path:    m.Path,
			Version: m.Version,
			Direct:  !m.Indirect,
		})
	}

	sort.Slice(deps, func(i, j int) bool { return deps[i].Path < deps[j].Path })

	doc := sbom{
		Format:       "incidentdna-sbom/v1",
		GeneratedFor: generatedFor,
		GoVersion:    runtime.Version(),
		Module:       module,
		Dependencies: deps,
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
EOF

( cd "$ROOT" && go list -m -json all | go run "$helper" "$VERSION" > "$OUT" )

echo "generate-sbom: wrote $OUT"
cat "$OUT"
