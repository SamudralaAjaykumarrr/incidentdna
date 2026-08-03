#!/usr/bin/env sh
# Builds the incidentdna binary (if not already built) and exercises the
# Phase 7 `incidentdna policy verify|evaluate` commands end-to-end. Like
# Phase 6, this introduces no new example directory — it composes
# already-existing, unmodified fixtures from examples/duplicate-payment/,
# examples/regression-scenario-demo/, and examples/regression-suite-demo/,
# plus the one new checked-in policies/release-gate-example.yaml, the same
# "pure composition of already-existing pieces" discipline
# scripts/verify-library-crossref-demo.sh already established.
#
# Uses a single temporary directory (created outside the repository via
# mktemp -d, removed on exit via the trap below) for every generated report,
# --library directory, and second (library-independent) policy file below —
# the default library root (.incidentdna/library/objects) is never invoked,
# so this script never creates, and never leaves behind, a .incidentdna
# directory anywhere in the repository tree. See docs/policy-evaluation.md
# for the underlying design this exercises.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/incidentdna"
DUPLICATE_PAYMENT="$ROOT/examples/duplicate-payment/incident.yaml"
SCENARIO_DEMO="$ROOT/examples/regression-scenario-demo"
SUITE_DEMO="$ROOT/examples/regression-suite-demo"
RELEASE_GATE_POLICY="$ROOT/policies/release-gate-example.yaml"

if [ ! -x "$BIN" ]; then
  echo "verify-policy-demo: building $BIN" >&2
  ( cd "$ROOT" && go build -buildvcs=false -o "$BIN" ./cmd/incidentdna )
fi

TMP="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP"
}
trap cleanup EXIT

# --- Prepare substituted copies of the scenario/suite demos, exactly as
#     verify-scenario-demo.sh/verify-suite-demo.sh already do, so `scenario
#     run`/`suite run` have a real, executable command[0]. ---

mkdir -p "$TMP/scenario-demo/fixtures"
cp "$SCENARIO_DEMO/fixtures/incident.yaml" "$TMP/scenario-demo/fixtures/incident.yaml"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$SCENARIO_DEMO/scenario-pass.yaml" > "$TMP/scenario-demo/scenario-pass.yaml"

mkdir -p "$TMP/suite-demo"
cp -r "$SUITE_DEMO"/. "$TMP/suite-demo/"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$SUITE_DEMO/scenarios/scenario-pass.yaml" > "$TMP/suite-demo/scenarios/scenario-pass.yaml"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$SUITE_DEMO/scenarios/scenario-fail.yaml" > "$TMP/suite-demo/scenarios/scenario-fail.yaml"

# --- 1. Generate real reports from the already-existing, unmodified
#        scenario/suite fixtures. ---

echo "verify-policy-demo: generating real scenario/suite reports" >&2
"$BIN" scenario run --report "$TMP/scenario-pass-report.json" "$TMP/scenario-demo/scenario-pass.yaml" > /dev/null
"$BIN" suite run --report "$TMP/suite-pass-report.json" "$TMP/suite-demo/suite-all-pass.yaml" > /dev/null
set +e
"$BIN" suite run --report "$TMP/suite-mixed-report.json" "$TMP/suite-demo/suite-mixed.yaml" > /dev/null
set -e

# --- 2. policy verify: the checked-in example policy is structurally valid. ---

echo "verify-policy-demo: policy verify policies/release-gate-example.yaml (expect: valid, exit 0)" >&2
VERIFY_OUT="$("$BIN" policy verify "$RELEASE_GATE_POLICY")"
echo "$VERIFY_OUT"
case "$VERIFY_OUT" in
  *"OK: policy is structurally valid"*) ;;
  *)
    echo "verify-policy-demo: expected the checked-in example policy to verify as valid" >&2
    exit 1
    ;;
esac

# --- A second, library-independent policy (require_result only), written
#     to the temporary directory rather than checked in, used for the
#     PASS/FAIL require_result checks below so they do not depend on
#     --library at all. ---

REQUIRE_RESULT_POLICY="$TMP/require-result-only.yaml"
cat > "$REQUIRE_RESULT_POLICY" <<EOF
schema_version: policy/v0.1
policy:
  id: require-result-only
rules:
  - type: require_result
    value: PASS
EOF

# --- 3. Evaluate: verdict PASS via require_result, library-independent. ---

echo "verify-policy-demo: policy evaluate (require_result only) against a PASS suite report (expect: verdict PASS, exit 0)" >&2
PASS_OUT="$("$BIN" policy evaluate --policy "$REQUIRE_RESULT_POLICY" --suite-report "$TMP/suite-pass-report.json")"
echo "$PASS_OUT"
case "$PASS_OUT" in
  *"Verdict: PASS"*) ;;
  *)
    echo "verify-policy-demo: expected Verdict: PASS" >&2
    exit 1
    ;;
esac

# --- 4. Evaluate: verdict FAIL via require_result. ---

echo "verify-policy-demo: policy evaluate (require_result only) against a FAIL suite report (expect: verdict FAIL, exit 2)" >&2
set +e
FAIL_OUT="$("$BIN" policy evaluate --policy "$REQUIRE_RESULT_POLICY" --suite-report "$TMP/suite-mixed-report.json")"
FAIL_EXIT=$?
set -e
echo "$FAIL_OUT"
if [ "$FAIL_EXIT" -ne 2 ]; then
  echo "verify-policy-demo: expected exit 2 for a FAIL verdict, got $FAIL_EXIT" >&2
  exit 1
fi
case "$FAIL_OUT" in
  *"[FAIL] require_result"*) ;;
  *)
    echo "verify-policy-demo: expected a [FAIL] require_result line" >&2
    exit 1
    ;;
esac
case "$FAIL_OUT" in
  *"Verdict: FAIL"*) ;;
  *)
    echo "verify-policy-demo: expected Verdict: FAIL" >&2
    exit 1
    ;;
esac

# --- 5. Evaluate: require_library_occurrence match. Seed a fresh temporary
#        library with examples/duplicate-payment/incident.yaml, whose
#        fingerprint is already the one scenario-pass.yaml's checked-in
#        linked_fingerprint declares (docs/regression-scenarios.md, "Sample
#        transcript") — no new fixture value invented. ---

MATCH_LIBRARY="$TMP/match-library"
echo "verify-policy-demo: library add duplicate-payment/incident.yaml into a fresh temporary library" >&2
"$BIN" library add --library "$MATCH_LIBRARY" --allow-unredacted "$DUPLICATE_PAYMENT" > /dev/null

echo "verify-policy-demo: policy evaluate (both rules) against the scenario PASS report with --library (expect: verdict PASS, exit 0)" >&2
LIBMATCH_OUT="$("$BIN" policy evaluate --policy "$RELEASE_GATE_POLICY" --scenario-report "$TMP/scenario-pass-report.json" --library "$MATCH_LIBRARY")"
echo "$LIBMATCH_OUT"
case "$LIBMATCH_OUT" in
  *"[OK] require_library_occurrence"*) ;;
  *)
    echo "verify-policy-demo: expected an [OK] require_library_occurrence line" >&2
    exit 1
    ;;
esac
case "$LIBMATCH_OUT" in
  *"Verdict: PASS"*) ;;
  *)
    echo "verify-policy-demo: expected Verdict: PASS" >&2
    exit 1
    ;;
esac

# --- 6. Evaluate: require_library_occurrence SKIP when --library is
#        omitted (expect: verdict FAIL, exit 2 — a SKIPped rule counts as
#        not satisfied, never a silent PASS). ---

echo "verify-policy-demo: policy evaluate (both rules) against the scenario PASS report without --library (expect: SKIP, verdict FAIL, exit 2)" >&2
set +e
SKIP_OUT="$("$BIN" policy evaluate --policy "$RELEASE_GATE_POLICY" --scenario-report "$TMP/scenario-pass-report.json")"
SKIP_EXIT=$?
set -e
echo "$SKIP_OUT"
if [ "$SKIP_EXIT" -ne 2 ]; then
  echo "verify-policy-demo: expected exit 2 for a SKIPped require_library_occurrence rule, got $SKIP_EXIT" >&2
  exit 1
fi
case "$SKIP_OUT" in
  *"[SKIP] require_library_occurrence: not evaluated (--library not given)"*) ;;
  *)
    echo "verify-policy-demo: expected a [SKIP] require_library_occurrence line" >&2
    exit 1
    ;;
esac
case "$SKIP_OUT" in
  *"Verdict: FAIL"*) ;;
  *)
    echo "verify-policy-demo: expected Verdict: FAIL" >&2
    exit 1
    ;;
esac

# --- 7. Malformed report file: deliberately corrupted/truncated report
#        JSON, asserting policy evaluate reports INVALID and exits 1. ---

MALFORMED_REPORT="$TMP/malformed-report.json"
printf '%s' '{not valid json' > "$MALFORMED_REPORT"

echo "verify-policy-demo: policy evaluate against a malformed report file (expect: INVALID, exit 1)" >&2
set +e
MALFORMED_OUT="$("$BIN" policy evaluate --policy "$REQUIRE_RESULT_POLICY" --scenario-report "$MALFORMED_REPORT" --report "$TMP/malformed-verdict-report.json" 2>&1)"
MALFORMED_EXIT=$?
set -e
echo "$MALFORMED_OUT"
if [ "$MALFORMED_EXIT" -ne 1 ]; then
  echo "verify-policy-demo: expected exit 1 for a malformed report file, got $MALFORMED_EXIT" >&2
  exit 1
fi
case "$(cat "$TMP/malformed-verdict-report.json")" in
  *'"verdict": "INVALID"'*) ;;
  *)
    echo "verify-policy-demo: expected the best-effort --report content to show verdict INVALID" >&2
    exit 1
    ;;
esac

echo "verify-policy-demo: OK"
