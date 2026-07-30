# Phase 2 Plan: Evidence Storage and Digest Verification

This is the approved plan for Phase 2. Nothing described here has been
implemented yet — this document is being updated to record approved
decisions before any code is written.

**Approved decisions (this revision):**

1. Resource limits are three separate constants — max object size (50 MiB),
   max evidence entries processed per command (100), max total bytes
   verified per command (500 MiB) — each with tests and a distinct, clear
   error message.
2. The existing `duplicate-payment` example and its golden fingerprint are
   never touched. A new, separate, fully fictional example is created for
   evidence storage instead.
3. Store root defaults to `.incidentdna/evidence/objects`; `--store` is
   supported on `store`, `verify`, `list`, and `inspect`; the root is
   resolved and validated once per invocation; object paths never
   incorporate evidence metadata or original filenames.
4. Four subcommands: `evidence store`, `evidence verify`, `evidence list`,
   `evidence inspect`. `store` never modifies an incident document and
   prints the canonical digest plus reference instructions. `inspect` never
   prints raw evidence bytes.

## 1. Exact Phase 2 scope

Phase 1 defined `evidence[].digest` (a `sha256:`-prefixed 64-hex-character
string) and format-validated it, but there was nothing to check it
*against* — `docs/threat-model.md` names this explicitly: "a forged digest
that's correctly formatted is currently indistinguishable from a real one."
Phase 2 closes exactly that gap and nothing else:

- A **local, content-addressable evidence store** on the filesystem: a
  directory tree where evidence bytes are written, keyed by their own
  SHA-256 digest.
- A **`store`** operation: given a local file, compute its digest, write its
  bytes into the store, and print the canonical digest plus instructions
  for referencing it — without touching any incident document.
- A **`verify`** operation: given an IDIR document, check every
  `evidence[].digest` against the store — is the object present, and does
  re-hashing its actual stored bytes still produce that same digest?
- A **`list`** operation: given an IDIR document, cheaply enumerate its
  evidence entries and whether each is present in the store, without doing
  full integrity re-hashing.
- An **`inspect`** operation: given a bare digest, report metadata about the
  corresponding stored object (existence, size, integrity) without ever
  printing its content.
- The filesystem-security properties that make that store trustworthy to use
  on arbitrary local input: path traversal safety, atomic writes, overwrite
  protection, and size/count/aggregate-byte limits.
- Updated docs so the threat model, architecture, and product-scope
  documents accurately describe what is and isn't covered.

Phase 2 does **not** change `idir.Document`, the JSON Schema, the semantic
validator's rules, canonicalization, or the fingerprint payload/algorithm.
`evidence[].digest`'s format rule already exists; Phase 2 gives it something
real to be checked against, without altering what "valid" or "fingerprint"
mean.

## 2. Files planned for creation and modification

### New

```
internal/evidence/
  store.go           — content-addressable store: Store(), object path derivation, root resolution
  verify.go          — Verify(doc, store) — full integrity check, entry-count + aggregate-byte caps
  list.go            — List(doc, store) — cheap presence-only enumeration, entry-count cap
  inspect.go         — Inspect(store, digest) — single-object metadata lookup, never returns content bytes
  limits.go          — MaxObjectSize, MaxEntriesPerCommand, MaxTotalVerifyBytes constants + limit-check helpers
  errors.go          — result/issue types, mirroring internal/validate's Result/Issue shape
  store_test.go
  verify_test.go
  list_test.go
  inspect_test.go
  limits_test.go
cmd/incidentdna/
  cmd_evidence.go     — `evidence` command, sub-dispatches to store/verify/list/inspect
examples/evidence-storage-demo/
  incident.yaml       — new, separate, fully fictional example incident (not duplicate-payment)
  evidence/            — small fictional evidence files matching the example's real digests
testdata/golden/evidence/
  corrupt-object      — a stored object whose bytes deliberately don't match its filename digest (tamper fixture)
docs/
  evidence-storage.md          — dedicated design doc, same role as fingerprint-design.md
                                  (implemented as docs/evidence-storage.md, not
                                  evidence-storage-design.md as originally named here)
  phase-2-report.md            — written after implementation + verification (not now)
```

### Modified

```
cmd/incidentdna/main.go        — register the new `evidence` command
docs/architecture.md           — package layout table, dependency direction, CLI conventions
docs/threat-model.md           — "Forged evidence references" rewritten to reflect what's now verified; new subsections for store-specific risks and resource limits
docs/product-scope.md          — move evidence storage from "future work" into Phase 2 scope
docs/privacy-model.md          — note: stored evidence bytes are not encrypted at rest, not scanned for PII
README.md                      — CLI commands list, roadmap, known-limitations section
.gitignore                     — add .incidentdna/ (local evidence store output must never be accidentally committed)
```

`go.mod`/`go.sum` are unchanged — the store uses only `crypto/sha256`,
`os`, `io`, and `path/filepath` from the standard library.

`examples/duplicate-payment/incident.yaml`,
`testdata/golden/duplicate-payment.fingerprint`, and every existing fixture
under `testdata/golden/invalid/` are **not modified** — see §12.

## 3. Acceptance criteria

1. `internal/evidence` implements `Store`, `Verify`, `List`, and `Inspect`
   as described in §4–§5 and §11, each covered by table-driven tests.
2. All four `incidentdna evidence` subcommands work end-to-end against real
   files and a real IDIR document, with real command transcripts recorded
   in `docs/phase-2-report.md`.
3. Full existing suite still passes unmodified: `gofmt -l` clean, `go vet
   ./...` clean, `go test ./... -race -count=1` green, **and the golden
   fingerprint value (`sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e`)
   is byte-for-byte unchanged** — the sharpest possible signal that Phase 2
   didn't touch the fingerprint payload or the duplicate-payment example.
4. `internal/idir`, `internal/validate`, `internal/canonical`,
   `internal/fingerprint`, `internal/compare` are untouched (or, if a
   change is later found unavoidable, it is called out explicitly, not
   silent).
5. A digest-shaped path cannot escape the store root — proven by a test
   that confirms object-path derivation only ever accepts the exact
   64-lowercase-hex-char shape and never incorporates any other
   user-supplied string, filename, or evidence metadata field.
6. Each of the three resource limits (§9) is enforced by a dedicated
   constant, produces a distinct clear error message naming the limit and
   the offending value, and is covered by a passing/failing test pair.
7. A deliberately corrupted stored object is reported CORRUPT by `verify`
   and by `inspect`, not silently accepted.
8. A referenced-but-never-stored digest is reported MISSING by `verify` and
   by `list`.
9. `store` never writes to, or otherwise modifies, any incident document —
   proven by a test that runs `store` next to an incident file and asserts
   the incident file's bytes are unchanged.
10. `inspect <digest>` never writes evidence content to stdout/stderr —
    proven by a test that stores a distinctive byte sequence, runs
    `inspect` on its digest, and asserts that sequence does not appear
    anywhere in the command's output.
11. `docs/architecture.md`, `docs/threat-model.md`, `docs/product-scope.md`,
    `docs/privacy-model.md`, and `README.md` are updated so no doc makes a
    now-false claim.
12. No network access, no telemetry introduced (`grep -rn '"net' cmd/
    internal/` stays empty, same check as `phase-1-report.md`).
13. The new example under `examples/evidence-storage-demo/` contains no
    credentials, personal information, customer data, or real incident
    evidence — same synthetic-only discipline as `duplicate-payment`.
14. Nothing is committed, staged, or pushed during implementation —
    explicit re-statement of this session's standing instruction.

## 4. Evidence storage architecture

A **content-addressable store (CAS)** on the local filesystem, structurally
similar to git's object store:

```
<store-root>/                          default: .incidentdna/evidence/objects
  <first 2 hex chars>/<remaining 62 hex chars>   — one file per stored object, raw bytes
  .tmp/
    <random>                                      — staging area for atomic writes (see §8)
```

- `<store-root>` is the directory objects are sharded directly under — the
  first two hex characters of a digest name a subdirectory, the remaining
  62 name the file. `--store <dir>` overrides the root; the default is
  `.incidentdna/evidence/objects` under the current working directory
  (see §6 for exactly how the root is resolved).
- `.tmp/` is a reserved staging subdirectory name that can never collide
  with a real shard directory (shard directories are always exactly two
  lowercase hex characters; `.tmp` starts with a dot, which a hex digest
  never produces).
- The object's path is derived **only** from its own digest — never from
  the original filename, never from `evidence[].location`, never from an
  evidence entry's `id` or `type`. This applies uniformly across `store`,
  `verify`, `list`, and `inspect` — none of the four commands ever build a
  store path from anything other than a validated digest. Concretely:
  - Storing the same bytes twice always produces the same path (idempotent).
  - A store is just a flat, inspectable set of files; `find .incidentdna/evidence/objects -type f` (excluding `.tmp/`) is a complete inventory. No manifest/index file is maintained separately (see §14 — rejected alternative).
- `evidence[].location` remains exactly what it was in Phase 1: free-text,
  presentational metadata, excluded from the fingerprint, **never opened or
  dereferenced by the CLI**. The store's lookup key is the digest, not the
  location string, and not the original filename passed to `store`. This is
  a deliberate design choice, not an oversight — see §14.

## 5. SHA-256 digest verification design

Three read operations exist, deliberately separated by cost and purpose:

**`List(store, doc)`** — cheap, presence-only. For each evidence entry (up
to `MaxEntriesPerCommand`, §9), derives the object path and `os.Stat`s it:
present or absent, no read, no re-hash. Used by `evidence list`.

**`Verify(store, doc)`** — full integrity check. For each evidence entry (up
to `MaxEntriesPerCommand`, and only after confirming the *sum* of all
entries' stored object sizes is within `MaxTotalVerifyBytes`, §9):

1. Derive the object path from the digest: strip the `sha256:` prefix,
   split into `<first 2 hex><remaining 62 hex>`.
2. `os.Lstat` the derived path.
   - Not found → **MISSING**.
   - Found but is a symlink → **CORRUPT** (rejected without following it —
     see §7).
3. Open and read the object under a size-capped `io.LimitReader` (cap:
   `evidence.MaxObjectSize`, same belt-and-suspenders pattern
   `idir.LoadFile` already uses), computing SHA-256 over the actual bytes
   read.
4. Compare the computed hex digest to the filename's digest.
   - Match → **OK**.
   - Mismatch → **CORRUPT** (the stored bytes were altered, truncated, or
     the object was placed at the wrong path).

Used by `evidence verify`.

**`Inspect(store, digest)`** — single-object, no document involved. Takes a
bare digest argument (validated against the same
`^sha256:[0-9a-f]{64}$` shape before any filesystem access), and runs the
same Lstat → size-check → read → re-hash sequence as one `Verify` step
would, against that one object only (so only `MaxObjectSize` applies — no
document, so no entry count or aggregate cap). Returns metadata only:
digest, size in bytes, OK/MISSING/CORRUPT status. **Never returns or prints
the object's content** — see §11.

`Result` types carry one outcome per evidence entry (by `id`) for `List`
and `Verify`, or a single outcome for `Inspect`. `incidentdna evidence
verify` exits `0` only if every processed entry is OK (zero evidence
entries is vacuously OK); any MISSING/CORRUPT entry, or a resource-limit
violation, causes exit code `2`/`1` respectively (see §11 for the exact
split). This preserves the existing "well-formed but not trustworthy"
meaning of exit code `2`.

## 6. Local filesystem security model

Unchanged trust boundary from Phase 1: `incidentdna` runs with exactly the
invoking user's OS-level filesystem permissions, no elevation, no service,
no multi-tenant concept. Phase 2 adds one new category of local file I/O
(the evidence store) under the same model:

- `evidence store <file>` reads a path given directly on the command line —
  same trust boundary as `validate <file>` already has.
- **Store root resolution and validation** (applies identically to `store`,
  `verify`, `list`, `inspect`): the `--store` value (or the default
  `.incidentdna/evidence/objects`) is resolved to an absolute path once,
  at the start of the command, via `filepath.Abs` + `filepath.Clean`. That
  single resolved path is used for every subsequent operation in that
  invocation — no re-resolution mid-command, which would otherwise open a
  window for the meaning of a relative `--store` value to change if the
  working directory changed underneath a long-running process. The
  resolved root must be a directory or a path where one can be created
  (`os.MkdirAll`, `0o755`); if the path exists and is *not* a directory
  (e.g. a regular file or a symlink to one), the command fails immediately
  with a clear error rather than attempting to write through it.
- Objects are written `0o444` (read-only) after the atomic rename in §8, as
  a lightweight guard against accidental in-place mutation — not a security
  boundary against a determined local actor with write access to the same
  directory (no sandboxing beyond OS permissions is claimed, same as Phase
  1's threat model already states).
- No code path in Phase 2 opens a file whose path came from *inside* a
  document. The store's lookup is digest → path, computed by `incidentdna`
  itself, never by decoding a path out of the IDIR document or from
  filenames/metadata passed on the command line.

## 7. Path traversal protections

Path traversal is structurally prevented, not just defended against:

- The only input to the object-path derivation is a digest that has
  already been validated against `^sha256:[0-9a-f]{64}$` — 64 lowercase hex
  characters, nothing else — either by `internal/validate`'s existing rule
  (for `verify`/`list`, which operate on documents already run through
  `Validate`) or by an equivalent check `internal/evidence` runs itself on
  the raw command-line argument (for `store`'s just-computed digest and
  `inspect`'s user-supplied digest argument). There is no character in that
  alphabet that can produce `..`, `/`, a null byte, or any other
  traversal-relevant sequence.
- Belt-and-suspenders, matching the idiom `idir.LoadFile` already uses for
  size limits: `evidence.objectPath` additionally runs the resulting path
  through `filepath.Clean` and asserts it still has the *resolved* store
  root (§6) as a prefix before any file operation, so even a future bug in
  the digest-shape assumption fails closed instead of silently escaping the
  store directory.
- `evidence store <file>`'s destination path is derived from the
  **just-computed digest of the file's own bytes**, never from the source
  filename or any command-line string — so there is no user-controlled
  string on the write path either.
- No command ever constructs an object path from evidence metadata
  (`id`, `type`, `location`) or from the original filename given to
  `store` — confirmed by §4 and tested per acceptance criterion 5.
- Symlinks at a would-be object path are rejected (via `Lstat`) on every
  read path (`List`'s presence check treats a symlink as "not a valid
  object" rather than following it; `Verify`/`Inspect` reject it as
  CORRUPT), and writes never follow an existing symlink at the destination
  (see §8) — prevents a symlink planted in the store directory (e.g. by
  another local process/user with write access) from redirecting a read or
  write outside the store.

## 8. Atomic-write and overwrite-protection strategy

- `evidence store` writes to a temp file inside `<store-root>/.tmp/` via
  `os.CreateTemp`, writes the size-capped bytes, `Sync()`s it, then
  `os.Rename`s it into its final `<shard>/<hash>` path. Rename within the
  same filesystem is atomic — a concurrent reader either sees no file or
  the complete file, never a partial write. `.tmp` is under the same store
  root specifically so the rename is always same-filesystem (a cross-device
  rename would fail or silently fall back to copy+delete, breaking
  atomicity).
- `os.Rename` replaces the destination directory entry directly rather than
  following any symlink that might exist there — documented explicitly here
  as a relied-upon property, not left implicit.
- **Overwrite protection is structural, not policy-based.** Because the
  path is the hash of the content, two different byte sequences can never
  collide on the same path (barring a SHA-256 collision, treated as
  cryptographically infeasible per Phase 1's existing "collision resistance
  is adequate for this use case" stance in `threat-model.md`). This means,
  unlike `init --force`, there is no `--force` flag and no "already exists"
  error for `evidence store` — re-storing identical bytes is always a safe
  no-op.
- On a no-op (destination already exists), `evidence store` still re-hashes
  the **existing** object before declaring success, rather than trusting
  the filename — cheap given the per-object size cap, and it turns a
  silently-corrupt pre-existing object into an immediate, visible error
  instead of a false "already stored" success.

## 9. Maximum evidence size and resource limits

Three independent, named constants in `internal/evidence/limits.go`, each
with its own test and its own distinct error message:

| Constant | Value | Applies to | Error message names |
|---|---|---|---|
| `MaxObjectSize` | 50 MiB | Any single object read or written, by any of the four commands | the digest/file and the byte count over the limit |
| `MaxEntriesPerCommand` | 100 | The number of `evidence[]` entries `List` or `Verify` will process from one document | the actual entry count and the limit |
| `MaxTotalVerifyBytes` | 500 MiB | The sum of stored object sizes `Verify` will read across one invocation | the computed total and the limit |

Enforcement details:

- `MaxObjectSize` uses the same two-layer pattern `idir.LoadFile` already
  establishes: an `os.Stat` pre-check before opening, plus an
  `io.LimitReader` cap during the actual read (belt-and-suspenders against
  a file that grows between stat and read). Applied on **every** read or
  write of a single object, in all four commands.
- `MaxEntriesPerCommand` is checked against `len(doc.Evidence)` **before**
  `List` or `Verify` does any filesystem work at all — a document with more
  than 100 evidence entries fails fast with a clear error naming the actual
  count, rather than partially processing 100 of them silently.
- `MaxTotalVerifyBytes` is checked by `Verify` in two passes: first,
  `os.Stat` every referenced object (cheap) and sum their sizes; if the sum
  exceeds 500 MiB, fail immediately with the computed total in the error
  message, before reading a single byte of any object. This avoids doing
  partial, wasted verification work on a document that was always going to
  be rejected. (`List` never reads object bytes, so `MaxTotalVerifyBytes`
  does not apply to it — only `MaxEntriesPerCommand` does.)
- `MaxObjectSize` and `MaxTotalVerifyBytes` are independent and both
  enforced: a single 60 MiB object is rejected by the per-object cap even
  if the aggregate would otherwise fit; ten 60 MiB objects would fail the
  per-object cap on the first one encountered during the size-summing pass.
- Every subcommand, including all four `evidence` ones, continues to run
  under `main.go`'s existing 30-second `context.Context` timeout — no
  change to that mechanism. Documented as a known limitation (§15) that,
  even within the 500 MiB/100-entry caps, verification on a slow disk could
  still approach this timeout.
- No total-store-size quota (across all objects ever stored, independent of
  any one document) is enforced in Phase 2 — single-user local CLI,
  consistent with Phase 1's already-accepted "no multi-tenant or quota
  model" limitation.

## 10. Tampering and forged-evidence threat analysis

| Scenario | Detected in Phase 2? | How |
|---|---|---|
| Digest is correctly formatted but nothing was ever stored | Yes | `list`/`verify` report MISSING |
| Stored object's bytes were altered/truncated after storage | Yes | `verify`/`inspect` report CORRUPT (re-hash mismatch) |
| Store directory tampered with a symlink at an object path | Yes | Rejected at `Lstat` before any read, on every read path |
| Evidence store read during a concurrent write to the same object | Yes (no false positive) | Atomic rename means a reader only ever sees the old or new complete file, never a partial one |
| Document references more evidence, or larger evidence, than the resource limits allow | Yes | `verify`/`list` fail fast with a limit-specific error (§9) rather than silently truncating the check |
| Attacker fabricates *both* a fake evidence file and updates the IDIR document's digest to match it, consistently | **No** | Content-addressing proves internal self-consistency (the stored bytes match the recorded digest), not truthfulness or provenance of what those bytes actually are. This is the same class of limitation Phase 1's threat model already accepts for semantic tampering ("the fingerprint answers 'is this the same failure class,' not 'is this document truthful'") — Phase 2 does not attempt to solve authorship/provenance, which would require signing, explicitly out of scope. |
| Digest field itself edited to a different, still-valid-shaped digest of *different* real evidence | **No** (by design) | Indistinguishable from "the incident was legitimately re-evidenced" without an out-of-band record of what the original digest was — version-controlling the IDIR document (e.g. in git) is the mitigation, external to this codebase. |

## 11. CLI command changes

```
incidentdna evidence store <evidence-file> [--store <dir>]
    Compute the SHA-256 digest of <evidence-file> and copy its bytes into
    the local evidence store (default: .incidentdna/evidence/objects).
    Does not read, write, or otherwise touch any incident document. Prints
    the canonical "sha256:..." digest and a short explanation of how to
    reference it in an IDIR document's evidence[].digest field.

incidentdna evidence verify <incident-file> [--store <dir>]
    Validate <incident-file> as an IDIR document, then check every
    evidence[].digest against the local evidence store: OK if the stored
    object's bytes hash to the recorded digest, MISSING if nothing is
    stored under that digest, CORRUPT if a stored object's bytes no longer
    match its digest. Subject to MaxEntriesPerCommand and
    MaxTotalVerifyBytes (see §9).

incidentdna evidence list <incident-file> [--store <dir>]
    Validate <incident-file> as an IDIR document, then print each
    evidence[] entry's id, type, digest, and whether an object is present
    in the store at that digest (a stat-only presence check — no
    re-hashing, no integrity guarantee; use verify for that). Subject to
    MaxEntriesPerCommand.

incidentdna evidence inspect <digest> [--store <dir>]
    Look up <digest> directly in the local evidence store (no incident
    document involved) and print its metadata: presence, size in bytes,
    and whether re-hashing its stored bytes matches <digest>
    (OK/MISSING/CORRUPT). Never prints the object's content.
```

Sample `store` output (exact wording to be finalized during
implementation, but this is the intended shape):

```
$ incidentdna evidence store payment-worker.log
Stored: sha256:1f3c9a7e5d2b8f4c6a0e9d3b7f1c5a8e2d4b6f0c8a2e4d6b8f0c2a4e6d8b0f2a

To reference this evidence in an IDIR document, add an entry under
evidence[]:
  - id: <choose-an-id>
    type: <choose-a-type, e.g. log>
    location: <optional free-text description>
    digest: "sha256:1f3c9a7e5d2b8f4c6a0e9d3b7f1c5a8e2d4b6f0c8a2e4d6b8f0c2a4e6d8b0f2a"

incidentdna evidence store never modifies incident documents automatically.
```

Exit codes: `0` success; `1` I/O/usage error (bad file, bad store
directory, can't load the document, malformed `--digest` argument to
`inspect`) **or** a resource-limit violation (§9) — treated as `1` rather
than `2` since exceeding a limit is an operational/usage condition, not a
finding about the evidence itself; `2` for `verify` when any processed
evidence entry is MISSING or CORRUPT, and for `inspect` when the looked-up
digest is MISSING or CORRUPT. `list` always exits `0` if the document loads
and validates (it is purely informational — presence/absence is shown in
its output, not encoded as a failure exit code, since "not yet stored" is
an expected, non-error state during authoring).

`init`, `validate`, `fingerprint`, `inspect` *(the existing document
`inspect`, unrelated to the new `evidence inspect`)*, `compare` are
**unchanged** — zero behavior difference; the existing `cli_test.go` cases
for them must keep passing exactly as written. `main.go`'s `commands` table
gains one entry, `{"evidence", runEvidence}`, and `runEvidence` does its
own four-way `store`/`verify`/`list`/`inspect` sub-dispatch using the same
`flag.NewFlagSet` + `fs.Usage` idiom every existing subcommand already
uses — no CLI framework introduced. (Naming note: the existing top-level
`incidentdna inspect <file>` command and the new `incidentdna evidence
inspect <digest>` command are distinct commands with the same verb in
different namespaces — flagged here explicitly to avoid confusion; no
naming collision exists since one is top-level and the other is under
`evidence`.)

## 12. Testing strategy

- **`internal/evidence` unit tests** (table-driven, real filesystem via
  `t.TempDir()` — no mocking of `os`, matching this repo's existing style):
  - `Store`: round-trips bytes to the correct digest-derived path; storing
    identical bytes twice is a no-op that re-verifies the existing object;
    storing different bytes never collides; rejects a source file over
    `MaxObjectSize` with a message naming the size and the limit; never
    touches any path outside the resolved store root.
  - `List`: reports present/absent correctly without reading object
    content (verified by a test object whose content would fail a re-hash
    but is still reported "present" by `List`, distinguishing it from
    `Verify`); rejects a document with more than `MaxEntriesPerCommand`
    entries with a message naming the count and the limit.
  - `Verify`: reports OK/MISSING/CORRUPT correctly, including against a
    deliberately hand-corrupted object
    (`testdata/golden/evidence/corrupt-object`) and a symlink planted at an
    object path; enforces `MaxEntriesPerCommand` and `MaxTotalVerifyBytes`
    independently, each with its own passing and failing test case. The
    `MaxTotalVerifyBytes` test drives the pure size-summing/limit-check
    function with fabricated `os.FileInfo`-like sizes rather than writing
    500+ MiB of real files to disk, so the test suite stays fast; a
    smaller, real-file integration test separately confirms the full
    `Verify` path calls that function correctly.
  - `Inspect`: reports OK/MISSING/CORRUPT for a single digest; confirms its
    return type carries no content field/byte slice that could leak
    evidence bytes into a caller by accident (a type-level guarantee, not
    just a behavioral test).
  - Object-path derivation: rejects any digest not matching
    `^sha256:[0-9a-f]{64}$` (defense-in-depth even though callers should
    never pass one) and confirms the resolved path always has the store
    root as a prefix.
- **`cmd/incidentdna/cli_test.go` additions**: build the real binary and
  exercise all four subcommands — `store` against a temp file (assert
  printed digest matches an independently computed `sha256sum`, and assert
  no incident file's bytes changed); `verify` against the new
  `examples/evidence-storage-demo/incident.yaml` after its evidence is
  stored (exit 0); `verify` after deleting/corrupting one stored object
  (exit 2, correct per-entry message); `verify` against a digest that was
  never stored (exit 2, MISSING); `list` showing present/absent without
  requiring valid content; `inspect` on a stored digest (asserts the
  command's combined stdout+stderr never contains the stored object's
  distinctive test content); resource-limit tests driving `verify`/`list`
  with a synthetic document that has 101 evidence entries (expect the
  `MaxEntriesPerCommand` error, not a partial result).
- **Regression discipline**: the full existing suite
  (`go test ./... -race -count=1`) must pass with zero modifications to any
  existing test file's expectations. The golden fingerprint test/script
  must produce the unchanged Phase 1 value.
- **New example, not a modification of the existing one**: a new directory
  `examples/evidence-storage-demo/` holds a separate, fully fictional IDIR
  document (distinct `incident.id`, distinct scenario — not a duplicate-payment
  variant) plus a small `evidence/` directory of fictional evidence files
  (e.g. a short synthetic log excerpt, a short synthetic ticket note — no
  credentials, no personal information, no customer data, no real incident
  material of any kind, matching the discipline already stated in
  `examples/duplicate-payment/incident.yaml`'s own header comment). This
  example's `evidence[].digest` values are real SHA-256 hashes of the real
  fictional files checked into `evidence/`, so `evidence store` +
  `evidence verify` can be demonstrated end-to-end against it in
  `docs/phase-2-report.md`. `examples/duplicate-payment/` and
  `testdata/golden/duplicate-payment.fingerprint` are not touched by this
  work at all.
- Whether to also wire this new example into `make example`/CI is left as
  an implementation-time detail, not a blocking decision — the Makefile
  and CI workflow may gain a lightweight additional check, but that is not
  required by any acceptance criterion above and can be deferred without
  affecting Phase 2's completion.

## 13. Backward compatibility with IDIR v0.1

- No change to `idir.Document`, `schemas/idir/v0.1/idir.schema.json`, or
  any `internal/validate` rule — `evidence[].digest`'s existing format
  regex is exactly what Phase 2's verification step depends on and does
  not modify.
- No change to `internal/canonical` or `internal/fingerprint` — the
  identity payload already excludes evidence entirely
  (`docs/fingerprint-design.md`), so the fingerprint of every existing
  document, including `examples/duplicate-payment/incident.yaml`, is
  byte-for-byte unchanged. That example file itself is not modified in
  Phase 2 (§12).
- `validate` continues to only format-check `evidence[].digest` — it does
  **not** start requiring the referenced object to exist in a store. That
  would silently break every Phase 1 document (including every fixture
  under `testdata/golden/invalid/`) the moment Phase 2 lands, and would
  conflate two independently useful, independently exit-coded checks (see
  §14). "Valid IDIR document," "evidence enumerable," and "evidence
  verifiable against a store" remain three separate questions, checked by
  three separate commands (`validate`, `evidence list`, `evidence verify`).
- An IDIR document authored before Phase 2 existed, whose evidence was
  never stored anywhere, is not an error — `evidence list`/`evidence
  verify` on it simply report every entry absent/MISSING. Nothing about
  `validate`/`fingerprint`/`inspect`/`compare` changes for such a document.

## 14. Architectural decisions and rejected alternatives

- **Content-addressable local filesystem store, not a database.**
  Directly matches `phase-1-report.md`'s own "Recommended Phase 2 scope"
  #1 ("an addressable store, even just local-filesystem-backed to start").
  No new dependency; trivially inspectable with ordinary file tools;
  consistent with the project's stated minimal-dependency philosophy.
  Rejected: SQLite or a custom index file — would add a dependency (or a
  second source of truth that can drift from actual store contents) for no
  benefit at this scale.
- **Digest-keyed lookup, decoupled entirely from `evidence[].location` and
  from filenames/metadata.** Preserves Phase 1's explicit, deliberate
  "never dereference location" invariant (`threat-model.md`, "Path
  traversal through CLI inputs") instead of reopening that decision, and
  extends the same principle to every new command, not just `store`.
  Rejected: using `location`, `id`, or the original filename to derive (or
  influence) the store lookup key — would resurrect exactly the
  path-traversal risk Phase 1 went out of its way to design around.
- **New leaf package `internal/evidence`**, not folded into `internal/idir`
  or `internal/validate`. Keeps `internal/idir` as pure vocabulary with no
  filesystem-store concerns, per `architecture.md`'s stated one-way
  dependency direction. `evidence` imports `idir` the same way
  `fingerprint` and `compare` already do; nothing imports `evidence` back.
- **Four separate subcommands (`store`/`verify`/`list`/`inspect`) rather
  than fewer, more overloaded ones.** `list` and `verify` are deliberately
  split by cost: `list` is a fast, always-safe-to-run presence check useful
  during authoring (before evidence is even fully stored); `verify` is the
  expensive, integrity-guaranteeing check meant for CI/gate-style use.
  Collapsing them into one command with a `--deep`/`--full` flag was
  considered and rejected: the exit-code semantics differ in a way that
  matters (`list` never fails on missing evidence; `verify` does), and
  making that a flag rather than a distinct verb makes the safer default
  behavior (`list`) easy to invoke accidentally with gate-blocking
  semantics. `inspect` is separated from `verify` because it addresses the
  store directly by digest with no document/incident context at all — a
  different input shape, not just a different depth of check. Considered
  `store-evidence`/`verify-evidence`/etc. (rejected: breaks the established
  single-verb naming convention of `init`/`validate`/`fingerprint`/
  `inspect`/`compare`) and folding verification into `validate` via a flag
  (rejected: conflates independently useful, independently exit-coded
  operations).
- **`store` never touches an incident document.** Considered having `store`
  optionally accept an incident file and an evidence `id` and auto-append a
  matching `evidence[]` entry. Rejected for Phase 2: it would require
  parsing and rewriting a user's YAML/JSON document in place (a new,
  nontrivial risk surface — preserving comments/formatting on rewrite,
  concurrent-edit safety, "what if the id already exists" semantics) for a
  convenience feature that isn't part of the stated scope (evidence storage
  and digest verification, not document authoring tooling). Explicit,
  copy-pasteable instructions in `store`'s output are the Phase 2 answer;
  automatic document mutation is left for a future phase if wanted.
- **`inspect` returns metadata only, never content, at the type level.**
  Considered a `--show-content`/`--raw` escape hatch. Rejected: evidence
  files may contain sensitive material (that's exactly why `privacy-model.md`
  and the "known sensitive locations" concept exist for documents); a CLI
  command whose entire job is confirming *that* evidence exists and is
  intact should not also be a general-purpose "dump arbitrary local file
  content to stdout" tool. A user who wants to read the raw evidence file
  already has normal filesystem access to
  `<store-root>/<shard>/<hash>` and can use any tool they like for that —
  `incidentdna` itself just never does it on their behalf.
- **Idempotent, structurally overwrite-proof storage**, not an explicit
  `--force` flag like `init` has. The hash-equals-path property of a CAS
  makes accidental overwrite structurally impossible rather than merely
  policy-guarded, so there's nothing for a `--force` flag to protect
  against.
- **Three fixed resource-limit constants, not configurable flags.** Mirrors
  `idir.MaxDocumentSize`'s existing precedent (a hardcoded constant, not a
  config surface) — avoids introducing a configuration system for three
  numbers. If real usage later shows these need to be tunable, that's a
  deliberate, visible future change (same discipline already applied to
  the fingerprint algorithm), not a default Phase 2 feature.
- **No manifest/index of what's stored.** The store-root directory listing
  is the complete, authoritative inventory. Rejected: a separate index
  file — a second source of truth for the same information, which could
  silently drift from the real directory contents.
- **A separate, new example rather than modifying `duplicate-payment`.**
  Directly per the approved decision: the existing example and its golden
  fingerprint are load-bearing for Phase 1's regression guarantees and are
  left untouched; a new example is strictly additive and lower-risk.

## 15. Known limitations

- Verifying a digest only proves "the stored bytes hash to the digest
  recorded in the document" — not that the digest, or the document, is
  truthful, or who stored it, or when (no provenance/signing chain; see
  §10's last two rows).
- Resource limits (100 entries, 500 MiB aggregate, 50 MiB per object) are
  fixed constants; a legitimate incident with more or larger evidence than
  that requires either splitting evidence across multiple documents/store
  invocations or a future phase's configurable-limits work — not solved
  here.
- No quota or total-store-size enforcement across all objects ever stored.
- No garbage collection of orphaned evidence objects (objects whose digest
  no longer appears in any known document) — not built, not required by
  the stated scope.
- No remote/shared store — local filesystem only, single machine.
- Even within the resource caps, `evidence verify`'s 30-second command
  timeout could still be reached on a slow disk; no incremental/streaming
  progress output — it would just time out and exit non-zero.
- Stored evidence bytes are not encrypted at rest and are not scanned for
  PII — consistent with, and an explicit extension of, `privacy-model.md`'s
  existing "no encryption at rest, no data retention/deletion policy"
  statement.
- `store` does not modify incident documents automatically (§14) — a user
  must manually copy the printed digest into their document; this is a
  deliberate scope boundary, not an oversight, but is worth restating here
  as a real day-to-day limitation for authors.

## 16. Explicit out-of-scope items

Restating the user's exclusion list — none of the following are touched by
Phase 2: release gates, OmniFlow integration, telemetry ingestion,
OpenTelemetry, Kafka, Kubernetes, React, authentication, billing, AI/LLM
features, cloud infrastructure, real fault injection.

Additionally, specific to evidence storage:

- Remote/cloud evidence storage (S3, GCS, or any object-storage API) — the
  store is a local directory only.
- Any network transfer of evidence bytes — no network access anywhere in
  this codebase, unchanged invariant.
- Encryption at rest for stored evidence.
- Automatic evidence capture/ingestion from logs or observability
  systems — evidence is always a local file the invoking user already has,
  supplied explicitly to `evidence store`.
- Automatic mutation of incident documents by `store` or any other
  `evidence` subcommand (§14).
- Configurable/tunable resource limits — the three constants in §9 are
  fixed for this phase.
- Multi-user or shared-store access control.
- Evidence retention/expiry policy or garbage collection.
- Any change to what the fingerprint payload includes — evidence remains
  fully excluded from the fingerprint, unchanged from Phase 1.
- Any modification to `examples/duplicate-payment/` or its golden
  fingerprint.
