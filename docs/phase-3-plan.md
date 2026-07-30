# Phase 3 Plan: Local Incident Library (Approved for Implementation)

**Status: approved for implementation.** The scope, privacy policy, storage
semantics, CLI surface, exit codes, resource limits, and naming decisions in
this document have been explicitly reviewed and approved by the user (see
§20, "Approved decisions"). **Implementation has not started.** No line of Go
code, no schema change, no example, and no test has been written against
this plan. This document records what was approved so that when
implementation does begin, it proceeds against a settled design rather than
an implementer's inference.

## 0. Why this scope, and what it is not

**There is no pre-existing, explicitly-named "Phase 3" scope anywhere in this
repository's prior documents** — this section is retained to show the
reasoning that led to the approved scope in §20.1, not because the scope is
still open.

- `docs/product-scope.md` defines Phase 1 and Phase 2 scope explicitly, by
  name. It did not define a Phase 3.
- `README.md`'s "Roadmap" section has exactly two headed subsections,
  **"Implemented (Phase 1)"** and **"Implemented (Phase 2, this repository)"**,
  followed by an unheaded, unnumbered **"Future work (not started, not
  scoped, not implemented in this codebase)"** paragraph. That paragraph is a
  flat list of six items with no phase numbers attached to any of them:
  *a shared incident library with storage and retrieval*, *ingestion from
  observability/event systems*, *integration into release gating*, *evidence
  signing/authenticity proof*, *remote/cloud evidence storage*, and *"any of
  the other items listed as explicitly out of scope in
  `docs/product-scope.md`."*
- `docs/phase-1-report.md`, "Recommended Phase 2 scope," lists five items —
  but these were *recommendations for Phase 2*, made before Phase 2 was
  planned, and only one of the five (evidence storage) was actually adopted
  into Phase 2. The other four (a release-gate integration point, OmniFlow
  integration, a real Docker-daemon CI run, reassessing fingerprint
  dimensions against a larger corpus) were **not** carried forward into any
  numbered phase.
- `docs/phase-2-report.md` §14 ("Deferred work") explicitly states: *"Phase 3
  has not been started or scoped as part of this work."*
- `docs/product-scope.md`'s "Explicitly out of scope for Phase 1 and Phase 2"
  list is large and heterogeneous. Nothing in that list was assigned to
  "Phase 3" specifically — it was simply everything Phase 1 and Phase 2 did
  not do. Treating that entire list as "the Phase 3 scope" would have
  violated this project's own established discipline: Phase 1 shipped one
  narrow layer (representation), Phase 2 shipped one narrow layer on top of
  it (evidence integrity), and each phase's plan explicitly rejected
  bundling in adjacent concerns (see `phase-2-plan.md` §14).

Given that, this document proposed **one** narrowly-scoped candidate — the
item most directly and repeatedly named in existing docs, most consistent
with the project's own two-phases-so-far pattern, and lowest-risk to attempt
without touching anything already built. That candidate, and the specific
policy and design decisions needed to make it concrete, were approved by the
user; see §20.

### The approved candidate: a local incident library

`README.md`'s future-work list's first item, verbatim, is *"a shared incident
library with storage and retrieval."* This is also the load-bearing
precondition for the *other* named future-work item, *"integration into
release gating"* (explicitly **not** built in this phase — see §4) — you
cannot check a candidate release against "previously observed failures"
until there is a place that holds more than one incident and a way to ask
"does this fingerprint already exist in that collection." Phase 1 built
pairwise `compare` (exactly two documents). Phase 2 built single-object
evidence storage and integrity checking. Neither builds an *n-many* store of
*incident occurrences themselves*, or a lookup operation against it. This
candidate can be built entirely with the tools Phase 1 and Phase 2 already
used: local filesystem I/O, the existing `idir`/`fingerprint` primitives, and
the same CLI conventions.

**OmniFlow is not part of this phase.** Per the standing instruction not to
introduce OmniFlow unless a document explicitly places it in Phase 3, and
per the finding above that no document does so, it is excluded here (§4,
§20.1) — approved, not open.

## 1. Phase 3 problem statement

An IDIR document can be authored, validated, fingerprinted, and compared
one-to-one against exactly one other document (`incidentdna compare a b`).
Evidence for one document can be stored and integrity-checked (`incidentdna
evidence ...`). But there was no way to accumulate a collection of
previously-observed incidents anywhere and ask, of a new or candidate
document, "has a failure with this fingerprint already been recorded?" This
phase closes that gap: it persists validated incident occurrences, groups
recurring occurrences by the existing deterministic failure fingerprint
(`internal/fingerprint.Compute`, unchanged), and provides an offline lookup
capability. It does not build a release gate, executable regression
scenarios, or any authenticity/signing layer — those remain future work
(§4).

## 2. User value

An engineer who has just authored (or received, e.g. from a postmortem) a
new IDIR document can, with this phase:

- Add it to a local library of previously-recorded incident occurrences
  (`incidentdna library add <file>`).
- Later, run `incidentdna library check <file>` against a *new* incident
  candidate (or a re-authored version of an old one) and learn immediately
  whether its fingerprint matches an existing failure class already in the
  library — deterministically, locally, without needing to know in advance
  which historical document to `compare` against.
- List what the library currently holds (`incidentdna library list`) to
  audit what failure classes are already tracked, and how many times each
  has recurred.

This is the direct, minimal technical precondition for a possible future
release-gate command (explicitly not built in this phase — see §4) that
would run `library check` against a build's test-failure record. Phase 3
builds the "remember" half only.

## 3. Explicit in-scope capabilities

- A new `internal/library` package: a local, filesystem-backed store of
  validated IDIR incident **occurrences**, grouped by their document's own
  fingerprint (reusing `internal/fingerprint.Compute`, unchanged). The
  fingerprint identifies a *failure class*, not a unique occurrence — see §5
  for why the store must hold potentially many occurrences per fingerprint,
  not one.
- Three new CLI subcommands under a new `incidentdna library` verb, following
  the exact dispatch idiom `cmd_evidence.go` already establishes:
  - `incidentdna library add [--library <dir>] [--allow-unredacted] <file>` —
    validates and fingerprints `<file>`, enforces the privacy policy (§10),
    and stores the occurrence per the semantics in §5/§12.
  - `incidentdna library check [--library <dir>] <file>` — validates and
    fingerprints `<file>`, and reports whether one or more occurrences with
    that fingerprint already exist in the library. Exit codes are defined
    explicitly in §7 (not left as an open question).
  - `incidentdna library list [--library <dir>]` — enumerates fingerprints
    currently in the library with a concise, bounded summary per §7 of
    `phase-3-plan.md`'s "List output" rules (approved as §20.7, not the
    richer "full identity payload" alternative previously left open).
- A default library root, `.incidentdna/library/objects`, under the same
  `.incidentdna` root the Phase 2 evidence store already uses, resolved via
  `--library <path>` (never `--store`, which already names the evidence
  store's root flag — see §7).
- Fixed, documented resource limits (§11): `MaxDocumentSize` (reused from
  `idir`), `MaxLibraryEntries`, `MaxOccurrencesPerFingerprint`, and
  `MaxListResults` — each a dedicated named constant with a distinct
  actionable error, not a single generic cap.
- An author-declared privacy gate on `add` (§10): `privacy.redacted == true`
  required by default, overridable only via an explicit `--allow-unredacted`
  flag that always emits a warning.
- A new, fully synthetic example demonstrating `library add`/`check`/`list`
  end-to-end, following the exact discipline `examples/evidence-storage-demo/`
  already set — **proposed for the implementation phase, not created now**
  (per "do not modify examples").
- Documentation: a new `docs/incident-library.md`, plus updates to
  `README.md`, `docs/architecture.md`, `docs/product-scope.md`,
  `docs/threat-model.md`, and `docs/privacy-model.md` — **proposed for the
  implementation phase, not written now.**

## 4. Explicit out-of-scope capabilities

Approved exclusions (§20.1), several restating what Phase 1/2 already
excluded and some new to this phase's own approval:

- **Release gates and CI blocking** — a library *lookup* command
  (`library check`) is in scope; wiring a pass/fail signal into any
  CI/CD pipeline or vendor gate mechanism is not.
- **Executable regression scenarios** — the library stores and groups
  incident documents; it does not generate, run, or replay any test or
  fault-injection scenario from them.
- **Evidence signing or authenticity proof** — a library entry's
  trustworthiness claim is identical in kind to a single document's or a
  single evidence object's: internal self-consistency, not truthfulness or
  provenance (see §10).
- **Remote or cloud storage** — local filesystem only, exactly like the
  evidence store.
- **Encryption** (at rest, or otherwise) — same stance as the evidence
  store.
- **Multi-tenancy** — single local user, same as both prior phases.
- **Garbage collection** — no retention/expiry policy, no "remove"/"delete"
  subcommand. An incident library is, by the product's own premise, meant to
  be a permanent record.
- **OmniFlow integration** — no changes to, or reuse of, the separate
  OmniFlow repository (see §0).
- **Network access** — `library add`/`check`/`list` are pure local file I/O.
- **Telemetry** — none, anywhere in this phase, unconditional.

Also restated from Phase 1/2's existing "out of scope" discipline, unchanged
by this approval:

- React, FastAPI, or any web/API framework; Kubernetes or cloud
  infrastructure; Kafka or live event ingestion; OpenTelemetry or any
  observability pipeline; AI/LLM calls of any kind; SaaS auth/billing; real
  fault injection against a running system.
- **Configurable resource limits** — fixed constants (§11), not a config
  system.
- **Any change to `idir.Document`, the JSON Schema, `internal/validate`,
  `internal/canonical`, or `internal/fingerprint`.** The library stores and
  looks up occurrences against *existing* fingerprints; it does not change
  what a fingerprint is or how one is computed.
- **Any change to `examples/duplicate-payment/` or its golden fingerprint.**
- **Ingestion from observability/event systems** — `library add` takes a
  local file path given directly on the command line, exactly like `evidence
  store` and every other Phase 1/2 command.
- **Fingerprint-dimension reassessment** (`phase-1-report.md`'s Phase 2
  recommendation #5) — independent of this phase, not addressed here.
- **A real Docker-daemon CI run** (`phase-1-report.md`'s Phase 2
  recommendation #4) — orthogonal to this phase's scope.

## 5. Approved architecture

A new, parallel leaf package, `internal/library`, following the same
dependency-direction precedent `internal/evidence` set relative to
`internal/idir`/`internal/fingerprint`.

**Key correction from the earlier draft of this plan:** the fingerprint
identifies a *failure class*, not a unique occurrence (§20 decision 5, and
the corrections in the task that produced this revision). The store must
therefore support **multiple distinct occurrences under one fingerprint**,
not a single dedup-to-first-writer entry. The previous "first one wins"
design is rejected.

```
idir.Document (library add/check)              fingerprint string (library add/check)
   │  idir.LoadFile + validate.Validate            │  fingerprint.Compute(doc)
   ▼                                                ▼
validated idir.Document ─────────────────────▶ fingerprint-derived directory
                                                    │  canonical.Marshal(doc) → canonical bytes
                                                    │  sha256(canonical bytes) → document digest
                                                    ▼
                                    <library-root>/<fp-shard>/<fingerprint-hex>/
                                        occurrences/<digest-shard>/<digest-hex>.json
                                        index.json   (digest → incident.id, occurred_at; used to
                                                       detect same-incident-id conflicts without
                                                       re-reading every occurrence body)
```

Key architectural properties:

- **Content-addressed at two levels, neither derived from untrusted input.**
  The *directory* for a failure class is addressed by the document's
  fingerprint (already a validated `sha256:`-prefixed, 64-lowercase-hex
  string produced by `internal/fingerprint.Compute`). The *occurrence
  object* within that directory is addressed by the SHA-256 digest of the
  occurrence's own canonical document bytes (`internal/canonical.Marshal`
  applied to the whole validated document, not just the identity payload).
  **`incident.id` is never used as, or concatenated into, a filesystem
  path** — it appears only as a value inside stored JSON content
  (`index.json`, the occurrence body), never as a path component. This is
  the direct extension of the evidence store's existing "digest-keyed
  lookup, decoupled entirely from filenames/metadata" decision
  (`phase-2-plan.md` §14) to a two-level (fingerprint, occurrence) key
  space.
- **Sharding**: both the fingerprint directory and the occurrence filename
  use a short hex-prefix shard (e.g. first 2 hex characters) exactly the
  way `internal/evidence`'s existing digest-derived path scheme already
  shards — reuse of an already-reviewed pattern, not a new design.
- **Path-traversal impossibility**: since both path components are the
  output of a fixed-format, validated hash (`internal/fingerprint.Compute`'s
  own `sha256:`+64-hex output for the directory; `sha256` of canonical bytes
  for the occurrence filename), neither component can ever contain `..`,
  `/`, or any other traversal-relevant byte — unrepresentable by
  construction, the same argument `internal/evidence.Digest`/`Parse`
  already relies on.
- **Collision handling**: two different canonical byte-strings mapping to
  the same SHA-256 digest is a cryptographic collision, treated as
  infeasible and out of scope for defense — the identical stance
  `internal/evidence` already takes for its own digest-addressed objects.
  Two different fingerprints sharing a directory shard prefix is ordinary,
  expected sharding, not a collision.
- **Occurrence semantics (§20 decision 5, replacing "first one wins")** —
  evaluated at `add` time, after computing the incoming document's
  fingerprint and canonical-bytes digest:
  1. No existing occurrence under that fingerprint shares the incoming
     document's `incident.id` → store a **new occurrence** (write the
     digest-addressed object, append an `index.json` entry), subject to
     `MaxOccurrencesPerFingerprint` (§11).
  2. An existing occurrence shares `incident.id` **and** has the same
     canonical-bytes digest (i.e. the incoming document is the same
     content, possibly reformatted/reworded in ways that don't change its
     canonical JSON — whitespace, key order, YAML vs. JSON, etc.) →
     **idempotent**: report already-present, write nothing.
  3. An existing occurrence shares `incident.id` **but** has a different
     canonical-bytes digest (materially different content under the same
     declared identity) → **conflict**: fail with a distinct error, do
     **not** overwrite, do not silently add a second entry.
  4. An occurrence referenced by `index.json` is unreadable, or its stored
     bytes don't re-canonicalize/re-digest to the name in its own path →
     **corrupted**: report it distinctly (mirroring `evidence verify`'s
     CORRUPT finding); never reuse, repair, or overwrite it as a side
     effect of an unrelated `add`.
- **What gets stored in an occurrence object**: the canonical JSON
  re-encoding of the entire validated document (not just the identity
  payload), so `library list`/`check` can report record-identity fields
  (`incident.id`, `incident.title`, `application.service`,
  `incident.occurred_at`) that the fingerprint payload itself deliberately
  excludes.
- **No new dependency-direction problem.** `internal/library` imports
  `internal/idir`, `internal/fingerprint`, and `internal/canonical` — the
  same one-way pattern `internal/evidence` already uses. Nothing in `idir`,
  `validate`, `canonical`, `fingerprint`, `compare`, or `evidence` imports
  `library`.
- **`library` and `evidence` remain independent, parallel concerns.** A
  library occurrence does not store or verify the evidence *files* a
  document's `evidence[]` block references — that stays `internal/evidence`'s
  job, unchanged. The two stores are not linked to each other by this
  proposal.

## 6. Package and file layout

Proposed (not created by this planning task):

```
internal/library/
  store.go            — Store type, Open() (root resolution, mirrors evidence.Open)
  add.go              — Add(): fingerprint, digest, occurrence-vs-conflict-vs-idempotent decision (§5)
  check.go            — Check(store, doc) — fingerprint lookup, match/no-match result
  list.go             — List(store) — enumerate fingerprints + bounded per-entry summary (§7)
  index.go            — index.json read/write helpers (digest ↔ incident.id, occurred_at)
  privacy.go          — privacy.redacted enforcement + --allow-unredacted warning path (§10)
  limits.go           — MaxLibraryEntries, MaxOccurrencesPerFingerprint, MaxListResults constants
                         (MaxDocumentSize reused from internal/idir, not redefined)
  errors.go           — structured *Error/ErrorKind, mirroring internal/evidence/errors.go
  result.go           — AddResult, CheckResult, ListResult types
  store_test.go, add_test.go, check_test.go, list_test.go, limits_test.go

cmd/incidentdna/
  cmd_library.go      — `library` command, dispatches to add/check/list
  cli_library_test.go — black-box binary-level integration tests

examples/incident-library-demo/     (proposed name — approved, see §20.8 for the CLI-noun approval
                                      this example name follows)
  README.md
  incident-a.yaml, incident-b.yaml  — two or more small synthetic documents,
                                       at least one pair sharing a fingerprint
                                       but with different incident.id (to
                                       demonstrate the "another occurrence
                                       stored" path), at least one exact
                                       re-add (to demonstrate idempotency),
                                       and at least one with a materially
                                       different fingerprint (to demonstrate
                                       a `check` miss, exit code 2)

docs/
  incident-library.md — new design doc, same role as evidence-storage.md
```

Modified (documentation only, proposed for the implementation phase):

```
README.md                — CLI commands list, roadmap, known-limitations section
docs/architecture.md      — package layout table, dependency direction, data-flow diagram
docs/product-scope.md     — move "shared incident library" from future work into Phase 3 scope
docs/threat-model.md      — new section analogous to "Evidence store: local filesystem risks"
docs/privacy-model.md     — document the privacy.redacted / --allow-unredacted policy (§10)
.gitignore                — add the new default library root, mirroring the existing `.incidentdna/` entry
```

No file under `internal/idir/`, `internal/validate/`, `internal/canonical/`,
`internal/fingerprint/`, `internal/compare/`, `internal/evidence/`,
`examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
`testdata/golden/duplicate-payment.fingerprint`, `Makefile`, or
`.github/workflows/ci.yml` is touched by the *content* of this plan —
though `Makefile`/CI will need a new target (`example-library`, wired into
`verify`, mirroring `example-evidence`) during implementation, which is
explicitly **not** done as part of this planning task, per the standing "do
not modify Makefile or CI" instruction.

## 7. CLI changes

```
incidentdna library add [--library <dir>] [--allow-unredacted] <file>
    Validate and fingerprint <file>. Enforce the privacy policy (§10):
    fails unless privacy.redacted == true, unless --allow-unredacted is
    given, in which case a warning is emitted and the add proceeds. Then
    apply the occurrence semantics of §5:
      - new incident.id under this fingerprint  → stored, new occurrence
      - same incident.id, identical canonical content → idempotent no-op
      - same incident.id, different canonical content → conflict, refused
    Exit 0 on stored-new or idempotent-no-op; exit 1 on any failure
    (invalid usage, invalid document, privacy policy violation, conflict,
    malformed store, permission, corruption, resource limit, internal
    failure).

incidentdna library check [--library <dir>] <file>
    Validate and fingerprint <file>, then report whether the library holds
    any occurrence with that fingerprint.
      exit 0 — one or more matching failure-class occurrences found
      exit 1 — invalid usage, invalid document, malformed store,
               permission, corruption, resource-limit, or internal failure
      exit 2 — valid document, but no matching fingerprint exists in the
               library

incidentdna library list [--library <dir>]
    Enumerate fingerprints currently in the library. Per-entry output is
    bounded to the fields listed in §7's "List output" rule below; not the
    full stored document. Exit 0 on success, exit 1 on failure (malformed
    store, permission, corruption, resource-limit, internal failure).
```

**Flag naming**: `--library <path>`, not `--store` — `--store` already names
the Phase 2 evidence store's root flag, and reusing it for a semantically
different root would be ambiguous across commands that could plausibly take
both (approved, §20.3).

**Default library root**: `.incidentdna/library/objects`, under the same
`.incidentdna` root the Phase 2 evidence store already uses (approved,
§20.3).

**List output** (approved, §20.7 — replacing the earlier open question about
a richer identity-payload listing): the default `library list` output must
remain concise, containing only:

- Fingerprint
- Occurrence count (for that fingerprint)
- Incident ID
- Title
- Application/service (`application.service`, possibly `application.name`)
- Occurred timestamp (`incident.occurred_at`)

It must **not** print business invariants, event timelines, evidence
contents, full remediation text, the complete identity payload, or raw
stored document contents. `library` commands never print raw document
contents under any flag. No `--verbose` flag and no JSON output mode are
introduced in Phase 3 (both explicitly deferred, not merely unspecified).

`main.go`'s `commands` table gains one entry (`{"library", runLibrary}`),
following the exact pattern `{"evidence", runEvidence}` already set. `init`,
`validate`, `fingerprint`, `inspect`, `compare`, and all four `evidence`
subcommands remain byte-for-byte unchanged in behavior — their existing
tests in `cli_test.go` and `cli_evidence_test.go` must keep passing exactly
as written.

## 8. Data-model or IDIR implications

None. `idir.Document`, `schemas/idir/v0.1/idir.schema.json`, and every
`internal/validate` rule are unchanged. The library operates strictly on
already-valid documents and their already-computed fingerprints; it
introduces no new field into the IDIR format itself. A document authored
before Phase 3 existed is fully compatible with `library add`/`check` —
there is nothing about a pre-Phase-3 document that needs to change for the
library to accept it (subject to the privacy gate in §10, which is a policy
check on the existing `privacy.redacted` field, not a new field).

## 9. Phase 1 and Phase 2 compatibility requirements

- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence` must remain
  unmodified (or, if a change is later found genuinely unavoidable during
  implementation, it must be called out explicitly and separately, exactly
  as `phase-2-plan.md` §2's acceptance criterion #4 required for Phase 2 over
  Phase 1).
- `examples/duplicate-payment/incident.yaml` and
  `testdata/golden/duplicate-payment.fingerprint` must remain byte-for-byte
  unchanged, verified the same way Phase 2 verified it (golden test +
  `scripts/verify-golden-fingerprint.sh`, both untouched by this proposal).
- `examples/evidence-storage-demo/` must remain unchanged; the library's own
  example, if approved for creation during implementation, is a new,
  separate directory.
- All existing CLI subcommands (`init`, `validate`, `fingerprint`, `inspect`,
  `compare`, and all four `evidence` subcommands) must keep their exact
  current exit-code and output behavior; all existing tests in
  `cmd/incidentdna/cli_test.go` and `cmd/incidentdna/cli_evidence_test.go`
  must keep passing without modification.
- A document authored under Phase 1 or Phase 2 (i.e., with no awareness the
  library exists) must be usable with `library add`/`check` exactly as-is,
  subject only to the privacy gate (§10) — no re-authoring or schema
  migration required (see §17).

## 10. Security and privacy considerations

- **Same trust boundary as Phase 1 and Phase 2**: `incidentdna` runs with
  exactly the invoking user's OS-level filesystem permissions; no elevation,
  no service, no multi-tenant concept.
- **Fingerprint- and digest-keyed lookup, not path/filename/metadata-keyed**
  — the direct analog of the evidence store's path-traversal defense
  (`threat-model.md`, "Evidence store: local filesystem risks"), extended to
  the two-level (fingerprint, occurrence-digest) key space described in §5.
- **Privacy policy (approved, §20.2)**: `library add` requires
  `privacy.redacted == true` by default. The check can be bypassed only with
  an explicit `--allow-unredacted` flag, which always produces a visible
  warning — never a silent bypass. Documentation (`docs/incident-library.md`,
  `docs/privacy-model.md`) must state plainly that `privacy.redacted` is
  **author-declared**: setting it to `true` does not itself prove the
  document was actually reviewed or sanitized, only that the author asserted
  it was. This is a stricter default than any other Phase 1/2 operation
  (which all treat `redacted` as author-declared and never mandatory), and
  is deliberate: the library is the first place in this codebase that
  persists a full document indefinitely rather than processing it
  transiently.
- **`library` commands never print raw document contents.** `check` and
  `list` report only the bounded field set in §7; there is no code path in
  any `library` subcommand that dumps a stored or input document's full
  body to stdout/stderr.
- **No new network surface.** `library add`/`check`/`list` are pure local
  file I/O; the "no network access, no telemetry" invariant
  (`docs/threat-model.md`) is unconditional and applies here without
  exception.
- **Symlink and atomic-write handling** mirrors `internal/evidence`'s
  existing, already-reviewed design (reject symlinks at read paths, write
  via temp-file-then-rename under the library root) rather than re-deriving
  a new scheme.
- **Stored occurrences are not encrypted at rest**, consistent with
  `docs/privacy-model.md`'s existing stance for both IDIR documents and
  stored evidence, extended here to library entries.
- **No authenticity/signing claim.** As in Phase 2, an entry's presence in
  the library proves only internal self-consistency (it re-canonicalizes
  and re-digests to its own stored path) — never that its content is true,
  provenance-verified, or actually reviewed. This is why `privacy.redacted`
  is documented as author-declared rather than treated as a verified
  guarantee.

## 11. Resource limits (approved)

| Constant | Value | Applies to | Error | Tests |
|---|---|---|---|---|
| `MaxDocumentSize` | 5 MiB (reused from `internal/idir`, not redefined) | Any document read by `add`/`check`, via the existing `idir.LoadFile` path | existing `idir` size-limit error, reused unchanged | existing `idir` boundary tests, reused; no duplicate test needed |
| `MaxLibraryEntries` | 10,000 | Total distinct fingerprints (failure classes) the library will hold before `add` refuses to create a new fingerprint directory | dedicated, distinct error naming the limit | new passing test at 9,999→10,000 entries; new failing test at 10,000→refused 10,001st |
| `MaxOccurrencesPerFingerprint` | 100 | Occurrences stored under a single fingerprint directory before `add` refuses a genuinely-new occurrence (does not block an idempotent re-add) | dedicated, distinct error naming the limit | new passing test at 99→100 occurrences; new failing test at 100→refused 101st |
| `MaxListResults` | 1,000 | Number of fingerprint entries `library list` will enumerate/print in one invocation | dedicated, distinct error (or explicit truncation signal — exact UX is an implementation detail, but must be an actionable, non-silent signal) | new passing test at 999/1,000; new failing/truncation test above 1,000 |

Every `library` subcommand continues to run under `main.go`'s existing
30-second overall command timeout, unchanged — no new timeout mechanism.
Configurable limits are explicitly out of scope (§4) — all four constants
above are fixed, not user-configurable.

## 12. Failure modes

- **Library root does not exist yet**: `check`/`list` on a not-yet-created
  library report "nothing found"/"empty," not an error — same stance
  `evidence.Open` already takes for `verify`/`list`/`inspect` against a
  store that hasn't been created.
- **Library root exists but is not a directory** (a regular file, or a
  symlink to one): fail immediately with a clear error, same as
  `evidence.Open`'s existing behavior.
- **`add` with a new `incident.id` under an existing fingerprint**: stored
  as another occurrence (§5 case 1) — not a dedup no-op. This replaces the
  earlier draft's "first one wins" design.
- **`add` re-submitting the same `incident.id` with semantically identical
  content** (including formatting-only YAML/JSON differences — different key
  order, whitespace, or YAML-vs-JSON, all of which collapse to the same
  canonical bytes and therefore the same digest): idempotent, already-present
  result, no write (§5 case 2).
- **`add` re-submitting the same `incident.id` with materially different
  content** (different canonical bytes/digest): conflict — a distinct,
  actionable error; the existing occurrence is never overwritten, and a
  second occurrence is never silently created under the same `incident.id`
  (§5 case 3).
- **A stored occurrence is corrupted** (bytes at a digest-derived path don't
  re-canonicalize/re-digest to that same path, or `index.json` references an
  occurrence that doesn't exist / doesn't parse): reported as corrupted,
  distinct from both "not found" and "conflict." A corrupted entry is never
  reused, repaired, or silently overwritten as a side effect of an unrelated
  `add`/`check`/`list` call (§5 case 4).
- **`add`/`check` given a document that fails `validate`**: fails exactly
  like `evidence verify`/`evidence list` already do for an invalid document
  — validation is not bypassed or duplicated with different rules.
- **`add` given a document with `privacy.redacted != true` and no
  `--allow-unredacted`**: fails with a distinct, actionable privacy-policy
  error (§10) — this is checked after validation succeeds and before any
  fingerprint/digest/storage work happens.
- **Resource limit exceeded** (`MaxLibraryEntries`, `MaxOccurrencesPerFingerprint`,
  or `MaxListResults`): fails with the corresponding dedicated error from
  §11's table — never a generic or shared error message across the three
  limits.

## 13. Determinism requirements

- `library add`/`check`'s fingerprint computation must be byte-for-byte the
  same `internal/fingerprint.Compute` call every other command already uses
  — no separate or "library-specific" fingerprinting logic.
- **Corrected acceptance behavior (replacing the earlier draft's blanket
  "reworded documents are automatically idempotent" claim):**
  - A formatting-only same-`incident.id` copy (different whitespace, key
    order, or YAML vs. JSON serialization of otherwise-identical content) is
    idempotent, because canonicalization normalizes these away before the
    digest is computed.
  - A materially changed same-`incident.id` document (different free text,
    different invariants, different anything that survives canonicalization)
    is a **conflict**, not an idempotent overwrite.
  - A different-`incident.id` document sharing the same fingerprint is
    stored as **another occurrence**, not deduped away.
- `library list`'s output order must be deterministic (e.g., sorted by
  fingerprint hex string, then by occurrence digest within a fingerprint),
  not directory-iteration order, so repeated runs against an unchanged
  library produce byte-identical output.

## 14. Testing strategy

Following the exact style Phase 1 and Phase 2 already established
(table-driven, real filesystem via `t.TempDir()`, no mocking):

- **`internal/library` unit tests**: round-trip add/lookup; the three §5
  occurrence-decision cases (new occurrence, idempotent re-add including a
  differently-formatted/reworded copy, conflict on materially different
  content with the same `incident.id`) as an explicit table; rejecting a
  document that fails `validate`; rejecting a document that fails the
  privacy gate, and accepting it with `--allow-unredacted` plus a warning;
  a corrupted stored occurrence detected as such and never reused; passing
  and failing boundary tests for each of the four §11 limits independently;
  library-root resolution edge cases mirroring `evidence.Open`'s existing
  test coverage (non-existent root, root is a file, symlinked root).
- **`cmd/incidentdna/cli_library_test.go`**: black-box against the real
  compiled binary — `add` then `check` reports a match (exit 0); `check`
  against an unrelated document reports no match (exit 2); `check` against
  an invalid document reports exit 1; `add` twice with the same document is
  idempotent; `add` with the same `incident.id` and different content is a
  conflict (exit 1); `add` of a second `incident.id` under the same
  fingerprint produces two occurrences visible in `list`'s occurrence count;
  `list` shows expected entries in deterministic order and only the §7
  field set; `--allow-unredacted` warning text is present; resource-limit
  tests with a synthetic library at each of the four §11 capacities.
- **Regression discipline**: the full existing suite
  (`go test ./... -race -count=1`) must pass unmodified; the golden
  fingerprint test/script must produce the unchanged Phase 1 value.
- **New example, not a modification of existing ones**: a new
  `examples/incident-library-demo/` with at least one matching-fingerprint,
  different-`incident.id` pair (new-occurrence path), one exact re-add
  (idempotency path), and one non-matching document (check-miss path), so
  `add`/`check`/`list` can be demonstrated end-to-end in a future
  `docs/phase-3-report.md`.

## 15. Documentation changes

Proposed for the implementation phase (not written as part of this planning
task):

- New `docs/incident-library.md` — the dedicated design reference, playing
  the same role `docs/evidence-storage.md` plays for Phase 2: store layout
  (fingerprint directory → occurrence objects, §5), commands, the four §11
  resource limits, the §10 privacy policy (including the "author-declared,
  not verified" caveat, stated explicitly), security properties, an
  "integrity versus authenticity" analog for occurrences, and recovery
  guidance for a corrupted or conflicting entry.
- `README.md` — new "Phase 3: local incident library" section, CLI commands
  list, roadmap update (move "a shared incident library with storage and
  retrieval" from "Future work" into "Implemented (Phase 3)"),
  known-limitations update.
- `docs/architecture.md` — package layout table gains `internal/library`;
  dependency-direction paragraph gains it; a new data-flow diagram section
  analogous to "Evidence storage (Phase 2)."
- `docs/product-scope.md` — new "Phase 3 scope" section, and the "out of
  scope" list updated to keep everything not adopted (release gating,
  executable regression scenarios, OmniFlow, remote storage, signing,
  encryption, multi-tenancy, GC) explicitly named as still excluded.
- `docs/threat-model.md` — new section analogous to "Evidence store: local
  filesystem risks (Phase 2)," covering the library's path-safety, symlink,
  and resource-limit posture at both the fingerprint and occurrence level.
- `docs/privacy-model.md` — new subsection stating the §10 privacy policy
  precisely: `privacy.redacted` required by default, `--allow-unredacted`
  override with mandatory warning, and the explicit caveat that
  `privacy.redacted` is author-declared and does not itself prove review or
  sanitization occurred.
- A future `docs/phase-3-report.md`, written only after real implementation
  and verification — not now.

## 16. CI and verification strategy

Proposed for the implementation phase only — **the current `Makefile` and
`.github/workflows/ci.yml` are not modified by this planning task**:

- A new `make example-library` target, built the same way
  `example-evidence` was added in Phase 2: builds the binary, then runs the
  new example end-to-end against a temporary library root (never the
  project's own default root), analogous to `scripts/verify-evidence-demo.sh`
  — a proposed `scripts/verify-library-demo.sh`.
- `make verify` would gain `example-library` in its dependency chain,
  mirroring how it already gained `example-evidence`.
- `.github/workflows/ci.yml` would gain one new step ("Incident library demo
  validation," running `make example-library`), mirroring the existing
  "Evidence store demo validation" step.
- The golden-fingerprint check itself (`scripts/verify-golden-fingerprint.sh`)
  is untouched — the library does not change the fingerprint algorithm or
  the duplicate-payment example, so there is nothing for that script to
  regress against.

## 17. Migration and rollback considerations

- **Purely additive**: no existing file format, CLI command, or exit code
  changes. A user who never runs `incidentdna library ...` sees zero
  behavior difference from Phase 2's shipped state.
- **No migration needed for existing documents**: any IDIR document that
  already validates today (Phase 1 or Phase 2 vintage) can be `library add`ed
  without modification, subject to the §10 privacy gate.
- **Rollback is trivial**: since nothing in `internal/idir`, `internal/validate`,
  `internal/canonical`, `internal/fingerprint`, `internal/compare`, or
  `internal/evidence` would be touched, reverting Phase 3 entirely (deleting
  `internal/library`, `cmd/incidentdna/cmd_library.go`, the new example, and
  the doc updates) cannot regress any Phase 1/2 behavior, by construction.
- **No data-loss risk from adopting, then abandoning, the library**: a
  library root is just a directory of files, deletable with ordinary
  filesystem tools, exactly like the evidence store already is.
- **No `schema_version` bump.** IDIR remains v0.1; this phase does not touch
  the document format at all (see §8).

## 18. Small reviewable implementation slices

Proposed order, each independently mergeable and testable:

1. `internal/library/store.go` + `store_test.go` — root resolution
   (`Open`), fingerprint/digest-derived directory scheme (§5) — no CLI yet.
2. `internal/library/add.go` + `add_test.go` — the §5 occurrence-decision
   logic (new occurrence / idempotent / conflict / corrupted), including
   `index.go`'s read/write helpers.
3. `internal/library/check.go`, `list.go` + tests — read-side operations
   against the store built in slices 1–2.
4. `internal/library/privacy.go` + tests — the §10 privacy gate and
   `--allow-unredacted` warning path.
5. `internal/library/limits.go` + `errors.go` + `result.go` + tests — the
   four §11 resource limits and structured result/error types.
6. `cmd/incidentdna/cmd_library.go` + `cli_library_test.go` — CLI wiring for
   all three subcommands, `--library`/`--allow-unredacted` flags, exit codes
   from §7, black-box tests against the real binary.
7. `examples/incident-library-demo/` + `docs/incident-library.md` — the new
   example and its dedicated design doc.
8. `README.md`, `docs/architecture.md`, `docs/product-scope.md`,
   `docs/threat-model.md`, `docs/privacy-model.md` updates — documentation
   catch-up, done last so it describes the actually-implemented behavior
   rather than the plan.
9. `Makefile` + `.github/workflows/ci.yml` updates — `example-library`
   target and CI step, wired into `verify`.
10. `docs/phase-3-report.md` — written last, after real command transcripts
    exist to record.

Each slice should independently pass `make verify` (once slice 9 lands) or
`go test ./... -race -count=1` (for earlier slices before the Makefile
changes land), so no slice depends on a later one to be verifiable.

## 19. Acceptance criteria

1. `internal/library` implements the store, add, check, and list behavior
   described in §5–§7, each covered by table-driven tests.
2. All three `incidentdna library` subcommands work end-to-end against real
   documents (new occurrence, idempotent re-add, conflict, and no-match
   cases), with real command transcripts recorded in a future
   `docs/phase-3-report.md`.
3. Full existing suite still passes unmodified: `gofmt -l` clean, `go vet
   ./...` clean, `go test ./... -race -count=1` green, and the golden
   fingerprint value is byte-for-byte unchanged.
4. `internal/idir`, `internal/validate`, `internal/canonical`,
   `internal/fingerprint`, `internal/compare`, `internal/evidence` are
   untouched (or any unavoidable change is called out explicitly, not
   silent).
5. Neither a fingerprint-derived directory path nor an occurrence-digest
   path can escape the library root — proven the same way Phase 2 proved it
   for evidence digests.
6. Each of the four §11 resource limits is enforced by its own dedicated
   constant, produces a distinct clear error, and is covered by an
   independent passing/failing test pair.
7. A deliberately corrupted stored occurrence is reported as such, not
   silently accepted, reused, or overwritten.
8. **`library add` is idempotent only for a repeated or
   formatting-reformatted (not materially reworded) copy of an
   already-added document sharing the same `incident.id`.** A materially
   different document sharing that same `incident.id` produces a conflict,
   not an idempotent overwrite. A different `incident.id` sharing the same
   fingerprint produces a new stored occurrence, not a dedup no-op. (This
   replaces the earlier draft's blanket "reworded documents are
   automatically idempotent" claim — see §13.)
9. `examples/duplicate-payment/` and `examples/evidence-storage-demo/` are
   confirmed byte-for-byte unchanged by `git diff --stat`.
10. No network access, no telemetry introduced (`grep -rn '"net' cmd/
    internal/` stays empty, same check as both prior phase reports ran).
11. The new example contains no credentials, personal information, customer
    data, or real incident material — same synthetic-only discipline as
    both existing examples.
12. `library add` refuses documents with `privacy.redacted != true` unless
    `--allow-unredacted` is given, and that override always emits a
    warning; documentation states plainly that `privacy.redacted` is
    author-declared, not a verified guarantee.
13. `library list`'s default output contains only the §7 field set and
    never raw document contents, business invariants, event timelines,
    evidence contents, full remediation text, or the complete identity
    payload; no `--verbose` or JSON output mode exists in Phase 3.
14. All documentation updates listed in §15 are made, and no doc makes a
    now-false claim about what is/isn't implemented.
15. Nothing is committed, staged, or pushed during implementation, unless
    the user explicitly instructs otherwise at that time.

## 20. Approved decisions

The following decisions were explicitly reviewed and approved by the user,
replacing the prior draft's open-questions list. Implementation has not
started; these are the settled constraints implementation must follow.

1. **Phase 3 scope**: the local incident library, as described in §1–§9.
   Purpose: persist validated incident occurrences, group recurring
   incidents by the existing deterministic failure fingerprint, and provide
   an offline lookup capability. Explicitly excluded: release gates and CI
   blocking, executable regression scenarios, evidence signing or
   authenticity proof, remote or cloud storage, encryption, multi-tenancy,
   garbage collection, OmniFlow integration, network access, telemetry
   (§4).
2. **Privacy policy**: `incidentdna library add` requires
   `privacy.redacted == true` by default. `--allow-unredacted` permits an
   explicit override, which must always produce a warning. Documentation
   must state that `privacy.redacted` is author-declared and does not prove
   the document was actually reviewed or sanitized. `library` commands
   never print raw document contents (§10).
3. **Default library root**: `.incidentdna/library/objects`, under the same
   `.incidentdna` root as the Phase 2 evidence store. Custom root via
   `--library <path>` — not `--store`, which already names the evidence
   store's root flag (§7).
4. **CLI and exit codes**: `incidentdna library add`, `library check`,
   `library list` are approved. `library check` exits 0 for a match found,
   1 for invalid usage/invalid document/malformed store/permission/
   corruption/resource-limit/internal failure, and 2 for a valid document
   with no matching fingerprint (§7).
5. **Storage semantics**: the fingerprint identifies a failure class, not a
   unique incident occurrence. All distinct occurrences sharing a
   fingerprint are stored. Same fingerprint + different `incident.id` →
   another occurrence stored. Same fingerprint + same `incident.id` +
   semantically identical document → idempotent already-present result.
   Same fingerprint + same `incident.id` + materially different document →
   conflict, never silently overwritten. Formatting-only YAML/JSON
   differences are treated as semantically identical (they canonicalize to
   the same bytes). Existing corrupted entries are reported as corrupted,
   never reused or overwritten. The earlier "first one wins" design is
   rejected. Storage layout: a fingerprint-keyed directory containing
   occurrence objects addressed by the SHA-256 digest of each occurrence's
   own canonical document bytes — never by `incident.id` or any other
   untrusted string (§5).
6. **Resource limits**: `MaxDocumentSize` = 5 MiB (reused from `idir`),
   `MaxLibraryEntries` = 10,000, `MaxOccurrencesPerFingerprint` = 100,
   `MaxListResults` = 1,000. Each has a dedicated constant, a distinct
   actionable error, passing and failing boundary tests, and documentation
   (§11).
7. **List output**: `library list`'s default output includes only
   fingerprint, occurrence count, incident ID, title, application/service,
   and occurred timestamp. It must never print business invariants, event
   timelines, evidence contents, full remediation text, the complete
   identity payload, or raw stored document contents. No `--verbose` flag
   and no JSON output mode in Phase 3 (§7).
8. **Naming**: the approved public CLI noun is `library`
   (`incidentdna library add|check|list`) — not `catalog`, `archive`, or
   `incidents`.

Preserved from prior approval, unaffected by this decision set: Phase 1 and
Phase 2 compatibility requirements (§9) and the existing golden fingerprint
(`testdata/golden/duplicate-payment.fingerprint`, verified by
`scripts/verify-golden-fingerprint.sh`).
