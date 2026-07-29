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

## What Phase 1 does not do

- No automatic redaction — nothing in this codebase removes or masks
  sensitive content; validation only checks that an author's manual
  redaction was complete against the known-location list.
- No PII detection across the whole document, only the specific fields
  listed above.
- No encryption at rest or in transit — Phase 1 has no storage or transport
  layer at all.
- No data retention or deletion policy — out of scope until there is a
  place incidents are actually stored.
