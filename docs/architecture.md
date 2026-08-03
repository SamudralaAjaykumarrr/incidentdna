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
internal/scenario/      Local, bounded, offline regression-scenario runner (IRS v0.1)
internal/suite/         Local, sequential, offline scenario suite runner (ISM v0.1)
                         (library cross-reference, Phase 6, lives in cmd/incidentdna — see below)
schemas/idir/v0.1/      Documentation-grade JSON Schema for the format
examples/duplicate-payment/     Synthetic example used by tests and `make example`
examples/evidence-storage-demo/ Separate synthetic example for `incidentdna evidence`
examples/incident-library-demo/ Separate synthetic example for `incidentdna library`
examples/regression-scenario-demo/ Separate synthetic example for `incidentdna scenario`
examples/regression-suite-demo/ Separate synthetic example for `incidentdna suite`
testdata/golden/        Golden fingerprint + one fixture per rejected validation case
```

Dependency direction is strictly one-way:
`idir` ← `validate`, `fingerprint`, `evidence`, `library`, `scenario`;
`canonical` ← `fingerprint`, `library`; `fingerprint` ← `compare`, `library`,
`scenario`; `validate` ← `scenario` (in addition to `cmd/incidentdna`); all
of the above ← `cmd/incidentdna`. Nothing in `internal/` imports
`cmd/incidentdna`, and nothing in `internal/idir` imports any other internal
package — it is the shared vocabulary everything else builds on.
`internal/evidence` imports `internal/idir` the same way `fingerprint` and
`compare` already do (to read `doc.Evidence` entries); it does not import,
and is not imported by, `validate`, `canonical`, `fingerprint`, or `compare`
— evidence storage is a parallel concern, not a dependency of the
document-representation/fingerprinting pipeline. `internal/library` imports
`internal/idir`, `internal/fingerprint`, and `internal/canonical` — reusing
`fingerprint.Compute` unchanged for identity and `canonical.Marshal` for an
occurrence's own stored bytes and digest — and is not imported by `idir`,
`validate`, `canonical`, `fingerprint`, `compare`, or `evidence`; `library`
and `evidence` remain independent, parallel concerns that do not import each
other. `internal/scenario` imports `internal/idir`, `internal/validate`, and
`internal/fingerprint` — the three needed for the optional `--source`
fingerprint cross-check, reusing them unchanged — and is not imported by
`idir`, `validate`, `canonical`, `fingerprint`, `compare`, `evidence`, or
`library`; `scenario` is a fourth independent, parallel leaf concern that
does not import `evidence` or `library` and is not imported by either.
`internal/suite` imports only `internal/scenario` — `LoadFile`, `Validate`,
and `Run`, all unchanged — and is not imported by `idir`, `validate`,
`canonical`, `fingerprint`, `compare`, `evidence`, `library`, or `scenario`;
`suite` is a fifth independent, parallel leaf concern, one level further
from the shared primitives than `scenario` itself. Phase 6 adds no new
package and changes this dependency graph in exactly one place: `library`
gains one new exported function (`CheckFingerprint`), read unchanged by
`cmd/incidentdna`; `scenario` and `suite` still do not import, and are not
imported by, `library` — the composition that connects a scenario's or
suite's `linked_fingerprint` to library occurrences lives entirely at the
`cmd/incidentdna` layer, which already imported all three packages before
Phase 6 (see "Library cross-reference (Phase 6)" below).

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

## Executable regression scenarios (Phase 4)

`internal/scenario` is a fourth parallel data flow, independent of
`internal/evidence` and `internal/library` as well as the main pipeline. It
is the first package in this codebase to perform local process execution —
a categorically new class of risk (see `docs/regression-scenarios.md`, "A
new class of risk") — so its data flow includes a step none of the earlier
packages have: launching and bounding a child process, rather than only
reading, hashing, or storing bytes.

```
IRS v0.1 scenario document (scenario verify/run)     linked_fingerprint (declared in the document)
   │  scenario.LoadFile — size cap, format sniff, typed decode
   ▼
scenario.Document
   │  scenario.Validate — semantic rules (schema, command shape,
   │  workspace_files safety, timeout bounds, expected shape)
   ▼
(valid) scenario.Document ──▶ scenario.CheckSourceFingerprint (--source only)
   │  scenario.Run                  │  idir.LoadFile + validate.Validate +
   │  (verify re-run internally,    │  fingerprint.Compute (unchanged)
   │   then workspace + exec)       ▼
   ▼                          match/mismatch against linked_fingerprint
bounded workspace (os.MkdirTemp or
 --workspace) + exec.Command,
 no shell, own process group
   │  timeout-bounded, output-capped
   ▼
PASS / FAIL / TIMEOUT / INVALID / INTERNAL_ERROR
   │  scenario.BuildReport + scenario.WriteReport (--report only)
   ▼
deterministic JSON execution report
```

A scenario's `linked_fingerprint` is a declared claim, format-checked
always and cross-checked against a fingerprint freshly computed from a
named `--source` incident file only when given — never looked up against
`internal/library`'s stored occurrences, keeping `internal/scenario` a true
parallel leaf package and keeping a scenario runnable in a checkout that has
never run `library add`. See [`docs/regression-scenarios.md`](regression-scenarios.md)
for the full design: the IRS v0.1 document format, the two `incidentdna
scenario` subcommands, the safety model, execution isolation boundaries,
resource limits, and path-traversal/symlink protections.

## Scenario suites (Phase 5)

`internal/suite` is a fifth parallel data flow, independent of
`internal/evidence` and `internal/library` as well as the main pipeline. It
is the first package in this codebase that never calls `exec.Command`
itself, directly or indirectly through a new code path — every process a
suite launches is launched by the unchanged Phase 4 `internal/scenario.Run`,
once per listed scenario:

```
ISM v0.1 suite manifest (suite verify/run)
   │  suite.LoadFile — size cap, format sniff, typed decode
   ▼
suite.Document
   │  suite.Validate — schema_version, non-empty scenarios, path safety,
   │  duplicate-path rejection, MaxScenariosPerSuite, aggregate timeout
   │  bound — and, for every listed scenario it resolves to a real file,
   │  scenario.LoadFile + scenario.Validate (unchanged)
   ▼
(valid) suite.Document, every listed scenario also valid
   │  suite.Run — sequential, declared-order iteration
   ▼
   for each listed scenario:
     scenario.LoadFile + scenario.Run (unchanged, one fresh workspace each)
        ▼
     PASS / FAIL / TIMEOUT / INVALID / INTERNAL_ERROR (per scenario, unchanged)
   │
   ▼
aggregate PASS (every scenario PASSed) or FAIL (otherwise) — a pure function
of the per-scenario outcomes
   │  suite.BuildReport + suite.WriteReport (--report only)
   ▼
deterministic JSON aggregate report, embedding each executed scenario's own
unmodified scenario.Report
```

A suite manifest *names* scenarios by declared, explicit path — no
directory walk, no glob — the same "nothing is resolved implicitly, only
what a reviewer can see in the file" discipline `execution.workspace_files`
already established for a single scenario in Phase 4, extended here to a
list of scenarios instead of one file's staged fixtures. See
[`scenario-suites.md`](scenario-suites.md) for the full design: the ISM
v0.1 manifest format, the two `incidentdna suite` subcommands, aggregate
outcome classification, `--fail-fast`, resource limits, and
path-traversal/symlink protections.

## Library cross-reference (Phase 6)

Unlike every prior phase, Phase 6 adds no new parallel data flow and no new
package — it composes two already-existing flows at the `cmd/incidentdna`
layer only, inside the two existing `verify` subcommands, behind an optional
`--library <dir>` flag:

```
scenario.Document / suite.ScenarioEntry     library.Store (opened via the
   │  .LinkedFingerprint (already declared,   existing library.Open, the
   │   already format-checked)                same store `library check`
   ▼                                          reads)
linked_fingerprint string ──────────────────────────┐
                                                      │  library.CheckFingerprint
                                                      │  (Phase 6, NEW — reuses
                                                      │  Check's lookup logic
                                                      │  after fingerprint
                                                      │  computation)
                                                      ▼
                                    CheckResult{Outcome, MatchCount}
                                                      │
                                                      ▼
                              "Library: N occurrence(s) found" /
                              "Library: no occurrences found" (printed only,
                              never affecting scenario/suite verify's own
                              exit code)
```

`cmd_scenario.go`'s `runScenarioVerify` and `cmd_suite.go`'s
`runSuiteVerify` each call `library.CheckFingerprint` directly — no new
function is added to `internal/scenario` or `internal/suite`, and neither
package gains a new import. `runSuiteVerify` additionally deduplicates the
listed scenarios' `LinkedFingerprint` values in first-occurrence order
before looking each up, so a suite listing many scenarios that share one
fingerprint performs at most one lookup per distinct fingerprint. See
[`library-crossref.md`](library-crossref.md) for the full design: the exact
CLI output, the exit-code contract, and what this phase explicitly does not
provide.

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
  `scenario run` is the one deliberate exception: `main.go` special-cases
  the `scenario` command to skip that fixed 30s wrapping, since `scenario
  run` needs its own, larger, document-declared timeout budget for the
  child process it executes (up to `MaxScenarioTimeoutSeconds`) — `scenario
  verify`, which never executes anything, restores the usual 30s budget
  itself. See [`regression-scenarios.md`](regression-scenarios.md),
  "Resource limits." `suite run` is the same kind of exception: `main.go`
  special-cases the `suite` command identically, since `suite run` needs
  its own budget for the (potentially many) bounded child-process
  executions it performs in sequence, bounded by
  `suite.MaxSuiteTotalTimeoutSeconds` — `suite verify` restores the usual
  30s budget itself. See [`scenario-suites.md`](scenario-suites.md),
  "Resource limits."
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
  see [`incident-library.md`](incident-library.md). `scenario verify`/`run`
  derive every workspace filesystem path from a scenario's own declared
  `workspace_files` entries, each re-checked to resolve within its
  respective root (the scenario file's own directory for `source`, the
  workspace root for `destination`) — see
  [`regression-scenarios.md`](regression-scenarios.md). `suite verify`/`run`
  derive every listed-scenario filesystem path from a suite manifest's own
  declared `scenarios[].path` entries, each re-checked to resolve within the
  manifest's own directory — see [`scenario-suites.md`](scenario-suites.md).

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
remote/shared storage, or a signing/authenticity layer — those remained
future work at the time. `internal/scenario` (Phase 4) closes the
"executable regression scenarios" gap specifically, again built entirely on
Phase 1 primitives (`idir`, `validate`, `fingerprint`) without any change to
them — but it still does not build a release gate, a suite runner, or
automatic coupling to `internal/library`; those remain future work. See
[`regression-scenarios.md`](regression-scenarios.md), "What Phase 4
explicitly does not provide." `internal/suite` (Phase 5) closes the "suite
runner" gap specifically, built entirely on the unchanged Phase 4
`internal/scenario` package (`LoadFile`, `Validate`, `Run`) without any
change to it — but it still does not build a release gate, directory/glob
scenario discovery, parallel execution, or automatic coupling to
`internal/library`; those remain future work. See
[`scenario-suites.md`](scenario-suites.md), "What Phase 5 explicitly does
not provide." `internal/library` (Phase 6) closes the "automatic
cross-reference" gap specifically — one new function
(`library.CheckFingerprint`), composed with the unchanged Phase 4/5 runners
entirely at the `cmd/incidentdna` layer — but it still does not build
release gating, and `internal/scenario`/`internal/suite` remain unaware the
incident library exists. See [`library-crossref.md`](library-crossref.md),
"What Phase 6 explicitly does not provide."
