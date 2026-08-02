#!/usr/bin/env sh
# Builds the incidentdna binary (if not already built) and exercises the full
# `incidentdna library` command surface end-to-end against the fictional
# examples/incident-library-demo/ example: validates all four documents,
# stores a first occurrence, stores a second occurrence under the same
# fingerprint (different incident id), re-adds the same incident reformatted
# as JSON (idempotent, nothing written), checks the fingerprint for a match
# (two occurrences), checks an unrelated failure for a documented no-match
# (exit code 2), and lists the library's grouped occurrences. This is a
# CLI-binary-level check, in the same spirit as verify-golden-fingerprint.sh
# and verify-evidence-demo.sh, but for the Phase 3 incident library instead
# of the fingerprint algorithm or the Phase 2 evidence store.
#
# Uses a single temporary --library directory (created outside the
# repository via mktemp -d, removed on exit via the trap below) for every
# command below — the default library root (.incidentdna/library/objects) is
# never invoked, so this script never creates, and never leaves behind, a
# .incidentdna directory anywhere in the repository tree. See
# docs/incident-library.md for the underlying design this exercises.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/incidentdna"
DEMO="$ROOT/examples/incident-library-demo"

if [ ! -x "$BIN" ]; then
  echo "verify-library-demo: building $BIN" >&2
  ( cd "$ROOT" && go build -buildvcs=false -o "$BIN" ./cmd/incidentdna )
fi

LIBRARY="$(mktemp -d)"
cleanup() {
  rm -rf "$LIBRARY"
}
trap cleanup EXIT

echo "verify-library-demo: validating all four demo documents" >&2
"$BIN" validate "$DEMO/incident-a.yaml"
"$BIN" validate "$DEMO/incident-a-recurrence.yaml"
"$BIN" validate "$DEMO/incident-a-repeat.json"
"$BIN" validate "$DEMO/incident-different-failure.yaml"

echo "verify-library-demo: library add incident-a.yaml (expect: new occurrence)" >&2
ADD1="$("$BIN" library add --library "$LIBRARY" "$DEMO/incident-a.yaml")"
echo "$ADD1"
case "$ADD1" in
  "Stored new occurrence: fingerprint sha256:"*"(incident: INC-2026-0501)") ;;
  *)
    echo "verify-library-demo: unexpected output for first add: $ADD1" >&2
    exit 1
    ;;
esac
FP1="$(printf '%s\n' "$ADD1" | grep -o 'sha256:[0-9a-f]\{64\}' | head -n1)"
if [ -z "$FP1" ]; then
  echo "verify-library-demo: could not extract fingerprint from first add output" >&2
  exit 1
fi

echo "verify-library-demo: library add incident-a-recurrence.yaml (expect: second occurrence, same fingerprint)" >&2
ADD2="$("$BIN" library add --library "$LIBRARY" "$DEMO/incident-a-recurrence.yaml")"
echo "$ADD2"
case "$ADD2" in
  "Stored new occurrence: fingerprint sha256:"*"(incident: INC-2026-0618)") ;;
  *)
    echo "verify-library-demo: unexpected output for recurrence add: $ADD2" >&2
    exit 1
    ;;
esac
FP2="$(printf '%s\n' "$ADD2" | grep -o 'sha256:[0-9a-f]\{64\}' | head -n1)"
if [ "$FP2" != "$FP1" ]; then
  echo "verify-library-demo: recurrence fingerprint ($FP2) does not match first occurrence's fingerprint ($FP1)" >&2
  exit 1
fi

echo "verify-library-demo: library add incident-a-repeat.json (expect: idempotent, nothing written)" >&2
ADD3="$("$BIN" library add --library "$LIBRARY" "$DEMO/incident-a-repeat.json")"
echo "$ADD3"
case "$ADD3" in
  "Already present (idempotent, nothing written): fingerprint sha256:"*"(incident: INC-2026-0501)") ;;
  *)
    echo "verify-library-demo: unexpected output for idempotent re-add: $ADD3" >&2
    exit 1
    ;;
esac
FP3="$(printf '%s\n' "$ADD3" | grep -o 'sha256:[0-9a-f]\{64\}' | head -n1)"
if [ "$FP3" != "$FP1" ]; then
  echo "verify-library-demo: idempotent re-add fingerprint ($FP3) does not match first occurrence's fingerprint ($FP1)" >&2
  exit 1
fi

echo "verify-library-demo: library check incident-a.yaml (expect: match, 2 occurrences, exit 0)" >&2
set +e
CHECK_MATCH="$("$BIN" library check --library "$LIBRARY" "$DEMO/incident-a.yaml")"
CHECK_MATCH_EXIT=$?
set -e
echo "$CHECK_MATCH"
if [ "$CHECK_MATCH_EXIT" -ne 0 ]; then
  echo "verify-library-demo: expected exit 0 from matching check, got $CHECK_MATCH_EXIT" >&2
  exit 1
fi
case "$CHECK_MATCH" in
  *"Fingerprint: $FP1"*) ;;
  *)
    echo "verify-library-demo: check output did not report the expected fingerprint $FP1" >&2
    exit 1
    ;;
esac
case "$CHECK_MATCH" in
  *"Match: 2 occurrence(s) found for this fingerprint"*) ;;
  *)
    echo "verify-library-demo: check output did not report 2 matching occurrences: $CHECK_MATCH" >&2
    exit 1
    ;;
esac

echo "verify-library-demo: library check incident-different-failure.yaml (expect: no match, exit 2)" >&2
set +e
CHECK_MISS="$("$BIN" library check --library "$LIBRARY" "$DEMO/incident-different-failure.yaml")"
CHECK_MISS_EXIT=$?
set -e
echo "$CHECK_MISS"
if [ "$CHECK_MISS_EXIT" -ne 2 ]; then
  echo "verify-library-demo: expected documented exit code 2 for a no-match check, got $CHECK_MISS_EXIT" >&2
  exit 1
fi
case "$CHECK_MISS" in
  *"No match: no occurrence with this fingerprint exists in the library"*) ;;
  *)
    echo "verify-library-demo: check output did not report the expected no-match message: $CHECK_MISS" >&2
    exit 1
    ;;
esac

echo "verify-library-demo: library list (expect: one fingerprint, 2 occurrences)" >&2
LIST_OUT="$("$BIN" library list --library "$LIBRARY")"
echo "$LIST_OUT"
case "$LIST_OUT" in
  *"Fingerprint: $FP1 (2 occurrence(s))"*) ;;
  *)
    echo "verify-library-demo: list output did not report the expected grouped fingerprint entry" >&2
    exit 1
    ;;
esac
for want in "INC-2026-0501" "INC-2026-0618"; do
  case "$LIST_OUT" in
    *"$want"*) ;;
    *)
      echo "verify-library-demo: list output missing expected occurrence $want" >&2
      exit 1
      ;;
  esac
done

# No raw incident document content should ever appear in any command's
# output above: none of the add/check/list transcripts may contain
# free-text fields that fall outside the documented bounded field set
# (fingerprint, occurrence count, incident id, title, service, occurred
# timestamp) — e.g. business-invariant statements, trigger/event
# descriptions, or remediation text. These phrases are drawn from fields
# that are never part of that bounded set, so their absence confirms
# `library` never prints raw document content, without false-flagging the
# titles it legitimately does print.
ALL_OUTPUT="$ADD1
$ADD2
$ADD3
$CHECK_MATCH
$CHECK_MISS
$LIST_OUT"
for forbidden in \
  "must never queue indefinitely" \
  "bounded exponential backoff" \
  "must never cause more than one" \
  "reached its configured maximum size"; do
  case "$ALL_OUTPUT" in
    *"$forbidden"*)
      echo "verify-library-demo: found raw document content ('$forbidden') in CLI output; library commands must never print raw incident content" >&2
      exit 1
      ;;
  esac
done

echo "verify-library-demo: OK"
