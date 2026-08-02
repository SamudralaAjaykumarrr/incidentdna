# Product Scope

## What IncidentDNA is

IncidentDNA converts evidence from a production incident into a permanent,
executable regression scenario. Every future release can run the historical
incident library and block software that reintroduces a previously observed
failure. The unit of record is **IDIR** — the Incident Deterministic
Intermediate Representation — a vendor-neutral document format described in
[`idir-specification-v0.1.md`](idir-specification-v0.1.md).

## Phase 1 scope

Phase 1 builds the foundation only: a way to **represent, validate, hash,
and compare** incidents. Concretely:

- The IDIR v0.1 document format and its Go type definitions.
- Semantic validation (`internal/validate`) that rejects incoherent or unsafe
  documents.
- Deterministic canonicalization and fingerprinting (`internal/canonical`,
  `internal/fingerprint`) so two documents describing the same underlying
  failure pattern hash identically regardless of formatting or record-level
  metadata.
- Comparison (`internal/compare`) that explains *why* two documents differ.
- A CLI (`incidentdna`) exposing `init`, `validate`, `fingerprint`,
  `inspect`, and `compare`.
- A complete synthetic example: duplicate payment after message
  redelivery (`examples/duplicate-payment/`).
- A containerized development workflow (Docker + Makefile) and CI.

## Phase 2 scope

Phase 2 builds directly on Phase 1's foundation, without changing it: a
local, content-addressable **evidence store** and digest verification, so
`evidence[].digest` (defined and format-validated since Phase 1) can be
checked against real stored bytes. Concretely:

- A local filesystem evidence store, keyed by an object's own SHA-256
  digest (`internal/evidence`), default root
  `.incidentdna/evidence/objects`.
- Four CLI subcommands: `incidentdna evidence store`, `verify`, `list`,
  `inspect`.
- Path-traversal and symlink protections, atomic deduplicating writes, and
  three fixed resource limits (max object size, max evidence entries per
  command, max aggregate verification bytes per command).
- A second, fully fictional example (`examples/evidence-storage-demo/`)
  demonstrating the four commands end-to-end, entirely separate from
  `examples/duplicate-payment/`.

Full design in [`evidence-storage.md`](evidence-storage.md). Phase 2 does
not change `idir.Document`, the JSON Schema, `internal/validate`'s rules,
`internal/canonical`, or `internal/fingerprint` — the fingerprint payload
already excluded evidence entirely in Phase 1, and continues to.
Verifying a stored object's integrity is not the same as proving the
evidence is truthful or establishing who produced it — see
[`evidence-storage.md`](evidence-storage.md), "Integrity versus
authenticity," for that distinction.

## Phase 3 scope

Phase 3 builds directly on Phase 1 and Phase 2, without changing either: a
local, filesystem-backed **incident library** that persists validated
incident occurrences and groups recurring occurrences by the existing
deterministic failure fingerprint (`internal/fingerprint.Compute`,
unchanged), plus an offline lookup capability. This closes the gap named in
`README.md`'s prior "Future work" list — *"a shared incident library with
storage and retrieval"* — as a **local**, single-user library; a
shared/remote library remains future work (see "Explicitly out of scope for
Phase 1 through Phase 3" below). Concretely:

- A local filesystem incident library (`internal/library`), keyed at two
  levels: a fingerprint-derived directory per failure class, and within it,
  occurrence objects keyed by the SHA-256 digest of each occurrence's own
  canonical document bytes. Default root
  `.incidentdna/library/objects`, under the same `.incidentdna` root the
  Phase 2 evidence store already uses.
- Three CLI subcommands: `incidentdna library add`, `check`, `list`.
- Occurrence semantics: a new `incident.id` under an existing fingerprint is
  stored as another occurrence (a fingerprint identifies a failure *class*,
  not a unique occurrence); an exact or formatting-only repeat of an
  already-stored `incident.id` is idempotent; a same-`incident.id` document
  with materially different content is refused as a conflict, never
  silently overwritten; a corrupted stored occurrence is reported as such,
  never reused or repaired automatically.
- An author-declared privacy gate on `add`: `privacy.redacted == true`
  required by default, overridable only via an explicit
  `--allow-unredacted` flag that always emits a warning.
- Path-traversal and symlink protections, atomic writes, and four fixed
  resource limits: `MaxDocumentSize` (5 MiB, reused from `internal/idir`),
  `MaxLibraryEntries` (10,000 fingerprints), `MaxOccurrencesPerFingerprint`
  (100 occurrences), `MaxListResults` (1,000 entries per `list` call).
- A third, fully fictional example (`examples/incident-library-demo/`)
  demonstrating the three commands end-to-end, entirely separate from
  `examples/duplicate-payment/` and `examples/evidence-storage-demo/`.

Full design in [`incident-library.md`](incident-library.md). Phase 3 does
not change `idir.Document`, the JSON Schema, `internal/validate`'s rules,
`internal/canonical`, `internal/fingerprint`, `internal/compare`, or
`internal/evidence` — the library operates strictly on already-valid
documents and their already-computed fingerprints. As with evidence
integrity in Phase 2, a library occurrence's integrity check proves internal
self-consistency, not that the incident is truthful or who added it — see
[`incident-library.md`](incident-library.md), "Integrity versus
authenticity." The incident library is not an authorization or trust
system: it has no user accounts, no access control, and no concept of an
incident being "approved."

## Explicitly out of scope for Phase 1 through Phase 3

This codebase, through the end of Phase 3, contains none of:

- React, FastAPI, or any web/API framework.
- Kubernetes or any cloud infrastructure.
- Kafka integration or any live event ingestion.
- OpenTelemetry ingestion or any observability pipeline integration.
- AI or LLM calls of any kind.
- SaaS authentication, billing, or customer management.
- Real fault injection against a running system.
- Any changes to, or reuse of, the separate OmniFlow repository.
- Release gating, release-gate integration, or CI/CD blocking of any kind —
  `incidentdna library check` is an offline lookup command, not a gate.
- Executable regression scenarios — the library stores and groups incident
  documents; it does not generate, run, or replay any test or
  fault-injection scenario from them.
- Ingestion from observability/event systems — `library add` (like
  `evidence store` before it) takes a local file path given directly on the
  command line, never an ingested event.
- Remote/cloud storage for either the evidence store or the incident
  library.
- Encryption at rest, for evidence, for library occurrences, or for IDIR
  documents generally.
- Evidence or incident-library signing or authenticity proof.
- Multi-tenancy, or any multi-tenant/shared-store access control, for
  either the evidence store or the incident library.
- Garbage collection of orphaned evidence objects, or any
  retention/expiry/"remove" capability for library occurrences — an
  incident library is, by the product's own premise, meant to be a
  permanent record.

These are all real future needs (see [`phase-1-report.md`](phase-1-report.md),
"Recommended Phase 2 scope", [`phase-2-plan.md`](phase-2-plan.md) §16, and
[`phase-3-plan.md`](phase-3-plan.md) §4), but each phase's job is to get its
own layer's guarantees right — Phase 1 the document representation, Phase 2
local evidence integrity, Phase 3 local occurrence storage and lookup —
before anything further is built on top.

## Why this order

A regression-scenario library is only trustworthy if two independently
authored incident records that describe the same failure are recognized as
the same, and if a record that has been tampered with or is structurally
incoherent is rejected before it ever reaches a release gate. Both of those
properties are pure data-representation problems, independent of how
incidents are ingested or how the library is eventually served — hence
building this layer first, deliberately without any transport, storage, or
UI dependency to get it wrong against.
