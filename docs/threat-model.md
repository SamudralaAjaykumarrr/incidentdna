# Threat Model

Scope: the `incidentdna` CLI and the `internal/*` libraries it's built on,
as they exist at the end of Phase 5 — local file input, local file/stdout
output, a local content-addressed evidence store, a local incident library,
a local, bounded, offline regression-scenario runner, and a local,
sequential, offline scenario-suite runner. No network, no multi-user or
multi-tenant concerns. Phase 4 introduced one new class of risk this scope
statement did not previously need to cover: local process execution (see
"Executable regression scenarios: local process-execution risks (Phase 4)"
below) — every prior phase's operations were pure data/filesystem
transformations that never executed the content they processed. Phase 5
introduces no further new class of risk: `internal/suite` never calls
`exec.Command` itself; every process a suite launches is launched by the
unchanged Phase 4 runner (see "Scenario suites: aggregated local
process-execution risk (Phase 5)" below).

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

## Executable regression scenarios: local process-execution risks (Phase 4)

Phase 4 introduces local process execution for the first time in this
codebase, so its safety model is explicitly weaker in one dimension than
every prior phase's pure-data-and-filesystem operations — this section
states that plainly rather than implying a guarantee that isn't real:

- **Malicious or careless scenario authorship.** A scenario file is
  reviewable text, but nothing in Phase 4 prevents its author from
  declaring a `command` that is itself destructive, reaches the network, or
  reads unrelated local files. The safety model is *review before running*,
  not runtime sandboxing — a real, testable sandboxing mechanism
  (containers/VMs/seccomp) is explicitly out of scope for this phase. This
  is an accepted, fundamental limitation, not a gap to be closed later
  without one.
- **Workspace path-traversal via `workspace_files`.** Mitigated
  structurally: both `source` (resolved against the scenario file's own
  directory) and `destination` (resolved against the workspace root) are
  `filepath.Clean`ed and re-checked to have their respective root as a
  prefix before any read/write; symlinks at either path are rejected, not
  followed — the same defense-in-depth pattern `internal/library`'s
  `checkContained` already established, applied to declared relative paths
  instead of digest-derived ones.
- **Resource exhaustion via a runaway or malicious command.** Mitigated by
  `timeout_seconds` (hard-killing the process's whole process group at the
  bound) and per-stream output caps — this bounds *this tool's* exposure
  (hang forever, consume unbounded memory buffering output), not the
  reviewed command's own resource usage on the host, which remains bounded
  only by whatever OS-level limits (ulimits, cgroups) the invoking
  environment already applies outside `incidentdna`.
- **A scenario's `command[0]` being itself a shell or interpreter.** The
  runner refuses to *invoke* a shell itself (`exec.Command`, never `sh -c`),
  but cannot prevent a reviewed scenario's own declared `command[0]` from
  being `/bin/sh` (or `python3`, `perl`, etc., referenced by an explicit
  path) — stated explicitly as a limitation of the format rather than
  implied to be prevented: the "no arbitrary shell execution" design
  constraint is a property of the *runner's own code path*, and the
  reviewability requirement above is the actual control on what an author
  declares.
- **TOCTOU between `scenario verify`'s pre-execution checks and `scenario
  run`'s actual execution.** A `workspace_files` source file or
  `command[0]` could change or disappear between the check and the use —
  the same class of gap `docs/incident-library.md`'s "Filesystem and TOCTOU
  limitations" already accepts for the library, restated here for process
  execution rather than newly discovered.
- **Detached child processes escaping timeout enforcement.**
  `context`-based cancellation, combined with killing the child's whole
  process group (`Setpgid` + a negative-PID `SIGKILL`), reliably terminates
  the direct child and any children it spawned that remained in that
  process group; a grandchild that double-forks or otherwise detaches from
  the process group may survive past the timeout — documented as a known,
  accepted gap, consistent with this project's stance that OS-level
  guarantees beyond what Go's standard library provides are not
  independently re-implemented.
- **No network access, no telemetry, from the runner's own code** —
  restated as unconditional and verified the same way every prior phase
  verified it: `grep -rn '"net' cmd/ internal/` stays empty for everything
  except the intentional, already-reviewed absence of any such import in
  `internal/scenario` itself. This is a claim about the *runner's own Go
  code*, not about what a scenario's *executed command* might itself do —
  that remains entirely outside this tool's control (see the first bullet
  above).

**Integrity of the runner's own operations, not of the reviewed command.**
Every guarantee in this section is about what `internal/scenario`'s own
code does or refuses to do (shell out, resolve `$PATH` implicitly, write
outside its workspace, exceed a timeout or output cap unboundedly) — never
about what a scenario author's *reviewed command* itself does once it
starts running. See [`regression-scenarios.md`](regression-scenarios.md),
"A new class of risk" and "Execution isolation boundaries," for the full
design and the exact table of what is structural versus what is a
documented limitation.

## Executable regression scenario resource limits (Phase 4)

Mirroring the evidence store's and incident library's existing precedent,
eight fixed constants in `internal/scenario/limits.go` bound the runner's
exposure to a maliciously or accidentally oversized scenario document,
oversized staged fixtures, oversized captured output, or a runaway/hung
command: `MaxScenarioDocumentSize` (1 MiB), `MinScenarioTimeoutSeconds` (1),
`DefaultScenarioTimeoutSeconds` (30), `MaxScenarioTimeoutSeconds` (300),
`MaxWorkspaceFiles` (50), `MaxWorkspaceFileSize` (10 MiB),
`MaxWorkspaceTotalBytes` (50 MiB), and `MaxScenarioOutputBytes` (1 MiB per
stream). Each is enforced independently and produces a distinct, actionable
error naming the limit and the offending value. `scenario verify` runs
under the same 30-second overall command timeout every other subcommand
already has; `scenario run` deliberately does **not** — it uses its own
context derived from the scenario's own `timeout_seconds` (bounded by
`MaxScenarioTimeoutSeconds` above), since that timeout was sized for the
categorically different workload of executing an arbitrary bounded child
process, not parsing/hashing a document. See
[`regression-scenarios.md`](regression-scenarios.md), "Resource limits,"
for the full detail.

## Scenario suites: aggregated local process-execution risk (Phase 5)

Phase 5 introduces no new category of risk beyond what this document
already documents for Phase 4 above: every process a suite launches is
launched by `internal/scenario.Run`, unchanged, so every one of that
section's bullets (malicious/careless scenario authorship, workspace
path-traversal, resource exhaustion, `command[0]` being a shell, TOCTOU,
detached-process timeout evasion, no network/telemetry from the runner's
own code) applies identically, per listed scenario, whether that scenario
is run standalone via `scenario run` or as part of a suite via `suite run`.
`internal/suite` is the first package in this codebase that never calls
`exec.Command` at all, directly or indirectly through a new code path.

The one genuinely new consideration:

- **A malicious or careless suite manifest can name scenario files whose
  content a reviewer of the manifest alone has not necessarily read.**
  Mitigated the same way `--source` cross-checking is mitigated in Phase 4:
  nothing about suite membership hides or mutates a scenario file's own
  content — `suite verify` prints every listed scenario's path and its own
  validation result, so a reviewer approving a suite manifest is explicitly
  shown which scenario files it will run, in what order, before ever
  running `suite run`. As with every other "review before running" control
  in this project, IncidentDNA does not verify that a human actually read
  each one — it makes doing so straightforward and the alternative (running
  an unreviewed suite) an explicit, visible choice.
- **Suite manifest path-traversal via `scenarios[].path`.** Mitigated
  structurally, the identical `checkContained`-style pattern
  `workspace_files` already established: each declared path is
  `filepath.Clean`ed and re-checked to have the suite manifest's own
  directory as a prefix before `scenario.LoadFile` is ever called on it;
  a symlink at that path is rejected, not followed.
- **A suite listing one invalid scenario does not silently skip it.**
  `suite verify`/`suite run` reject the entire suite manifest — nothing
  executes — if any one listed scenario fails `scenario.Validate`, so a
  reviewer's approval of "this suite is valid" always means "every scenario
  in it is valid," never "most of them are." See
  [`scenario-suites.md`](scenario-suites.md), "Corruption and
  malformed-manifest handling."

## Scenario suite resource limits (Phase 5)

Mirroring the evidence store's, incident library's, and regression
scenario runner's existing precedent, three fixed constants in
`internal/suite/limits.go` bound the suite runner's exposure to a
maliciously or accidentally oversized suite manifest, an excessive number
of listed scenarios, or an excessive aggregate declared timeout:
`MaxSuiteDocumentSize` (256 KiB), `MaxScenariosPerSuite` (100), and
`MaxSuiteTotalTimeoutSeconds` (1800 seconds/30 minutes, the sum of every
listed scenario's declared/default `timeout_seconds`, checked at verify
time before any scenario ever runs). Each is enforced independently and
produces a distinct, actionable error naming the limit and the offending
value. Every per-scenario limit from `internal/scenario/limits.go`
(document size, workspace file count/size/aggregate size, output capture
size, and timeout bounds) is inherited unchanged, per listed scenario.
`suite verify` runs under the same 30-second overall command timeout every
other subcommand already has; `suite run` deliberately does **not** — it
uses its own context bounded by `MaxSuiteTotalTimeoutSeconds`, the same
"categorically different workload" reasoning already stated for `scenario
run`. See [`scenario-suites.md`](scenario-suites.md), "Resource limits,"
for the full detail.

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

`incidentdna scenario verify`/`run` similarly open exactly the scenario
file path given directly on the command line, and (for `verify --source`)
the incident file path given directly on the command line. Every workspace
path is derived from a `workspace_files` entry's own declared `source`/
`destination`, each re-checked to resolve within its respective root — see
"Executable regression scenarios: local process-execution risks (Phase 4)"
above. The one deliberate exception to "never reads/writes anywhere the
invoking user didn't explicitly authorize" is `execution.command` itself:
once a reviewed scenario's declared command starts, what *it* reads or
writes is bounded only by the invoking user's own OS-level permissions, not
by `incidentdna` — this is the documented "bounded, not sandboxed" limit,
not a path-traversal gap in the runner's own code.

`incidentdna suite verify`/`run` similarly open exactly the suite manifest
file path given directly on the command line. Every listed scenario's
filesystem path is derived from a `scenarios[].path` entry's own declared
value, re-checked to resolve within the suite manifest's own directory —
see "Scenario suites: aggregated local process-execution risk (Phase 5)"
above. Once `suite run` hands off to `scenario.Run` for one listed
scenario, the same "bounded, not sandboxed" limit stated above applies
identically — nothing about running from inside a suite changes what an
already-reviewed scenario's own declared command can read or write.

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

## Future multi-tenant risks (explicitly out of scope through Phase 5)

Phase 3's incident library, Phase 4's regression-scenario runner, and Phase
5's scenario-suite runner are **local and single-user, not shared or
multi-tenant** — the same trust boundary as the evidence store before them:
no concept of a tenant, user account, or access-control layer of its own.
Every invocation, including every `library`, `evidence`, `scenario`, and
`suite` subcommand, operates on files (and, for `library`/`evidence`, a
store) the invoking user already has filesystem access to. Risks that
become relevant only if a shared/remote/multi-tenant incident library,
evidence store, or scenario/suite execution service is built in a later
phase — none of this exists today, and Phase 5 explicitly does not build it
(see [`product-scope.md`](product-scope.md)):

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
- Any notion of a shared, remote, or multi-tenant *execution* service for
  regression scenarios or scenario suites — `incidentdna scenario run` and
  `incidentdna suite run` execute locally, sequentially, under the invoking
  user's own OS-level permissions; a hypothetical remote runner would face
  an entirely different, much larger threat surface (untrusted-command
  execution as a service) that this phase does not attempt to address.

None of these are mitigated today because none of the underlying
capabilities (network service, multi-user storage, multiple callers) exist
yet in this codebase.
