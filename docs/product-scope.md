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

## Explicitly out of scope for Phase 1 and Phase 2

This codebase, through the end of Phase 2, contains none of:

- React, FastAPI, or any web/API framework.
- Kubernetes or any cloud infrastructure.
- Kafka integration or any live event ingestion.
- OpenTelemetry ingestion or any observability pipeline integration.
- AI or LLM calls of any kind.
- SaaS authentication, billing, or customer management.
- Real fault injection against a running system.
- Any changes to, or reuse of, the separate OmniFlow repository.
- Release gating or integration with any release pipeline.
- Remote/cloud evidence storage, encryption at rest for evidence, evidence
  signing or authenticity proof, multi-tenant/shared-store access control,
  or garbage collection of orphaned evidence objects.

These are all real future needs (see [`phase-1-report.md`](phase-1-report.md),
"Recommended Phase 2 scope", and [`phase-2-plan.md`](phase-2-plan.md) §16),
but each phase's job is to get its own layer's guarantees right — Phase 1 the
document representation, Phase 2 local evidence integrity — before anything
further is built on top.

## Why this order

A regression-scenario library is only trustworthy if two independently
authored incident records that describe the same failure are recognized as
the same, and if a record that has been tampered with or is structurally
incoherent is rejected before it ever reaches a release gate. Both of those
properties are pure data-representation problems, independent of how
incidents are ingested or how the library is eventually served — hence
building this layer first, deliberately without any transport, storage, or
UI dependency to get it wrong against.
