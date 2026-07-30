# evidence-storage-demo

A second, self-contained IDIR example — unrelated to
[`examples/duplicate-payment/`](../duplicate-payment/) — that exists to
demonstrate the Phase 2 `incidentdna evidence` commands
(`store`, `verify`, `list`, `inspect`) end-to-end against real, checked-in
fictional evidence files.

**Scenario:** a scheduled restart of one session-cache node causes a burst of
cache misses; the fallback path overloads `auth-service`, which sheds load;
`session-gateway` misreads the resulting timeouts as invalid sessions and
forces a mass logout on the affected shard. All names, timestamps, and
content in this directory are fictional. There are no credentials, API keys,
personal information, customer data, or real production/incident evidence
anywhere in this directory.

## Contents

```
incident.yaml           — the IDIR v0.1 document (evidence[].digest values
                           are the real sha256 digests of the files below)
evidence/
  session-gateway.log              — fictional log excerpt
  cache-node-restart-event.json    — fictional structured event
  incident-ticket-5502.txt         — fictional support ticket
```

## Running the demo

Everything below is offline, local-filesystem-only, and uses a **temporary**
`--store` directory so nothing is written to a real `.incidentdna/` anywhere.
Run these from the repository root, after `make build` (or
`go build -o bin/incidentdna ./cmd/incidentdna`):

```sh
STORE=$(mktemp -d)

# 1. Validate the incident document.
./bin/incidentdna validate examples/evidence-storage-demo/incident.yaml

# 2. Store each fictional evidence file (prints its canonical sha256 digest —
#    compare against the digests already recorded in incident.yaml's
#    evidence[] entries).
./bin/incidentdna evidence store --store "$STORE" examples/evidence-storage-demo/evidence/session-gateway.log
./bin/incidentdna evidence store --store "$STORE" examples/evidence-storage-demo/evidence/cache-node-restart-event.json
./bin/incidentdna evidence store --store "$STORE" examples/evidence-storage-demo/evidence/incident-ticket-5502.txt

# 3. List the evidence declared in the document (presence only, no re-hash).
./bin/incidentdna evidence list --store "$STORE" examples/evidence-storage-demo/incident.yaml

# 4. Verify every evidence[] entry's digest against the store (full
#    integrity re-hash). Exits 0 only if every entry is OK.
./bin/incidentdna evidence verify --store "$STORE" examples/evidence-storage-demo/incident.yaml

# 5. Inspect one stored object directly by digest (metadata only — never
#    prints evidence content).
./bin/incidentdna evidence inspect --store "$STORE" sha256:f010c383e443d6ead7da242d34f47b31728bffe97ff6ccd7e9c053d2a7757182

rm -rf "$STORE"
```

Expected outcome: `validate` and `verify` both exit `0`; `verify`'s summary
line reads `OK: all 3 evidence entries verified`; `inspect` reports
`Status:    OK (integrity verified)` for the digest above.

## Notes

- The `evidence[].digest` values in `incident.yaml` are the actual sha256
  digests of the three files under `evidence/` — computed once with
  `sha256sum` and never modified since. Re-running `evidence store` against
  those same files will always reproduce the same digests.
- `evidence[].location` (e.g. `evidence/session-gateway.log`) is inert,
  presentational metadata — `incidentdna` never opens or dereferences it. The
  store commands above take the file path directly on the command line.
- This example and its evidence files are intentionally separate from
  `examples/duplicate-payment/` and do not affect it, its fingerprint, or
  `testdata/golden/duplicate-payment.fingerprint` in any way.
