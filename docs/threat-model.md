# Threat Model

Scope: the `incidentdna` CLI and the `internal/*` libraries it's built on,
as they exist at the end of Phase 3 — local file input, local file/stdout
output plus a local content-addressed evidence store and a local incident
library, no network, no multi-user or multi-tenant concerns yet.

## Maliciously modified incident documents

A document could be edited to misrepresent what happened (e.g. remove a
causal step, soften a business invariant) while still passing structural
validation. Phase 1 cannot detect *semantic* tampering — validation checks
internal coherence (no dangling refs, no cycles, non-empty required fields),
not truthfulness against some external source of record. What it does
guarantee: any such edit that changes a fingerprint-relevant field (see
[`fingerprint-design.md`](fingerprint-design.md)) produces a **different**
fingerprint, so a tampered document cannot silently masquerade as the
original incident's regression scenario under the same identity. Evidence
digests (`sha256:`-prefixed, format-checked) let a *future* phase verify
evidence integrity against externally stored material, once evidence
storage/retrieval is built — Phase 1 only enforces the digest's format, not
that it matches any actual bytes (there is nothing to fetch and check
against yet).

## Fingerprint collisions or canonicalization ambiguity

Mitigated by: (1) a dedicated, independently-tested canonical JSON encoder
(`internal/canonical`) rather than relying on incidental struct-marshaling
behavior; (2) SHA-256, whose collision resistance is adequate for this use
case; (3) golden tests pinning the exact canonical bytes' hash for a known
document, so a canonicalization regression is caught immediately rather than
silently changing what "the same incident" means. Residual risk: the
identity payload's field selection is itself a judgment call (see
`fingerprint-design.md`) — two failures that a human would consider
different could theoretically map to the same fingerprint if they happen to
share every included dimension. This is a design tradeoff, not a bug, and is
the reason `compare` explains *which* dimensions matched rather than only
reporting a boolean.

## Sensitive information entering incident evidence

Mitigated by the `privacy` block: a fixed set of "known sensitive
locations" (`trigger.raw_payload_excerpt`, `events[].raw_payload_excerpt`,
`evidence[].raw_excerpt`) that validation requires to be empty when
`privacy.redacted` is `true`, plus a regex backstop (email/phone/long-digit
patterns) over free-text description fields. This is **not** general
PII detection — see [`privacy-model.md`](privacy-model.md) for the explicit
scope and limitation. A document that is *not* marked `redacted` is not
scanned at all; authors are responsible for setting `redacted` accurately
and for what they write into non-"known-sensitive" free-text fields
(`description`, `summary`, etc.) even when redacted, beyond what the regex
backstop happens to catch.

## Forged evidence references

`evidence[].digest` must be `sha256:`-prefixed and exactly 64 lowercase hex
characters; a malformed digest is rejected by `internal/validate` — this was
true in Phase 1 and remains unchanged. As of Phase 2, that format
precondition has something real to be checked against: `incidentdna evidence
verify`/`evidence inspect` (`internal/evidence`) check whether an object is
actually present under the store at a declared digest, and whether
re-hashing its stored bytes reproduces that digest. A digest that is
correctly formatted but was never stored is reported MISSING; a stored
object whose bytes have been altered, truncated, or replaced is reported
CORRUPTED. `evidence[].location` remains treated as inert metadata — the CLI
never opens, fetches, or otherwise acts on it, in Phase 2 exactly as in
Phase 1 (see "Path traversal through CLI inputs" below).

This closes the *integrity* gap Phase 1 named here, but not an *authenticity*
gap: an attacker who fabricates a fake evidence file and updates the
document's digest to match it, consistently, produces a result that verifies
successfully — content-addressing proves the stored bytes match the recorded
digest, not that those bytes are a truthful account of what happened, or who
produced them. That is the same class of limitation this document's
"Maliciously modified incident documents" section already accepts for
semantic tampering: fingerprinting and digest verification both answer "is
this internally self-consistent," not "is this true." Solving that would
require a signing/provenance scheme, explicitly out of scope for Phase 2 —
see [`evidence-storage.md`](evidence-storage.md), "Integrity versus
authenticity," and "Tampering and forged-evidence threat analysis" in
[`phase-2-plan.md`](phase-2-plan.md) §10 for the full scenario-by-scenario
breakdown.

## Evidence store: local filesystem risks (Phase 2)

The evidence store (`internal/evidence`) is local file I/O under the same
trust boundary as the rest of the CLI: `incidentdna` runs with exactly the
invoking user's OS-level filesystem permissions, with no elevation, no
service, and no multi-tenant concept. New risks specific to the store, and
how each is addressed:

- **Path traversal via a stored object's location.** Structurally
  prevented, not just defended against: the only value ever converted into a
  filesystem path is a `Digest`, constructible only from exactly 64
  lowercase hex characters (`internal/evidence.Parse`) or from hashing bytes
  directly — no character in that alphabet can produce `..`, `/`, or a null
  byte. `Store.objectPath` additionally re-checks the resulting path against
  the resolved store root as belt-and-suspenders. No object path is ever
  derived from `evidence[].location`, an entry's `id`/`type`, or the
  original filename passed to `store`.
- **Symlink abuse.** `evidence store` rejects a symbolic-link source file
  outright rather than following it. Every read path (`list`'s presence
  check, `verify`/`inspect`'s re-hash, `store`'s dedup re-check) rejects a
  symlink found at an object's expected path rather than following it, so a
  symlink planted in the store directory by another local actor with write
  access cannot redirect a read or write outside the store.
- **Concurrent writes / partial reads.** `evidence store` writes to a
  temporary file under the store root and `os.Rename`s it into place, which
  is atomic on the same filesystem — a concurrent reader sees either no
  object or the complete object, never a partial write. This is not backed
  by any OS-level file lock; see [`evidence-storage.md`](evidence-storage.md),
  "Filesystem and TOCTOU limitations," for the residual gaps this does not
  close.
- **Store root confusion.** The `--store` value (or the default) is resolved
  to an absolute, symlink-resolved path exactly once per invocation, at the
  start of the command, so a relative `--store` value cannot silently change
  meaning if the working directory changes mid-invocation.
- **Resource exhaustion via evidence.** See the next section.

## Evidence resource limits (Phase 2)

Mirroring `internal/idir.MaxDocumentSize`'s existing precedent, three fixed
constants in `internal/evidence/limits.go` bound the store's exposure to a
maliciously or accidentally oversized document or evidence file:
`MaxObjectSize` (50 MiB, enforced during streaming, not just a preliminary
stat), `MaxEntriesPerCommand` (100 `evidence[]` entries processed per `list`/
`verify` invocation), and `MaxTotalVerifyBytes` (500 MiB aggregate stored
bytes `verify` will read per invocation, checked via a stat-only pre-pass
before any content is read). Each is enforced independently and produces a
distinct, actionable error naming the limit and the offending value. All
`evidence` subcommands additionally run under the same 30-second overall
command timeout every other subcommand already has. See
[`evidence-storage.md`](evidence-storage.md), "Resource limits," for the
full detail.

## Incident library: local filesystem risks (Phase 3)

The incident library (`internal/library`) is local file I/O under the same
trust boundary as the rest of the CLI and the evidence store: no elevation,
no service, no multi-tenant concept. It extends the evidence store's
already-reviewed defenses to a two-level (fingerprint, occurrence-digest)
key space:

- **Path traversal via a stored occurrence's location.** Structurally
  prevented, not just defended against: the only values ever turned into a
  library filesystem path are a fingerprint (validated to be exactly
  `sha256:` + 64 lowercase hex characters, the same format
  `internal/fingerprint.Compute` already produces) and an occurrence's
  canonical-bytes digest (64 lowercase hex characters) — neither alphabet
  can produce `..`, `/`, or a null byte. `Store.checkContained` additionally
  re-checks every derived path against the resolved library root before any
  file operation. `incident.id` is never used as, or concatenated into, a
  filesystem path — it appears only as a value inside stored JSON content
  (`index.json`, the occurrence body).
- **Symlink abuse.** Every read path rejects an unexpected shard or
  fingerprint directory (wrong length, not lowercase hex, or a symlink) as
  malformed rather than following it; an occurrence file that is a symlink
  or non-regular file is rejected as corrupted rather than trusted or
  followed.
- **Concurrent writes / partial reads.** `library add` writes a new
  occurrence's canonical bytes to a temporary file under the library root,
  `Sync()`s it, and `os.Rename`s it into place, atomic on the same
  filesystem — a concurrent reader sees either no occurrence or the
  complete occurrence, never a partial write. As with the evidence store,
  this is not backed by any OS-level file lock; `index.json`'s
  read-modify-write specifically is not lock-protected, so two concurrent
  `add` calls racing to append to the *same* fingerprint's index could, in
  principle, lose one of the two updates — a known, accepted gap consistent
  with this project's single-local-user threat model, not a claim of safety
  under concurrent multi-process writes. See
  [`incident-library.md`](incident-library.md), "Filesystem and TOCTOU
  limitations," for the full detail.
- **Library root confusion.** The `--library` value (or the default
  `.incidentdna/library/objects`) is resolved to an absolute,
  symlink-resolved path exactly once per invocation, at the start of the
  command (`library.Open`), the same discipline the evidence store's
  `--store` flag already follows.
- **Occurrence conflicts and corruption are not silently resolved.** A
  same-`incident.id` document with materially different canonical bytes
  under an existing fingerprint is refused as a conflict; an occurrence that
  fails to re-hash to its own declared digest is reported as corrupted.
  Neither case is ever repaired or overwritten automatically as a side
  effect of an unrelated `add`/`check`/`list` call.
- **Resource exhaustion via the library.** See "Incident library resource
  limits (Phase 3)" below.

**Integrity, not authenticity.** As with evidence digest verification above,
a library occurrence's integrity check (`check`/`list` re-hashing stored
bytes against the digest `index.json` recorded) proves internal
self-consistency, not that the incident is truthful, or who added it, or
when. The library's privacy gate (`privacy.redacted == true` required by
`add` unless `--allow-unredacted` is given) is likewise author-declared, not
independently verified — setting it does not itself prove the document was
actually reviewed or sanitized. The incident library is not an
authorization or trust system: it has no user accounts, no access control,
and no concept of an incident being "approved." See
[`incident-library.md`](incident-library.md), "Integrity versus
authenticity," and "Privacy policy," for the full detail.

## Incident library resource limits (Phase 3)

Mirroring the evidence store's existing precedent, four fixed constants in
`internal/library/limits.go` bound the library's exposure to a maliciously
or accidentally oversized document, an oversized library, or an oversized
single failure class: `MaxDocumentSize` (5 MiB, reused unchanged from
`internal/idir`, enforced at the same `idir.LoadFile` boundary every other
command already uses), `MaxLibraryEntries` (10,000 distinct fingerprints
before `add` refuses to create a new fingerprint directory),
`MaxOccurrencesPerFingerprint` (100 occurrences under a single fingerprint
before `add` refuses a genuinely new one — never blocks an idempotent
re-add), and `MaxListResults` (1,000 fingerprint entries `list` will
enumerate in one invocation). Each is enforced independently and produces a
distinct, actionable error naming the limit and the offending value. Every
`library` subcommand runs under the same 30-second overall command timeout
every other subcommand already has. See
[`incident-library.md`](incident-library.md), "Resource limits," for the
full detail.

## Path traversal through CLI inputs

`incidentdna validate/fingerprint/inspect` open exactly the file path(s)
given directly on the command line by the invoking user — the same trust
boundary as any CLI tool that takes a filename argument (`cat`, `jq`, etc.).
The one place this could go wrong is if the tool followed a path *embedded
inside* a document (e.g. `evidence[].location`) — it deliberately never
does; that field is presentational only. `incidentdna init` writes to a
fixed relative path (`.incidentdna/incident.yaml`) under the current
directory, never a user- or document-supplied path, and refuses to
overwrite an existing file unless `--force` is passed (see next item).

`incidentdna evidence store` similarly opens exactly the file path given
directly on the command line. Its destination inside the store, and every
path `evidence verify`/`list`/`inspect` read from, is derived only from a
validated digest — never from `evidence[].location`, an entry's `id`/`type`,
or the source filename passed to `store` — so a document cannot cause the
CLI to read or write any path other than the one the invoking user
explicitly named or a digest-derived path under the resolved `--store` root.
See "Evidence store: local filesystem risks (Phase 2)" above.

## Resource exhaustion from maliciously large documents

`internal/idir.LoadFile` checks the file's size via `os.Stat` before opening
it, and independently caps the actual bytes read via `io.LimitReader` (belt
and suspenders against a file that grows between stat and read, or a
non-regular file with an unreliable reported size) at `MaxDocumentSize` (5
MiB). Beyond that, every CLI subcommand runs under a 30-second
`context.Context` timeout (`cmd/incidentdna/main.go`), and
`internal/validate`'s loops check `ctx.Err()` periodically, so a
pathological-but-under-the-size-cap document (e.g. a huge `events` array)
cannot hang the process indefinitely.

## YAML parser abuse

Decoding always targets a concrete typed `idir.Document` struct (`yaml.v3`
via `Decode`), never `interface{}` or `map[string]interface{}`. This rules
out the classic "YAML decodes into arbitrary Go types/interfaces and code
acts on them unexpectedly" class of issue, since unknown fields and unknown
YAML tags simply have nowhere to go. Combined with the size cap above (which
bounds alias/anchor expansion in absolute terms, since the *source* bytes
are capped before parsing even starts) and `gopkg.in/yaml.v3`'s own
built-in alias-count limits, a "billion laughs"-style expansion attack is
bounded. Residual risk: this project does not independently verify yaml.v3's
internal expansion limits are sufficient on their own without the size cap —
the size cap is treated as the primary control, not a backstop.

## Dependency compromise

The dependency surface is deliberately minimal: `gopkg.in/yaml.v3` is the
**only** non-stdlib Go dependency in the module (`go.mod` — verified via
`go mod tidy`, see `docs/phase-1-report.md`). `go.sum` pins exact checksums.
No lint/build tooling is fetched as a Go dependency either (see
`architecture.md`, "golangci-lint → go vet + gofmt"). This does not
eliminate supply-chain risk, but it minimizes the attack surface to review.

## Generated files overwriting existing user files

`incidentdna init` checks whether `.incidentdna/incident.yaml` already
exists and refuses to proceed (non-zero exit, no write) unless `--force` is
passed — verified in `cmd/incidentdna/cli_test.go` and manually in
`phase-1-report.md`. No other subcommand writes any file.

## Future multi-tenant risks (explicitly out of scope through Phase 3)

Phase 3's incident library is **local and single-user, not shared or
multi-tenant** — it is the same trust boundary as the evidence store before
it: no concept of a tenant, user account, or access-control layer of its
own. Every invocation, including every `library` and `evidence` subcommand,
operates on files and a store the invoking user already has filesystem
access to. Risks that become relevant only if a shared/remote/multi-tenant
incident library or evidence store is built in a later phase — none of this
exists today, and Phase 3 explicitly does not build it (see
[`product-scope.md`](product-scope.md)):

- Cross-tenant fingerprint/identity leakage (can one tenant infer another
  tenant's incident existed, from a shared fingerprint namespace?).
- Authorization for who may write, read, or mark an incident "resolved" in
  a shared library — Phase 3's library has no concept of an incident being
  "approved," "resolved," or owned by anyone; presence in the library
  proves only internal self-consistency (see "Incident library: local
  filesystem risks (Phase 3)" above).
- Rate limiting / quota enforcement once ingestion is not "a person runs a
  CLI against a file they already have."
- Tenant isolation for the evidence store or the incident library: both are
  a single flat local directory tree with no per-tenant namespacing, access
  control, or quota beyond the per-invocation resource limits in
  [`evidence-storage.md`](evidence-storage.md) and
  [`incident-library.md`](incident-library.md) — appropriate for a single
  local user, not for a store shared across untrusted parties.

None of these are mitigated today because none of the underlying
capabilities (network service, multi-user storage, multiple callers) exist
yet in this codebase.
