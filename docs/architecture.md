# Architecture

## Package layout

```
cmd/incidentdna/       CLI entrypoint and one file per subcommand
internal/idir/         IDIR v0.1 Go types + size-capped JSON/YAML loader
internal/validate/      Semantic rule engine (Result/Issue + rule functions)
internal/canonical/     Deterministic ("canonical") JSON re-encoder
internal/fingerprint/   Identity-payload extraction -> canonical -> sha256
internal/compare/       Fingerprint comparison + per-dimension diff
schemas/idir/v0.1/      Documentation-grade JSON Schema for the format
examples/duplicate-payment/  Synthetic example used by tests and `make example`
testdata/golden/        Golden fingerprint + one fixture per rejected validation case
```

Dependency direction is strictly one-way:
`idir` ← `validate`, `fingerprint`; `canonical` ← `fingerprint`; `fingerprint`
← `compare`; all of the above ← `cmd/incidentdna`. Nothing in `internal/`
imports `cmd/incidentdna`, and nothing in `internal/idir` imports any other
internal package — it is the shared vocabulary everything else builds on.

## Data flow

```
file bytes (JSON or YAML)
   │  internal/idir.LoadFile — size cap, format sniff, typed decode
   ▼
idir.Document (typed struct)
   │  internal/validate.Validate — semantic rules, returns Result{Issues}
   ▼
(valid) idir.Document
   │  internal/fingerprint.buildIdentityPayload — extract identity-only subset
   ▼
identityPayload (fixed field order)
   │  internal/canonical.Marshal — sorted keys, fixed formatting
   ▼
canonical JSON bytes
   │  sha256 + "sha256:" prefix
   ▼
fingerprint string
```

`internal/compare` runs this pipeline for two documents and, if the
resulting fingerprints differ, additionally builds a human-readable
dimension-by-dimension diff (trigger, causal structure, invariants, side
effect types, recovery behavior, technology categories) — that diff logic is
presentation only and never affects the fingerprint value itself.

## CLI conventions

- **Exit codes**: `0` success; `1` I/O, parse, or usage error; `2` semantic
  validation failure. CI and scripting rely on `2` specifically meaning "the
  document is well-formed but not a valid IDIR document," distinct from `1`
  meaning "something environmental or structural went wrong."
- **No network access, no telemetry.** Every subcommand operates purely on
  local files named on the command line.
- **Context-aware execution.** `main.go` wires `signal.NotifyContext` and a
  30s overall timeout into the `context.Context` passed to every subcommand;
  `internal/validate`'s loops check `ctx.Err()` periodically so a bounded
  timeout actually has an effect on a large document, not just on I/O.
- **Never dereferences evidence locations.** `evidence[].location` is
  free-text metadata; the CLI never opens, fetches, or otherwise interprets
  it. This is a deliberate scope boundary, not an oversight — see
  [`threat-model.md`](threat-model.md), "Path traversal through CLI inputs."

## Rejected alternatives

- **cobra / urfave-cli → stdlib `flag`.** Five subcommands with simple
  positional arguments do not justify a CLI framework dependency.
- **JSON Schema validation library (e.g. santhosh-tekuri/jsonschema) →
  hand-written semantic validator.** Cycle detection, dangling-reference
  checks, and the redaction rule cannot be expressed in plain JSON Schema.
  `schemas/idir/v0.1/idir.schema.json` is kept as a structural,
  documentation-grade/interchange contract; `internal/validate` is the
  actual enforcement mechanism.
- **golangci-lint → `go vet` + `gofmt -l`.** The spec explicitly allows "go
  vet or appropriate linting"; this avoids pulling in a large external
  linter binary and dependency tree for a Phase 1 codebase this size.
- **`map[string]interface{}` canonicalization → typed `identityPayload`
  struct + a general-purpose recursive canonical encoder as a separate
  primitive.** This gives the fingerprint payload fixed field order *and* a
  real, independently-testable `internal/canonical` package, rather than
  folding canonicalization logic invisibly into the fingerprint package.
- **General PII/regex scanning as the primary redaction control → field-
  presence check on a fixed, documented list of "known sensitive locations,"
  with regex as a secondary backstop.** Deterministic and testable, and
  honestly scoped — see [`privacy-model.md`](privacy-model.md) for why this
  is not general-purpose PII detection.

## What Phase 1 deliberately does not build

See [`product-scope.md`](product-scope.md) for the full list. Architecturally,
the important point is that nothing in `internal/` assumes a particular
transport, storage engine, or caller — `idir.Document` is a plain Go struct,
every package function is a pure transformation over it, and the CLI is a
thin wrapper. That's what makes it safe to build a web API, a storage layer,
or an ingestion pipeline on top of this in a later phase without having to
revisit the core representation.
