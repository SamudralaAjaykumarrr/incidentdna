.PHONY: format lint test build example example-evidence verify clean

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

verify: lint test build example example-evidence
	$(RUN) sh scripts/verify-golden-fingerprint.sh

clean:
	rm -rf bin
