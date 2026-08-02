#!/usr/bin/env sh
# Builds the incidentdna binary (if not already built) and exercises the
# full `incidentdna suite` command surface end-to-end against the fictional
# examples/regression-suite-demo/ example: structurally verifies both
# checked-in suite manifests (and every scenario each one lists), then runs
# `suite run` against both aggregate outcomes (PASS, FAIL), --fail-fast
# short-circuiting, aggregate INVALID (a suite listing one structurally
# invalid scenario), --keep-workspaces, and --workspace-root — checking exit
# codes, printed summaries, and --report JSON content throughout. This is a
# CLI-binary-level check, in the same spirit as verify-golden-fingerprint.sh,
# verify-evidence-demo.sh, verify-library-demo.sh, and
# verify-scenario-demo.sh, but for the Phase 5 suite runner.
#
# scenario-pass.yaml and scenario-fail.yaml (under scenarios/) declare
# command[0] as the placeholder "/INCIDENTDNA_BIN_PLACEHOLDER" (see
# examples/regression-suite-demo/README.md, "Why this demo uses an absolute
# command path") — this script substitutes it for the real,
# checkout-specific absolute path to the built binary, into a temporary copy
# of the whole demo directory, and runs `suite run` against that copy.
# `suite verify` (structural validation only) always runs against the
# original, checked-in, unmodified files.
#
# Uses a single temporary directory (created outside the repository via
# mktemp -d, removed on exit via the trap below) for every
# --workspace-root/--report path below, so this script never leaves
# generated output anywhere in the repository tree. See
# docs/scenario-suites.md for the underlying design this exercises.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/incidentdna"
DEMO="$ROOT/examples/regression-suite-demo"

if [ ! -x "$BIN" ]; then
  echo "verify-suite-demo: building $BIN" >&2
  ( cd "$ROOT" && go build -buildvcs=false -o "$BIN" ./cmd/incidentdna )
fi

TMP="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP"
}
trap cleanup EXIT

# --- 1. suite verify: structural validation against the checked-in,
#        unmodified files. Never executes anything. ---

echo "verify-suite-demo: suite verify suite-all-pass.yaml (expect: valid, exit 0)" >&2
ALL_PASS_VERIFY_OUT="$("$BIN" suite verify "$DEMO/suite-all-pass.yaml")"
echo "$ALL_PASS_VERIFY_OUT"
case "$ALL_PASS_VERIFY_OUT" in
  *"OK: suite manifest and all 2 listed scenarios are structurally valid"*) ;;
  *)
    echo "verify-suite-demo: expected suite-all-pass.yaml to verify as valid with 2 scenarios" >&2
    exit 1
    ;;
esac

echo "verify-suite-demo: suite verify suite-mixed.yaml (expect: valid, exit 0)" >&2
MIXED_VERIFY_OUT="$("$BIN" suite verify "$DEMO/suite-mixed.yaml")"
echo "$MIXED_VERIFY_OUT"
case "$MIXED_VERIFY_OUT" in
  *"OK: suite manifest and all 3 listed scenarios are structurally valid"*) ;;
  *)
    echo "verify-suite-demo: expected suite-mixed.yaml to verify as valid with 3 scenarios" >&2
    exit 1
    ;;
esac

# --- Prepare a substituted copy of the whole demo for `suite run` ---

mkdir -p "$TMP/demo"
cp -r "$DEMO"/. "$TMP/demo/"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$DEMO/scenarios/scenario-pass.yaml" > "$TMP/demo/scenarios/scenario-pass.yaml"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$DEMO/scenarios/scenario-fail.yaml" > "$TMP/demo/scenarios/scenario-fail.yaml"

# --- 2. suite run: aggregate PASS ---

echo "verify-suite-demo: suite run suite-all-pass.yaml (expect: aggregate PASS, exit 0)" >&2
PASS_OUT="$("$BIN" suite run --report "$TMP/all-pass-report.json" "$TMP/demo/suite-all-pass.yaml")"
echo "$PASS_OUT"
case "$PASS_OUT" in
  *"Result: PASS (2 PASS, 0 FAIL, 0 TIMEOUT, 0 INVALID, 0 INTERNAL_ERROR)"*) ;;
  *)
    echo "verify-suite-demo: expected aggregate PASS with 2 PASS counts" >&2
    exit 1
    ;;
esac
case "$(cat "$TMP/all-pass-report.json")" in
  *'"result": "PASS"'*) ;;
  *)
    echo "verify-suite-demo: expected --report content to show result PASS" >&2
    exit 1
    ;;
esac

# --- 3. suite run: aggregate FAIL ---

echo "verify-suite-demo: suite run suite-mixed.yaml (expect: aggregate FAIL, exit 2)" >&2
set +e
MIXED_OUT="$("$BIN" suite run --report "$TMP/mixed-report.json" "$TMP/demo/suite-mixed.yaml")"
MIXED_EXIT=$?
set -e
echo "$MIXED_OUT"
if [ "$MIXED_EXIT" -ne 2 ]; then
  echo "verify-suite-demo: expected exit 2 for aggregate FAIL, got $MIXED_EXIT" >&2
  exit 1
fi
case "$MIXED_OUT" in
  *"Result: FAIL (1 PASS, 1 FAIL, 1 TIMEOUT, 0 INVALID, 0 INTERNAL_ERROR)"*) ;;
  *)
    echo "verify-suite-demo: expected aggregate FAIL with 1 PASS, 1 FAIL, 1 TIMEOUT counts" >&2
    exit 1
    ;;
esac
case "$(cat "$TMP/mixed-report.json")" in
  *'"result": "FAIL"'*) ;;
  *)
    echo "verify-suite-demo: expected --report content to show result FAIL" >&2
    exit 1
    ;;
esac

# --- 4. suite run --fail-fast: early termination, SKIPPED entries ---

echo "verify-suite-demo: suite run --fail-fast suite-mixed.yaml (expect: aggregate FAIL, exit 2, one SKIPPED)" >&2
set +e
FAILFAST_OUT="$("$BIN" suite run --fail-fast --report "$TMP/fail-fast-report.json" "$TMP/demo/suite-mixed.yaml")"
FAILFAST_EXIT=$?
set -e
echo "$FAILFAST_OUT"
if [ "$FAILFAST_EXIT" -ne 2 ]; then
  echo "verify-suite-demo: expected exit 2 for --fail-fast aggregate FAIL, got $FAILFAST_EXIT" >&2
  exit 1
fi
case "$FAILFAST_OUT" in
  *"SKIPPED"*) ;;
  *)
    echo "verify-suite-demo: expected --fail-fast to report a SKIPPED scenario" >&2
    exit 1
    ;;
esac
case "$(cat "$TMP/fail-fast-report.json")" in
  *'"skipped": 1'*) ;;
  *)
    echo "verify-suite-demo: expected --report content to show skipped: 1" >&2
    exit 1
    ;;
esac

# --- 5. suite verify / suite run: aggregate INVALID (a suite listing one
#        structurally invalid scenario is rejected as a whole; see
#        examples/regression-suite-demo/README.md, "Demonstrating aggregate
#        INVALID") ---

cat > "$TMP/suite-invalid-listed.yaml" <<EOF
schema_version: suite/v0.1
suite:
  id: suite-demo-invalid-listed
scenarios:
  - path: scenario-invalid.yaml
EOF
cp "$DEMO/scenarios/scenario-invalid.yaml" "$TMP/scenario-invalid.yaml"

echo "verify-suite-demo: suite verify suite-invalid-listed.yaml (expect: exit 2)" >&2
set +e
INVALID_VERIFY_OUT="$("$BIN" suite verify "$TMP/suite-invalid-listed.yaml" 2>&1)"
INVALID_VERIFY_EXIT=$?
set -e
echo "$INVALID_VERIFY_OUT"
if [ "$INVALID_VERIFY_EXIT" -ne 2 ]; then
  echo "verify-suite-demo: expected exit 2 for a suite listing an invalid scenario, got $INVALID_VERIFY_EXIT" >&2
  exit 1
fi
case "$INVALID_VERIFY_OUT" in
  *"[FAIL]"*) ;;
  *)
    echo "verify-suite-demo: expected a [FAIL] line for the invalid listed scenario" >&2
    exit 1
    ;;
esac

echo "verify-suite-demo: suite run suite-invalid-listed.yaml (expect: aggregate INVALID, exit 1, nothing executed)" >&2
set +e
INVALID_RUN_OUT="$("$BIN" suite run --report "$TMP/invalid-report.json" "$TMP/suite-invalid-listed.yaml")"
INVALID_RUN_EXIT=$?
set -e
echo "$INVALID_RUN_OUT"
if [ "$INVALID_RUN_EXIT" -ne 1 ]; then
  echo "verify-suite-demo: expected exit 1 for aggregate INVALID, got $INVALID_RUN_EXIT" >&2
  exit 1
fi
case "$INVALID_RUN_OUT" in
  *"Result: INVALID"*) ;;
  *)
    echo "verify-suite-demo: expected Result: INVALID" >&2
    exit 1
    ;;
esac
case "$(cat "$TMP/invalid-report.json")" in
  *'"result": "INVALID"'*) ;;
  *)
    echo "verify-suite-demo: expected --report content to show result INVALID" >&2
    exit 1
    ;;
esac

# --- 6. --keep-workspaces / --workspace-root: leaves real, inspectable
#        per-scenario directories ---

echo "verify-suite-demo: suite run --workspace-root --keep-workspaces (expect: per-scenario directories retained on disk)" >&2
"$BIN" suite run --workspace-root "$TMP/kept-workspaces" --keep-workspaces "$TMP/demo/suite-all-pass.yaml" > /dev/null
KEPT_COUNT=$(find "$TMP/kept-workspaces" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
if [ "$KEPT_COUNT" -ne 2 ]; then
  echo "verify-suite-demo: expected 2 kept per-scenario workspace directories under --workspace-root, found $KEPT_COUNT" >&2
  exit 1
fi

# --- 7. A pre-existing, non-empty --workspace-root is refused ---

echo "verify-suite-demo: suite run against a pre-existing non-empty --workspace-root (expect: aggregate INTERNAL_ERROR, exit 1)" >&2
mkdir -p "$TMP/nonempty-root"
: > "$TMP/nonempty-root/pre-existing.txt"
set +e
NONEMPTY_OUT="$("$BIN" suite run --workspace-root "$TMP/nonempty-root" "$TMP/demo/suite-all-pass.yaml")"
NONEMPTY_EXIT=$?
set -e
echo "$NONEMPTY_OUT"
if [ "$NONEMPTY_EXIT" -ne 1 ]; then
  echo "verify-suite-demo: expected exit 1 for a pre-existing non-empty --workspace-root, got $NONEMPTY_EXIT" >&2
  exit 1
fi
case "$NONEMPTY_OUT" in
  *"Result: INTERNAL_ERROR"*) ;;
  *)
    echo "verify-suite-demo: expected Result: INTERNAL_ERROR" >&2
    exit 1
    ;;
esac

echo "verify-suite-demo: OK"
