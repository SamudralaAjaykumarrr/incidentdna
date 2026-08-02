#!/usr/bin/env sh
# Builds the incidentdna binary (if not already built) and exercises the
# full `incidentdna scenario` command surface end-to-end against the
# fictional examples/regression-scenario-demo/ example: structurally
# verifies all four checked-in scenario files (including the deliberately
# invalid one, which is expected to fail verify), cross-checks
# linked_fingerprint against the existing, unmodified duplicate-payment
# example via --source, then runs scenario run against all five outcomes
# (PASS, FAIL, TIMEOUT, INVALID, INTERNAL_ERROR), checking exit codes,
# printed summaries, and --report JSON content for each. This is a
# CLI-binary-level check, in the same spirit as verify-golden-fingerprint.sh,
# verify-evidence-demo.sh, and verify-library-demo.sh, but for the Phase 4
# regression-scenario runner instead of the fingerprint algorithm, the
# evidence store, or the incident library.
#
# scenario-pass.yaml and scenario-fail.yaml declare command[0] as the
# placeholder "/INCIDENTDNA_BIN_PLACEHOLDER" (see
# examples/regression-scenario-demo/README.md, "Why this demo uses an
# absolute command path") — this script substitutes it for the real,
# checkout-specific absolute path to the built binary, into a temporary copy
# of the demo directory, and runs `scenario run` against that copy.
# `scenario verify` (structural, and the --source cross-check) always runs
# against the original, checked-in, unmodified files.
#
# Uses a single temporary directory (created outside the repository via
# mktemp -d, removed on exit via the trap below) for every workspace/report
# path below, so this script never leaves generated output anywhere in the
# repository tree. See docs/regression-scenarios.md for the underlying
# design this exercises.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/incidentdna"
DEMO="$ROOT/examples/regression-scenario-demo"
SOURCE_INCIDENT="$ROOT/examples/duplicate-payment/incident.yaml"

if [ ! -x "$BIN" ]; then
  echo "verify-scenario-demo: building $BIN" >&2
  ( cd "$ROOT" && go build -buildvcs=false -o "$BIN" ./cmd/incidentdna )
fi

TMP="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP"
}
trap cleanup EXIT

# --- 1. scenario verify: structural validation against the checked-in,
#        unmodified files. Never executes anything. ---

echo "verify-scenario-demo: scenario verify scenario-pass.yaml (expect: valid, exit 0)" >&2
"$BIN" scenario verify "$DEMO/scenario-pass.yaml"

echo "verify-scenario-demo: scenario verify scenario-fail.yaml (expect: valid, exit 0)" >&2
"$BIN" scenario verify "$DEMO/scenario-fail.yaml"

echo "verify-scenario-demo: scenario verify scenario-timeout.yaml (expect: valid, exit 0)" >&2
"$BIN" scenario verify "$DEMO/scenario-timeout.yaml"

echo "verify-scenario-demo: scenario verify scenario-invalid.yaml (expect: INVALID at verify time, exit 2)" >&2
set +e
VERIFY_INVALID_OUT="$("$BIN" scenario verify "$DEMO/scenario-invalid.yaml" 2>&1)"
VERIFY_INVALID_EXIT=$?
set -e
echo "$VERIFY_INVALID_OUT"
if [ "$VERIFY_INVALID_EXIT" -ne 2 ]; then
  echo "verify-scenario-demo: expected exit 2 for scenario-invalid.yaml's structural verify, got $VERIFY_INVALID_EXIT" >&2
  exit 1
fi
case "$VERIFY_INVALID_OUT" in
  *"bare command name"*) ;;
  *)
    echo "verify-scenario-demo: expected scenario verify to explain the bare command[0] rejection" >&2
    exit 1
    ;;
esac

# --- 2. scenario verify --source: cross-checks linked_fingerprint against
#        the existing, unmodified Phase 1 example. ---

echo "verify-scenario-demo: scenario verify --source (expect: match, exit 0)" >&2
SOURCE_OUT="$("$BIN" scenario verify --source "$SOURCE_INCIDENT" "$DEMO/scenario-pass.yaml")"
echo "$SOURCE_OUT"
case "$SOURCE_OUT" in
  *"linked_fingerprint matches --source"*) ;;
  *)
    echo "verify-scenario-demo: expected the --source fingerprint match confirmation" >&2
    exit 1
    ;;
esac

# --- Prepare a substituted copy of the demo for `scenario run` ---

mkdir -p "$TMP/demo/fixtures"
cp "$DEMO/fixtures/incident.yaml" "$TMP/demo/fixtures/incident.yaml"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$DEMO/scenario-pass.yaml" > "$TMP/demo/scenario-pass.yaml"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$DEMO/scenario-fail.yaml" > "$TMP/demo/scenario-fail.yaml"

# --- 3. scenario run: PASS ---

echo "verify-scenario-demo: scenario run scenario-pass.yaml (expect: PASS, exit 0)" >&2
PASS_OUT="$("$BIN" scenario run --report "$TMP/pass-report.json" "$TMP/demo/scenario-pass.yaml")"
echo "$PASS_OUT"
case "$PASS_OUT" in
  *"Result: PASS"*) ;;
  *)
    echo "verify-scenario-demo: expected Result: PASS" >&2
    exit 1
    ;;
esac
case "$(cat "$TMP/pass-report.json")" in
  *'"result": "PASS"'*) ;;
  *)
    echo "verify-scenario-demo: expected --report content to show result PASS" >&2
    exit 1
    ;;
esac

# --- 4. scenario run: FAIL ---

echo "verify-scenario-demo: scenario run scenario-fail.yaml (expect: FAIL, exit 2)" >&2
set +e
FAIL_OUT="$("$BIN" scenario run --report "$TMP/fail-report.json" "$TMP/demo/scenario-fail.yaml")"
FAIL_EXIT=$?
set -e
echo "$FAIL_OUT"
if [ "$FAIL_EXIT" -ne 2 ]; then
  echo "verify-scenario-demo: expected exit 2 for FAIL, got $FAIL_EXIT" >&2
  exit 1
fi
case "$FAIL_OUT" in
  *"Result: FAIL"*) ;;
  *)
    echo "verify-scenario-demo: expected Result: FAIL" >&2
    exit 1
    ;;
esac

# --- 5. scenario run: TIMEOUT ---

echo "verify-scenario-demo: scenario run scenario-timeout.yaml (expect: TIMEOUT, exit 2, bounded wall-clock)" >&2
START=$(date +%s)
set +e
TIMEOUT_OUT="$("$BIN" scenario run --report "$TMP/timeout-report.json" "$DEMO/scenario-timeout.yaml")"
TIMEOUT_EXIT=$?
set -e
END=$(date +%s)
echo "$TIMEOUT_OUT"
if [ "$TIMEOUT_EXIT" -ne 2 ]; then
  echo "verify-scenario-demo: expected exit 2 for TIMEOUT, got $TIMEOUT_EXIT" >&2
  exit 1
fi
case "$TIMEOUT_OUT" in
  *"Result: TIMEOUT"*) ;;
  *)
    echo "verify-scenario-demo: expected Result: TIMEOUT" >&2
    exit 1
    ;;
esac
ELAPSED=$((END - START))
if [ "$ELAPSED" -gt 10 ]; then
  echo "verify-scenario-demo: expected the TIMEOUT run to complete well under 10s, took ${ELAPSED}s" >&2
  exit 1
fi

# --- 6. scenario run: INVALID ---

echo "verify-scenario-demo: scenario run scenario-invalid.yaml (expect: INVALID, exit 1)" >&2
set +e
INVALID_OUT="$("$BIN" scenario run --report "$TMP/invalid-report.json" "$DEMO/scenario-invalid.yaml")"
INVALID_EXIT=$?
set -e
echo "$INVALID_OUT"
if [ "$INVALID_EXIT" -ne 1 ]; then
  echo "verify-scenario-demo: expected exit 1 for INVALID, got $INVALID_EXIT" >&2
  exit 1
fi
case "$INVALID_OUT" in
  *"Result: INVALID"*) ;;
  *)
    echo "verify-scenario-demo: expected Result: INVALID" >&2
    exit 1
    ;;
esac

# --- 7. scenario run: INTERNAL_ERROR (nonexistent command[0]) ---

echo "verify-scenario-demo: scenario run with a nonexistent command[0] (expect: INTERNAL_ERROR, exit 1)" >&2
cat > "$TMP/scenario-internal-error.yaml" <<EOF
schema_version: irs/v0.1
scenario:
  id: internal-error-demo
linked_fingerprint: "sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e"
execution:
  command: ["/no/such/executable-xyz"]
expected:
  exit_code: 0
EOF
set +e
INTERNAL_ERROR_OUT="$("$BIN" scenario run --report "$TMP/internal-error-report.json" "$TMP/scenario-internal-error.yaml")"
INTERNAL_ERROR_EXIT=$?
set -e
echo "$INTERNAL_ERROR_OUT"
if [ "$INTERNAL_ERROR_EXIT" -ne 1 ]; then
  echo "verify-scenario-demo: expected exit 1 for INTERNAL_ERROR, got $INTERNAL_ERROR_EXIT" >&2
  exit 1
fi
case "$INTERNAL_ERROR_OUT" in
  *"Result: INTERNAL_ERROR"*) ;;
  *)
    echo "verify-scenario-demo: expected Result: INTERNAL_ERROR" >&2
    exit 1
    ;;
esac

# --- 8. --keep-workspace leaves a real, inspectable directory ---

echo "verify-scenario-demo: scenario run --keep-workspace (expect: workspace retained on disk)" >&2
KEEP_OUT="$("$BIN" scenario run --keep-workspace --workspace "$TMP/kept-workspace" "$TMP/demo/scenario-pass.yaml")"
echo "$KEEP_OUT"
case "$KEEP_OUT" in
  *"Workspace: kept at $TMP/kept-workspace"*) ;;
  *)
    echo "verify-scenario-demo: expected the printed summary to report the kept workspace path" >&2
    exit 1
    ;;
esac
if [ ! -f "$TMP/kept-workspace/incident.yaml" ]; then
  echo "verify-scenario-demo: expected the kept workspace to contain the staged incident.yaml" >&2
  exit 1
fi

echo "verify-scenario-demo: OK"
