# Incident Library

Phase 3 adds a local, filesystem-backed library of validated incident
*occurrences*, grouped by the existing deterministic failure fingerprint
(`internal/fingerprint.Compute`, unchanged), plus three `incidentdna library`
subcommands to write to and read from it. This document describes what that
library is, what it guarantees, and — just as importantly — what it does
not. `docs/phase-3-plan.md` is the approved plan this implements; this
document describes the resulting design as built, for the same audience
`docs/evidence-storage.md` and `docs/fingerprint-design.md` serve.

Implementation: `internal/library/` (`store.go`, `add.go`, `check.go`,
`list.go`, `index.go`, `privacy.go`, `limits.go`, `errors.go`, `result.go`)
and `cmd/incidentdna/cmd_library.go`.

## What problem this closes

An IDIR document can be authored, validated, fingerprinted, and compared
one-to-one against exactly one other document (`incidentdna compare a b`).
Phase 2 added evidence storage for a single document's referenced evidence
files. But there was no way to accumulate a collection of previously-observed
incidents anywhere and ask, of a new or candidate document, "has a failure
with this fingerprint already been recorded?" Phase 3 closes exactly that
gap: it persists validated incident occurrences, groups recurring
occurrences by fingerprint, and provides an offline lookup capability.

Phase 3 does not change `idir.Document`, the JSON Schema, `internal/validate`,
`internal/canonical`, `internal/fingerprint`, `internal/compare`, or
`internal/evidence`. The library operates strictly on already-valid documents
and their already-computed fingerprints; it introduces no new field into the
IDIR format itself, and does not build a release gate, executable regression
scenarios, or any authenticity/signing layer.

## The key correction from the plan's first draft

The fingerprint identifies a **failure class**, not a unique occurrence. The
library therefore stores **multiple distinct occurrences under one
fingerprint** — a "first one wins" dedup-to-one-entry design was
deliberately rejected (see `docs/phase-3-plan.md` §5). Two documents
describing the same underlying failure pattern, authored independently for
two different incident records, both belong in the library as two separate
occurrences of the same failure class.

## The store: layout and default root

The library is content-addressed at **two levels**, neither derived from
untrusted input:

```
<library-root>/
  <fp shard: 2 hex chars>/<fp remainder: 62 hex chars>/     one directory per failure-class fingerprint
    index.json                                              digest -> incident_id, occurred_at
    occurrences/
      <digest shard: 2 hex chars>/<digest remainder: 62 hex chars>.json   one file per occurrence
```

- **Default root**: `.incidentdna/library/objects`, under the same
  `.incidentdna` root the Phase 2 evidence store already uses, resolved
  relative to the current working directory
  (`internal/library/store.go`, `DefaultLibraryRoot`).
- **Fingerprint-keyed directory**: the directory for a failure class is
  addressed by the document's own fingerprint — the "sha256:"-prefixed,
  64-lowercase-hex string `internal/fingerprint.Compute` already produces.
  The first two hex characters are the shard, the remaining 62 the
  directory name, mirroring `internal/evidence`'s existing sharding scheme.
- **Digest-keyed occurrence object**: within a fingerprint's directory, each
  occurrence is addressed by the SHA-256 digest of that occurrence's own
  **canonical document bytes** (`internal/canonical.Marshal` applied to the
  *whole* validated document, not just the identity payload the fingerprint
  is computed from) — sharded the same way.
- **`incident.id` is never used as, or concatenated into, a filesystem
  path.** It appears only as a value inside stored JSON content
  (`index.json`, the occurrence body itself) — the direct extension of the
  evidence store's existing "digest-keyed lookup, decoupled entirely from
  filenames/metadata" decision to a two-level (fingerprint, occurrence) key
  space.
- **`index.json`** maps each stored occurrence's digest to its `incident.id`
  and `incident.occurred_at`, so `Add` can detect a same-`incident.id`
  conflict, and `List` can order/summarize occurrences, without re-reading
  every occurrence body just to check identity. `List` still opens (and
  integrity-checks) each occurrence body to read `incident.title` and
  `application.service`, since `index.json` deliberately holds only digest,
  incident id, and occurred timestamp.
- **What's stored in an occurrence object**: the canonical JSON re-encoding
  of the *entire* validated document, not just the identity payload — so
  `library list`/`check` can report record-identity fields the fingerprint
  payload itself deliberately excludes.
- `library` and `evidence` remain independent, parallel concerns: a library
  occurrence does not store or verify the evidence *files* a document's
  `evidence[]` block references — that stays `internal/evidence`'s job,
  unchanged. The two stores are not linked to each other.

## The three commands

```
incidentdna library add [--library <dir>] [--allow-unredacted] <file>
incidentdna library check [--library <dir>] <file>
incidentdna library list [--library <dir>]
```

**`library add <file>`** — loads and validates `<file>` (the same
`validate.Validate` semantic rules Phase 1 already runs), enforces the
privacy gate (below), computes its fingerprint and canonical-bytes digest,
and applies the occurrence-decision semantics below. It never overwrites an
existing occurrence and never silently discards data.

**`library check <file>`** — loads and validates `<file>`, computes its
fingerprint using the exact same `internal/fingerprint.Compute` call every
other command uses (no separate "library-specific" fingerprinting logic),
and reports whether the library holds one or more *intact* occurrences under
that fingerprint. Before reporting a match, `check` verifies every indexed
occurrence under that fingerprint is present, a regular file, and re-hashes
to its own declared digest — a corrupted library entry is never reported as
a match; if any indexed occurrence is missing or corrupted, `check` fails
distinctly (see "Occurrence semantics" below) instead of silently reporting
a match based on the remaining entries or falling back to no-match.

**`library list`** — enumerates every fingerprint group currently stored,
each with its occurrence count and, per occurrence, the bounded field set:
incident id, title, application/service, occurred timestamp. Never the full
stored document, business invariants, event timeline, evidence contents, or
remediation text. No `--verbose` flag and no JSON output mode exist in
Phase 3.

Sample transcript (see `examples/incident-library-demo/` for the full,
runnable version):

```
$ incidentdna library add --library /tmp/demo-lib incident-a.yaml
Stored new occurrence: fingerprint sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade (incident: INC-2026-0501)

$ incidentdna library add --library /tmp/demo-lib incident-a-recurrence.yaml
Stored new occurrence: fingerprint sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade (incident: INC-2026-0618)

$ incidentdna library check --library /tmp/demo-lib incident-a.yaml
Fingerprint: sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade
Match: 2 occurrence(s) found for this fingerprint

$ incidentdna library list --library /tmp/demo-lib
Fingerprint: sha256:dc5bbc532759499e91da83d35e4e37d513a468b81f3145cbaa760b021c71bade (2 occurrence(s))
  - incident: INC-2026-0618
    title:    checkout-service-eu unresponsive during connection pool saturation
    service:  checkout-service-eu
    occurred: 2026-06-18T03:07:41Z
  - incident: INC-2026-0501
    title:    Checkout connection pool exhaustion triggers retry storm
    service:  checkout-service
    occurred: 2026-05-01T14:22:10Z
```

### Exit codes

- `add`: `0` on stored-new or idempotent-no-op; `1` on any failure (invalid
  usage, invalid document, privacy policy violation, conflict, malformed
  library, permission, corruption, resource limit, internal failure).
- `check`: `0` if one or more matching occurrences are found; `1` for
  invalid usage, invalid document, malformed library, permission,
  corruption, resource-limit, or internal failure; `2` for a valid document
  with no matching fingerprint. This reuses exit code `2`'s existing meaning
  — "well-formed input, but a materially untrustworthy/negative finding" —
  but note it is a *different specific condition* from `evidence
  verify`/`validate`'s use of the same code; `library check`'s own help text
  and this document state the distinction explicitly rather than leaving it
  implicit.
- `list`: `0` on success, including an empty library (a library that has
  never had anything added is a successful, empty result, not an error); `1`
  on failure (malformed library, permission, corruption, resource-limit,
  internal failure).

## Occurrence semantics

Evaluated at `add` time, after computing the incoming document's fingerprint
and canonical-bytes digest:

1. **No existing occurrence under that fingerprint shares the incoming
   document's `incident.id`** → store a new occurrence (subject to
   `MaxOccurrencesPerFingerprint`, and `MaxLibraryEntries` if this is the
   fingerprint's first occurrence).
2. **An existing occurrence shares `incident.id` and has the same
   canonical-bytes digest** (the incoming document is the same content,
   possibly reformatted in a way that doesn't change its canonical JSON —
   whitespace, key order, or YAML vs. JSON serialization) → idempotent:
   report already-present, write nothing.
3. **An existing occurrence shares `incident.id` but has a different
   canonical-bytes digest** (materially different content under the same
   declared identity) → conflict: fail with `library.ErrConflict`, never
   overwrite, never silently add a second entry.
4. **An occurrence `add`/`check`/`list` needed to consult is unreadable, not
   a regular file, or doesn't re-hash to the digest its own path/index entry
   declares** → corrupted: fail with `library.ErrCorruptedOccurrence`,
   distinct from both "not found" and "conflict"; never reused, repaired, or
   silently overwritten as a side effect of an unrelated call.

A different-`incident.id` document sharing the same fingerprint is always
case 1 (a new occurrence) — never deduplicated away, and never treated as a
conflict merely for sharing a fingerprint with something else.

## `--library` behavior

Every one of the three commands accepts `--library <directory>`. The value
(or the default `.incidentdna/library/objects` if omitted) is resolved to an
absolute, cleaned path exactly once, at the start of the command
(`library.Open`), and that single resolved path is used for the rest of the
invocation.

If a symlink exists at the given library location, `Open` resolves it via
`filepath.EvalSymlinks` and uses the real target as the root. If the
resolved location exists and is not a directory, `Open` fails immediately
with a clear error. If nothing exists there yet, `check`/`list` simply find
an empty library — a library that hasn't been created yet is not an error
for a read.

## Privacy policy

`library add` requires `privacy.redacted == true` by default. The check can
be bypassed only with an explicit `--allow-unredacted` flag, which always
produces a visible warning on stderr — never a silent bypass:

```
warning: --allow-unredacted override was used: this document does not declare privacy.redacted == true; it was stored anyway
```

The warning is emitted only when the override actually changed the outcome
(`AddResult.PrivacyOverridden`) — never when the document already declared
`privacy.redacted == true`, and it never includes any part of the document's
content.

**`privacy.redacted` is author-declared, not verified.** Setting it to
`true` records only that the document's author asserts it was reviewed and
sanitized before authoring — nothing in `internal/library`, or anywhere else
in this codebase, confirms that assertion is actually true. This is a
stricter default than any other Phase 1/2 operation (which all treat
`redacted` as author-declared and never mandatory for the operation to
proceed): the library is the first place in this codebase that persists a
full document indefinitely rather than processing it transiently, so it is
also the first place that refuses non-redacted input by default. See
`docs/privacy-model.md` for the fuller statement of this policy.

`library` commands never print raw stored document contents under any flag:
`check` and `list` report only the bounded field sets described above; there
is no code path in any `library` subcommand that dumps a stored or input
document's full body to stdout/stderr.

## Resource limits

Four independent, fixed constants (`internal/library/limits.go`), each with
its own distinct error naming the limit and the offending value:

| Constant | Value | Applies to |
|---|---|---|
| `MaxDocumentSize` | 5 MiB (reused from `internal/idir`, not redefined) | Any document's original on-disk file size, enforced by `idir.LoadFile` at the CLI's file-loading boundary — the same limit every other command already enforces |
| `MaxLibraryEntries` | 10,000 | Total distinct fingerprints (failure classes) the library will hold before `add` refuses to create a new fingerprint directory |
| `MaxOccurrencesPerFingerprint` | 100 | Occurrences stored under a single fingerprint directory before `add` refuses a genuinely new occurrence (never blocks an idempotent re-add) |
| `MaxListResults` | 1,000 | Number of fingerprint entries `list` will enumerate in one invocation |

`MaxLibraryEntries` is checked only when `add` is about to create a
fingerprint directory that doesn't yet exist; adding another occurrence to
an already-existing fingerprint group never changes the total fingerprint
count, so it never triggers this check. `MaxListResults` is checked once
`list`'s structural directory walk has counted every fingerprint entry, but
before any of the per-entry work (opening and verifying occurrence files) —
a library over the limit fails fast rather than paying for unnecessary work
it will refuse to return.

Every `library` subcommand runs under `main.go`'s existing 30-second overall
command timeout, unchanged. All four constants are fixed, not
user-configurable — a config surface for four numbers was judged not worth
the added surface area for this phase, the same stance Phase 2 took for its
own three resource limits.

## Integrity versus authenticity

`check` and `list` (via their shared intact-occurrence verification) answer
one question precisely: **do the bytes currently on disk at this
occurrence's digest-derived path hash to the digest that index.json
recorded for it?** That is an integrity check — it proves internal
self-consistency. It does not, and cannot, prove:

- That the incident is *truthful* — i.e., that it actually reflects what
  happened in production.
- *Who* added it, or *when* (beyond `incident.occurred_at`, which is itself
  author-declared content, not a store-verified timestamp).
- That `privacy.redacted == true` means the document was actually reviewed
  or sanitized — only that its author asserted so (see "Privacy policy"
  above).

This is the same class of limitation `docs/evidence-storage.md` already
documents for evidence integrity, and the same one `docs/threat-model.md`
accepts for semantic tampering of a document generally: the fingerprint
answers "is this the same failure class," an occurrence's integrity check
answers "are these exact bytes what index.json says they should be," and
neither answers "is this document's account of what happened true."
Proving authorship or provenance would require a signing scheme, explicitly
out of scope for Phase 3 (as it was for Phase 2).

## Symlink and path-traversal protections

Path traversal is structurally prevented, not merely defended against:

- The only values ever turned into a library filesystem path are a
  fingerprint (validated by `Store.FingerprintDir`'s `parseFingerprint` to
  be exactly `sha256:` + 64 lowercase hex characters) and an occurrence
  digest (validated by `Store.OccurrencePath`'s `parseDigestHex` to be
  exactly 64 lowercase hex characters). Neither alphabet can produce `..`,
  `/`, a null byte, or any other traversal-relevant sequence.
- Belt-and-suspenders: `Store.checkContained` additionally asserts every
  derived path still has the resolved library root as a prefix before any
  file operation, so even a hypothetical future bug in the hex-shape
  assumption fails closed instead of silently escaping the library
  directory.
- `incident.id` (or any other document-supplied field) is never used as, or
  concatenated into, a filesystem path — only fingerprint and digest values
  ever become path components.
- On every read path, an unexpected shard or fingerprint directory (wrong
  length, not lowercase hex, or a symlink) is rejected as
  `ErrMalformedLibrary` rather than followed or silently skipped; an
  occurrence file that is a symlink or non-regular file is rejected as
  `ErrCorruptedOccurrence` rather than trusted or followed.

## Filesystem and TOCTOU limitations

The library's checks reduce, but do not eliminate, time-of-check/time-of-use
gaps inherent to any filesystem-based tool without OS-level file locking —
the same class of limitation `docs/evidence-storage.md` documents for the
Phase 2 evidence store:

- `add` never leaves a partially-written occurrence at a final path (see
  "Atomic write" below), so an occurrence appearing corrupted requires
  either a second actor bypassing the CLI entirely, or genuine disk-level
  corruption.
- No OS-level advisory or mandatory locking is taken on the library
  directory or on individual occurrence/index files during any operation.
  Two concurrent `incidentdna library add` invocations of genuinely
  different content are safe from colliding on the same occurrence path
  (each writes its own temp file, then renames to its own distinct
  digest-derived path) — but `index.json`'s read-modify-write is not
  protected by any lock, so two concurrent `add` calls racing to append to
  the *same* fingerprint's index could, in principle, lose one of the two
  updates. This is a known, accepted gap consistent with this project's
  single-local-user threat model (`docs/threat-model.md`), not a claim of
  safety under concurrent multi-process writes to the same library root.

## Permission and durability limitations

- Occurrence objects are written with mode `0o444` (read-only) after the
  atomic rename, as a lightweight guard against accidental in-place
  mutation — not a security boundary against a determined local actor who
  already has write access to the same directory tree.
- `add` calls `Sync()` on the temporary occurrence file before renaming it,
  so the occurrence's own bytes are flushed to durable storage before it
  becomes visible at its final path. It does not additionally `fsync` the
  shard or fingerprint directory entries after the rename — the same known,
  accepted gap `docs/evidence-storage.md` documents for the evidence store.
- `incidentdna` runs with exactly the invoking user's OS-level filesystem
  permissions; there is no privilege elevation, no service process, and no
  multi-tenant or access-control concept.

## Atomic write behavior

`library add` writes a new occurrence's canonical bytes into a temporary
file in the same directory as its final path (so the finalizing rename is
same-filesystem and atomic), `Sync()`s it, `chmod`s it `0o444`, and
`os.Rename`s it into place. If something already exists at that exact
digest-derived path (an orphaned object from a prior `add` that wrote the
occurrence but was interrupted before its `index.json` update completed),
its content is re-verified against the digest rather than blindly rewritten
or trusted by filename alone — a mismatch is reported as corrupted, never
silently overwritten.

## No network access, no telemetry

Every `library` subcommand is pure local file I/O, consistent with the rest
of this module: `grep -rn '"net' cmd/ internal/` finds no network package
imported anywhere in the codebase. Nothing in `internal/library` or
`cmd/incidentdna/cmd_library.go` opens a socket, makes an HTTP request, or
reports any data externally. This is a stated invariant of the whole project
(`docs/threat-model.md`), extended to the incident library without
exception.

## What Phase 3 explicitly does not provide

- **No release gates or CI blocking.** `library check` is a lookup command;
  wiring a pass/fail signal into any CI/CD pipeline or vendor gate mechanism
  is not built.
- **No executable regression scenarios.** The library stores and groups
  incident documents; it does not generate, run, or replay any test or
  fault-injection scenario from them.
- **No evidence signing or authenticity proof.** A library entry's
  trustworthiness claim is identical in kind to a single document's or a
  single evidence object's: internal self-consistency, not truthfulness or
  provenance.
- **No remote or cloud storage.** Local filesystem only, exactly like the
  evidence store.
- **No encryption at rest.** Stored occurrence bytes are written and read as
  plain files; anyone with filesystem read access to the library root can
  read them directly.
- **No multi-tenancy.** Single local user, same as both prior phases.
- **No garbage collection.** No retention/expiry policy, no
  "remove"/"delete" subcommand — an incident library is, by the product's
  own premise, meant to be a permanent record.
- **No OmniFlow integration.**
- **No network access, no telemetry**, anywhere in this phase,
  unconditional.
- **No configurable resource limits** — the four constants above are fixed
  for this phase.

None of these are claimed to be solved, partially solved, or planned for a
specific near-term release — they are simply out of scope for Phase 3, the
same way analogous items were out of scope for Phase 1 and Phase 2.

## Why the incident library does not affect fingerprints

`internal/fingerprint`'s identity payload is unchanged by Phase 3: nothing
the library introduces or reads (a `--library` directory, an occurrence's
on-disk path, presence/absence in the library, or its integrity status) is
part of `idir.Document` at all, let alone the identity payload. A document's
fingerprint is identical regardless of whether it has ever been added to any
library, or which library root it is checked against.
`examples/duplicate-payment/incident.yaml` and
`testdata/golden/duplicate-payment.fingerprint` are untouched by this phase.

## The `incident-library-demo` example

`examples/incident-library-demo/` is a third, self-contained example,
entirely separate from `examples/duplicate-payment/` and
`examples/evidence-storage-demo/`. It is fully fictional: a slow query holds
a database connection past its expected duration, saturating
`checkout-service`'s connection pool, and upstream clients amplify the
resulting queue with a retry storm. `examples/incident-library-demo/README.md`
documents the exact commands demonstrating all three approved paths — a new
occurrence stored under an existing fingerprint (`incident-a-recurrence.yaml`,
a different `incident.id` reporting the same failure class), an idempotent
re-add (`incident-a-repeat.json`, the same incident reformatted as JSON), and
a `check` miss (`incident-different-failure.yaml`, an unrelated cache
stampede scenario, exit code 2). No real credentials, personal information,
or customer data appear anywhere in the directory.

## Recovery guidance for corrupted or conflicting entries

- **`library check`/`library list` fail with a corrupted-occurrence
  error**: an occurrence referenced by a fingerprint's `index.json` is
  missing, not a regular file, or re-hashes to something other than its own
  declared digest. The stored copy cannot be trusted or automatically
  repaired — `incidentdna` never overwrites an occurrence it cannot verify.
  Because the library is just a set of files (no hidden index beyond each
  fingerprint's own small `index.json`), the entry can be inspected directly
  with ordinary filesystem tools at
  `<library-root>/<fp shard>/<fp remainder>/occurrences/<digest shard>/<digest remainder>.json`,
  and a human must decide how to resolve the discrepancy — `incidentdna`
  cannot judge which side is correct on its own.
- **`library add` reports a conflict**: an occurrence with the same
  `incident.id` already exists under this fingerprint, with materially
  different canonical bytes. This means either the same incident was
  re-authored with different content under the same id (in which case the
  existing entry is the historical record and the new content needs a new,
  distinct `incident.id` if it should be tracked separately), or two
  unrelated incidents were accidentally assigned the same id (in which case
  one of the two source documents needs to be corrected before either can be
  added). `incidentdna` refuses to guess which; it only refuses to
  overwrite.
- A resource-limit error (`MaxLibraryEntries`, `MaxOccurrencesPerFingerprint`,
  `MaxListResults`) is not itself a corruption or conflict — it means the
  library (or one fingerprint group within it) has reached its fixed Phase 3
  capacity; there is no override flag for any of the four limits.

## Current Phase 3 status and remaining limitations

Implemented, per `internal/library/` and `cmd/incidentdna/cmd_library.go`: a
local, two-level content-addressed incident library; `library add`,
`library check`, and `library list`; the four resource limits above; the
privacy gate and `--allow-unredacted` override; path-traversal and symlink
protections; atomic, conflict-checked, idempotency-aware writes. This
document describes that implementation as it exists in the code today.

Known, accepted limitations, restated from the sections above so they are
findable in one place:

- Verifying an occurrence's digest proves internal consistency only, not
  truthfulness, authorship, or provenance (see "Integrity versus
  authenticity").
- `privacy.redacted` is author-declared, never independently verified (see
  "Privacy policy").
- The 10,000-fingerprint / 100-occurrence-per-fingerprint / 1,000-list-result
  limits are fixed for this phase; a library that needs more requires a
  future phase to reconsider them.
- No garbage collection, no remote/shared library, no encryption at rest, no
  signing/authenticity proof, no multi-tenant access control, no release-gate
  integration.
- `index.json`'s read-modify-write is not protected by any lock; concurrent
  `add` calls against the *same* fingerprint from separate processes are not
  guaranteed safe (see "Filesystem and TOCTOU limitations").
- `library check`/`library list` still run under the existing 30-second
  command timeout; a very large library on a slow disk could still reach it.

This document intentionally does not claim CI results, production usage, or
performance benchmarks for this phase — none of those have been established
here.
