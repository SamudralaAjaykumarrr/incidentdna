# Privacy Model

## Sensitivity classification

Every IDIR document declares `privacy.sensitivity`, one of `public`,
`internal`, `confidential`, `restricted`. This is a self-declared
classification — validation only checks that the value is one of the four
known values (`internal/validate`'s `checkSensitivity`), not that the
classification is *correct* for the document's actual content. Choosing the
right classification is the document author's responsibility.

## Redaction status and "known sensitive locations"

`privacy.redacted` is a boolean the author sets to assert that any sensitive
material originally present in the incident (customer identifiers, contact
information, raw request/response payloads, etc.) has been removed from the
document before it's shared or stored as a regression scenario.

Validation enforces this assertion against a **fixed, documented list** of
optional free-text fields, called the known sensitive locations:

- `trigger.raw_payload_excerpt`
- `events[].raw_payload_excerpt`
- `evidence[].raw_excerpt`

These three fields exist specifically as a place to put a raw excerpt during
incident authoring/investigation (e.g. a snippet of an actual request body)
that must be scrubbed before the document is considered redacted. When
`privacy.redacted` is `true`, `internal/validate.checkRedaction` requires
each of these fields to be empty on every event/evidence entry. This is a
deterministic, mechanically-checkable rule — full coverage, zero ambiguity
about what "known sensitive location" means, and directly testable (see
`internal/validate/validate_test.go`, cases `"redacted but sensitive field
remains"`).

## Backstop PII scan — explicitly not a substitute for the above

In addition, when `redacted` is `true`, `checkRedaction` runs a small set of
regexes (email address, phone-number-like digit sequences, long numeric
sequences resembling a card/account number) over `trigger.description`,
`events[].description`, and `side_effects[].description` — fields that
*aren't* on the known-sensitive-locations list but are free text an author
could still paste PII into.

**This is explicitly a heuristic backstop, not general-purpose PII
detection**, for three reasons documented here so no future reader
mistakes it for a compliance control:

1. It only looks at description-style fields, not every string in the
   document (e.g. `incident.summary`, `application.name` are not scanned).
2. The patterns are simple regexes tuned for obvious cases (a
   recognizable email address, a run of 13-19 digits). Names, addresses,
   government identifiers in non-numeric formats, and anything obfuscated
   even slightly will not be caught.
3. It runs only when `redacted` is already `true` — an *unredacted*
   document is not scanned at all, by design, since being unredacted is a
   valid, expected state (most incidents are internal-only and never need
   redaction).

Anyone building a real compliance/redaction pipeline on top of IncidentDNA
later should treat this check as a low-cost tripwire for obviously-missed
redactions, not as the mechanism that makes a document safe to share.

## Evidence storage (Phase 2)

Phase 2 adds a local content-addressable store for the raw evidence files
`evidence[].digest` references (see [`evidence-storage.md`](evidence-storage.md)).
That store has no privacy controls of its own:

- **Stored evidence bytes are not encrypted at rest.** They are written and
  read as plain files; anyone with filesystem read access to the store root
  can read them directly.
- **Stored evidence bytes are not scanned for PII.** The redaction and
  backstop-regex checks described above run only over specific fields of the
  IDIR *document* (`trigger`/`events`/`side_effects` description-style
  fields and the known sensitive locations) — they never inspect the
  content of a file passed to `evidence store`, or any object already in the
  store. Choosing what to put into an evidence file, and whether it needs
  redaction before storing, is entirely the author's responsibility, with no
  mechanical backstop at the storage layer.
- **No data retention or deletion policy for stored evidence** — consistent
  with the document-level statement below, extended to the store.

## Incident library privacy policy (Phase 3)

Phase 3 adds a local incident library (see
[`incident-library.md`](incident-library.md)) that persists a full,
validated document indefinitely rather than processing it transiently —
the first place in this codebase to do so. Because of that, `incidentdna
library add` applies a **stricter default** than any other Phase 1/2/3
operation:

- **`privacy.redacted == true` is required by default.** A document that
  does not declare `redacted: true` is refused by `library add` unless the
  caller explicitly passes `--allow-unredacted`.
- **`--allow-unredacted` is an explicit, visible override, never a silent
  bypass.** When used to store a document that does not declare
  `redacted: true`, it always emits a warning on stderr:
  `warning: --allow-unredacted override was used: this document does not
  declare privacy.redacted == true; it was stored anyway`. The warning
  never includes any part of the document's content, and is emitted only
  when the override actually changed the outcome (i.e. never when the
  document already declared `redacted: true`).
- **`privacy.redacted` remains author-declared, not verified**, exactly as
  it is everywhere else in this codebase (see "Redaction status and 'known
  sensitive locations'" above). Storing a document in the library, with or
  without `--allow-unredacted`, does not run any additional PII scan beyond
  the same `internal/validate.checkRedaction` rule (known-sensitive-location
  check plus the backstop regex) every other command already applies before
  the library ever sees the document. The library trusts that same
  assertion; it does not independently confirm the document was actually
  reviewed or sanitized.
- **`library check` and `library list` never print raw document contents.**
  Their output is limited to a bounded field set (fingerprint, occurrence
  count, incident id, title, application/service, occurred timestamp) —
  never business invariants, event timelines, evidence contents, full
  remediation text, or the complete identity payload. There is no
  `--verbose` flag and no JSON output mode in Phase 3.
- **Stored occurrences are not encrypted at rest**, and are not scanned for
  PII beyond the same validation rule already enforced before storage —
  consistent with the stance already stated above for stored evidence.
  Anyone with filesystem read access to the library root can read a stored
  occurrence's canonical JSON directly.
- **No data retention or deletion policy for library occurrences** —
  consistent with the document- and evidence-level statements elsewhere in
  this document; an incident library is, by the product's own premise,
  meant to be a permanent record, so there is no "remove"/expire capability
  in Phase 3.

## Regression scenario privacy implications (Phase 4)

Phase 4 adds a local, bounded, offline regression-scenario runner (see
[`regression-scenarios.md`](regression-scenarios.md)) that, unlike the
evidence store and incident library, persists nothing durable by default:

- **Scenario documents carry no dedicated privacy/redaction block** — no
  `privacy.redacted` equivalent. Unlike an IDIR document, a scenario is not
  a permanent incident record; it is short-lived, reviewable execution
  metadata. If a scenario's captured `stdout`/`stderr` excerpt or a
  `workspace_files` fixture happens to contain sensitive content, that is
  entirely the author's responsibility, exactly as it already is for
  evidence files and library occurrence content — Phase 4 introduces no new
  automatic redaction or PII scanning.
- **Captured stdout/stderr in a `--report` file are bounded excerpts, not
  full dumps** beyond `MaxScenarioOutputBytes`, but nothing scans them for
  sensitive content before writing the report file — the same "no PII
  detection over captured content" stance already stated above for stored
  evidence bytes and library occurrences.
- **No network access means no telemetry, no external transmission of
  scenario content, ever** — restating the unconditional project invariant.
- **The child process's own privacy behavior is entirely outside this
  tool's control.** If a reviewed `execution.command` itself reads
  sensitive local files or reaches the network, that is a property of the
  reviewed command, not of `incidentdna scenario run`, and is explicitly out
  of scope to detect or prevent — see
  [`regression-scenarios.md`](regression-scenarios.md), "A new class of
  risk."
- **`scenario run`'s only durable output is a caller-named `--report`
  file** — there is no default store, no `.incidentdna/scenario/...` root,
  and no accumulation of scenario execution history anywhere. The
  workspace itself (which may contain staged fixtures and whatever the
  reviewed command wrote) is removed after the run unless `--keep-workspace`
  is explicitly given, in which case its path is printed/reported for
  manual inspection and cleanup.

## What Phase 1 through Phase 4 do not do

- No automatic redaction — nothing in this codebase removes or masks
  sensitive content; validation only checks that an author's manual
  redaction was complete against the known-location list.
- No PII detection across the whole document, or across stored evidence
  file content, stored library occurrences, or scenario execution
  output/reports — only the specific document fields listed above.
- No encryption at rest or in transit — for IDIR documents, stored evidence
  bytes, stored library occurrences, or scenario documents/workspaces/reports.
- No data retention or deletion policy — for IDIR documents, the evidence
  store, or the incident library. (Scenario workspaces are removed by
  default after each run, as stated above — a different, narrower behavior
  than "no retention policy," since there is nothing durable to retain in
  the first place unless `--keep-workspace`/`--report` is explicitly used.)
- No signing or authenticity proof for a library occurrence — its presence
  in the library, and a passing integrity check, prove internal
  self-consistency only, never that the incident is truthful or who added
  it (see [`incident-library.md`](incident-library.md), "Integrity versus
  authenticity"). The incident library is not an authorization or trust
  system. The regression-scenario runner makes no trust claim at all about
  the command it executes — see
  [`regression-scenarios.md`](regression-scenarios.md), "A new class of
  risk."
