# incident-library-demo

A third, self-contained IDIR example — unrelated to
[`examples/duplicate-payment/`](../duplicate-payment/) and
[`examples/evidence-storage-demo/`](../evidence-storage-demo/) — that exists
to demonstrate the Phase 3 `incidentdna library` commands (`add`, `check`,
`list`) end-to-end, including the two-level (fingerprint, occurrence)
storage semantics described in `docs/incident-library.md`.

**Scenario:** a slow query holds a database connection past its expected
duration, saturating `checkout-service`'s connection pool; new requests queue
indefinitely with no acquisition timeout, and upstream clients begin
retrying aggressively, amplifying load until the service becomes
unresponsive. All names, timestamps, and content in this directory are
fictional. There are no credentials, API keys, personal information,
customer data, or real production/incident evidence anywhere in this
directory.

## Contents

```
incident-a.yaml                  — the pool-exhaustion incident, first occurrence
                                    (incident id INC-2026-0501)
incident-a-recurrence.yaml       — the SAME failure class, reported independently
                                    against a different service instance
                                    (incident id INC-2026-0618) — demonstrates a
                                    second occurrence stored under the same
                                    fingerprint
incident-a-repeat.json           — the exact same document as incident-a.yaml
                                    (same incident id, same field values),
                                    re-encoded as JSON with different key
                                    order and formatting — demonstrates that
                                    `library add` is idempotent for a
                                    formatting-only copy of an already-stored
                                    incident
incident-different-failure.yaml  — an unrelated failure class (a cache
                                    stampede, incident id INC-2026-0722) —
                                    demonstrates `library check` reporting no
                                    match (exit code 2)
```

`incident-a.yaml` and `incident-a-recurrence.yaml` hold identical values for
every field the fingerprint's identity payload covers — `trigger` (type,
description, preconditions), each event's type and `caused_by` structure,
`business_invariants[].statement`, `side_effects[].type`,
`expected_corrected_behavior`, and `services[].technology_category` — while
differing in everything the fingerprint excludes: incident id, title,
summary, timestamps, application identity, event ids/descriptions, service
names, and reproduction step wording. See `docs/fingerprint-design.md` for
the exact included/excluded field list this relies on.

## Running the demo

Everything below is offline, local-filesystem-only, and uses a **temporary**
`--library` directory so nothing is written to a real `.incidentdna/`
anywhere. Run these from the repository root, after `make build` (or
`go build -o bin/incidentdna ./cmd/incidentdna`):

```sh
LIBRARY=$(mktemp -d)
DEMO=examples/incident-library-demo

# 1. Validate all four documents.
./bin/incidentdna validate "$DEMO/incident-a.yaml"
./bin/incidentdna validate "$DEMO/incident-a-recurrence.yaml"
./bin/incidentdna validate "$DEMO/incident-a-repeat.json"
./bin/incidentdna validate "$DEMO/incident-different-failure.yaml"

# 2. Add the first occurrence. Stored as a new occurrence.
./bin/incidentdna library add --library "$LIBRARY" "$DEMO/incident-a.yaml"

# 3. Add the independently-reported recurrence: same fingerprint, different
#    incident id -> stored as a SECOND occurrence under the same fingerprint,
#    not deduplicated away.
./bin/incidentdna library add --library "$LIBRARY" "$DEMO/incident-a-recurrence.yaml"

# 4. Re-add the same incident, reformatted as JSON with different key order
#    and whitespace -> idempotent: reports already-present, writes nothing.
./bin/incidentdna library add --library "$LIBRARY" "$DEMO/incident-a-repeat.json"

# 5. Check the pool-exhaustion fingerprint: reports a match with 2
#    occurrences. Exits 0.
./bin/incidentdna library check --library "$LIBRARY" "$DEMO/incident-a.yaml"

# 6. Check the unrelated cache-stampede document: reports no match, since
#    only the pool-exhaustion failure class has been added. Exits 2.
./bin/incidentdna library check --library "$LIBRARY" "$DEMO/incident-different-failure.yaml"

# 7. List the library: one fingerprint entry, 2 occurrences, in
#    deterministic order.
./bin/incidentdna library list --library "$LIBRARY"

rm -rf "$LIBRARY"
```

Expected outcome: every `validate` call and steps 2-4's `library add` calls
exit `0`. Step 5's `library check` prints `Match: 2 occurrence(s) found for
this fingerprint` and exits `0`. Step 6's `library check` prints `No match:
no occurrence with this fingerprint exists in the library` and exits `2`.
Step 7's `library list` prints one `Fingerprint: ...(2 occurrence(s))` entry
followed by the bounded per-occurrence summary (incident id, title, service,
occurred timestamp) for both `INC-2026-0501` and `INC-2026-0618` — never the
full stored document.

## Notes

- Every document here declares `privacy.redacted: true`, so none of these
  `library add` calls need `--allow-unredacted`.
- `incident-a-repeat.json` was generated by loading `incident-a.yaml` through
  `internal/idir.LoadFile` and re-encoding the resulting document as JSON —
  it is not hand-typed, which is what guarantees its field values (and
  therefore its canonical bytes and digest) are identical to
  `incident-a.yaml`'s, not merely similar.
- This example and its documents are intentionally separate from
  `examples/duplicate-payment/` and `examples/evidence-storage-demo/` and do
  not affect either, their fingerprints, or
  `testdata/golden/duplicate-payment.fingerprint` in any way.
- `evidence: []` in every document here is deliberate: this example
  demonstrates the library commands, not evidence storage, and Phase 3's
  library store does not read or reference `evidence[]` at all.
