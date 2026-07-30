# Fingerprint Design

## Goal

Two IDIR documents that describe the *same underlying failure pattern*
should produce the same fingerprint, even if they were authored
independently, use different formatting, or belong to different
applications/releases/incident records. Two documents that describe a
*materially different* failure should produce different fingerprints.

## What's included and why

The fingerprint is computed over a dedicated `identityPayload` (see
[`internal/fingerprint/fingerprint.go`](../internal/fingerprint/fingerprint.go)),
containing only:

| Included | Source | Why |
|---|---|---|
| `schema_version` | `doc.SchemaVersion` | A v0.1 and (future) v0.2 document should never collide. |
| `trigger` (type, description, preconditions) | `doc.Trigger` | Defines the initiating condition of the failure class. |
| `events[]` reduced to `{type, caused_by_types[]}` | `doc.Events` | The causal *structure* of the failure — see below. |
| `business_invariants[].statement` | `doc.BusinessInvariants` | The rule that was violated is the core identity of the incident. |
| `side_effects[].type` | `doc.SideEffects` | The externally observable consequence class. |
| `expected_corrected_behavior` | as-is | "Recovery behavior" per the spec — what fixing this incident means. |
| `services[].technology_category` | `doc.Services` | The technology categories implicated, deduplicated. |

Everything else is **excluded**: incident id/title/summary/timestamps,
application/release identity, event ids/descriptions/raw payloads, evidence
and its storage locations, and privacy/redaction metadata. None of these
change what class of failure is being described — they identify the record,
not the pattern.

This exclusion is unchanged by Phase 2's evidence store
(`internal/evidence`, see [`evidence-storage.md`](evidence-storage.md)): no
field the evidence store introduces or reads (a `--store` directory, an
object's on-disk path, presence/absence, or verification status) is part of
`idir.Document` at all, so none of it can reach the identity payload. A
document's fingerprint is identical regardless of which store its evidence
is checked against, or whether it verifies successfully.

## Causal structure, not causal free text

Each event contributes only its `type` and the `type`s of the events listed
in its `caused_by` (deduplicated and sorted, since a given event could in
principle be caused by multiple same-typed predecessors). Event `id`s and
free-text `description`s are dropped entirely. This means:

- Renaming event ids, or rewording descriptions, does not change the
  fingerprint.
- Removing or adding a causal edge between two event types *does* change the
  fingerprint — this is a materially different failure mechanism, even if
  the same event types are still present in the document.

Events are kept **in file order** (not re-sorted into a canonical
topological order): the IDIR spec defines `events[]` order as the intended
causal/chronological sequence, so preserving it is preserving signal, not
insignificant formatting.

## Order-independent vs. order-significant collections

- `business_invariants`, `side_effects` (by type), `expected_corrected_behavior`,
  and `services` (by technology category) are treated as **sets**: sorted
  and deduplicated before hashing, so listing them in a different order in
  the source document does not change the fingerprint.
- `events[]` and each event's `caused_by_types` are **not** re-sorted at the
  top level (order = causal sequence) but `caused_by_types` *within* one
  event is sorted (an event's set of immediate causes has no inherent
  order).

## Canonicalization

`internal/canonical.Marshal` re-encodes the `identityPayload` as JSON with:

- Recursively sorted object keys (relevant for any nested map-like data —
  in practice the payload uses fixed-order structs, so this mainly protects
  against future payload shapes that do use maps).
- No insignificant whitespace.
- Numbers preserved via `json.Number` (no float precision loss — IDIR has no
  float fields, but this makes the canonicalizer correct as a general
  primitive, independently tested in `internal/canonical/canonical_test.go`).
- String escaping delegated to `encoding/json` itself, so it never diverges
  from the standard library's own escaping rules.

The final value is `"sha256:" + hex(sha256(canonicalBytes))`.

## Golden tests

Two layers guard the fingerprint against accidental drift:

1. `internal/fingerprint/golden_test.go` — Go-level: loads
   `examples/duplicate-payment/incident.yaml` and asserts its fingerprint
   matches `testdata/golden/duplicate-payment.fingerprint`.
2. `scripts/verify-golden-fingerprint.sh` — CLI-binary-level: builds
   `bin/incidentdna` and asserts `incidentdna fingerprint` on the same
   example matches the same golden file, exercising the full load → validate
   → fingerprint path through the compiled binary. Run by `make verify` and
   CI.

A deliberate algorithm or example change must update
`testdata/golden/duplicate-payment.fingerprint` explicitly, as its own
visible change — never silently.

## Verified properties

See [`phase-1-report.md`](phase-1-report.md) for the actual commands and
output, but the properties enforced by both unit tests and a live CLI
end-to-end run are:

- A byte-for-byte reformatted (different JSON key order/indentation) copy of
  the example produces an **identical** fingerprint.
- A copy with a materially changed causal structure (removing a causal edge)
  produces a **different** fingerprint.
- Reordering `business_invariants` does not change the fingerprint;
  *changing the content* of an invariant does.
- Changing `side_effects[].type`, `services[].technology_category`, or
  `expected_corrected_behavior` each independently changes the fingerprint.
- Changing only record-identity fields (incident id/title/timestamps,
  application name/release, evidence location) does **not** change the
  fingerprint.
