#!/usr/bin/env sh
# Builds the incidentdna binary (if not already built) and exercises the full
# `incidentdna evidence` command surface end-to-end against the fictional
# examples/evidence-storage-demo/ example: validates the incident document,
# stores each of its checked-in evidence files, lists and verifies their
# declared digests against the store, and inspects one stored object
# directly by digest. This is a CLI-binary-level check, in the same spirit as
# verify-golden-fingerprint.sh, but for the Phase 2 evidence store instead of
# the fingerprint algorithm.
#
# Uses a single temporary --store directory (created outside the repository
# via mktemp -d, removed on exit via the trap below) for every command below
# — the default store root (.incidentdna/evidence/objects) is never invoked,
# so this script never creates, and never leaves behind, a .incidentdna
# directory anywhere in the repository tree. See docs/evidence-storage.md for
# the underlying design this exercises.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/incidentdna"
DEMO="$ROOT/examples/evidence-storage-demo"
INCIDENT="$DEMO/incident.yaml"

if [ ! -x "$BIN" ]; then
  echo "verify-evidence-demo: building $BIN" >&2
  ( cd "$ROOT" && go build -buildvcs=false -o "$BIN" ./cmd/incidentdna )
fi

STORE="$(mktemp -d)"
cleanup() {
  rm -rf "$STORE"
}
trap cleanup EXIT

echo "verify-evidence-demo: validating $INCIDENT" >&2
"$BIN" validate "$INCIDENT"

FIRST_DIGEST=""
for f in "$DEMO"/evidence/*; do
  echo "verify-evidence-demo: storing $f" >&2
  OUT="$("$BIN" evidence store --store "$STORE" "$f")"
  echo "$OUT"
  if [ -z "$FIRST_DIGEST" ]; then
    FIRST_DIGEST="$(printf '%s\n' "$OUT" | grep -o 'sha256:[0-9a-f]\{64\}' | head -n1)"
  fi
done

if [ -z "$FIRST_DIGEST" ]; then
  echo "verify-evidence-demo: could not determine a digest to inspect" >&2
  exit 1
fi

echo "verify-evidence-demo: evidence list" >&2
"$BIN" evidence list --store "$STORE" "$INCIDENT"

echo "verify-evidence-demo: evidence verify" >&2
"$BIN" evidence verify --store "$STORE" "$INCIDENT"

echo "verify-evidence-demo: evidence inspect $FIRST_DIGEST" >&2
"$BIN" evidence inspect --store "$STORE" "$FIRST_DIGEST"

echo "verify-evidence-demo: OK"
