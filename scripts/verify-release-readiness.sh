#!/usr/bin/env sh
# scripts/verify-release-readiness.sh — the single entry point that proves
# every Phase 8 release claim is actually true of a given build
# (docs/phase-8-plan.md §19). Mirrors scripts/verify-golden-fingerprint.sh's
# role as the final, non-negotiable check, scoped one level up: where that
# script proves one algorithm's output is unchanged, this script proves the
# *release itself* is trustworthy.
#
# Not wired into `make verify`'s existing chain (that chain is unchanged by
# Phase 8) — wired into the new, separate `make release-verify` target,
# since it is meaningfully slower (it builds five platform targets twice
# each) and is only needed at release-cut time.
#
# VERSION (env, default "dev") is passed straight through to
# scripts/build-release.sh / generate-checksums.sh / generate-sbom.sh /
# run-benchmarks.sh, exactly as `make release-verify` does.
#
# Any single stage failure aborts with a non-zero exit and a clearly
# labeled stage name — the same "fail loudly, name the stage" discipline
# scripts/verify-golden-fingerprint.sh already applies to a single check.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
LICENSE_FILE="$ROOT/LICENSE"
VERSION="${VERSION:-dev}"

stage() {
  echo "" >&2
  echo "=== verify-release-readiness: $1 ===" >&2
}

fail() {
  echo "verify-release-readiness: FAILED at stage: $1" >&2
  exit 1
}

# ----------------------------------------------------------------------
# Stage 1: platform artifact matrix + build-twice-and-diff reproducibility
# (already enforced inside scripts/build-release.sh, which aborts loudly
# on any mismatch) — this stage confirms the resulting dist/ actually
# contains one archive per required platform, per docs/phase-8-plan.md §9's
# five-platform matrix. Any missing artifact fails this script.
# ----------------------------------------------------------------------
stage "1/7 platform build + reproducibility (scripts/build-release.sh)"
VERSION="$VERSION" sh "$ROOT/scripts/build-release.sh" || fail "build-release.sh"

EXPECTED_PLATFORMS="linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64"
for plat in $EXPECTED_PLATFORMS; do
  case "$plat" in
    windows-*) ext=zip ;;
    *) ext=tar.gz ;;
  esac
  artifact="$DIST/incidentdna-$VERSION-$plat.$ext"
  [ -f "$artifact" ] || fail "missing required artifact $artifact"
  echo "verify-release-readiness: found $artifact" >&2
done

# ----------------------------------------------------------------------
# Stage 2: SHA-256 checksums, independently re-verified.
# ----------------------------------------------------------------------
stage "2/7 checksums (scripts/generate-checksums.sh)"
VERSION="$VERSION" sh "$ROOT/scripts/generate-checksums.sh" || fail "generate-checksums.sh"
( cd "$DIST" && sha256sum -c SHA256SUMS ) || fail "sha256sum -c SHA256SUMS"

# ----------------------------------------------------------------------
# Stage 3: SBOM, validated as well-formed JSON matching the
# incidentdna-sbom/v1 shape with exactly go.sum's dependencies.
# ----------------------------------------------------------------------
stage "3/7 SBOM (scripts/generate-sbom.sh)"
VERSION="$VERSION" sh "$ROOT/scripts/generate-sbom.sh" || fail "generate-sbom.sh"

SBOM_FILE="$DIST/incidentdna-$VERSION-sbom.json"
[ -s "$SBOM_FILE" ] || fail "missing or empty $SBOM_FILE"

SBOM_CHECK_HELPER="$(mktemp -d)/checksbom.go"
cat > "$SBOM_CHECK_HELPER" <<'EOF'
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

type dependency struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	Direct  bool   `json:"direct"`
}

type sbom struct {
	Format       string       `json:"format"`
	GeneratedFor string       `json:"generated_for"`
	GoVersion    string       `json:"go_version"`
	Module       string       `json:"module"`
	Dependencies []dependency `json:"dependencies"`
}

type moduleJSON struct {
	Path     string `json:"Path"`
	Version  string `json:"Version"`
	Main     bool   `json:"Main"`
	Indirect bool   `json:"Indirect"`
}

func main() {
	sbomPath := os.Args[1]

	sbomBytes, err := os.ReadFile(sbomPath)
	if err != nil {
		panic(err)
	}
	var doc sbom
	if err := json.Unmarshal(sbomBytes, &doc); err != nil {
		fmt.Fprintf(os.Stderr, "sbom is not valid JSON: %v\n", err)
		os.Exit(1)
	}
	if doc.Format != "incidentdna-sbom/v1" {
		fmt.Fprintf(os.Stderr, "sbom format = %q, want incidentdna-sbom/v1\n", doc.Format)
		os.Exit(1)
	}

	dec := json.NewDecoder(os.Stdin)
	var want []dependency
	for {
		var m moduleJSON
		if err := dec.Decode(&m); err != nil {
			break
		}
		if m.Main {
			continue
		}
		want = append(want, dependency{Path: m.Path, Version: m.Version, Direct: !m.Indirect})
	}
	sort.Slice(want, func(i, j int) bool { return want[i].Path < want[j].Path })

	if len(want) != len(doc.Dependencies) {
		fmt.Fprintf(os.Stderr, "sbom has %d dependencies, go.sum names %d\n", len(doc.Dependencies), len(want))
		os.Exit(1)
	}
	for i := range want {
		if doc.Dependencies[i] != want[i] {
			fmt.Fprintf(os.Stderr, "sbom dependency[%d] = %+v, want %+v\n", i, doc.Dependencies[i], want[i])
			os.Exit(1)
		}
	}
	fmt.Println("sbom OK: matches go.sum exactly")
}
EOF
( cd "$ROOT" && go list -m -json all | go run "$SBOM_CHECK_HELPER" "$SBOM_FILE" ) || fail "SBOM validation"

# ----------------------------------------------------------------------
# Stage 4: benchmarks run to completion and produce non-empty output.
# Never asserts anything about the numbers themselves (docs/phase-8-plan.md
# §13's "informational only, never gating" constraint, enforced
# structurally: this stage cannot see the numbers to gate on).
# ----------------------------------------------------------------------
stage "4/7 benchmarks (scripts/run-benchmarks.sh)"
( cd "$ROOT" && VERSION="$VERSION" sh -c 'go test -bench=. -benchmem -run=^$ ./... | sh scripts/run-benchmarks.sh' ) || fail "run-benchmarks.sh"
BENCH_FILE="$DIST/incidentdna-$VERSION-bench.txt"
[ -s "$BENCH_FILE" ] || fail "missing or empty $BENCH_FILE"

# ----------------------------------------------------------------------
# Stage 5: extract the linux-amd64 archive (the platform CI itself runs
# on) into a clean temporary directory and execute the literal
# quick-start sequence (docs/phase-8-plan.md §17), then the full flagship
# workflow (§6, stages 1-9) end to end against the checked-in fixtures.
# ----------------------------------------------------------------------
stage "5/7 quick start + flagship workflow (linux-amd64 archive)"

EXTRACT_DIR="$(mktemp -d)"
tar xzf "$DIST/incidentdna-$VERSION-linux-amd64.tar.gz" -C "$EXTRACT_DIR"
EXTRACTED_BIN="$EXTRACT_DIR/incidentdna"
[ -x "$EXTRACTED_BIN" ] || fail "extracted archive has no executable incidentdna"
[ -s "$EXTRACT_DIR/LICENSE" ] || fail "extracted archive has no LICENSE"

VERSION_OUT="$("$EXTRACTED_BIN" version)"
[ "$VERSION_OUT" = "incidentdna $VERSION" ] || fail "incidentdna version printed '$VERSION_OUT', want 'incidentdna $VERSION'"
echo "verify-release-readiness: $VERSION_OUT" >&2

FLAGSHIP="$(mktemp -d)"
cleanup_flagship() { rm -rf "$FLAGSHIP" "$EXTRACT_DIR"; }
trap cleanup_flagship EXIT

DUPLICATE_PAYMENT="$ROOT/examples/duplicate-payment/incident.yaml"
GOLDEN="$ROOT/testdata/golden/duplicate-payment.fingerprint"
EVIDENCE_DEMO="$ROOT/examples/evidence-storage-demo"
SCENARIO_DEMO="$ROOT/examples/regression-scenario-demo"
SUITE_DEMO="$ROOT/examples/regression-suite-demo"
RELEASE_GATE_POLICY="$ROOT/policies/release-gate-example.yaml"

# 1-2. incident -> canonicalization/fingerprint, checked against golden.
echo "verify-release-readiness: [1/9] validate + fingerprint (vs. golden)" >&2
"$EXTRACTED_BIN" validate "$DUPLICATE_PAYMENT" > /dev/null || fail "flagship: validate"
FP="$("$EXTRACTED_BIN" fingerprint "$DUPLICATE_PAYMENT")"
GOLDEN_FP="$(cat "$GOLDEN" | tr -d '[:space:]')"
[ "$(printf '%s' "$FP" | tr -d '[:space:]')" = "$GOLDEN_FP" ] || fail "flagship: fingerprint mismatch"

# 3. evidence integrity.
echo "verify-release-readiness: [2/9] evidence store + verify" >&2
EVIDENCE_STORE="$FLAGSHIP/evidence-store"
for f in "$EVIDENCE_DEMO"/evidence/*; do
  "$EXTRACTED_BIN" evidence store --store "$EVIDENCE_STORE" "$f" > /dev/null || fail "flagship: evidence store"
done
"$EXTRACTED_BIN" evidence verify --store "$EVIDENCE_STORE" "$EVIDENCE_DEMO/incident.yaml" > /dev/null || fail "flagship: evidence verify"

# 4. incident library.
echo "verify-release-readiness: [3/9] library add + check" >&2
LIBRARY="$FLAGSHIP/library"
"$EXTRACTED_BIN" library add --library "$LIBRARY" --allow-unredacted "$DUPLICATE_PAYMENT" > /dev/null || fail "flagship: library add"
"$EXTRACTED_BIN" library check --library "$LIBRARY" "$DUPLICATE_PAYMENT" > /dev/null || fail "flagship: library check"

# Prepare substituted scenario/suite fixtures (INCIDENTDNA_BIN_PLACEHOLDER
# -> the freshly extracted binary), exactly as scripts/verify-policy-demo.sh
# already does for bin/incidentdna.
mkdir -p "$FLAGSHIP/scenario-demo/fixtures"
cp "$SCENARIO_DEMO/fixtures/incident.yaml" "$FLAGSHIP/scenario-demo/fixtures/incident.yaml"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$EXTRACTED_BIN#" "$SCENARIO_DEMO/scenario-pass.yaml" > "$FLAGSHIP/scenario-demo/scenario-pass.yaml"

mkdir -p "$FLAGSHIP/suite-demo"
cp -r "$SUITE_DEMO"/. "$FLAGSHIP/suite-demo/"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$EXTRACTED_BIN#" "$SUITE_DEMO/scenarios/scenario-pass.yaml" > "$FLAGSHIP/suite-demo/scenarios/scenario-pass.yaml"
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$EXTRACTED_BIN#" "$SUITE_DEMO/scenarios/scenario-fail.yaml" > "$FLAGSHIP/suite-demo/scenarios/scenario-fail.yaml"

# 5. regression scenario.
echo "verify-release-readiness: [4/9] scenario verify (--source) + run" >&2
"$EXTRACTED_BIN" scenario verify --source "$DUPLICATE_PAYMENT" --library "$LIBRARY" "$FLAGSHIP/scenario-demo/scenario-pass.yaml" > /dev/null || fail "flagship: scenario verify"
"$EXTRACTED_BIN" scenario run --report "$FLAGSHIP/scenario-report.json" "$FLAGSHIP/scenario-demo/scenario-pass.yaml" > /dev/null || fail "flagship: scenario run"

# 6-7. scenario suite + library correlation (exercised via --library above
# and again here).
echo "verify-release-readiness: [5/9] suite verify (--library) + run" >&2
"$EXTRACTED_BIN" suite verify --library "$LIBRARY" "$FLAGSHIP/suite-demo/suite-all-pass.yaml" > /dev/null || fail "flagship: suite verify"
"$EXTRACTED_BIN" suite run --report "$FLAGSHIP/suite-report.json" "$FLAGSHIP/suite-demo/suite-all-pass.yaml" > /dev/null || fail "flagship: suite run"

# 8-9. policy evaluation -> deterministic release decision.
echo "verify-release-readiness: [6/9] policy verify + evaluate (expect verdict PASS, exit 0)" >&2
"$EXTRACTED_BIN" policy verify "$RELEASE_GATE_POLICY" > /dev/null || fail "flagship: policy verify"
set +e
POLICY_OUT="$("$EXTRACTED_BIN" policy evaluate --policy "$RELEASE_GATE_POLICY" --suite-report "$FLAGSHIP/suite-report.json" --library "$LIBRARY" --report "$FLAGSHIP/verdict.json")"
POLICY_EXIT=$?
set -e
echo "$POLICY_OUT" >&2
[ "$POLICY_EXIT" -eq 0 ] || fail "flagship: policy evaluate expected exit 0, got $POLICY_EXIT"
case "$POLICY_OUT" in
  *"Verdict: PASS"*) ;;
  *) fail "flagship: expected Verdict: PASS" ;;
esac
echo "verify-release-readiness: [7-9/9] deterministic release decision = PASS (exit 0)" >&2

# ----------------------------------------------------------------------
# Stage 6: vendor-neutral CI consumption example, both PASS and FAIL paths.
# ----------------------------------------------------------------------
stage "6/7 CI consumption example (scripts/verify-ci-consumption-example.sh)"
sh "$ROOT/scripts/verify-ci-consumption-example.sh" || fail "verify-ci-consumption-example.sh"

# ----------------------------------------------------------------------
# Stage 7: LICENSE exists, non-empty, and is included in every archive.
# ----------------------------------------------------------------------
stage "7/7 LICENSE presence in every archive"
[ -s "$LICENSE_FILE" ] || fail "missing or empty $LICENSE_FILE"
for artifact in "$DIST"/*.tar.gz; do
  [ -e "$artifact" ] || continue
  tar tzf "$artifact" | grep -qx "LICENSE" || fail "$artifact missing LICENSE at archive root"
done
for artifact in "$DIST"/*.zip; do
  [ -e "$artifact" ] || continue
  unzip -l "$artifact" | awk '{print $NF}' | grep -qx "LICENSE" || fail "$artifact missing LICENSE at archive root"
done

echo "" >&2
echo "verify-release-readiness: OK (VERSION=$VERSION)" >&2
