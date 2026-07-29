.PHONY: format lint test build example verify clean

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

verify: lint test build example
	$(RUN) sh scripts/verify-golden-fingerprint.sh

clean:
	rm -rf bin
