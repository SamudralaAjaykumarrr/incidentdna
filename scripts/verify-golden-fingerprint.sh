#!/usr/bin/env sh
# Builds the incidentdna binary (if not already built) and verifies that its
# `fingerprint` command reproduces the golden fingerprint recorded for
# examples/duplicate-payment/incident.yaml. This is a CLI-binary-level check,
# distinct from (and in addition to) the Go-level golden test in
# internal/fingerprint/golden_test.go — it exercises the whole load ->
# validate -> fingerprint path through the actual compiled binary.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/incidentdna"
EXAMPLE="$ROOT/examples/duplicate-payment/incident.yaml"
GOLDEN="$ROOT/testdata/golden/duplicate-payment.fingerprint"

if [ ! -x "$BIN" ]; then
  echo "verify-golden-fingerprint: building $BIN" >&2
  ( cd "$ROOT" && go build -o "$BIN" ./cmd/incidentdna )
fi

ACTUAL="$("$BIN" fingerprint "$EXAMPLE")"
EXPECTED="$(cat "$GOLDEN" | tr -d '[:space:]')"
ACTUAL_TRIMMED="$(printf '%s' "$ACTUAL" | tr -d '[:space:]')"

if [ "$ACTUAL_TRIMMED" != "$EXPECTED" ]; then
  echo "verify-golden-fingerprint: MISMATCH" >&2
  echo "  expected: $EXPECTED" >&2
  echo "  actual:   $ACTUAL_TRIMMED" >&2
  exit 1
fi

echo "verify-golden-fingerprint: OK ($ACTUAL_TRIMMED)"
