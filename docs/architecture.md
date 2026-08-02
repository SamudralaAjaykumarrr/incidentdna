# Architecture

## Package layout

```
cmd/incidentdna/       CLI entrypoint and one file per subcommand
internal/idir/         IDIR v0.1 Go types + size-capped JSON/YAML loader
internal/validate/      Semantic rule engine (Result/Issue + rule functions)
internal/canonical/     Deterministic ("canonical") JSON re-encoder
internal/fingerprint/   Identity-payload extraction -> canonical -> sha256
internal/compare/       Fingerprint comparison + per-dimension diff
internal/evidence/      Local content-addressed evidence store + digest verification
internal/library/       Local incident library: occurrences grouped by fingerprint
schemas/idir/v0.1/      Documentation-grade JSON Schema for the format
examples/duplicate-payment/     Synthetic example used by tests and `make example`
examples/evidence-storage-demo/ Separate synthetic example for `incidentdna evidence`
examples/incident-library-demo/ Separate synthetic example for `incidentdna library`
testdata/golden/        Golden fingerprint + one fixture per rejected validation case
```

Dependency direction is strictly one-way:
`idir` ← `validate`, `fingerprint`, `evidence`, `library`; `canonical` ←
`fingerprint`, `library`; `fingerprint` ← `compare`, `library`; all of the
above ← `cmd/incidentdna`. Nothing in `internal/` imports `cmd/incidentdna`,
and nothing in `internal/idir` imports any other internal package — it is
the shared vocabulary everything else builds on. `internal/evidence` imports
`internal/idir` the same way `fingerprint` and `compare` already do (to read
`doc.Evidence` entries); it does not import, and is not imported by,
`validate`, `canonical`, `fingerprint`, or `compare` — evidence storage is a
parallel concern, not a dependency of the document-representation/
fingerprinting pipeline. `internal/library` imports `internal/idir`,
`internal/fingerprint`, and `internal/canonical` — reusing
`fingerprint.Compute` unchanged for identity and `canonical.Marshal` for an
occurrence's own stored bytes and digest — and is not imported by `idir`,
`validate`, `canonical`, `fingerprint`, `compare`, or `evidence`; `library`
and `evidence` remain independent, parallel concerns that do not import each
other.

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

## Evidence storage (Phase 2)

`internal/evidence` is a parallel data flow, not a stage inserted into the
one above — it never runs as part of `validate`/`fingerprint`/`compare`, and
nothing about its output changes a fingerprint:

```
evidence file bytes (evidence store)          idir.Document (evidence verify/list)
   │  stream + sha256                            │  for each evidence[] entry
   ▼                                              ▼
digest-derived object path                    Store.objectPath(digest) lookup
   │  atomic write (temp file + rename)           │  Lstat (list) or open+re-hash (verify)
   ▼                                              ▼
object at <store-root>/<shard>/<hash>         OK / MISSING / CORRUPTED / INVALID per entry
```

See [`docs/evidence-storage.md`](evidence-storage.md) for the full design:
store layout, the four `incidentdna evidence` subcommands, resource limits,
and path-traversal/symlink protections.

## Incident library (Phase 3)

`internal/library` is also a parallel data flow, independent of both the
main validate/fingerprint/compare pipeline and `internal/evidence` — it
reuses `fingerprint.Compute` and `canonical.Marshal` unchanged rather than
recomputing identity or canonicalization itself, and nothing about its
output changes a fingerprint:

```
idir.Document (library add/check)              fingerprint string (library add/check)
   │  idir.LoadFile + validate.Validate            │  fingerprint.Compute(doc)
   ▼                                                ▼
validated idir.Document ─────────────────────▶ fingerprint-derived directory
                                                    │  canonical.Marshal(doc) -> canonical bytes
                                                    │  sha256(canonical bytes) -> occurrence digest
                                                    ▼
                                    <library-root>/<fp shard>/<fp remainder>/
                                        index.json   (digest -> incident.id, occurred_at)
                                        occurrences/<digest shard>/<digest remainder>.json
```

The directory for a failure class is addressed by the document's own
fingerprint; the occurrence object within it is addressed by the SHA-256
digest of the occurrence's own canonical document bytes — never by
`incident.id` or any other document-supplied value. A fingerprint groups
potentially many occurrences (it identifies a failure *class*, not a unique
occurrence): `library add` decides, per incoming document, whether to store
a new occurrence, treat the add as an idempotent no-op, or refuse it as a
conflict. See [`docs/incident-library.md`](incident-library.md) for the full
design: store layout, the three `incidentdna library` subcommands, the
occurrence-decision semantics, resource limits, the privacy gate, and
path-traversal/symlink protections.

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
  This holds for the evidence store too: `evidence store`/`verify`/`list`/
  `inspect` derive every filesystem path from a validated digest, never from
  `location`, an evidence entry's `id`/`type`, or the original filename
  passed to `store` — see [`evidence-storage.md`](evidence-storage.md). The
  incident library follows the identical rule: `library add`/`check`/`list`
  derive every filesystem path from a validated fingerprint or occurrence
  digest, never from `incident.id` or any other document-supplied value —
  see [`incident-library.md`](incident-library.md).

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

## What this codebase deliberately does not build

See [`product-scope.md`](product-scope.md) for the full list. Architecturally,
the important point is that nothing in `internal/` assumes a particular
transport, storage engine, or caller — `idir.Document` is a plain Go struct,
every package function is a pure transformation over it, and the CLI is a
thin wrapper. That's what makes it safe to build a web API, a storage layer,
or an ingestion pipeline on top of this in a later phase without having to
revisit the core representation. `internal/library` (Phase 3) is itself an
example of this: it is a new local store built entirely on Phase 1/2
primitives (`idir`, `fingerprint`, `canonical`) without any change to them,
and it still does not build a release gate, executable regression scenarios,
remote/shared storage, or a signing/authenticity layer — those remain future
work.
