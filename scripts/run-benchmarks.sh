#!/usr/bin/env sh
# scripts/run-benchmarks.sh — runs the four Phase 8 informational benchmarks
# and records raw output (docs/phase-8-plan.md §13).
#
# Reads `go test -bench` output from stdin (Makefile's `bench` target pipes
# `go test -bench=. -benchmem -run=^$ ./...` straight into this script) and
# writes it verbatim to dist/incidentdna-$VERSION-bench.txt, then echoes it
# back to stdout so the caller still sees it live without this script
# depending on a writable /dev/stderr (not always available/writable —
# notably absent on Windows, and permission-denied in some sandboxed/
# container environments — so piping through `tee /dev/stderr` is not a
# portable way to view-while-capturing; echoing the already-buffered
# content after the fact achieves the same "show and save" result without
# writing to that device at all). No threshold, no pass/fail comparison
# against a prior run, no regression gate — this script's only job is "did
# it produce non-empty output," never anything about the numbers
# themselves. Benchmarks are informational only and never gate CI or a
# release (docs/phase-8-plan.md §13, enforced structurally: this script
# cannot fail on slow numbers because it never inspects them).
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
VERSION="${VERSION:-dev}"
OUT="$DIST/incidentdna-$VERSION-bench.txt"

mkdir -p "$DIST"

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT
cat > "$TMP"

if [ ! -s "$TMP" ]; then
  echo "run-benchmarks: no benchmark output received on stdin" >&2
  exit 1
fi

cp "$TMP" "$OUT"
cat "$TMP"
echo "run-benchmarks: wrote $OUT"
