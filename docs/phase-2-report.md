# Phase 2 Report: Evidence Storage and Digest Verification

This report documents the state of Phase 2 as implemented in the working
tree of branch `phase-2-evidence-storage`, after the final verification pass
described below. It follows the same role `phase-1-report.md` served for
Phase 1: a record of what was built, how it was verified, and what is
explicitly not yet true.

## 1. Phase 2 goals

Phase 1 defined `evidence[].digest` (a `sha256:`-prefixed, 64-lowercase-hex
string) and format-validated it, but there was nothing to check it
*against* — `docs/threat-model.md` named this gap explicitly. Phase 2's
entire scope is closing that gap, and nothing else:

1. A local, content-addressable evidence store on the filesystem.
2. Four CLI subcommands (`evidence store`, `verify`, `list`, `inspect`) to
   write to and read from it.
3. The filesystem-security properties (path-traversal safety, atomic
   writes, symlink rejection, resource limits) that make that store
   trustworthy to use against arbitrary local input.
4. Documentation updates so the threat model, architecture, and
   product-scope docs accurately describe what Phase 2 covers.
5. A Makefile/CI verification path that exercises the above end-to-end.

Explicitly not part of Phase 2: any change to `idir.Document`, the JSON
Schema, `internal/validate`'s rules, `internal/canonical`, or
`internal/fingerprint`; release gating; remote/cloud storage; encryption at
rest; signing/authenticity; multi-tenancy; OmniFlow integration; Phase 3 of
any kind. See `docs/product-scope.md` and `docs/phase-2-plan.md` §16 for the
full exclusion list.

## 2. Implemented architecture

`internal/evidence` is a new, parallel leaf package — it imports
`internal/idir` (to read `doc.Evidence` entries) the same way
`internal/fingerprint` and `internal/compare` already do, but nothing in
`internal/validate`, `internal/canonical`, or `internal/fingerprint` imports
it or is changed by it. It is not a stage inserted into the existing
validate → fingerprint pipeline; it is a separate data flow:

```
evidence file bytes (evidence store)          idir.Document (evidence verify/list)
   │  stream + sha256                            │  for each evidence[] entry
   ▼                                              ▼
digest-derived object path                    Store.objectPath(digest) lookup
   │  atomic write (temp file + rename)           │  Lstat (list) or open+re-hash (verify)
   ▼                                              ▼
object at <store-root>/<shard>/<hash>         OK / MISSING / CORRUPTED / INVALID per entry
```

`cmd/incidentdna/cmd_evidence.go` adds a single new top-level command,
`evidence`, registered in `main.go`'s `commands` table, which self-dispatches
to four subcommands using the same `flag.NewFlagSet` idiom every existing
subcommand already uses — no CLI framework was introduced.

Full design writeup: `docs/evidence-storage.md`.

## 3. Files and packages added or changed

**New package**, `internal/evidence/` (14 files, ~3,974 combined lines
across all new Phase 2 Go/doc/script files):

| File | Purpose |
|---|---|
| `digest.go` | Validated `Digest` type; `Parse` is the sole entry point turning untrusted strings into a digest — path traversal is unrepresentable by construction |
| `store.go` | `Store`, `Open` (root resolution), `objectPath` derivation, `Put` (ingest with atomic write + dedup) |
| `verify.go` | `Store.Verify` — full integrity re-hash against a document's `evidence[]` |
| `list.go` | `Store.List` — cheap, stat-only presence check |
| `inspect.go` | `Store.Inspect` — single-digest metadata lookup, no document involved |
| `limits.go` | The three resource-limit constants and their enforcement helpers |
| `errors.go` | Structured `*Error`/`ErrorKind` type for programmatic dispatch |
| `result.go` | `EntryStatus`, `EntryResult`, `ListResult`, `VerifyResult`, `InspectResult`, `StoreResult` |
| `*_test.go` (6 files) | Table-driven tests for each of the above, using real filesystem via `t.TempDir()` |

**New CLI files**:
- `cmd/incidentdna/cmd_evidence.go` — `runEvidence` dispatcher + four subcommand handlers + usage text.
- `cmd/incidentdna/cli_evidence_test.go` — black-box binary-level integration tests (26 top-level test functions).

**Modified CLI file**:
- `cmd/incidentdna/main.go` — one new entry in the `commands` table (`{"evidence", runEvidence}`) and one new usage line. No other subcommand's behavior changed.

**New example**:
- `examples/evidence-storage-demo/` — `incident.yaml`, `README.md`, and `evidence/` (three fictional files: a log excerpt, a JSON event, a ticket note). Entirely separate from `examples/duplicate-payment/`.

**New scripts**:
- `scripts/verify-evidence-demo.sh` — CLI-binary-level end-to-end check of the evidence demo, same style as the existing `scripts/verify-golden-fingerprint.sh`.

**New docs**:
- `docs/evidence-storage.md` — the dedicated design reference for the evidence store (store layout, commands, resource limits, security properties, integrity-vs-authenticity, recovery guidance).
- `docs/phase-2-plan.md` — the approved plan this phase implements (pre-existing in the working tree; one small correction made to its file-list section to match the actual `docs/evidence-storage.md` filename).

**Modified docs** (updated only where the change is supported by the actual implemented code): `README.md`, `docs/architecture.md`, `docs/product-scope.md`, `docs/idir-specification-v0.1.md`, `docs/fingerprint-design.md`, `docs/threat-model.md`, `docs/privacy-model.md`.

**Modified build/CI files**:
- `Makefile` — added `example-evidence` target, wired into `verify`.
- `.gitignore` — added `.incidentdna/` (generated store output) and a negation so the checked-in fictional `examples/evidence-storage-demo/evidence/session-gateway.log` is not swallowed by the generic `*.log` rule.
- `.github/workflows/ci.yml` — added an "Evidence store demo validation" step (`make example-evidence`).

No file under `internal/idir/`, `internal/validate/`, `internal/canonical/`, `internal/fingerprint/`, `internal/compare/`, `examples/duplicate-payment/`, or `testdata/golden/duplicate-payment.fingerprint` was touched.

## 4. CLI commands

```
incidentdna evidence store [--store <dir>] <evidence-file>
incidentdna evidence verify [--store <dir>] <incident-file>
incidentdna evidence list [--store <dir>] <incident-file>
incidentdna evidence inspect [--store <dir>] <digest>
```

- **`store`** computes the SHA-256 digest of a local file, writes its bytes into the store, and prints the canonical digest plus copy-pasteable instructions for adding an `evidence[]` entry. It never reads or writes any incident document.
- **`verify`** validates an incident document, then checks every `evidence[].digest` against the store: presence and a full re-hash. Exit `0` only if every entry is OK.
- **`list`** validates an incident document, then reports presence only (no re-hash) for each entry. Always exits `0` if the document loads and validates — "not yet stored" is treated as expected, not a failure.
- **`inspect`** looks up one bare digest directly, with no document involved, and reports presence/size/integrity — never the object's content.

Exit codes: `0` success; `1` I/O/usage/resource-limit error; `2` semantic validation failure or a MISSING/CORRUPTED finding from `verify`/`inspect`. This preserves Phase 1's existing meaning for exit code `2`.

## 5. Evidence-store layout

```
<store-root>/                          default: .incidentdna/evidence/objects
  <first 2 hex chars>/<remaining 62 hex chars>   one file per object, raw bytes
  .tmp/
    <random>                                      staging area for atomic writes
```

The object path is derived **only** from the object's own digest — never from `evidence[].location`, an entry's `id`/`type`, or the original filename passed to `store`. `--store <dir>` overrides the root; it is resolved to an absolute, symlink-resolved path exactly once per invocation. There is no separate manifest/index — the store directory tree is the complete, authoritative inventory.

## 6. Resource limits

| Constant | Value | Applies to |
|---|---|---|
| `MaxObjectSize` | 50 MiB | Any single object read or written, enforced during streaming |
| `MaxEntriesPerCommand` | 100 | `evidence[]` entries `list`/`verify` will process, checked before any filesystem work |
| `MaxTotalVerifyBytes` | 500 MiB | Aggregate stored-object bytes `verify` reads per invocation, checked via a stat-only pre-pass |

Each limit is independent, fixed (not configurable), and produces a distinct error naming the limit and the offending value.

## 7. Security controls

- **Path traversal is structurally unrepresentable**, not just filtered: the only value ever converted into a filesystem path is a `Digest`, constructible only from exactly 64 lowercase hex characters or by hashing bytes directly — no character in that alphabet can produce `..`, `/`, or a null byte. `objectPath` additionally re-checks the resulting path against the resolved store root.
- **Symlinks are rejected, never followed**, on every path: `store`'s source file, and any object found at a would-be path during `list`/`verify`/`inspect`/dedup re-check.
- **Atomic writes**: `store` writes to a temp file under `<store-root>/.tmp/`, `Sync()`s it, then `os.Rename`s it into place — same-filesystem rename is atomic, so a concurrent reader never sees a partial object.
- **Deduplication is structural**: identical bytes always resolve to the same path; a would-be duplicate is re-verified (not blindly trusted) before being reported as already-present.
- **Store root resolved once per invocation**, so a relative `--store` value can't change meaning mid-command.
- **No network access, no telemetry** — confirmed by `grep -rn '"net' cmd/ internal/` returning empty.

## 8. Integrity versus authenticity

`verify`/`inspect` prove exactly one thing: the bytes currently at a digest's path hash to that digest. This is **integrity**, not **authenticity** — it cannot prove the evidence is truthful, who stored it, or when. An attacker who fabricates both a fake evidence file and a matching digest, consistently, produces a result that verifies successfully; content-addressing cannot distinguish that from legitimate re-evidencing. This is the same class of limitation Phase 1's threat model already accepts for semantic document tampering, and is documented in detail in `docs/evidence-storage.md`, "Integrity versus authenticity," and `docs/phase-2-plan.md` §10.

## 9. Testing performed

- `internal/evidence`: 59 top-level `Test*` functions across 6 test files (`digest_test.go`, `store_test.go`, `verify_test.go`, `list_test.go`, `inspect_test.go`, `limits_test.go`), table-driven, using real filesystem I/O via `t.TempDir()` — no mocking. Covers: round-trip storage, deduplication (including re-verification of the existing object on a would-be duplicate), rejecting oversized/symlink/directory sources, corrupted-object detection (modified bytes, truncation, trailing bytes), symlinks planted at object paths, both resource limits independently (with passing and failing cases), and object-path containment under the store root.
- `cmd/incidentdna`: 26 top-level `Test*` functions in `cli_evidence_test.go`, black-box against the real compiled binary — covers all four subcommands' success and failure paths, custom vs. default `--store` resolution, digest format rejection, "never leaks stored content" assertions for `list`/`inspect`, and confirms `store` never modifies the incident file or the source file's bytes.
- `go test ./... -race -count=1` passes across all 7 packages (see §11).

## 10. Evidence demo verification

`scripts/verify-evidence-demo.sh` (invoked via `make example-evidence`) runs the full command surface against `examples/evidence-storage-demo/`, using a fresh `mktemp -d` store (never the default root, so it never touches `.incidentdna/` anywhere) that is removed on exit:

1. `validate` the demo incident — passes.
2. `evidence store` each of the three fictional evidence files — each stored, digest printed.
3. `evidence list` — all 3 entries reported present.
4. `evidence verify` — all 3 entries reported `OK (integrity verified)`.
5. `evidence inspect` on one stored digest — reports `Status: OK (integrity verified)`.

This ran successfully multiple times during this verification pass (see exact output in §11). One cosmetic observation: when Docker's output is captured non-interactively, the demo script's stdout (each `evidence store` call's "Stored: ..." banner) and stderr (the script's own `echo ... >&2` progress lines) can appear visually interleaved out of their logical order in the combined log — this is a stream-buffering artifact of piping a non-TTY `sh` process through `docker compose run`, not a defect in the script's control flow or in `incidentdna` itself: the script is strictly sequential, and the exit code and final list/verify/inspect results were correct and internally consistent on every run.

## 11. Exact final command results

All commands below were run in this verification pass, from the repository root, on branch `phase-2-evidence-storage`.

| # | Command | Result |
|---|---|---|
| 1 | `git diff --check` | Exit 0 — no whitespace errors, no conflict markers |
| 2 | `make lint` | Exit 0 — `gofmt -l` clean, `go vet ./...` clean |
| 3 | `make test` (`go test ./... -race -count=1`) | Exit 0 — all 7 packages `ok`: `cmd/incidentdna`, `internal/canonical`, `internal/compare`, `internal/evidence`, `internal/fingerprint`, `internal/idir`, `internal/validate` |
| 4 | `make build` | Exit 0 — `bin/incidentdna` built |
| 5 | `make example` | Exit 0 — `OK: examples/duplicate-payment/incident.yaml is a valid IDIR v0.1 document`; fingerprint printed as `sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e` |
| 6 | `make example-evidence` | Exit 0 — validate OK, 3/3 stored, list shows 3/3 present, verify reports `OK: all 3 evidence entries verified`, inspect reports `OK (integrity verified)` |
| 7 | `make verify` (full chain: lint → test → build → example → example-evidence → golden fingerprint script) | Exit 0 throughout; final line `verify-golden-fingerprint: OK (sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e)` |

## 12. Phase 1 compatibility confirmation

- `examples/duplicate-payment/incident.yaml`: `git diff --stat` and `git status --porcelain` against this path both produced no output — byte-for-byte unchanged.
- `testdata/golden/duplicate-payment.fingerprint`: unchanged on disk (`sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e`), and reproduced identically by both the Go-level golden test (via `make test`) and the CLI-binary-level check (`scripts/verify-golden-fingerprint.sh`, run standalone and as the last step of `make verify`).
- `internal/idir`, `internal/validate`, `internal/canonical`, `internal/fingerprint`, `internal/compare`: no files modified (confirmed via `git status`/`git diff` — none appear in the changed-files list).
- All pre-existing Phase 1 CLI subcommands (`init`, `validate`, `fingerprint`, `inspect`, `compare`) and their existing tests in `cmd/incidentdna/cli_test.go` are unmodified and pass under `make test`.

## 13. Known limitations

Restated from `docs/evidence-storage.md` for completeness:

- Verifying a digest proves internal consistency only — not truthfulness, authorship, or provenance (§8 above).
- The three resource limits are fixed for this phase, not configurable.
- No total-store-size quota, no garbage collection of orphaned objects.
- No remote/shared store — local filesystem, single machine only.
- No encryption at rest for stored evidence; stored bytes are not scanned for PII.
- `store` never mutates incident documents automatically — referencing newly stored evidence is a manual, copy-paste step.
- TOCTOU and durability gaps inherent to any lock-free local filesystem tool are reduced (streamed size caps, atomic rename, symlink rejection) but not eliminated — no OS-level file locking is taken, and the temp-file `Sync()` before rename does not additionally `fsync` the containing directory entry.
- `evidence verify` still runs under the existing 30-second overall command timeout; a large verification on a slow disk could still reach it.

## 14. Deferred work

Everything listed as explicitly out of scope in `docs/product-scope.md` and `docs/phase-2-plan.md` §16 remains deferred, unstarted, and unscoped in this codebase: release gating or integration with any release pipeline, remote/cloud evidence storage, encryption at rest, evidence signing/authenticity proof, multi-tenant/shared-store access control, garbage collection, configurable resource limits, ingestion from observability/event systems, and any integration with OmniFlow. Phase 3 has not been started or scoped as part of this work.

## 15. GitHub Actions status

`.github/workflows/ci.yml` has been updated to add an "Evidence store demo validation" step (`make example-evidence`) to the existing job, so the workflow-as-configured now covers the complete Phase 2 verification path (lint, full test suite including `internal/evidence` and the CLI evidence integration tests, build, the duplicate-payment example, the evidence-storage demo, and the golden fingerprint check).

**This configuration has not yet been exercised by the GitHub-hosted runner.** Every command above was run locally, in the same Docker dev container CI uses, with results recorded in §11 — but the actual hosted GitHub Actions workflow run has not been confirmed, because these changes have not been pushed. CI status for this branch will only be known once it is pushed and the hosted workflow runs. This report makes no claim that the hosted CI workflow has passed.

## 16. Final completion checklist

- [x] `docs/evidence-storage.md` created, describing the store as actually implemented.
- [x] `README.md`, `docs/architecture.md`, `docs/product-scope.md`, `docs/idir-specification-v0.1.md`, `docs/fingerprint-design.md`, `docs/threat-model.md`, `docs/privacy-model.md` updated to match implemented behavior.
- [x] `internal/evidence` package implemented with `Store`/`Verify`/`List`/`Inspect`, each covered by table-driven tests.
- [x] Four `incidentdna evidence` subcommands wired into `main.go` and covered by black-box CLI tests.
- [x] `examples/evidence-storage-demo/` created: fictional incident + three fictional evidence files, no real/sensitive data.
- [x] `Makefile` gained `example-evidence`, wired into `verify`.
- [x] `.gitignore` updated: `.incidentdna/` ignored; checked-in fictional evidence log un-ignored.
- [x] `.github/workflows/ci.yml` updated to run the evidence demo step.
- [x] `git diff --check`, `make lint`, `make test`, `make build`, `make example`, `make example-evidence`, and `make verify` all passed locally (exit 0 throughout, §11).
- [x] No `.incidentdna` directory or generated evidence-store artifacts remain anywhere in the repository.
- [x] `examples/duplicate-payment/` and `testdata/golden/duplicate-payment.fingerprint` confirmed byte-for-byte unchanged.
- [x] All expected Phase 2 files confirmed visible to git (not accidentally ignored).
- [x] Diff inspected for security regressions, incorrect documentation claims, incomplete CLI wiring, unsafe filesystem behavior, test gaps, accidental Phase 3 scope, sensitive/real data, and generated artifacts — none found.
- [ ] Hosted GitHub Actions run — **not yet confirmed** (requires pushing this branch, not done as part of this task).
- [ ] Commit / push — **not done**, per explicit instruction for this task.

No production adoption, deployment, customer usage, or performance benchmarking is claimed anywhere in this report or in Phase 2 generally.
