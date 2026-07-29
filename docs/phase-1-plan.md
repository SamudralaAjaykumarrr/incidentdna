# Phase 1 Plan

This is the as-built plan for Phase 1: the vendor-neutral foundation for
representing, validating, hashing, and comparing production incidents. It
reflects what was actually implemented, not just what was proposed before
implementation started — see [`phase-1-report.md`](phase-1-report.md) for
the verification transcript proving it.

## Objective

Build IDIR (Incident Deterministic Intermediate Representation), a CLI
(`incidentdna`) to validate/fingerprint/inspect/compare IDIR documents, one
complete synthetic example, and a containerized dev/CI workflow — with no
web framework, cloud infra, event ingestion, AI, or SaaS concerns. See
[`product-scope.md`](product-scope.md) for the full in/out-of-scope list.

## Files created

```
go.mod, go.sum
cmd/incidentdna/
  main.go, helpers.go
  cmd_init.go, cmd_validate.go, cmd_fingerprint.go, cmd_inspect.go, cmd_compare.go
  cli_test.go
internal/idir/
  version.go, types.go, load.go, load_test.go
internal/validate/
  validate.go, errors.go, validate_test.go, testdata_test.go
internal/canonical/
  canonical.go, canonical_test.go
internal/fingerprint/
  fingerprint.go, fingerprint_test.go, golden_test.go
internal/compare/
  compare.go, compare_test.go
schemas/idir/v0.1/idir.schema.json
examples/duplicate-payment/incident.yaml
testdata/golden/
  duplicate-payment.fingerprint
  invalid/*.yaml (12 fixtures, one per rejected validation case)
scripts/verify-golden-fingerprint.sh
Dockerfile.dev, compose.yaml, Makefile
.github/workflows/ci.yml
docs/
  product-scope.md, architecture.md, idir-specification-v0.1.md,
  fingerprint-design.md, threat-model.md, privacy-model.md,
  phase-1-plan.md (this file), phase-1-report.md
```

## Architectural decisions

See [`architecture.md`](architecture.md) ("Rejected alternatives") for the
full reasoning; summarized:

- **stdlib `flag`**, not a CLI framework — five simple subcommands don't
  justify the dependency.
- **`gopkg.in/yaml.v3`** is the only non-stdlib dependency in the whole
  module. Required because IDIR supports YAML input; hand-rolling a YAML
  parser would be both more work and a larger attack surface.
- **Hand-written semantic validator**, not a JSON Schema validation library.
  Acyclic-causality, dangling-reference, and redaction-consistency checks
  cannot be expressed in JSON Schema. The schema file is kept as a
  documentation/interchange artifact only.
- **`go vet` + `gofmt -l`**, not golangci-lint — the spec explicitly allows
  "go vet or appropriate linting," and this avoids a large external linter
  dependency for a codebase this size.
- **Dedicated `identityPayload` struct with fixed field order**, plus a
  separately-testable `internal/canonical` package, rather than ad hoc
  map-based canonicalization buried inside the fingerprint code.
- **Field-presence redaction check with a regex backstop**, not general PII
  detection — see [`privacy-model.md`](privacy-model.md) for why this is
  the honest scope for Phase 1.

## Acceptance criteria (all met — see phase-1-report.md)

1. All 8 required docs exist.
2. All 5 CLI commands work end-to-end against
   `examples/duplicate-payment/incident.yaml`.
3. `go test ./... -race -count=1` passes, including: formatting-only copy →
   identical fingerprint; causal-structure change → different fingerprint;
   cyclic-causality fixture → rejected; every VALIDATION REQUIREMENTS bullet
   has a table-driven test case and a dedicated fixture.
4. `gofmt -l` reports no files; `go vet ./...` is clean; `go build ./...`
   succeeds.
5. `scripts/verify-golden-fingerprint.sh` passes against the built binary.
6. No network calls and no telemetry in any CLI code path.
7. `docs/phase-1-report.md` contains real, executed command transcripts.
8. Nothing was committed, staged, or pushed — working tree only.

## Security-sensitive assumptions

Recorded in full in [`threat-model.md`](threat-model.md); the load-bearing
ones for Phase 1's design:

- A CLI's trust boundary is the file path a human gives it on the command
  line — the tool must never dereference a path found *inside* a document
  (`evidence[].location` is inert metadata for this reason).
- Structural decoding into typed Go structs (never `interface{}`/maps) is
  itself a YAML-parser-abuse mitigation, not just a convenience.
- A size cap enforced *before* parsing is the primary defense against
  resource exhaustion; a context timeout is a secondary defense against
  CPU-bound work within an under-the-cap document.
- "Redacted" is a mechanically-checkable claim only against a fixed,
  documented field list — never conflated with actual PII-safety of free
  text elsewhere in the document.

## Environment note

The sandbox this phase was implemented in has no `docker`, `go`, or
`podman`, and no passwordless sudo. Verification below was run against a
Go 1.26.5 toolchain installed to `$HOME/.local/go1.26.5` (user-space, no
root) rather than through the `Dockerfile.dev`/`compose.yaml` path a real
contributor would use — `Dockerfile.dev` is pinned to the identical
`golang:1.26.5` base image, and `Makefile`/`compose.yaml` were sanity-checked
for syntax (`make -n <target>` dry-run expansion, `docker compose config`
via manual YAML parse) but not executed through an actual Docker daemon.
See [`phase-1-report.md`](phase-1-report.md) for exactly what was and
wasn't run.
