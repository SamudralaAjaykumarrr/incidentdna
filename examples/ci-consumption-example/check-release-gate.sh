#!/usr/bin/env sh
# check-release-gate.sh — vendor-neutral release-gate consumption pattern
# (docs/phase-8-plan.md §14).
#
# This is the "lowest common denominator" any CI vendor can invoke as a
# single step: it contains zero references to GitHub Actions, GitLab,
# CircleCI, Jenkins, or any other specific platform's API or YAML schema.
# It runs exactly two already-shipped incidentdna commands — `suite run`
# then `policy evaluate` — against caller-supplied files, and exits with
# `policy evaluate`'s own exit code, unmodified:
#
#   0 = release allowed  (verdict PASS)
#   2 = release blocked  (verdict FAIL)
#   1 = environmental failure needing human attention (I/O/usage/INVALID/
#       INTERNAL_ERROR — including a suite run that itself could not
#       complete, as opposed to one that completed and reported FAIL)
#
# This is the identical exit-code contract docs/policy-evaluation.md §11
# already documents, restated here as a runnable consumption recipe.
#
# Usage:
#   check-release-gate.sh <incidentdna-binary> <suite-file> <policy-file> [library-dir]
#
# <library-dir> is optional; when given, it is passed to `policy evaluate
# --library`, enabling require_library_occurrence rules. When omitted,
# such rules are reported SKIP (and therefore count as not satisfied) —
# the same behavior `incidentdna policy evaluate` always has without
# --library.
set -eu

if [ "$#" -lt 3 ] || [ "$#" -gt 4 ]; then
  echo "Usage: check-release-gate.sh <incidentdna-binary> <suite-file> <policy-file> [library-dir]" >&2
  exit 1
fi

BIN="$1"
SUITE_FILE="$2"
POLICY_FILE="$3"
LIBRARY_DIR="${4:-}"

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
SUITE_REPORT="$WORKDIR/suite-report.json"

echo "check-release-gate: $BIN suite run --report $SUITE_REPORT $SUITE_FILE" >&2
set +e
"$BIN" suite run --report "$SUITE_REPORT" "$SUITE_FILE"
SUITE_EXIT=$?
set -e

# suite run's own exit code (0 = aggregate PASS, 2 = aggregate FAIL, 1 =
# environmental failure) is not itself the release decision — a suite that
# completed and reported FAIL (exit 2) still produced a valid report for
# policy evaluate to reason about below. Only suite run's own exit 1 means
# no usable report exists at all.
if [ "$SUITE_EXIT" -eq 1 ]; then
  echo "check-release-gate: suite run failed environmentally (exit 1); no report to evaluate" >&2
  exit 1
fi

if [ -n "$LIBRARY_DIR" ]; then
  echo "check-release-gate: $BIN policy evaluate --policy $POLICY_FILE --suite-report $SUITE_REPORT --library $LIBRARY_DIR" >&2
  "$BIN" policy evaluate --policy "$POLICY_FILE" --suite-report "$SUITE_REPORT" --library "$LIBRARY_DIR"
else
  echo "check-release-gate: $BIN policy evaluate --policy $POLICY_FILE --suite-report $SUITE_REPORT" >&2
  "$BIN" policy evaluate --policy "$POLICY_FILE" --suite-report "$SUITE_REPORT"
fi
