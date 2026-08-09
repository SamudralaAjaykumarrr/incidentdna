.PHONY: format lint test build example example-evidence example-library example-scenario example-suite example-library-crossref example-policy example-ci-consumption verify dist checksums sbom bench release-verify clean

# Every target runs inside the dev container defined by Dockerfile.dev /
# compose.yaml, so a contributor never needs Go installed on the host.
RUN := docker compose run --rm --remove-orphans dev

# Phase 8 release targets (dist, checksums, sbom, bench, release-verify)
# read VERSION from the environment (default "dev" inside each script, per
# scripts/build-release.sh). `docker compose run` does not forward host
# environment variables into the container unless named explicitly via -e,
# so these targets use RUN_VERSIONED instead of RUN to pass VERSION
# through — e.g. `VERSION=v0.1.0 make release-verify`.
RUN_VERSIONED := docker compose run --rm --remove-orphans -e VERSION dev

format:
	$(RUN) gofmt -w .

lint:
	$(RUN) sh -c '\
		unformatted="$$(gofmt -l .)"; \
		if [ -n "$$unformatted" ]; then \
			echo "gofmt: the following files are not formatted:" >&2; \
			echo "$$unformatted" >&2; \
			exit 1; \
		fi; \
		go vet ./...'

test:
	$(RUN) go test ./... -race -count=1

build:
	$(RUN) go build -buildvcs=false -o bin/incidentdna ./cmd/incidentdna

example: build
	$(RUN) sh -c '\
		./bin/incidentdna validate examples/duplicate-payment/incident.yaml && \
		./bin/incidentdna fingerprint examples/duplicate-payment/incident.yaml'

# Phase 2: end-to-end check of `incidentdna evidence store|verify|list|inspect`
# against examples/evidence-storage-demo/, using a temporary --store directory
# (never the default .incidentdna/evidence/objects) so this never leaves
# generated evidence-store output in the repository. See
# scripts/verify-evidence-demo.sh and docs/evidence-storage.md.
example-evidence: build
	$(RUN) sh scripts/verify-evidence-demo.sh

# Phase 3: end-to-end check of `incidentdna library add|check|list` against
# examples/incident-library-demo/, using a temporary --library directory
# (never the default .incidentdna/library/objects) so this never leaves
# generated library output in the repository. See
# scripts/verify-library-demo.sh and docs/incident-library.md.
example-library: build
	$(RUN) sh scripts/verify-library-demo.sh

# Phase 4: end-to-end check of `incidentdna scenario verify|run` against
# examples/regression-scenario-demo/, covering all five outcomes
# (PASS/FAIL/TIMEOUT/INVALID/INTERNAL_ERROR) plus the --source
# fingerprint-linkage cross-check, using temporary --workspace/--report
# locations (never the current directory) so this never leaves generated
# output in the repository. See scripts/verify-scenario-demo.sh and
# docs/regression-scenarios.md.
example-scenario: build
	$(RUN) sh scripts/verify-scenario-demo.sh

# Phase 5: end-to-end check of `incidentdna suite verify|run` against
# examples/regression-suite-demo/, covering both aggregate outcomes
# (PASS/FAIL), --fail-fast, aggregate INVALID, and --workspace-root/
# --keep-workspaces, using temporary --workspace-root/--report locations
# (never the current directory) so this never leaves generated output in
# the repository. See scripts/verify-suite-demo.sh and
# docs/scenario-suites.md.
example-suite: build
	$(RUN) sh scripts/verify-suite-demo.sh

# Phase 6: end-to-end check of the `--library` cross-reference flag on
# `incidentdna scenario verify` and `incidentdna suite verify`, composing
# already-existing fixtures from examples/duplicate-payment/,
# examples/regression-scenario-demo/, and examples/regression-suite-demo/
# (no new example directory — Phase 6 introduces no new document format),
# using temporary --library directories (never the default
# .incidentdna/library/objects) so this never leaves generated output in
# the repository. See scripts/verify-library-crossref-demo.sh and
# docs/library-crossref.md.
example-library-crossref: build
	$(RUN) sh scripts/verify-library-crossref-demo.sh

# Phase 7: end-to-end check of `incidentdna policy verify|evaluate` against
# policies/release-gate-example.yaml, composing already-existing fixtures
# from examples/duplicate-payment/, examples/regression-scenario-demo/, and
# examples/regression-suite-demo/ (no new example directory), using
# temporary report/--library/--report locations (never the current
# directory or the default .incidentdna/library/objects) so this never
# leaves generated output in the repository. See
# scripts/verify-policy-demo.sh and docs/policy-evaluation.md.
example-policy: build
	$(RUN) sh scripts/verify-policy-demo.sh

# Phase 8: end-to-end check of examples/ci-consumption-example/
# check-release-gate.sh against policies/release-gate-example.yaml and
# examples/regression-suite-demo/, covering both the PASS (exit 0) and FAIL
# (exit 2) release-gate outcomes. Not part of verify's chain (see
# docs/phase-8-plan.md §14 vs. §21/§25: this Makefile follows §21/§25's
# explicit "verify's existing dependency chain is unchanged" requirement,
# the more specific and more repeated of the two) — available standalone,
# and exercised as stage 6 of `make release-verify`
# (scripts/verify-release-readiness.sh). See
# scripts/verify-ci-consumption-example.sh and
# examples/ci-consumption-example/README.md.
example-ci-consumption: build
	$(RUN) sh scripts/verify-ci-consumption-example.sh

verify: lint test build example example-evidence example-library example-scenario example-suite example-library-crossref example-policy
	$(RUN) sh scripts/verify-golden-fingerprint.sh

# --- Phase 8: release engineering targets (docs/phase-8-plan.md §23) ---
# Additive only, never inserted into verify's dependency chain above —
# that chain, and its pass/fail semantics, are unchanged by Phase 8.

# Reproducible multi-platform release build (scripts/build-release.sh) into
# dist/. VERSION (env, default "dev") selects the injected version string.
dist: build
	$(RUN_VERSIONED) sh scripts/build-release.sh

# SHA-256 checksums for every archive in dist/ (scripts/generate-checksums.sh).
checksums: dist
	$(RUN_VERSIONED) sh scripts/generate-checksums.sh

# Minimal SBOM / dependency inventory (scripts/generate-sbom.sh). Independent
# of dist/checksums — reads only go.sum via `go list -m -json all`.
sbom:
	$(RUN_VERSIONED) sh scripts/generate-sbom.sh

# Informational Go benchmarks (scripts/run-benchmarks.sh). Never gates CI or
# a release — see docs/phase-8-plan.md §13. Piped straight into
# run-benchmarks.sh, which both writes dist/*-bench.txt and echoes the
# output back to stdout itself — no `tee /dev/stderr` here, since that
# device isn't reliably writable in every local/container/Windows
# environment (see scripts/run-benchmarks.sh's header comment).
bench:
	$(RUN_VERSIONED) sh -c 'go test -bench=. -benchmem -run=^$$ ./... | sh scripts/run-benchmarks.sh'

# Full release-readiness acceptance check (scripts/verify-release-readiness.sh,
# docs/phase-8-plan.md §19). Meaningfully slower than `verify` (builds five
# platform targets twice each) — run explicitly when preparing a release,
# e.g. `VERSION=v0.1.0 make release-verify`, never on every ordinary push.
release-verify: dist checksums sbom bench
	$(RUN_VERSIONED) sh scripts/verify-release-readiness.sh

clean:
	rm -rf bin dist
