# IDIR Specification v0.1

IDIR (Incident Deterministic Intermediate Representation) is a JSON- or
YAML-encoded document describing one production incident as a permanent,
executable regression scenario. This document is the field-level reference;
`schemas/idir/v0.1/idir.schema.json` is the structural companion and
`internal/idir/types.go` is the authoritative Go representation.

`schema_version` must be the literal string `"0.1"`. Any other value is
rejected by validation before any other rule runs.

## Top-level fields

| Field | Required | Purpose |
|---|---|---|
| `schema_version` | yes | Format version gate. Only `"0.1"` accepted. |
| `incident` | yes | Identity of this specific incident record. |
| `application` | yes | Application, service, environment, and release identity. |
| `trigger` | yes | The condition(s) that set the incident in motion. |
| `events` | yes | Ordered, causally-related events (a DAG keyed by `id`). |
| `services` | yes | Services/dependencies implicated in the incident. |
| `side_effects` | yes | Externally observable business consequences. |
| `business_invariants` | yes, ≥1 | Business rules the incident violated. |
| `expected_corrected_behavior` | yes, ≥1 non-empty | What should happen instead once fixed. |
| `evidence` | yes | References to substantiating material, with SHA-256 digests. |
| `reproduction_sequence` | yes | Minimum ordered steps that reproduce the incident. |
| `privacy` | yes | Sensitivity classification and redaction status. |

### `incident`

`id`, `title`, `summary`, `occurred_at` are required (`id` must be
non-empty); `detected_at` and `resolved_at` are optional. None of these
fields affect the fingerprint (see
[`fingerprint-design.md`](fingerprint-design.md)) — they identify the
*record*, not the *class of failure* it represents.

### `application`

`name`, `service`, `environment`, `release_version` are required;
`commit_ref` is optional. Excluded from the fingerprint, so the same
underlying failure pattern is recognized across different apps and releases.

### `trigger`

`type` and `description` are required. `preconditions` is an optional list
of strings. `raw_payload_excerpt` is optional and is a **known sensitive
location** (see [`privacy-model.md`](privacy-model.md)): when
`privacy.redacted` is `true`, it must be empty.

### `events`

Each event has a required `id` (non-empty, unique across the document) and
`type`, a required `description`, and an optional `caused_by` list of other
events' `id`s that causally precede this one. The resulting graph, read as
edges `event -> each id in event.caused_by`, must be acyclic, and every
referenced id must resolve to a defined event. `raw_payload_excerpt` is a
known sensitive location, as in `trigger`.

The order events appear in the document is the intended causal/chronological
sequence and is preserved — not re-sorted — through fingerprinting.

### `services`

Each entry has `name`, `role`, and `technology_category` — the latter is one
of the six fingerprint-relevant dimensions (see below); use a short,
consistent vocabulary (e.g. `message-queue`, `relational-database`,
`payment-gateway`) rather than free text, since two incidents in the same
failure class should use the same category string.

### `side_effects`

Each entry has `type`, `description`, `affected_entity`, and `reversible`
(boolean). `type` is fingerprint-relevant; `description` and
`affected_entity` are not.

### `business_invariants`

At least one entry is required; each has an `id` and a non-empty
`statement`. This is the field validation exists specifically to enforce
non-emptiness on — an incident with no stated violated invariant isn't
useful as a regression scenario.

### `expected_corrected_behavior`

A list of strings; at least one non-empty entry is required.

### `evidence`

Each entry has `id`, `type`, `location`, and `digest`. `digest` must match
`^sha256:[0-9a-f]{64}$` — a bare hex digest without the `sha256:` prefix is
rejected, as is any digest of the wrong length. `location` is free-text
metadata (e.g. an object storage path) that the CLI never dereferences and
that is excluded from the fingerprint. `raw_excerpt` is an optional known
sensitive location.

### `reproduction_sequence`

An ordered list of `{step, description, ref_event_id?}`. If `ref_event_id`
is set, it must resolve to a defined `events[].id`.

### `privacy`

`sensitivity` must be one of `public`, `internal`, `confidential`,
`restricted` — any other value is rejected. `redacted` is a boolean; when
`true`, validation checks the fixed set of known sensitive locations (see
[`privacy-model.md`](privacy-model.md)) are actually empty, plus a regex
backstop for obvious PII in free-text description fields.

## Why not pure JSON Schema validation

`schemas/idir/v0.1/idir.schema.json` documents the shape of a v0.1 document
and is useful as an interchange contract for other tooling, but it is not
what `incidentdna validate` runs. Several required rules cannot be expressed
in JSON Schema at all: acyclic causality over `events[].caused_by`, dangling
reference detection (an id referenced but never defined), and the
redaction-consistency check. `internal/validate` implements the full rule
set directly against the typed `idir.Document`, which is both more precise
and easier to give actionable error messages from (see
[`errors.go`](../internal/validate/errors.go)).
