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
```

Exit codes are meaningful and relied on by CI: `0` success, `1`
I/O/parse/usage error, `2` semantic validation failure (or, for `evidence
verify`/`evidence inspect`, a MISSING/CORRUPTED finding). See
[`docs/evidence-storage.md`](docs/evidence-storage.md) for the full
`evidence` command reference.

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
make verify   # lint + test + build + example + golden fingerprint check
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
schemas/idir/v0.1/     Documentation-grade JSON Schema for the format
examples/duplicate-payment/     Synthetic example used by tests and `make example`
examples/evidence-storage-demo/ Separate synthetic example for `incidentdna evidence`
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
internal/           idir, validate, canonical, fingerprint, compare, evidence packages
schemas/idir/v0.1/  Documentation-grade JSON Schema for IDIR v0.1
examples/           Synthetic example incident(s)
testdata/golden/    Golden fingerprint and validation-rejection fixtures
scripts/            Golden-fingerprint verification script
docs/               Architecture, IDIR spec, fingerprint design, evidence storage, threat model, privacy model, product scope
Dockerfile.dev, compose.yaml, Makefile   Containerized dev/build/test workflow
```

## Status and known limitations

Phase 1 is complete for its own stated scope: representing, validating,
fingerprinting, and comparing IDIR documents through a CLI, with no
dependency on any storage, transport, or UI layer. Phase 2 adds a local
evidence store and digest verification on top of that foundation, without
changing it. Within that combined scope, the following limitations are by
design — see [`docs/threat-model.md`](docs/threat-model.md),
[`docs/privacy-model.md`](docs/privacy-model.md), and
[`docs/evidence-storage.md`](docs/evidence-storage.md):

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

**Future work (not started, not scoped, not implemented in this codebase):**
a shared incident library with storage and retrieval, ingestion from
observability/event systems, integration into release gating, evidence
signing/authenticity proof, remote/cloud evidence storage, and any of the
other items listed as explicitly out of scope in
[`docs/product-scope.md`](docs/product-scope.md). None of this exists yet;
treat any description of it as forward-looking, not current capability.

## Contributing and development

```
make lint     # gofmt -l check + go vet
make test     # go test ./... -race -count=1
make verify   # full local == CI check: lint + test + build + example + golden fingerprint
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
