# IncidentDNA

**Production Failure Memory** — IncidentDNA converts a production incident
into a permanent, executable regression scenario, so future releases can be
tested against failures that have already happened instead of relying on
whoever remembers the postmortem.

[![CI](https://github.com/SamudralaAjaykumarrr/incidentdna/actions/workflows/ci.yml/badge.svg)](https://github.com/SamudralaAjaykumarrr/incidentdna/actions/workflows/ci.yml)

## The problem

Production incidents produce postmortems, tickets, and tribal knowledge —
but rarely a durable, machine-checkable artifact. The specific causal
sequence that produced a duplicate charge, a data race, or a cascading
timeout is easy to describe in prose and easy to reintroduce eighteen months
later, once the engineers who lived through it have moved on or forgotten
the details. Nothing about "we fixed this in March" is checked automatically
against "did we just reintroduce it in November."

## Why incidents should become permanent regression scenarios

If an incident is captured as a structured, versionable record instead of a
paragraph in a wiki, it can be treated the same way as any other regression
test: run automatically, compared deterministically, and never silently
forgotten. Two independently authored records that describe the same
underlying failure — different app, different release, reworded
descriptions — should be recognized as *the same failure class*. A record
that has been tampered with, or is internally incoherent (a dangling
reference, a causal cycle, an unredacted field that claims to be redacted),
should be rejected before it can ever be trusted as a regression gate. Both
of these are pure data-representation problems, which is why IncidentDNA
starts there.

## Phase 1: the foundation

This repository is Phase 1 of IncidentDNA. It builds *only* the
vendor-neutral foundation — a format for representing incidents and a CLI to
work with them — deliberately before any storage, transport, UI, or release
integration is built on top of it. See
[`docs/product-scope.md`](docs/product-scope.md) for the full scope
statement, including what is explicitly out of scope for this phase.

What Phase 1 implements:

- **IDIR v0.1** — a JSON/YAML document format for representing one incident
  as a permanent, executable regression scenario
  ([`docs/idir-specification-v0.1.md`](docs/idir-specification-v0.1.md)).
- **Semantic validation** — rejects incoherent or unsafe documents: causal
  cycles, dangling event/evidence references, malformed evidence digests,
  incomplete redaction, and more.
- **Deterministic canonicalization and fingerprinting** — two documents
  describing the same underlying failure pattern hash identically,
  regardless of formatting, record identity, or which application/release
  they were observed in.
- **Comparison** — explains *why* two documents differ, dimension by
  dimension, when their fingerprints don't match.
- **A CLI** (`incidentdna`) exposing `init`, `validate`, `fingerprint`,
  `inspect`, and `compare`.
- **A complete synthetic example** — duplicate payment after message
  redelivery — used by tests, `make example`, and the golden fingerprint
  check.
- **A containerized development workflow** (Docker + Makefile) and CI, so
  the toolchain is identical for every contributor and for CI itself.

## Phase 2: evidence storage and digest verification

Phase 2 adds a local, content-addressable evidence store and four
`incidentdna evidence` subcommands (`store`, `verify`, `list`, `inspect`) so
an `evidence[].digest` declared in an IDIR document can be checked against
real stored bytes, not just format-validated. See
[`docs/evidence-storage.md`](docs/evidence-storage.md) for the full design —
store layout, resource limits, path-traversal/symlink protections, and what
is explicitly *not* covered (no encryption at rest, no signing/authenticity
proof, no remote storage). A separate, fully fictional example,
[`examples/evidence-storage-demo/`](examples/evidence-storage-demo/), exists
to demonstrate the four commands end-to-end without touching
`examples/duplicate-payment/` or its golden fingerprint.

## Phase 3: local incident library

Phase 3 adds a local, filesystem-backed library of validated incident
*occurrences*, grouped by the existing deterministic failure fingerprint
(`internal/fingerprint.Compute`, unchanged), plus three `incidentdna library`
subcommands (`add`, `check`, `list`) to write to and read from it. The
fingerprint identifies a *failure class*, not a unique occurrence, so the
library stores multiple distinct occurrences under one fingerprint rather
than deduplicating to a single entry: a new `incident.id` under an existing
fingerprint is stored as another occurrence, a repeated (or
formatting-reformatted) `incident.id` is idempotent, and a same-`incident.id`
document with materially different content is refused as a conflict. See
[`docs/incident-library.md`](docs/incident-library.md) for the full design —
store layout, resource limits, the privacy gate, path-traversal/symlink
protections, and what is explicitly *not* covered (no encryption at rest, no
signing/authenticity proof, no remote storage, no release gating, no
executable regression scenarios). A third, fully fictional example,
[`examples/incident-library-demo/`](examples/incident-library-demo/), exists
to demonstrate `add`/`check`/`list` end-to-end without touching
`examples/duplicate-payment/` or `examples/evidence-storage-demo/`.

## Phase 4: deterministic executable regression scenarios

Phase 4 adds a small, reviewable, versioned document format — a **regression
scenario** (IRS v0.1) — and a local, bounded, offline runner
(`incidentdna scenario verify|run`) that executes the one command a scenario
declares and classifies the result as `PASS`, `FAIL`, `TIMEOUT`, `INVALID`,
or `INTERNAL_ERROR`. A scenario declares which failure class it
regression-tests by referencing an existing IDIR fingerprint
(`internal/fingerprint.Compute`, unchanged), optionally checked against a
real incident file with `scenario verify --source`. The runner never invokes
a shell and never performs an implicit `$PATH` lookup, runs the declared
command inside a freshly created, bounded, non-persistent workspace under a
timeout and output cap, and writes a deterministic JSON execution report on
request. See [`docs/regression-scenarios.md`](docs/regression-scenarios.md)
for the full design — the scenario document format, the safety model
("bounded, not sandboxed" — this is the first phase to execute local
processes, and the runner does not sandbox what it launches), resource
limits, and what is explicitly *not* covered (no release gating, no suite
runner, no coupling to the incident library). A fourth, fully fictional
example, [`examples/regression-scenario-demo/`](examples/regression-scenario-demo/),
demonstrates all five outcomes end-to-end without touching
`examples/duplicate-payment/`, `examples/evidence-storage-demo/`, or
`examples/incident-library-demo/`.

## CLI commands

```
incidentdna init [--force]
    Create a starter .incidentdna/incident.yaml in the current directory.

incidentdna validate <file>
    Validate an IDIR document against the supported schema version and
    semantic rules.

incidentdna fingerprint <file>
    Print the deterministic sha256: incident fingerprint of an IDIR
    document.

incidentdna inspect <file>
    Print a human-readable summary of an IDIR document.

incidentdna compare <file-a> <file-b>
    Report whether two IDIR documents share the same normalized incident
    fingerprint, and explain material differences if they do not.

incidentdna evidence store [--store <dir>] <evidence-file>
    Compute the SHA-256 digest of <evidence-file> and copy its bytes into a
    local content-addressed evidence store. Never modifies any incident
    document.

incidentdna evidence verify [--store <dir>] <incident-file>
    Validate <incident-file> and check every evidence[].digest against the
    store: presence and full content-integrity re-hash.

incidentdna evidence list [--store <dir>] <incident-file>
    Show each evidence[] entry and whether an object is present in the
    store (presence only — not an integrity guarantee; use verify for that).

incidentdna evidence inspect [--store <dir>] <digest>
    Report metadata (presence, size, integrity) about one stored object,
    addressed directly by digest. Never prints its content.

incidentdna library add [--library <dir>] [--allow-unredacted] <file>
    Validate and fingerprint <file>, enforce the privacy gate
    (privacy.redacted == true by default), and store it as a library
    occurrence: a new incident.id under an existing fingerprint is stored as
    another occurrence, a repeated (or reformatted) incident.id is
    idempotent, and a same-incident.id document with materially different
    content is refused as a conflict.

incidentdna library check [--library <dir>] <file>
    Validate and fingerprint <file>, and report whether the library holds
    one or more intact occurrences under that fingerprint.

incidentdna library list [--library <dir>]
    Enumerate every fingerprint currently stored, each with its occurrence
    count and a bounded per-occurrence summary (incident id, title,
    application/service, occurred timestamp) — never the full stored
    document.

incidentdna scenario verify [--source <incident-file>] <scenario-file>
    Structurally/semantically validate <scenario-file> against the IRS v0.1
    rules. Never executes anything. If --source is given, additionally
    validate and fingerprint <incident-file> through the unchanged Phase 1
    pipeline and assert the result equals the scenario's declared
    linked_fingerprint.

incidentdna scenario run [--workspace <dir>] [--report <file>]
                          [--keep-workspace] <scenario-file>
    Run the same checks as scenario verify. If they pass, create a bounded,
    non-persistent workspace, stage execution.workspace_files, execute
    execution.command under its declared/default timeout with output capped
    per stream, and compare the result against expected. Reports PASS, FAIL,
    TIMEOUT, INVALID, or INTERNAL_ERROR, and optionally writes a
    deterministic JSON execution report to --report.
```

Exit codes are meaningful and relied on by CI: `0` success, `1`
I/O/parse/usage error, `2` semantic validation failure (or, for `evidence
verify`/`evidence inspect`, a MISSING/CORRUPTED finding; for `library
check`, no matching fingerprint found; or, for `scenario run`, a FAIL or
TIMEOUT outcome). See [`docs/evidence-storage.md`](docs/evidence-storage.md),
[`docs/incident-library.md`](docs/incident-library.md), and
[`docs/regression-scenarios.md`](docs/regression-scenarios.md) for the full
`evidence`, `library`, and `scenario` command references.

`incidentdna` performs no network access and collects no telemetry —
every subcommand is pure local file I/O.

## Quick start (Docker)

The host machine is not assumed to have Go installed. Every command runs
through Docker via the Makefile:

```
make format   # gofmt -w .
make lint     # gofmt -l check + go vet
make test     # go test ./... -race -count=1
make build    # go build -o bin/incidentdna ./cmd/incidentdna
make example  # build, then validate + fingerprint examples/duplicate-payment/incident.yaml
make verify   # lint + test + build + example + example-evidence + example-library + example-scenario + golden fingerprint check
make clean    # rm -rf bin
```

If Go is available directly (e.g. a local toolchain install), drop the
`docker compose run --rm dev` prefix and run the same `go`/`gofmt` commands
yourself — the Makefile is a thin wrapper, not a requirement.

## Try it: the duplicate-payment example

[`examples/duplicate-payment/incident.yaml`](examples/duplicate-payment/incident.yaml)
is a complete, synthetic IDIR document: a payment worker crashes after
committing a charge but before acknowledging the triggering message, the
message broker redelivers it, and a second charge is committed for the same
order. Run it directly, once you have a built binary (`make build`):

```
./bin/incidentdna validate examples/duplicate-payment/incident.yaml
./bin/incidentdna fingerprint examples/duplicate-payment/incident.yaml
./bin/incidentdna inspect examples/duplicate-payment/incident.yaml
```

Or via Docker, with no local build step:

```
make example
```

`make verify` additionally checks that this example's fingerprint matches
the pinned golden value in
[`testdata/golden/duplicate-payment.fingerprint`](testdata/golden/duplicate-payment.fingerprint),
guarding the fingerprint algorithm against silent drift.

## Architecture

```
cmd/incidentdna/       CLI entrypoint and one file per subcommand
internal/idir/         IDIR v0.1 Go types + size-capped JSON/YAML loader
internal/validate/     Semantic rule engine (cycles, dangling refs, digest format, redaction, ...)
internal/canonical/    Deterministic ("canonical") JSON re-encoder
internal/fingerprint/  Identity-payload extraction -> canonical -> sha256
internal/compare/      Fingerprint comparison + per-dimension diff
internal/evidence/     Local content-addressed evidence store + digest verification
internal/library/      Local incident library: occurrences grouped by fingerprint
internal/scenario/     Local, bounded, offline regression-scenario runner (IRS v0.1)
schemas/idir/v0.1/     Documentation-grade JSON Schema for the format
examples/duplicate-payment/     Synthetic example used by tests and `make example`
examples/evidence-storage-demo/ Separate synthetic example for `incidentdna evidence`
examples/incident-library-demo/ Separate synthetic example for `incidentdna library`
examples/regression-scenario-demo/ Separate synthetic example for `incidentdna scenario`
testdata/golden/       Golden fingerprint + one fixture per rejected validation case
```

Data flow:

```
file bytes (JSON or YAML)
   │  idir.LoadFile — size cap, format sniff, typed decode
   ▼
idir.Document
   │  validate.Validate — semantic rules
   ▼
(valid) idir.Document
   │  fingerprint.Compute — identity-payload extraction → canonicalize → sha256
   ▼
sha256:... fingerprint string
```

`compare.Documents` runs this pipeline for two documents and, if their
fingerprints differ, builds a human-readable diff across the
fingerprint-relevant dimensions (trigger, causal structure, invariants, side
effect types, recovery behavior, technology categories). See
[`docs/architecture.md`](docs/architecture.md) for the full package layout,
dependency direction, and rejected alternatives (why stdlib `flag` instead
of a CLI framework, why a hand-written validator instead of pure JSON
Schema, etc.).

The only non-stdlib dependency in the module is `gopkg.in/yaml.v3`.

## IDIR and deterministic fingerprinting

**IDIR** (Incident Deterministic Intermediate Representation) is a
JSON- or YAML-encoded document describing one production incident: its
trigger, an ordered/causal graph of events, implicated services, observable
side effects, the business invariant it violated, the corrected behavior
expected once fixed, evidence references, a reproduction sequence, and a
privacy/redaction block. The full field-by-field reference is
[`docs/idir-specification-v0.1.md`](docs/idir-specification-v0.1.md).

The **fingerprint** is a `sha256:`-prefixed hash computed over a reduced
*identity payload* extracted from a valid document — not the whole document.
It deliberately includes only what defines the *class* of failure (trigger,
causal structure of events by type, violated invariants, side-effect types,
expected corrected behavior, technology categories) and excludes what
identifies the *record* (incident id/title/timestamps, application/release
identity, event ids and free-text descriptions, evidence storage locations,
privacy metadata). The result: a reformatted copy of a document, or the same
failure observed in a different application or release, fingerprints
identically; a document with a materially different causal structure does
not. See [`docs/fingerprint-design.md`](docs/fingerprint-design.md) for the
exact field-by-field inclusion table and the reasoning behind it.

## Security and privacy principles

- **No network access, no telemetry, anywhere.** Every subcommand is pure
  local file I/O. This is a stated invariant — see
  [`docs/threat-model.md`](docs/threat-model.md).
- **Evidence locations are never dereferenced.** `evidence[].location` is
  free-text metadata; the CLI never opens, fetches, or otherwise interprets
  it.
- **Evidence digest verification is integrity-only, not authenticity.**
  `incidentdna evidence verify`/`inspect` prove that stored bytes hash to
  their declared digest — not that the evidence is truthful, or who stored
  it. Stored evidence is not encrypted at rest. See
  [`docs/evidence-storage.md`](docs/evidence-storage.md).
- **Library occurrence verification is integrity-only, not authenticity,
  either.** `incidentdna library check`/`list` prove a stored occurrence's
  bytes re-hash to the digest recorded for it — not that the incident is
  truthful, or who added it. Stored occurrences are not encrypted at rest.
  `library add` requires `privacy.redacted == true` by default
  (`--allow-unredacted` overrides it with a mandatory warning); that flag is
  author-declared, not independently verified. See
  [`docs/incident-library.md`](docs/incident-library.md).
- **Regression scenario execution is bounded, not sandboxed.** `incidentdna
  scenario run` never invokes a shell, never performs an implicit `$PATH`
  lookup, and never writes outside its resolved workspace and the
  user-named `--report` path — but it does not sandbox the process it
  launches: a reviewed scenario's command runs with the full OS-level
  permissions of the invoking user. The safety mechanism is human review of
  the scenario file before running, not runtime containment. See
  [`docs/regression-scenarios.md`](docs/regression-scenarios.md).
- **Documents decode into a fixed typed struct**, never
  `interface{}`/`map[string]interface{}`, closing off a class of YAML-parser
  abuse.
- **Size-capped, timeout-bounded input handling.** Documents are capped at 5
  MiB before parsing, and every subcommand runs under a 30-second context
  timeout.
- **Redaction is a mechanically-checked assertion, not automatic PII
  removal.** When `privacy.redacted` is `true`, validation requires a fixed,
  documented list of "known sensitive locations" to be empty, plus a small
  regex backstop over free-text description fields. This is explicitly a
  tripwire, not general-purpose PII detection — see
  [`docs/privacy-model.md`](docs/privacy-model.md) for the exact scope and
  its limitations.
- **Minimal dependency surface.** `gopkg.in/yaml.v3` is the only non-stdlib
  Go dependency in the module.

Full details, including residual risks explicitly accepted for this phase,
are in [`docs/threat-model.md`](docs/threat-model.md) and
[`docs/privacy-model.md`](docs/privacy-model.md).

## Repository structure

```
cmd/incidentdna/    CLI entrypoint and subcommands
internal/           idir, validate, canonical, fingerprint, compare, evidence, library, scenario packages
schemas/idir/v0.1/  Documentation-grade JSON Schema for IDIR v0.1
examples/           Synthetic example incident(s)
testdata/golden/    Golden fingerprint and validation-rejection fixtures
scripts/            Golden-fingerprint and demo verification scripts
docs/               Architecture, IDIR spec, fingerprint design, evidence storage, incident library, regression scenarios, threat model, privacy model, product scope
Dockerfile.dev, compose.yaml, Makefile   Containerized dev/build/test workflow
```

## Status and known limitations

Phase 1 is complete for its own stated scope: representing, validating,
fingerprinting, and comparing IDIR documents through a CLI, with no
dependency on any storage, transport, or UI layer. Phase 2 adds a local
evidence store and digest verification on top of that foundation, without
changing it. Phase 3 adds a local incident library — validated occurrences
grouped by fingerprint, with lookup — on top of both, again without changing
either. Phase 4 adds a local, bounded, offline regression-scenario runner on
top of all three, again without changing any of them. Within that combined
scope, the following limitations are by design — see
[`docs/threat-model.md`](docs/threat-model.md),
[`docs/privacy-model.md`](docs/privacy-model.md),
[`docs/evidence-storage.md`](docs/evidence-storage.md),
[`docs/incident-library.md`](docs/incident-library.md), and
[`docs/regression-scenarios.md`](docs/regression-scenarios.md):

- **No semantic tamper detection.** Validation checks internal coherence
  (no dangling refs, no cycles, required fields present), not whether a
  document truthfully represents what actually happened.
- **Evidence digest verification is integrity-only, not authenticity.**
  `evidence verify`/`evidence inspect` prove stored bytes hash to their
  declared digest; they cannot prove the evidence is truthful, or who stored
  it, or when. There is no signing or provenance chain.
- **Redaction and sensitivity classification are self-declared and
  mechanically checked against a fixed field list**, not automatically
  detected or enforced across the whole document.
- **No multi-tenant or access-control model.** Every invocation operates on
  files the invoking user already has filesystem access to.
- **No encryption at rest for stored evidence, no remote/cloud storage, no
  garbage collection, and fixed (non-configurable) resource limits** on
  evidence size/count per command. See
  [`docs/evidence-storage.md`](docs/evidence-storage.md) for the full list.
- **The incident library groups occurrences, it does not gate or execute
  anything.** `library check` is an offline lookup command; it does not wire
  into any CI/CD pipeline, and the library does not generate, run, or replay
  any test or fault-injection scenario. Library occurrences are not
  encrypted at rest, not signed, not stored remotely, and not
  garbage-collected. The library's four resource limits
  (`MaxLibraryEntries`, `MaxOccurrencesPerFingerprint`, `MaxListResults`,
  and the reused `MaxDocumentSize`) are fixed, not configurable. See
  [`docs/incident-library.md`](docs/incident-library.md) for the full list.
- **The incident library is not an authorization or trust system.** Its
  integrity checks prove a stored occurrence's bytes match the digest
  recorded for it — not that the incident is truthful, that its author
  reviewed it, or who added it. It has no user accounts, no access control,
  and no concept of an incident being "approved."
- **Regression scenario execution is bounded, not sandboxed, and is not a
  release gate.** `incidentdna scenario run` executes exactly one declared
  local command per invocation with no shell interpretation and no implicit
  `$PATH` lookup, inside a freshly created, bounded, non-persistent
  workspace — but it does not sandbox the process it launches (no seccomp,
  cgroups, container, or VM isolation), and its exit code/JSON report are
  not wired into any CI/CD pipeline or merge check by this codebase. There
  is no suite runner (exactly one scenario per invocation) and no automatic
  coupling to the incident library. The eight resource limits in
  `internal/scenario/limits.go` are fixed, not configurable. See
  [`docs/regression-scenarios.md`](docs/regression-scenarios.md) for the
  full list.

This codebase contains no React/web framework, no Kubernetes or cloud
infrastructure, no Kafka or event-ingestion integration, no OpenTelemetry or
observability-pipeline integration, no AI/LLM calls, no SaaS
authentication/billing, and no real fault injection against a running
system. It does not perform release blocking or telemetry ingestion, and it
is not integrated with any other repository.

## Roadmap

**Implemented (Phase 1):**

- IDIR v0.1 format, Go types, and size-capped loader
- Semantic validation engine
- Deterministic canonicalization and fingerprinting
- Fingerprint comparison and dimension-level diffing
- `incidentdna` CLI (`init`, `validate`, `fingerprint`, `inspect`, `compare`)
- Duplicate-payment synthetic example
- Containerized dev workflow and CI

**Implemented (Phase 2, this repository):**

- A local, content-addressable evidence store
  (`.incidentdna/evidence/objects` by default)
- `incidentdna evidence store` / `verify` / `list` / `inspect`
- Path-traversal and symlink protections, atomic deduplicating writes, and
  fixed resource limits (50 MiB/object, 100 entries/command, 500 MiB
  aggregate verify bytes)
- A separate, fully fictional `examples/evidence-storage-demo/` example
- See [`docs/evidence-storage.md`](docs/evidence-storage.md) for the full
  design and its explicit limitations.

**Implemented (Phase 3, this repository):**

- A local, filesystem-backed incident library
  (`.incidentdna/library/objects` by default), grouping validated
  occurrences by the existing deterministic failure fingerprint
- `incidentdna library add` / `check` / `list`
- Exact-document idempotency, multiple occurrences per fingerprint,
  conflict detection on same-`incident.id`/different-content documents, and
  corrupted-occurrence detection
- A privacy gate (`privacy.redacted == true` required by default,
  `--allow-unredacted` override with a mandatory warning) enforced before
  any occurrence is persisted
- Path-traversal and symlink protections, atomic writes, and four fixed
  resource limits (`MaxDocumentSize`, `MaxLibraryEntries`,
  `MaxOccurrencesPerFingerprint`, `MaxListResults`)
- A third, fully fictional `examples/incident-library-demo/` example
- See [`docs/incident-library.md`](docs/incident-library.md) for the full
  design and its explicit limitations.

**Implemented (Phase 4, this repository):**

- A local, bounded, offline regression-scenario runner (IRS v0.1 document
  format, `internal/scenario/`)
- `incidentdna scenario verify` / `run`, including the `--source`
  fingerprint-linkage cross-check
- Five-outcome classification (`PASS`/`FAIL`/`TIMEOUT`/`INVALID`/`INTERNAL_ERROR`)
  with a deterministic JSON execution report (`--report`)
- No shell interpretation, no implicit `$PATH` lookup, workspace
  containment, timeout enforcement (process-group kill), bounded
  stream-separated output capture, and eight fixed resource limits
- A fourth, fully fictional `examples/regression-scenario-demo/` example
- See [`docs/regression-scenarios.md`](docs/regression-scenarios.md) for the
  full design, the safety model, and its explicit limitations.

**Future work (not started, not scoped, not implemented in this codebase):**
ingestion from observability/event systems, integration into release gating,
a suite/aggregation layer for running many scenarios at once, automatic
cross-referencing of a scenario's `linked_fingerprint` against incident
library occurrences, evidence/library signing or authenticity proof,
remote/cloud storage for the evidence store, incident library, or
scenarios, and any of the other items listed as explicitly out of scope in
[`docs/product-scope.md`](docs/product-scope.md). None of this exists yet;
treat any description of it as forward-looking, not current capability.

## Contributing and development

```
make lint     # gofmt -l check + go vet
make test     # go test ./... -race -count=1
make verify   # full local == CI check: lint + test + build + examples + golden fingerprint
```

Run a single test:

```
docker compose run --rm dev go test ./internal/validate/... -run TestValidate_RejectsEachRequiredCase -v
```

CI (`.github/workflows/ci.yml`) runs the same `make` targets against the
same Docker image, so a passing `make verify` locally means CI will pass
too. Before changing what the fingerprint payload includes or excludes,
read [`docs/fingerprint-design.md`](docs/fingerprint-design.md) — the
golden tests are designed to fail loudly on any such change, and
`testdata/golden/duplicate-payment.fingerprint` must be updated
deliberately, not silently, when that happens.
