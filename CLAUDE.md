# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

IncidentDNA converts production incidents into permanent executable regression scenarios, so future releases can be tested against previously observed failures ("production failure memory"). Phase 1 (current) builds only the vendor-neutral foundation: a format for representing incidents (IDIR), and a CLI to validate/fingerprint/inspect/compare them. See `docs/product-scope.md` for what is and isn't in scope for this phase.

## Commands

The host machine is not assumed to have Go installed — all commands run through Docker via the Makefile:

```
make format   # gofmt -w .
make lint     # gofmt -l check + go vet
make test     # go test ./... -race -count=1
make build    # go build -o bin/incidentdna ./cmd/incidentdna
make example  # build, then validate + fingerprint examples/duplicate-payment/incident.yaml
make verify   # lint + test + build + example + scripts/verify-golden-fingerprint.sh
make clean    # rm -rf bin
```

Run a single test: `docker compose run --rm dev go test ./internal/validate/... -run TestValidate_RejectsEachRequiredCase -v` (or any package/pattern). If Go is available directly (e.g. in CI or a local toolchain install), drop the `docker compose run --rm dev` prefix and run the same `go`/`gofmt` commands directly — the Makefile is a thin wrapper, not a requirement.

CI (`.github/workflows/ci.yml`) runs the same `make` targets, so local and CI verification never diverge.

## Architecture

Full detail in `docs/architecture.md`; the key shape:

```
internal/idir        — IDIR v0.1 Go types + size-capped JSON/YAML loader (the shared vocabulary; imports nothing else internal)
internal/validate     — semantic rule engine (cycles, dangling refs, digest format, redaction, etc.)
internal/canonical    — deterministic ("canonical") JSON re-encoder, sorted keys, fixed formatting
internal/fingerprint  — extracts an identity-only payload from a Document, canonicalizes it, sha256-hashes it
internal/compare      — compares two documents' fingerprints and explains material differences
cmd/incidentdna       — CLI: init, validate, fingerprint, inspect, compare (stdlib flag, no framework)
```

Data flow: file bytes → `idir.LoadFile` (size cap + format sniff + typed decode) → `idir.Document` → `validate.Validate` → (if valid) `fingerprint.Compute` (identity-payload extraction → `canonical.Marshal` → sha256) → `sha256:...` string. `compare.Documents` runs this for two documents and diffs them if they differ.

Only non-stdlib dependency in the whole module: `gopkg.in/yaml.v3` (deliberate — see `docs/architecture.md`, "Rejected alternatives", for why cobra/golangci-lint/a JSON-schema library were each considered and not used).

Exit codes are meaningful and relied on by CI: `0` success, `1` I/O/parse/usage error, `2` semantic validation failure.

The fingerprint's included/excluded fields are a deliberate design decision documented in `docs/fingerprint-design.md` — read it before changing what the fingerprint payload covers; golden tests (`internal/fingerprint/golden_test.go`, `scripts/verify-golden-fingerprint.sh`) will fail loudly (by design) if the algorithm or the example changes, and `testdata/golden/duplicate-payment.fingerprint` must be updated deliberately, not silently, when that happens.

No network access and no telemetry anywhere in `cmd/` or `internal/` — every subcommand is pure local file I/O. Do not add either without discussing it first; it's a stated invariant in `docs/threat-model.md`.

## Repository state note

This repository is separate from `/home/ajay/projects/omniflow` (a different, unrelated project) — do not read from or write to it as part of work here unless explicitly asked to integrate the two.
