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

## Explicitly out of scope for Phase 1

This phase does not include, and this codebase contains none of:

- React, FastAPI, or any web/API framework.
- Kubernetes or any cloud infrastructure.
- Kafka integration or any live event ingestion.
- OpenTelemetry ingestion or any observability pipeline integration.
- AI or LLM calls of any kind.
- SaaS authentication, billing, or customer management.
- Real fault injection against a running system.
- Any changes to, or reuse of, the separate OmniFlow repository.

These are all real future needs (see [`phase-1-report.md`](phase-1-report.md),
"Recommended Phase 2 scope"), but Phase 1's job is to get the underlying
representation and its guarantees (determinism, validation completeness,
safety against hostile input) right before anything is built on top of it.

## Why this order

A regression-scenario library is only trustworthy if two independently
authored incident records that describe the same failure are recognized as
the same, and if a record that has been tampered with or is structurally
incoherent is rejected before it ever reaches a release gate. Both of those
properties are pure data-representation problems, independent of how
incidents are ingested or how the library is eventually served — hence
building this layer first, deliberately without any transport, storage, or
UI dependency to get it wrong against.
