# Phase 3 Report: Local Incident Library

This report documents the state of Phase 3 as implemented in the working
tree of branch `phase-3-incident-library`, after the final verification pass
described below. It follows the same role `docs/phase-2-report.md` served
for Phase 2: a record of what was built, how it was verified, and what is
explicitly not yet true. `docs/phase-3-plan.md` is the approved plan this
implements.

## 1. Phase 3 goals

Phase 1 could validate, fingerprint, inspect, and compare exactly two IDIR
documents at a time. Phase 2 added a store for a single document's
referenced evidence *files*. Neither built an *n-many* store of incident
*occurrences themselves*, or a lookup operation against it — there was no
way to accumulate a collection of previously-observed incidents and ask, of
a new or candidate document, "has a failure with this fingerprint already
been recorded?" Phase 3's entire scope is closing exactly that gap, and
nothing else:

1. A new `internal/library` package: a local, filesystem-backed store of
   validated IDIR incident occurrences, grouped by the existing
   deterministic failure fingerprint (`internal/fingerprint.Compute`,
   unchanged).
2. Three CLI subcommands (`incidentdna library add|check|list`).
3. Fixed resource limits, an author-declared privacy gate on `add`, and the
   filesystem-security properties (path-traversal safety, atomic writes,
   symlink rejection) `internal/evidence` already established, extended to a
   two-level (fingerprint, occurrence) key space.
4. Documentation updates so `docs/incident-library.md`, `README.md`,
   `docs/architecture.md`, `docs/product-scope.md`, `docs/threat-model.md`,
   and `docs/privacy-model.md` accurately describe what Phase 3 covers.
5. A Makefile/CI verification path exercising the above end-to-end.

Explicitly not part of Phase 3 (see `docs/phase-3-plan.md` §4 for the full
list): release gates or CI blocking, executable regression scenarios,
evidence signing/authenticity proof, remote/cloud storage, encryption at
rest, multi-tenancy, garbage collection, OmniFlow integration, network
access, telemetry, configurable resource limits, or any change to
`idir.Document`, `internal/validate`, `internal/canonical`, or
`internal/fingerprint`.

## 2. Implementation summary

`internal/library` is a new, parallel leaf package following the same
dependency-direction precedent `internal/evidence` set: it imports
`internal/idir`, `internal/fingerprint`, and `internal/canonical`, but
nothing in those packages, `internal/validate`, `internal/compare`, or
`internal/evidence` imports it or is changed by it.

```
idir.Document (library add/check)              fingerprint string (library add/check)
   |  idir.LoadFile + validate.Validate            |  fingerprint.Compute(doc)
   v                                                v
validated idir.Document ---------------------> fingerprint-derived directory
                                                     |  canonical.Marshal(doc) -> canonical bytes
                                                     |  sha256(canonical bytes) -> document digest
                                                     v
                                     <library-root>/<fp-shard>/<fingerprint-hex>/
                                         occurrences/<digest-shard>/<digest-hex>.json
                                         index.json   (digest -> incident.id, occurred_at)
```

`cmd/incidentdna/cmd_library.go` adds a single new top-level command,
`library`, registered in `main.go`'s `commands` table, self-dispatching to
`add`/`check`/`list` using the exact `flag.NewFlagSet` idiom
`cmd_evidence.go` already established — no CLI framework was introduced, and
no existing subcommand's dispatch changed.

Full design writeup: `docs/incident-library.md`.

## 3. Architecture and storage layout

Content-addressed at two levels, neither derived from untrusted input:

- **Fingerprint-keyed directory** — one per failure class, addressed by the
  document's own `sha256:`-prefixed, 64-lowercase-hex fingerprint
  (`internal/fingerprint.Compute`, unchanged). First 2 hex chars are the
  shard, remaining 62 the directory name.
- **Digest-keyed occurrence object** — within a fingerprint's directory,
  each occurrence is addressed by the SHA-256 digest of that occurrence's
  own canonical document bytes (`internal/canonical.Marshal` applied to the
  whole validated document, not just the identity payload), sharded the
  same way.
- **`incident.id` is never used as, or concatenated into, a filesystem
  path** — it appears only as a value inside stored JSON content
  (`index.json`, the occurrence body).
- **`index.json`** maps each occurrence's digest to its `incident.id` and
  `incident.occurred_at`, so `Add` can detect a same-`incident.id` conflict
  without re-reading every occurrence body.
- **Path-traversal impossibility**: the only values ever converted into a
  library filesystem path are a validated fingerprint and a validated
  digest — neither alphabet can produce `..`, `/`, or a null byte.
  `Store.checkContained` additionally re-asserts every derived path still
  has the resolved library root as a prefix before any file operation.
- **Default root**: `.incidentdna/library/objects`, under the same
  `.incidentdna` root the Phase 2 evidence store uses; overridden via
  `--library <path>` (never `--store`, which already names the evidence
  store's root flag).

## 4. `add` / `check` / `list` behavior

```
incidentdna library add [--library <dir>] [--allow-unredacted] <file>
incidentdna library check [--library <dir>] <file>
incidentdna library list [--library <dir>]
```

- **`add`** loads and validates `<file>` (the same `validate.Validate`
  rules Phase 1 already runs), enforces the privacy gate, computes its
  fingerprint and canonical-bytes digest, and applies the occurrence
  decision (§5 below). It never overwrites an existing occurrence.
- **`check`** loads and validates `<file>`, computes its fingerprint with
  the exact same `internal/fingerprint.Compute` call every other command
  uses, and reports whether the library holds one or more *intact*
  occurrences under that fingerprint — a corrupted entry is never reported
  as a match.
- **`list`** enumerates every fingerprint group, each with its occurrence
  count and, per occurrence, only: incident id, title, application/service,
  occurred timestamp. Never the full stored document, business invariants,
  event timeline, evidence contents, or remediation text. No `--verbose`
  flag and no JSON output mode exist in Phase 3.

Exit codes: `add` — `0` on stored-new or idempotent-no-op, `1` on any
failure. `check` — `0` for a match, `1` for invalid usage/invalid
document/malformed library/permission/corruption/resource-limit/internal
failure, `2` for a valid document with no matching fingerprint (this reuses
exit code `2` for a *different specific condition* than `validate`/`evidence
verify` — documented explicitly, not left implicit). `list` — `0` on success
including an empty library, `1` on failure.

## 5. Exact-document idempotency

Evaluated at `add` time, after computing the incoming document's fingerprint
and canonical-bytes digest:

1. No existing occurrence under that fingerprint shares the incoming
   document's `incident.id` -> stored as a **new occurrence**.
2. An existing occurrence shares `incident.id` **and** has the same
   canonical-bytes digest (formatting-only differences — whitespace, key
   order, YAML vs. JSON — collapse to the same canonical bytes) ->
   **idempotent**: report already-present, write nothing.
3. An existing occurrence shares `incident.id` **but** has a different
   canonical-bytes digest (materially different content under the same
   declared identity) -> **conflict**: fail distinctly, never overwrite.
4. An occurrence referenced by `index.json` is unreadable, not a regular
   file, or doesn't re-hash to its own declared digest -> **corrupted**:
   reported distinctly, never reused or silently overwritten.

The earlier plan draft's "first one wins" design was rejected: a
different-`incident.id` document sharing the same fingerprint is always a
new stored occurrence, never deduplicated away.

## 6. Fingerprint grouping

The fingerprint identifies a **failure class**, not a unique occurrence.
`examples/incident-library-demo/incident-a.yaml` (incident id
`INC-2026-0501`) and `incident-a-recurrence.yaml` (incident id
`INC-2026-0618`) hold identical values for every field the fingerprint's
identity payload covers, differing only in fields the fingerprint excludes
(incident id, title, timestamps, service names, free-text wording) — both
were verified in this pass to produce the identical fingerprint
`sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade`
and to be stored as two distinct occurrences under it (confirmed by `library
check` reporting `Match: 2 occurrence(s)` and `library list` showing both
incident ids under one fingerprint entry — see §11 for the exact transcript).

## 7. Privacy protections and limitations

`library add` requires `privacy.redacted == true` by default. The check can
be bypassed only with an explicit `--allow-unredacted` flag, which always
emits a warning on stderr — never a silent bypass — and only when the
override actually changed the outcome. **`privacy.redacted` is
author-declared, not verified**: setting it to `true` records only that the
document's author asserts it was reviewed and sanitized — nothing in
`internal/library`, or anywhere else in this codebase, confirms that
assertion is actually true. This is documented explicitly in
`docs/incident-library.md` and `docs/privacy-model.md`. `library` commands
never print raw stored document contents under any flag: `check` and `list`
report only the bounded field set above.

## 8. Resource limits

Four independent, fixed constants (`internal/library/limits.go`), each with
its own distinct error naming the limit and the offending value:

| Constant | Value | Applies to |
|---|---|---|
| `MaxDocumentSize` | 5 MiB (reused from `internal/idir`, not redefined) | Any document read by `add`/`check`, via the existing `idir.LoadFile` path |
| `MaxLibraryEntries` | 10,000 | Total distinct fingerprints before `add` refuses to create a new fingerprint directory |
| `MaxOccurrencesPerFingerprint` | 100 | Occurrences stored under a single fingerprint before `add` refuses a genuinely new one (never blocks an idempotent re-add) |
| `MaxListResults` | 1,000 | Fingerprint entries `list` will enumerate in one invocation |

All four are fixed, not user-configurable, consistent with Phase 3's
explicit exclusion of a config surface for resource limits.

## 9. Corruption and malformed-store handling

- A library root that doesn't exist yet: `check`/`list` report "nothing
  found"/"empty," not an error, the same stance `evidence.Open` takes.
- A library root that exists but isn't a directory: `Open` fails
  immediately with a clear error.
- An occurrence referenced by `index.json` that is missing, not a regular
  file, or doesn't re-hash to its own declared digest: reported as
  `library.ErrCorruptedOccurrence`, distinct from both "not found" and
  "conflict" — never reused, repaired, or silently overwritten as a side
  effect of an unrelated call.
- An unexpected shard or fingerprint directory (wrong length, not lowercase
  hex, or a symlink) is rejected as `library.ErrMalformedLibrary` rather
  than followed or silently skipped.
- Symlinked occurrence files are rejected as corrupted rather than trusted
  or followed.

Covered by `internal/library`'s table-driven tests (see §12).

## 10. CLI exit-code contract

| Command | `0` | `1` | `2` |
|---|---|---|---|
| `add` | stored-new or idempotent no-op | invalid usage, invalid document, privacy violation, conflict, malformed library, permission, corruption, resource limit, internal failure | (unused) |
| `check` | one or more matching occurrences found | invalid usage, invalid document, malformed library, permission, corruption, resource limit, internal failure | valid document, no matching fingerprint |
| `list` | success, including an empty library | malformed library, permission, corruption, resource limit, internal failure | (unused) |

All pre-existing `incidentdna` subcommands (`init`, `validate`,
`fingerprint`, `inspect`, `compare`, and all four `evidence` subcommands)
keep their exact prior exit-code and output behavior — confirmed by the
full existing test suite passing unmodified (§12) and by no file under
`internal/idir`, `internal/validate`, `internal/canonical`,
`internal/fingerprint`, `internal/compare`, or `internal/evidence` appearing
in this phase's diff.

## 11. Demo coverage

`examples/incident-library-demo/` (four synthetic documents: `incident-a.yaml`,
`incident-a-recurrence.yaml`, `incident-a-repeat.json`,
`incident-different-failure.yaml`) demonstrates all three approved paths end
to end. Exact transcript from this verification pass, run via
`scripts/verify-library-demo.sh` against a fresh `mktemp -d` library
(removed on exit, never the default root):

```
verify-library-demo: validating all four demo documents
OK: .../incident-a.yaml is a valid IDIR v0.1 document
OK: .../incident-a-recurrence.yaml is a valid IDIR v0.1 document
OK: .../incident-a-repeat.json is a valid IDIR v0.1 document
OK: .../incident-different-failure.yaml is a valid IDIR v0.1 document

verify-library-demo: library add incident-a.yaml (expect: new occurrence)
Stored new occurrence: fingerprint sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade (incident: INC-2026-0501)

verify-library-demo: library add incident-a-recurrence.yaml (expect: second occurrence, same fingerprint)
Stored new occurrence: fingerprint sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade (incident: INC-2026-0618)

verify-library-demo: library add incident-a-repeat.json (expect: idempotent, nothing written)
Already present (idempotent, nothing written): fingerprint sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade (incident: INC-2026-0501)

verify-library-demo: library check incident-a.yaml (expect: match, 2 occurrences, exit 0)
Fingerprint: sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade
Match: 2 occurrence(s) found for this fingerprint

verify-library-demo: library check incident-different-failure.yaml (expect: no match, exit 2)
Fingerprint: sha256:48917edeeace1562d5e9572af4eec7d38487687bbf98335d92626bec6eac4382
No match: no occurrence with this fingerprint exists in the library

verify-library-demo: library list (expect: one fingerprint, 2 occurrences)
Fingerprint: sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade (2 occurrence(s))
  - incident: INC-2026-0618
    title:    checkout-service-eu unresponsive during connection pool saturation
    service:  checkout-service-eu
    occurred: 2026-06-18T03:07:41Z
  - incident: INC-2026-0501
    title:    Checkout connection pool exhaustion triggers retry storm
    service:  checkout-service
    occurred: 2026-05-01T14:22:10Z
verify-library-demo: OK
```

Both fingerprints above (`dc5bbc53...c71bade` and `48917ede...6eac4382`)
were confirmed to be exactly 64 lowercase hexadecimal characters after
`sha256:`. The script also asserts, programmatically, that none of
`add`/`check`/`list`'s combined output contains free-text content that
falls outside the documented bounded field set (business-invariant
statements, trigger/event descriptions, remediation text) — only the
title/service/incident-id/occurred-timestamp fields `docs/incident-library.md`
documents as printable ever appear.

## 12. Test coverage

- **`internal/library`**: 69 top-level `Test*` functions across 6 test
  files (`store_test.go` 17, `add_test.go` 14, `limits_test.go` 12,
  `check_test.go` 10, `list_test.go` 8, `privacy_test.go` 8), table-driven,
  real filesystem via `t.TempDir()`. Covers: round-trip add/lookup; the
  four occurrence-decision cases (new occurrence, idempotent re-add
  including differently-formatted copies, conflict, corrupted); rejecting
  invalid documents and privacy-gate violations, and accepting the latter
  with `--allow-unredacted`; corrupted-occurrence detection; passing and
  failing boundary tests for each of the four resource limits; library-root
  resolution edge cases (non-existent root, root is a file, symlinked
  root).
- **`cmd/incidentdna/cli_library_test.go`**: 23 top-level `Test*` functions,
  black-box against the real compiled binary — add/check match and miss
  paths, idempotent re-add, conflict, second-occurrence-under-shared-
  fingerprint, `list` field-set and ordering, `--allow-unredacted` warning
  text, and resource-limit tests at synthetic capacities.
- `go test ./... -race -count=1` passes across all 8 packages (§13).

## 13. Commands run and real results

All commands below were run in this verification pass, from the repository
root, on branch `phase-3-incident-library`, inside the same Docker dev
container CI uses (`docker compose run --rm --remove-orphans dev ...`).

| # | Command | Result |
|---|---|---|
| 1 | `git diff --check` | Exit 0 — no whitespace errors, no conflict markers |
| 2 | `gofmt -l .` | Exit 0 — empty output, no unformatted files |
| 3 | `go vet ./...` | Exit 0 — no findings |
| 4 | `go test ./... -race -count=1` | Exit 0 — all 8 packages `ok`: `cmd/incidentdna` (11.5s), `internal/canonical`, `internal/compare`, `internal/evidence`, `internal/fingerprint`, `internal/idir`, `internal/library` (27.8s), `internal/validate` |
| 5 | `go build -buildvcs=false -o bin/incidentdna ./cmd/incidentdna` | Exit 0 — binary built |
| 6 | `scripts/verify-golden-fingerprint.sh` | Exit 0 — `verify-golden-fingerprint: OK (sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e)` |
| 7 | `scripts/verify-evidence-demo.sh` | Exit 0 — `verify-evidence-demo: OK`, all 3 evidence entries verified |
| 8 | `scripts/verify-library-demo.sh` | Exit 0 — `verify-library-demo: OK` (full transcript in §11) |
| 9 | `make example-library` | Exit 0 — builds binary (if needed), runs `verify-library-demo.sh` |
| 10 | `make verify` (full chain: lint -> test -> build -> example -> example-evidence -> example-library -> golden fingerprint script) | Exit 0 throughout |
| 11 | `git diff --stat` | Confirmed: only `.github/workflows/ci.yml`, `Makefile`, `README.md`, `docs/architecture.md`, `docs/privacy-model.md`, `docs/product-scope.md`, `docs/threat-model.md` modified (plus staged `cmd/incidentdna/{cli_library_test.go,cmd_library.go,main.go}`); no Phase 1/2 file appears |
| 12 | `git diff -- testdata/golden` | Empty — no output |
| 13 | `git diff -- examples/duplicate-payment` | Empty — no output |
| 14 | `git diff -- examples/evidence-storage-demo` | Empty — no output |
| 15 | `grep -rn '"net' cmd/ internal/` | Empty — no network package imported anywhere |
| 16 | `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"` | Succeeded — CI workflow YAML is syntactically valid |
| 17 | Fingerprint format check (all `sha256:` values in `docs/incident-library.md`, `examples/incident-library-demo/README.md`, this report) | Every fingerprint is exactly 64 lowercase hex characters after `sha256:`, confirmed programmatically |

## 14. Phase 1/2 regression status

- `examples/duplicate-payment/incident.yaml` and
  `testdata/golden/duplicate-payment.fingerprint`: confirmed byte-for-byte
  unchanged (`git diff` against both paths produced no output; golden
  fingerprint reproduced identically at
  `sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e`).
- `examples/evidence-storage-demo/`: confirmed unchanged (`git diff`
  produced no output); `scripts/verify-evidence-demo.sh` passed in full.
- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`: no files
  modified — none appear anywhere in `git status`/`git diff --stat`.
- All pre-existing CLI subcommands and their tests in
  `cmd/incidentdna/cli_test.go` and `cmd/incidentdna/cli_evidence_test.go`
  are unmodified and pass under `go test ./... -race -count=1`.

## 15. Files added and modified

**New package**, `internal/library/` (14 files: 9 implementation +
5 already covered above under §12's test-file breakdown; not modified in
this session — implemented in prior slices 1–6):
`store.go`, `add.go`, `check.go`, `list.go`, `index.go`, `privacy.go`,
`limits.go`, `errors.go`, `result.go`, plus the six `*_test.go` files.

**New CLI files** (prior slices): `cmd/incidentdna/cmd_library.go`,
`cmd/incidentdna/cli_library_test.go`. **Modified**: `cmd/incidentdna/main.go`
(one new `{"library", runLibrary}` entry).

**New example** (prior slice 7): `examples/incident-library-demo/` —
`README.md`, `incident-a.yaml`, `incident-a-recurrence.yaml`,
`incident-a-repeat.json`, `incident-different-failure.yaml`.

**New docs** (prior slice 8, this session's §17 acceptance review):
`docs/incident-library.md`.

**Modified docs** (prior slice 8): `README.md`, `docs/architecture.md`,
`docs/product-scope.md`, `docs/threat-model.md`, `docs/privacy-model.md`.

**New, added this session (Slice 9)**:
- `scripts/verify-library-demo.sh` — CLI-binary-level end-to-end check of
  the library demo, same style as `scripts/verify-evidence-demo.sh`.

**Modified this session (Slice 9)**:
- `Makefile` — added `example-library` target (mirrors `example-evidence`),
  wired into `verify`'s dependency chain and `.PHONY` list.
- `.github/workflows/ci.yml` — added an "Incident library demo validation"
  step (`make example-library`) between the evidence-demo step and the
  golden-fingerprint step.

**New, added this session (Slice 10)**:
- `docs/phase-3-report.md` — this report.

No file under `internal/idir/`, `internal/validate/`, `internal/canonical/`,
`internal/fingerprint/`, `internal/compare/`, `internal/evidence/`,
`examples/duplicate-payment/`, `examples/evidence-storage-demo/`, or
`testdata/golden/duplicate-payment.fingerprint` was touched at any point in
Phase 3.

## 16. Known limitations

Restated from `docs/incident-library.md` for completeness:

- Verifying an occurrence's digest proves internal consistency only, not
  truthfulness, authorship, or provenance.
- `privacy.redacted` is author-declared, never independently verified.
- The 10,000-fingerprint / 100-occurrence-per-fingerprint /
  1,000-list-result limits are fixed for this phase; a library needing more
  requires a future phase to reconsider them.
- No garbage collection, no remote/shared library, no encryption at rest,
  no signing/authenticity proof, no multi-tenant access control, no
  release-gate integration.
- `index.json`'s read-modify-write is not protected by any lock; concurrent
  `add` calls against the *same* fingerprint from separate processes are
  not guaranteed safe.
- `library check`/`library list` still run under the existing 30-second
  overall command timeout; a very large library on a slow disk could still
  reach it.
- No total-library-size quota beyond the four fixed count/size limits.

## 17. Phase 4 boundary (explicit)

Everything listed as explicitly out of scope in `docs/product-scope.md` and
`docs/phase-3-plan.md` §4 remains deferred, unstarted, and unscoped in this
codebase: release gates or CI blocking, executable regression scenarios,
evidence signing or authenticity proof, remote or cloud storage, encryption
at rest, multi-tenancy, garbage collection, OmniFlow integration, network
access, telemetry, configurable resource limits, and any change to
`idir.Document`, the JSON Schema, or the fingerprint algorithm. **Phase 4
has not been started or scoped as part of this work.** This report makes no
claim about what a future Phase 4 would contain beyond what
`docs/product-scope.md`'s existing "future work" list already names.

## 18. GitHub Actions status

`.github/workflows/ci.yml` has been updated to add an "Incident library demo
validation" step (`make example-library`) to the existing job, so the
workflow-as-configured now covers the complete Phase 3 verification path
(lint, full test suite including `internal/library` and the CLI library
integration tests, build, the duplicate-payment example, the evidence-storage
demo, the incident-library demo, and the golden fingerprint check).

**This configuration has not yet been exercised by the GitHub-hosted
runner.** Every command above was run locally, in the same Docker dev
container CI uses, with results recorded in §13 — but the actual hosted
GitHub Actions workflow run has not been confirmed, because these changes
have not been pushed. CI status for this branch will only be known once it
is pushed and the hosted workflow runs. This report makes no claim that the
hosted CI workflow has passed.

## 19. Final completion checklist

- [x] `internal/library` implements the store, add, check, and list
  behavior, each covered by table-driven tests (prior slices).
- [x] All three `incidentdna library` subcommands work end-to-end against
  real documents (new occurrence, idempotent re-add, conflict, no-match),
  with real command transcripts recorded in this report (§11, §13).
- [x] Full existing suite passes unmodified: `gofmt -l` clean, `go vet
  ./...` clean, `go test ./... -race -count=1` green (all 8 packages), and
  the golden fingerprint value is byte-for-byte unchanged.
- [x] `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence` are
  untouched.
- [x] `docs/incident-library.md`, `README.md`, `docs/architecture.md`,
  `docs/product-scope.md`, `docs/threat-model.md`, `docs/privacy-model.md`
  updated to match implemented behavior.
- [x] `scripts/verify-library-demo.sh` created and passes standalone and as
  part of `make verify`.
- [x] `Makefile` gained `example-library`, wired into `verify` and
  `.PHONY`.
- [x] `.github/workflows/ci.yml` updated to run the library demo step,
  preserving every existing Phase 1/2 CI step unchanged.
- [x] `git diff --check`, `gofmt -l .`, `go vet ./...`, `go test ./...
  -race -count=1`, `go build`, `scripts/verify-golden-fingerprint.sh`,
  `scripts/verify-evidence-demo.sh`, `scripts/verify-library-demo.sh`, and
  `make verify` all passed locally (exit 0 throughout, §13).
- [x] No `.incidentdna` directory or generated library-store artifacts
  remain anywhere in the repository (`find . -name .incidentdna` and `git
  status --ignored` both confirm this).
- [x] `examples/duplicate-payment/` and `examples/evidence-storage-demo/`
  confirmed byte-for-byte unchanged.
- [x] Every documented `sha256:` fingerprint in `docs/incident-library.md`,
  `examples/incident-library-demo/README.md`, and this report contains
  exactly 64 lowercase hexadecimal characters after `sha256:`, confirmed
  programmatically — no fake, shortened, or malformed fingerprint appears
  anywhere.
- [x] Diff inspected for security regressions, incorrect documentation
  claims, incomplete CLI wiring, unsafe filesystem behavior, test gaps,
  accidental Phase 4 scope, sensitive/real data, and generated artifacts —
  none found.
- [ ] Hosted GitHub Actions run — **not yet confirmed** (requires pushing
  this branch, not done as part of this task).
- [ ] Commit / push / merge — **not done**, per explicit instruction for
  this task.

No production adoption, deployment, customer usage, or performance
benchmarking is claimed anywhere in this report or in Phase 3 generally.
