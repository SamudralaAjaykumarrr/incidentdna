# Evidence Storage

Phase 2 adds a local, content-addressable store for the raw evidence files an
IDIR document's `evidence[]` entries reference by digest, plus four
`incidentdna evidence` subcommands to write to and read from it. This
document describes what that store is, what it guarantees, and — just as
importantly — what it does not. `docs/phase-2-plan.md` is the approved plan
this implements; this document describes the resulting design as built,
for the same audience `docs/fingerprint-design.md` and `docs/threat-model.md`
serve.

Implementation: `internal/evidence/` (`store.go`, `verify.go`, `list.go`,
`inspect.go`, `digest.go`, `limits.go`, `errors.go`, `result.go`) and
`cmd/incidentdna/cmd_evidence.go`.

## What problem this closes

Phase 1 defined `evidence[].digest` — a `sha256:`-prefixed, 64-lowercase-hex
string — and validated its *format*, but there was nothing to check it
*against*. `docs/threat-model.md` named this gap explicitly: "a forged digest
that's correctly formatted is currently indistinguishable from a real one."
Phase 2 closes exactly that gap: a place to put evidence bytes, keyed by
their own hash, and commands to check a document's declared digests against
what is actually stored.

Phase 2 does not change `idir.Document`, the JSON Schema, `internal/validate`,
`internal/canonical`, or `internal/fingerprint`. `evidence[].digest`'s
existing format rule is unchanged; Phase 2 simply gives it something real to
be checked against.

## The store: layout and default root

The store is a content-addressable object store on the local filesystem,
structurally similar to git's object store:

```
<store-root>/
  <first 2 hex chars>/<remaining 62 hex chars>   one file per stored object, raw bytes
  .tmp/
    <random>                                      staging area for atomic writes
```

- **Default root**: `.incidentdna/evidence/objects`, resolved relative to the
  current working directory (`internal/evidence/store.go`,
  `DefaultStoreRoot`).
- **Digest-derived layout**: an object's path is the store root, followed by
  a subdirectory named by the digest's first two hex characters (the
  "shard"), followed by a file named by the remaining 62 hex characters.
  For example, digest `sha256:f010c383e443d6ead7da242d34f47b31728bffe97ff6ccd7e9c053d2a7757182`
  is stored at `<store-root>/f0/10c383e443d6ead7da242d34f47b31728bffe97ff6ccd7e9c053d2a7757182`.
- `.tmp/` is a reserved staging subdirectory for atomic writes (see below). It
  can never collide with a real shard directory: shard directories are
  always exactly two lowercase hex characters, and a hex digest can never
  start with `.`.
- The object's path is derived **only** from its own digest — never from the
  original filename, `evidence[].location`, or an evidence entry's `id` or
  `type`. This holds uniformly across all four commands. Storing the same
  bytes twice always produces the same path; the store is a flat,
  inspectable set of files, and `find .incidentdna/evidence/objects -type f`
  (excluding `.tmp/`) is a complete inventory — there is no separate
  manifest or index file to drift out of sync with it.
- `evidence[].location` remains exactly what it was in Phase 1: free-text,
  presentational metadata, excluded from the fingerprint and never opened or
  dereferenced by the CLI. The store's lookup key is the digest, never the
  location string or the original filename passed to `store`.

## The four commands

```
incidentdna evidence store [--store <directory>] <evidence-file>
incidentdna evidence verify [--store <directory>] <incident-file>
incidentdna evidence list [--store <directory>] <incident-file>
incidentdna evidence inspect [--store <directory>] <digest>
```

**`evidence store <file>`** — reads `<file>` from the local filesystem,
computes its SHA-256 digest while streaming it into the store, and prints the
canonical `sha256:...` digest plus copy-pasteable instructions for adding a
matching `evidence[]` entry to an IDIR document. It never reads from, writes
to, or otherwise touches any incident document — a user must manually copy
the printed digest into their document. This is a deliberate scope boundary,
not an oversight (`docs/phase-2-plan.md` §14): auto-mutating a user's
YAML/JSON document in place would introduce a nontrivial new risk surface
(preserving formatting/comments, concurrent-edit safety) for a convenience
feature outside Phase 2's stated scope.

**`evidence verify <incident-file>`** — loads and validates `<incident-file>`
as an IDIR document (the same `validate.Validate` semantic rules Phase 1
already runs), then, for every `evidence[]` entry, checks whether an object
is present at its declared digest and whether re-hashing that object's actual
bytes still reproduces the digest. This is the expensive, full
integrity-guaranteeing check, intended for CI/gate-style use.

**`evidence list <incident-file>`** — loads and validates `<incident-file>`,
then reports, for each `evidence[]` entry, whether an object is present at
its declared digest — a stat-only presence check, no re-hashing. This is the
cheap, always-safe-to-run check, useful during authoring before evidence has
necessarily been stored yet.

**`evidence inspect <digest>`** — looks up a bare digest directly in the
store, with no incident document involved, and reports metadata: presence,
size in bytes, and whether re-hashing matches the digest. It never prints the
object's content — `InspectResult` has no field capable of holding it, and no
code path reads the object into anything but a hash function.

Sample `store` output:

```
$ incidentdna evidence store --store /tmp/demo-store payment-worker.log
Stored: sha256:1f3c9a7e5d2b8f4c6a0e9d3b7f1c5a8e2d4b6f0c8a2e4d6b8f0c2a4e6d8b0f2a (52 bytes)

To reference this evidence in an IDIR document, add an entry under evidence[]:
  - id: <choose-an-id>
    type: <choose-a-type, e.g. log>
    location: <optional free-text description>
    digest: "sha256:1f3c9a7e5d2b8f4c6a0e9d3b7f1c5a8e2d4b6f0c8a2e4d6b8f0c2a4e6d8b0f2a"

incidentdna evidence store never modifies incident documents automatically.
```

### Exit codes

- `store`: `0` on success (including a deduplicated no-op); `1` for any I/O,
  usage, symlink-source, or resource-limit error.
- `verify`: `0` only if every processed entry is OK (zero entries is
  vacuously OK); `2` if any processed entry is MISSING or CORRUPTED, or if
  the document fails semantic validation; `1` for a resource-limit violation
  or other I/O/usage error.
- `list`: `0` whenever the document loads and validates — presence/absence is
  shown in the output, not encoded as a failure exit code, since "not yet
  stored" is an expected, non-error state during authoring; `1`/`2` only for
  the same load/validate/resource-limit failures the other commands share.
- `inspect`: `0` if the digest is OK; `2` if MISSING or CORRUPTED; `1` for a
  malformed digest argument or other I/O/usage error.

This preserves Phase 1's existing meaning for exit code `2` — "well-formed
input, but a materially untrustworthy finding" — rather than introducing a
new meaning for it.

## `--store` behavior

Every one of the four commands accepts `--store <directory>`. The value (or
the default `.incidentdna/evidence/objects` if omitted) is resolved to an
absolute, cleaned path exactly once, at the start of the command
(`evidence.Open`), and that single resolved path is used for the rest of the
invocation — it is never re-resolved mid-command, which would otherwise let
the meaning of a relative `--store` value change if the working directory
changed underneath a long-running process.

If a symlink exists at the given store location, `Open` resolves it via
`filepath.EvalSymlinks` and uses the real target as the root, so "under the
store root" is unambiguous for the rest of the invocation. If the resolved
location exists and is not a directory (a regular file, or a symlink to one),
`Open` fails immediately with a clear error rather than attempting to write
through it. If nothing exists there yet, a read-only command (`verify`,
`list`, `inspect`) simply finds every object missing — a store that hasn't
been created yet is not an error condition for a read.

## SHA-256 digest format

The only supported algorithm in Phase 2 is SHA-256. The canonical external
form is `sha256:` followed by exactly 64 lowercase hexadecimal characters —
the same format `internal/validate` already enforces on `evidence[].digest`.
`internal/evidence.Parse` is the sole entry point by which an untrusted
string (a CLI argument, or a value read from a document) becomes a validated
`Digest`; a value with the wrong algorithm, wrong length, uppercase
characters, embedded whitespace, or any non-hex character is rejected before
it is ever used to construct a filesystem path.

## Deduplication

Storage is content-addressed, so storing identical bytes twice is
structurally a no-op: the second `store` computes the same digest, finds an
object already at that path, and reports it as **already present** rather
than writing again. Two different byte sequences can never collide on the
same path (barring a SHA-256 collision, treated as cryptographically
infeasible, consistent with Phase 1's existing stance in
`docs/threat-model.md`). Because of this, there is no `--force` flag and no
"already exists" error for `evidence store`, unlike `incidentdna init`.

On a would-be no-op, `store` re-hashes the **existing** object's actual bytes
before declaring success, rather than trusting the filename alone. If that
re-verification fails, `store` reports the existing object as corrupted
instead of silently accepting it or silently overwriting it.

## Atomic write and overwrite-protection behavior

`evidence store` writes the source file's bytes into a temporary file inside
`<store-root>/.tmp/` (`os.CreateTemp`), computing its SHA-256 digest as it
streams, `Sync()`s the temporary file, `chmod`s it `0o444` (read-only), and
then `os.Rename`s it to its final `<shard>/<hash>` path. `.tmp/` lives under
the same store root specifically so the rename is always same-filesystem —
rename is atomic within one filesystem, so a concurrent reader observes
either no file or the complete file, never a partial write; a cross-device
rename would fail or silently fall back to copy+delete, which would break
that guarantee. `os.Rename` also replaces whatever directory entry exists at
the destination directly, rather than following a symlink planted there.

Overwrite protection follows structurally from content-addressing rather than
from a policy check: since the path is a hash of the content, two different
byte sequences cannot occupy the same path.

## Resource limits

Three independent, fixed constants (`internal/evidence/limits.go`), each with
its own distinct error message naming the limit and the offending value:

| Constant | Value | Applies to |
|---|---|---|
| `MaxObjectSize` | 50 MiB | Any single object read or written by any of the four commands, enforced during streaming (not just a preliminary `os.Stat`) |
| `MaxEntriesPerCommand` | 100 | The number of `evidence[]` entries `list` or `verify` will process from one document, checked before any filesystem work begins |
| `MaxTotalVerifyBytes` | 500 MiB | The sum of stored object sizes `verify` will read across one invocation, checked via a stat-only pre-pass before any object's content is read |

`MaxObjectSize` and `MaxTotalVerifyBytes` are independent: a single 60 MiB
object is rejected by the per-object cap even if the aggregate would
otherwise fit under 500 MiB. `list` never reads object content, so
`MaxTotalVerifyBytes` does not apply to it — only `MaxEntriesPerCommand`
does. These are fixed constants, not configurable flags, mirroring
`idir.MaxDocumentSize`'s existing precedent — a config surface for three
numbers was judged not worth the added surface area for this phase.

Every `evidence` subcommand also runs under `main.go`'s existing 30-second
overall command timeout, unchanged from Phase 1. Even within the 100-entry/
500-MiB caps, `evidence verify` against a large document on a slow disk could
still approach that timeout; there is no incremental/streaming progress
output, so it would simply exit non-zero if reached.

## Integrity versus authenticity

`verify` and `inspect` answer one question precisely: **do the bytes
currently on disk at this digest's path hash to the digest that was
recorded?** That is an integrity check — it proves internal
self-consistency. It does not, and cannot, prove:

- That the evidence is *truthful* — i.e., that it actually reflects what
  happened during the incident.
- *Who* stored it, or *when*.
- That the digest in the document wasn't edited, alongside re-storing
  different (but still real) evidence under the new digest, in a way that is
  internally consistent. Content-addressing cannot distinguish "the incident
  was legitimately re-evidenced" from "the evidence reference was swapped" —
  both look identical to a re-hash. Version-controlling the IDIR document
  (e.g., in git) is the practical mitigation for this, and is external to
  this codebase.

This is the same class of limitation `docs/threat-model.md` already accepts
for semantic tampering of the document itself: the fingerprint answers "is
this the same failure class," not "is this document truthful." Evidence
verification answers "are these bytes what they claim to hash to," not
"are these bytes what actually happened, and who put them here." Proving
authorship or provenance would require a signing scheme, which is explicitly
out of scope for Phase 2.

## Symlink and path-traversal protections

Path traversal is structurally prevented, not merely defended against:

- The only value ever turned into a store filesystem path is a `Digest`,
  which is only constructible via `Parse` (rejecting anything but exactly 64
  lowercase hex characters) or by hashing bytes directly. There is no
  character in that alphabet that can produce `..`, `/`, a null byte, or any
  other traversal-relevant sequence — a `Digest` cannot represent a path
  traversal attempt by construction (`internal/evidence/digest.go`).
- Belt-and-suspenders: `Store.objectPath` additionally runs the derived path
  through `filepath.Clean` and asserts it still has the resolved store root
  as a prefix before any file operation, so even a hypothetical future bug in
  the digest-shape assumption fails closed instead of silently escaping the
  store directory.
- No command ever constructs an object path from evidence metadata (`id`,
  `type`, `location`) or from the original filename given to `store`.
- `evidence store`'s source file is rejected outright if it is a symbolic
  link (`Lstat` check before `Open`) — the destination path is derived from
  the just-computed digest of the file's own bytes, but the *source* symlink
  itself is refused rather than followed, so a symlink cannot be used to read
  an unexpected file into the store.
- On every read path, a symlink found at a would-be object location is
  rejected rather than followed: `list`'s presence check treats it as "not
  present," and `verify`/`inspect`/`store`'s dedup re-check treat it as
  CORRUPTED. This prevents a symlink planted in the store directory (by
  another local process or user with write access to it) from redirecting a
  read or a write outside the store.

## Filesystem and TOCTOU limitations

The store's checks reduce, but do not eliminate, time-of-check/time-of-use
gaps inherent to any filesystem-based tool without OS-level file locking:

- `MaxObjectSize` is enforced by capping the actual bytes read during a
  streamed copy (`io.LimitReader`), not solely by an earlier `os.Stat` — this
  closes the specific gap of "a file grows after being stat'd but before
  being read." It does not, and cannot, prevent every possible race: a file
  could still be modified concurrently with `store` reading it, since
  `incidentdna` takes no OS-level file lock on the source.
- `Verify`'s two-pass design (stat every object first to sum sizes against
  `MaxTotalVerifyBytes`, then re-hash) means an object's size or content
  could in principle change between the stat pass and the read pass if
  another process is concurrently writing to the store outside of
  `incidentdna evidence store`'s own atomic-rename path. `incidentdna` itself
  never leaves a partially-written object at a final path (see "Atomic
  write" above), so this scenario requires a second actor bypassing the CLI
  entirely.
- No OS-level advisory or mandatory locking is taken on the store directory
  or on individual objects during any operation. Two concurrent `incidentdna
  evidence store` invocations of different content are safe (each writes to
  its own temp file, then renames to its own distinct digest-derived path,
  so they cannot collide), but this is a property of content-addressing, not
  of any explicit locking.

## Permission and durability limitations

- Objects are written with mode `0o444` (read-only) after the atomic rename,
  as a lightweight guard against accidental in-place mutation — this is not
  a security boundary against a determined local actor who already has write
  access to the same directory tree; no sandboxing beyond ordinary OS file
  permissions is claimed, consistent with Phase 1's existing threat model.
- `store` calls `Sync()` on the temporary file before renaming it, so the
  object's *own* bytes are flushed to durable storage before it becomes
  visible at its final path. It does **not** additionally `fsync` the shard
  or store-root directory entries after the rename. On most local
  filesystems and under normal operation this is not observable, but under a
  hard crash immediately after a rename, whether the directory entry itself
  is durably recorded depends on the underlying filesystem and OS — this is
  a known, accepted gap, not a guarantee this design makes.
- `incidentdna` runs with exactly the invoking user's OS-level filesystem
  permissions; there is no privilege elevation, no service process, and no
  multi-tenant or access-control concept, unchanged from Phase 1.

## No network access, no telemetry

Every `evidence` subcommand is pure local file I/O, consistent with the rest
of this module: `grep -rn '"net' cmd/ internal/` finds no network package
imported anywhere in the codebase. Nothing in `internal/evidence` or
`cmd/incidentdna/cmd_evidence.go` opens a socket, makes an HTTP request, or
reports any data externally. This is a stated invariant of the whole project
(`docs/threat-model.md`), extended to evidence storage without exception.

## What Phase 2 explicitly does not provide

- **No encryption at rest.** Stored evidence bytes are written and read as
  plain files; anyone with filesystem read access to the store root can read
  them directly. This extends `docs/privacy-model.md`'s existing "no
  encryption at rest" statement to the new evidence store specifically.
- **No cloud or remote storage.** The store is a local directory on a single
  machine; there is no S3/GCS/object-storage backend and no network transfer
  of evidence bytes of any kind.
- **No multi-tenancy or access control.** Every invocation operates with the
  filesystem permissions of the invoking user; there is no concept of
  separate tenants, users, or a shared library with per-user permissions.
- **No signing or authenticity proof.** Verification proves content
  integrity (see "Integrity versus authenticity" above), not who created or
  stored an object, or that the referenced evidence is a truthful account of
  what happened.
- **No garbage collection.** Objects whose digest no longer appears in any
  known document are never automatically removed.
- **No quota or total-store-size enforcement** across everything ever
  stored — only the three per-invocation limits above are enforced.
- **No configurable resource limits** — `MaxObjectSize`,
  `MaxEntriesPerCommand`, and `MaxTotalVerifyBytes` are fixed constants for
  this phase.

None of these are claimed to be solved, partially solved, or planned for a
specific near-term release — they are simply out of scope for Phase 2, the
same way they were out of scope for Phase 1's document layer.

## Why evidence storage locations do not affect incident fingerprints

`internal/fingerprint`'s identity payload has excluded all of `evidence[]`
since Phase 1 (see `docs/fingerprint-design.md`) — the fingerprint is
computed only from fields that define the *class* of failure (trigger,
causal structure, invariants, side-effect types, corrected behavior,
technology categories), and evidence entries identify supporting material for
one specific *record*, not the failure pattern itself. Phase 2 does not
change this: no field the evidence store introduces or reads (`--store`
directory, an object's on-disk path, presence/absence in a store, integrity
status) is part of `idir.Document` at all, let alone the identity payload. A
document's fingerprint is therefore identical regardless of which store root
its evidence happens to be checked against, whether that evidence has been
stored anywhere yet, or whether it verifies successfully — exactly as it was
before Phase 2 existed. `examples/duplicate-payment/incident.yaml` and
`testdata/golden/duplicate-payment.fingerprint` are untouched by this phase.

## Why `evidence list` presence does not prove integrity

`list` performs only an `os.Stat`/`os.Lstat` at each entry's digest-derived
path: it can tell you *something* is stored there, and, when present, its
size, but it never opens the object or re-hashes it. This is a deliberate,
documented design choice, not an oversight: `list` is meant to be a cheap,
always-safe check useful during authoring, before every piece of evidence
has necessarily been stored yet, and its cost should not scale with object
size. A file could exist at the right path yet fail to match its declared
digest — truncated, corrupted, or simply the wrong bytes placed there by
something other than `incidentdna evidence store` — and `list` would still
report it "present." `evidence verify` is the command that gives an actual
integrity guarantee (full re-hash of every distinct object); `incidentdna
evidence list`'s own CLI output states this distinction directly ("presence
only, not content integrity").

## The `evidence-storage-demo` example

`examples/evidence-storage-demo/` is a second, self-contained example,
entirely separate from `examples/duplicate-payment/` and never touching it
or its golden fingerprint. It is fully fictional: a scheduled restart of a
session-cache node triggers a cache-miss burst, overloads `auth-service` as a
fallback dependency, and causes `session-gateway` to force a mass logout on
the affected shard. No real credentials, personal information, or customer
data appear anywhere in the directory. `examples/evidence-storage-demo/README.md`
documents the exact commands; abbreviated here (all offline, using a
temporary `--store` directory so nothing touches a real `.incidentdna/`):

```sh
STORE=$(mktemp -d)

./bin/incidentdna validate examples/evidence-storage-demo/incident.yaml

./bin/incidentdna evidence store --store "$STORE" examples/evidence-storage-demo/evidence/session-gateway.log
./bin/incidentdna evidence store --store "$STORE" examples/evidence-storage-demo/evidence/cache-node-restart-event.json
./bin/incidentdna evidence store --store "$STORE" examples/evidence-storage-demo/evidence/incident-ticket-5502.txt

./bin/incidentdna evidence list --store "$STORE" examples/evidence-storage-demo/incident.yaml
./bin/incidentdna evidence verify --store "$STORE" examples/evidence-storage-demo/incident.yaml
./bin/incidentdna evidence inspect --store "$STORE" sha256:f010c383e443d6ead7da242d34f47b31728bffe97ff6ccd7e9c053d2a7757182

rm -rf "$STORE"
```

`evidence[].digest` values in `incident.yaml` are the real SHA-256 digests of
the three fictional files under `evidence/` (verified independently with
`sha256sum` while writing this document — they match). `validate` and
`verify` both exit `0`; `verify`'s summary reads `OK: all 3 evidence entries
verified`; `inspect` on the digest above reports
`Status:    OK (integrity verified)`.

## Recovery guidance for missing or corrupted evidence objects

- **`evidence list`/`evidence verify` report an entry MISSING**: nothing is
  stored at the declared digest under the `--store` root being checked.
  Either run `evidence store` on the original evidence file again (if you
  still have it — this reproduces the same digest and re-populates the
  object), or check whether you meant to point `--store` at a different
  root (the store the evidence was originally written to). A MISSING result
  is not itself evidence of tampering — it is equally, and more commonly,
  what an authored-but-not-yet-stored document looks like.
- **`evidence verify`/`evidence inspect` report an entry CORRUPTED**: an
  object exists at the digest's path, but re-hashing its actual bytes
  produces a different digest, or the path is a symlink/non-regular file
  rather than a real stored object. The stored copy cannot be trusted or
  automatically repaired — `incidentdna` never overwrites an object it
  cannot verify. Recovery means re-obtaining the original evidence bytes
  (from a backup, from wherever `evidence[].location` says it was originally
  sourced from, or from the original incident investigation) and running
  `evidence store` again with the *original, untampered* file; if the
  freshly computed digest matches what the document already declares, the
  object is restored correctly. If it does not match, the document's
  declared digest and the actual evidence have diverged, and that
  discrepancy needs to be resolved by a human before the entry can be
  trusted again — `incidentdna` cannot resolve it automatically, precisely
  because it cannot judge which side is correct on its own.
- Because the store is just a flat set of files (no hidden index), a
  CORRUPTED or MISSING object can also be inspected or replaced directly
  with ordinary filesystem tools using the path scheme described above
  (`<store-root>/<first-two-hex>/<remaining-62-hex>`) — `incidentdna` does
  not need to be involved in a manual recovery, only in re-verifying it
  afterward.

## Current Phase 2 status and remaining limitations

Implemented, per `internal/evidence/` and `cmd/incidentdna/cmd_evidence.go`:
a local content-addressable evidence store; `evidence store`, `evidence
verify`, `evidence list`, and `evidence inspect`; the three resource limits
above; path-traversal and symlink protections; atomic, deduplicating writes.
This document describes that implementation as it exists in the code today.

Known, accepted limitations, restated from the sections above so they are
findable in one place:

- Verifying a digest proves internal consistency only, not truthfulness,
  authorship, or provenance (see "Integrity versus authenticity").
- The 100-entry / 500 MiB-aggregate / 50 MiB-per-object limits are fixed for
  this phase; a legitimate incident with more or larger evidence needs to
  split across multiple documents or store invocations.
- No total-store-size quota, no garbage collection of orphaned objects, no
  remote/shared store, no encryption at rest, no signing/authenticity proof,
  no multi-tenant access control.
- `evidence verify` still runs under the existing 30-second command timeout;
  a large verification on a slow disk could still reach it.
- `store` never mutates incident documents automatically — referencing newly
  stored evidence in a document is a manual, copy-paste step.
- TOCTOU and durability gaps inherent to any lock-free local filesystem tool
  are reduced, not eliminated (see "Filesystem and TOCTOU limitations" and
  "Permission and durability limitations").

This document intentionally does not claim CI results, production usage, or
performance benchmarks for this phase — none of those have been established
here.
