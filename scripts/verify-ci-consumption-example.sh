#!/usr/bin/env sh
# Builds the incidentdna binary (if not already built) and exercises
# examples/ci-consumption-example/check-release-gate.sh end-to-end, proving
# it actually produces the documented exit codes (0 = PASS, 2 = FAIL,
# 1 = environmental failure) against real fixtures — the same "a demo
# that isn't tested isn't trustworthy" discipline every other
# scripts/verify-*-demo.sh already applies (docs/phase-8-plan.md §14).
#
# Composes already-existing, unmodified fixtures from
# examples/regression-suite-demo/ and policies/release-gate-example.yaml —
# no new fixture is introduced. Uses a single temporary directory for every
# generated report/library, so this never leaves generated output in the
# repository.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/incidentdna"
SUITE_DEMO="$ROOT/examples/regression-suite-demo"
RELEASE_GATE_POLICY="$ROOT/policies/release-gate-example.yaml"
DUPLICATE_PAYMENT="$ROOT/examples/duplicate-payment/incident.yaml"
CHECK_SCRIPT="$ROOT/examples/ci-consumption-example/check-release-gate.sh"

if [ ! -x "$BIN" ]; then
  echo "verify-ci-consumption-example: building $BIN" >&2
  ( cd "$ROOT" && go build -buildvcs=false -o "$BIN" ./cmd/incidentdna )
fi

TMP="$(mktemp -d)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

# --- Prepare a substituted copy of the suite demo, exactly as
#     verify-suite-demo.sh/verify-policy-demo.sh already do, so `suite run`
#     has a real, executable command[0]. ---

mkdir -p "$TMP/suite-demo"
cp -r "$SUITE_DEMO"/. "$TMP/suite-demo/"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$SUITE_DEMO/scenarios/scenario-pass.yaml" > "$TMP/suite-demo/scenarios/scenario-pass.yaml"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$SUITE_DEMO/scenarios/scenario-fail.yaml" > "$TMP/suite-demo/scenarios/scenario-fail.yaml"

# --- Seed a fresh temporary library with duplicate-payment/incident.yaml,
#     whose fingerprint is the one the suite demo's scenarios declare as
#     linked_fingerprint (same fixture verify-policy-demo.sh already
#     relies on) — required for release-gate-example.yaml's
#     require_library_occurrence rule to be satisfiable. ---

LIBRARY="$TMP/library"
echo "verify-ci-consumption-example: library add duplicate-payment/incident.yaml into a fresh temporary library" >&2
"$BIN" library add --library "$LIBRARY" --allow-unredacted "$DUPLICATE_PAYMENT" > /dev/null

# --- 1. PASS path: suite-all-pass.yaml + release-gate-example.yaml +
#        --library (expect exit 0). ---

echo "verify-ci-consumption-example: check-release-gate.sh against suite-all-pass.yaml (expect exit 0)" >&2
set +e
PASS_OUT="$(sh "$CHECK_SCRIPT" "$BIN" "$TMP/suite-demo/suite-all-pass.yaml" "$RELEASE_GATE_POLICY" "$LIBRARY" 2>&1)"
PASS_EXIT=$?
set -e
echo "$PASS_OUT"
if [ "$PASS_EXIT" -ne 0 ]; then
  echo "verify-ci-consumption-example: expected exit 0 for the PASS path, got $PASS_EXIT" >&2
  exit 1
fi
case "$PASS_OUT" in
  *"Verdict: PASS"*) ;;
  *)
    echo "verify-ci-consumption-example: expected Verdict: PASS in output" >&2
    exit 1
    ;;
esac

# --- 2. FAIL path: suite-mixed.yaml + release-gate-example.yaml +
#        --library (expect exit 2). ---

echo "verify-ci-consumption-example: check-release-gate.sh against suite-mixed.yaml (expect exit 2)" >&2
set +e
FAIL_OUT="$(sh "$CHECK_SCRIPT" "$BIN" "$TMP/suite-demo/suite-mixed.yaml" "$RELEASE_GATE_POLICY" "$LIBRARY" 2>&1)"
FAIL_EXIT=$?
set -e
echo "$FAIL_OUT"
if [ "$FAIL_EXIT" -ne 2 ]; then
  echo "verify-ci-consumption-example: expected exit 2 for the FAIL path, got $FAIL_EXIT" >&2
  exit 1
fi
case "$FAIL_OUT" in
  *"Verdict: FAIL"*) ;;
  *)
    echo "verify-ci-consumption-example: expected Verdict: FAIL in output" >&2
    exit 1
    ;;
esac

# --- 3. Environmental-failure path: a suite file that does not exist at
#        all, so `suite run` itself fails with exit 1 before any report is
#        written (expect check-release-gate.sh to exit 1). ---

echo "verify-ci-consumption-example: check-release-gate.sh against a nonexistent suite file (expect exit 1)" >&2
set +e
ENV_OUT="$(sh "$CHECK_SCRIPT" "$BIN" "$TMP/suite-demo/does-not-exist.yaml" "$RELEASE_GATE_POLICY" "$LIBRARY" 2>&1)"
ENV_EXIT=$?
set -e
echo "$ENV_OUT"
if [ "$ENV_EXIT" -ne 1 ]; then
  echo "verify-ci-consumption-example: expected exit 1 for the environmental-failure path, got $ENV_EXIT" >&2
  exit 1
fi

# --- 4. Usage check: wrong argument count exits 1 with a usage message. ---

echo "verify-ci-consumption-example: check-release-gate.sh with missing arguments (expect exit 1)" >&2
set +e
USAGE_OUT="$(sh "$CHECK_SCRIPT" "$BIN" 2>&1)"
USAGE_EXIT=$?
set -e
if [ "$USAGE_EXIT" -ne 1 ]; then
  echo "verify-ci-consumption-example: expected exit 1 for missing arguments, got $USAGE_EXIT" >&2
  exit 1
fi
case "$USAGE_OUT" in
  *"Usage:"*) ;;
  *)
    echo "verify-ci-consumption-example: expected a Usage: message" >&2
    exit 1
    ;;
esac

echo "verify-ci-consumption-example: OK"
