.PHONY: format lint test build example example-evidence example-library example-scenario example-suite example-library-crossref verify clean

# Every target runs inside the dev container defined by Dockerfile.dev /
# compose.yaml, so a contributor never needs Go installed on the host.
RUN := docker compose run --rm --remove-orphans dev

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

verify: lint test build example example-evidence example-library example-scenario example-suite example-library-crossref
	$(RUN) sh scripts/verify-golden-fingerprint.sh

clean:
	rm -rf bin
