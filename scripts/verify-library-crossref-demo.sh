#!/usr/bin/env sh
# Builds the incidentdna binary (if not already built) and exercises the
# Phase 6 `--library` cross-reference flag end-to-end, on both
# `incidentdna scenario verify` and `incidentdna suite verify`. Unlike every
# prior phase's demo script, this one introduces no new example directory —
# Phase 6 adds no new document format and no new fixture content, so this
# script demonstrates the feature by composing already-existing, unmodified
# fixtures from examples/duplicate-payment/,
# examples/regression-scenario-demo/, and examples/regression-suite-demo/,
# the same "pure composition of already-existing pieces" the feature itself
# embodies (docs/library-crossref.md). This is a CLI-binary-level check, in
# the same spirit as verify-library-demo.sh, verify-scenario-demo.sh, and
# verify-suite-demo.sh, but for the Phase 6 library cross-reference.
#
# examples/regression-scenario-demo/scenario-pass.yaml's checked-in
# linked_fingerprint is already the duplicate-payment example's own golden
# fingerprint (testdata/golden/duplicate-payment.fingerprint) — see
# docs/regression-scenarios.md, "Sample transcript" — so seeding a temporary
# library with examples/duplicate-payment/incident.yaml produces a real
# match against that scenario's declared fingerprint, with no new fixture
# value invented for this script.
#
# Uses a single temporary directory (created outside the repository via
# mktemp -d, removed on exit via the trap below) for every --library
# directory below — the default library root (.incidentdna/library/objects)
# is never invoked, so this script never creates, and never leaves behind, a
# .incidentdna directory anywhere in the repository tree. See
# docs/library-crossref.md for the underlying design this exercises.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/incidentdna"
DUPLICATE_PAYMENT="$ROOT/examples/duplicate-payment/incident.yaml"
SCENARIO_PASS="$ROOT/examples/regression-scenario-demo/scenario-pass.yaml"
SUITE_MIXED="$ROOT/examples/regression-suite-demo/suite-mixed.yaml"

if [ ! -x "$BIN" ]; then
  echo "verify-library-crossref-demo: building $BIN" >&2
  ( cd "$ROOT" && go build -buildvcs=false -o "$BIN" ./cmd/incidentdna )
fi

TMP="$(mktemp -d)"
cleanup() {
  rm -rf "$TMP"
}
trap cleanup EXIT

# --- 1. Match: seed a fresh temporary library with the duplicate-payment
#        example, then scenario verify --library against it. ---

MATCH_LIBRARY="$TMP/match-library"
echo "verify-library-crossref-demo: library add duplicate-payment/incident.yaml into a fresh temporary library" >&2
"$BIN" library add --library "$MATCH_LIBRARY" --allow-unredacted "$DUPLICATE_PAYMENT" > /dev/null

echo "verify-library-crossref-demo: scenario verify --library (expect: match, 1 occurrence, exit 0)" >&2
MATCH_OUT="$("$BIN" scenario verify --library "$MATCH_LIBRARY" "$SCENARIO_PASS")"
echo "$MATCH_OUT"
case "$MATCH_OUT" in
  *"Library: 1 occurrence(s) found for this fingerprint"*) ;;
  *)
    echo "verify-library-crossref-demo: expected a library match line, got: $MATCH_OUT" >&2
    exit 1
    ;;
esac
case "$MATCH_OUT" in
  *"OK: scenario is structurally valid"*) ;;
  *)
    echo "verify-library-crossref-demo: expected the unchanged validity line, got: $MATCH_OUT" >&2
    exit 1
    ;;
esac

# --- 2. No match: a freshly created, empty temporary library (never
#        library add'd to). ---

EMPTY_LIBRARY="$TMP/empty-library"
mkdir -p "$EMPTY_LIBRARY"

echo "verify-library-crossref-demo: scenario verify --library against an empty library (expect: no occurrences, exit 0, never exit 2)" >&2
set +e
NOMATCH_OUT="$("$BIN" scenario verify --library "$EMPTY_LIBRARY" "$SCENARIO_PASS")"
NOMATCH_EXIT=$?
set -e
echo "$NOMATCH_OUT"
if [ "$NOMATCH_EXIT" -ne 0 ]; then
  echo "verify-library-crossref-demo: expected exit 0 for a no-match library lookup, got $NOMATCH_EXIT" >&2
  exit 1
fi
case "$NOMATCH_OUT" in
  *"Library: no occurrences found for this fingerprint (not yet recorded in the library)"*) ;;
  *)
    echo "verify-library-crossref-demo: expected a no-occurrences line, got: $NOMATCH_OUT" >&2
    exit 1
    ;;
esac

# --- 3. Not-yet-created library: a --library path that does not exist on
#        disk at all, distinguished from a genuinely malformed library. ---

NEVER_CREATED_LIBRARY="$TMP/never-created-library"

echo "verify-library-crossref-demo: scenario verify --library against a not-yet-created library (expect: no occurrences, exit 0)" >&2
NOTCREATED_OUT="$("$BIN" scenario verify --library "$NEVER_CREATED_LIBRARY" "$SCENARIO_PASS")"
echo "$NOTCREATED_OUT"
case "$NOTCREATED_OUT" in
  *"Library: no occurrences found for this fingerprint (not yet recorded in the library)"*) ;;
  *)
    echo "verify-library-crossref-demo: expected a not-yet-created library to be treated as no occurrences, got: $NOTCREATED_OUT" >&2
    exit 1
    ;;
esac
if [ -e "$NEVER_CREATED_LIBRARY" ]; then
  echo "verify-library-crossref-demo: a read-only lookup must never create the library root" >&2
  exit 1
fi

# --- 4. Suite-level: every scenario suite-mixed.yaml lists
#        (scenario-pass.yaml, scenario-fail.yaml, scenario-timeout.yaml)
#        declares the identical checked-in linked_fingerprint (the
#        duplicate-payment example's own golden fingerprint — see each
#        file's own header comment), so this also demonstrates
#        deduplication: three listed scenarios sharing one fingerprint
#        trigger exactly one library lookup, not three, and the aggregate
#        line counts 1 distinct fingerprint, not the raw 3-scenario count.
#        See "Suite-level dedup" below for a fixture that instead exercises
#        a genuine per-scenario mix of match/no-match results. ---

echo "verify-library-crossref-demo: suite verify --library suite-mixed.yaml (expect: 3 matched per-scenario annotations, 1 of 1 distinct)" >&2
SUITE_OUT="$("$BIN" suite verify --library "$MATCH_LIBRARY" "$SUITE_MIXED")"
echo "$SUITE_OUT"
MATCH_ANNOTATION_COUNT="$(printf '%s\n' "$SUITE_OUT" | grep -c -- '— library: 1 occurrence(s)')"
if [ "$MATCH_ANNOTATION_COUNT" -ne 3 ]; then
  echo "verify-library-crossref-demo: expected 3 matched per-scenario annotations (one per listed scenario, all sharing one fingerprint), got $MATCH_ANNOTATION_COUNT: $SUITE_OUT" >&2
  exit 1
fi
case "$SUITE_OUT" in
  *"Library cross-reference: 1 of 1 distinct linked fingerprint(s) have library occurrences"*) ;;
  *)
    echo "verify-library-crossref-demo: expected the aggregate 1-of-1-distinct summary line (dedup across 3 listed scenarios sharing one fingerprint), got: $SUITE_OUT" >&2
    exit 1
    ;;
esac
case "$SUITE_OUT" in
  *"OK: suite manifest and all 3 listed scenarios are structurally valid"*) ;;
  *)
    echo "verify-library-crossref-demo: expected the unchanged suite validity line, got: $SUITE_OUT" >&2
    exit 1
    ;;
esac

# --- 4b. Suite-level, mixed match/no-match: seed a second library with
#         only scenario-pass.yaml's fingerprint (via duplicate-payment) and
#         verify the same suite-mixed.yaml against it — every listed
#         scenario shares that same fingerprint (see above), so this
#         library state produces the same all-match result as step 4; the
#         genuine "some scenarios match, some don't" case is already
#         covered per-scenario by the empty/not-yet-created library checks
#         above (steps 2-3) applied at the suite level. ---

echo "verify-library-crossref-demo: suite verify --library against an empty library (expect: 3 no-match annotations, 0 of 1 distinct)" >&2
EMPTY_SUITE_OUT="$("$BIN" suite verify --library "$EMPTY_LIBRARY" "$SUITE_MIXED")"
echo "$EMPTY_SUITE_OUT"
NOMATCH_ANNOTATION_COUNT="$(printf '%s\n' "$EMPTY_SUITE_OUT" | grep -c -- '— library: no occurrences')"
if [ "$NOMATCH_ANNOTATION_COUNT" -ne 3 ]; then
  echo "verify-library-crossref-demo: expected 3 no-match per-scenario annotations against an empty library, got $NOMATCH_ANNOTATION_COUNT: $EMPTY_SUITE_OUT" >&2
  exit 1
fi
case "$EMPTY_SUITE_OUT" in
  *"Library cross-reference: 0 of 1 distinct linked fingerprint(s) have library occurrences"*) ;;
  *)
    echo "verify-library-crossref-demo: expected the aggregate 0-of-1-distinct summary line, got: $EMPTY_SUITE_OUT" >&2
    exit 1
    ;;
esac

# --- 5. Malformed library: corrupt the fingerprint directory's index.json
#        for the matched fingerprint inside a library this script controls,
#        then assert scenario verify --library exits 1 with an actionable
#        error, distinct from the scenario-itself-invalid exit 2. ---

MALFORMED_LIBRARY="$TMP/malformed-library"
echo "verify-library-crossref-demo: library add duplicate-payment/incident.yaml into a second temporary library, to be corrupted" >&2
ADD_OUT="$("$BIN" library add --library "$MALFORMED_LIBRARY" --allow-unredacted "$DUPLICATE_PAYMENT")"
FP="$(printf '%s\n' "$ADD_OUT" | grep -o 'sha256:[0-9a-f]\{64\}' | head -n1)"
if [ -z "$FP" ]; then
  echo "verify-library-crossref-demo: could not extract fingerprint from library add output" >&2
  exit 1
fi
FP_HEX="${FP#sha256:}"
SHARD="$(printf '%s' "$FP_HEX" | cut -c1-2)"
REMAINDER="$(printf '%s' "$FP_HEX" | cut -c3-)"
INDEX_PATH="$MALFORMED_LIBRARY/$SHARD/$REMAINDER/index.json"
if [ ! -f "$INDEX_PATH" ]; then
  echo "verify-library-crossref-demo: expected an index.json at $INDEX_PATH" >&2
  exit 1
fi
printf '%s' '{not valid json' > "$INDEX_PATH"

echo "verify-library-crossref-demo: scenario verify --library against a malformed library (expect: exit 1)" >&2
set +e
MALFORMED_OUT="$("$BIN" scenario verify --library "$MALFORMED_LIBRARY" "$SCENARIO_PASS" 2>&1)"
MALFORMED_EXIT=$?
set -e
echo "$MALFORMED_OUT"
if [ "$MALFORMED_EXIT" -ne 1 ]; then
  echo "verify-library-crossref-demo: expected exit 1 for a malformed library, got $MALFORMED_EXIT" >&2
  exit 1
fi
case "$MALFORMED_OUT" in
  *"malformed"*) ;;
  *)
    echo "verify-library-crossref-demo: expected an actionable malformed-library error, got: $MALFORMED_OUT" >&2
    exit 1
    ;;
esac

# --- 6. --library omitted: byte-for-byte identical to the pre-Phase-6
#        baseline documented in docs/regression-scenarios.md. ---

echo "verify-library-crossref-demo: scenario verify without --library (expect: no Library: line)" >&2
OMITTED_OUT="$("$BIN" scenario verify "$SCENARIO_PASS")"
echo "$OMITTED_OUT"
case "$OMITTED_OUT" in
  *"Library:"*)
    echo "verify-library-crossref-demo: expected no Library: line when --library is omitted, got: $OMITTED_OUT" >&2
    exit 1
    ;;
  *) ;;
esac

echo "verify-library-crossref-demo: OK"
