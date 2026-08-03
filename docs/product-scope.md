# Product Scope

## What IncidentDNA is

IncidentDNA converts evidence from a production incident into a permanent,
executable regression scenario. Every future release can run the historical
incident library and block software that reintroduces a previously observed
failure. The unit of record is **IDIR** — the Incident Deterministic
Intermediate Representation — a vendor-neutral document format described in
[`idir-specification-v0.1.md`](idir-specification-v0.1.md).

## Phase 1 scope

Phase 1 builds the foundation only: a way to **represent, validate, hash,
and compare** incidents. Concretely:

- The IDIR v0.1 document format and its Go type definitions.
- Semantic validation (`internal/validate`) that rejects incoherent or unsafe
  documents.
- Deterministic canonicalization and fingerprinting (`internal/canonical`,
  `internal/fingerprint`) so two documents describing the same underlying
  failure pattern hash identically regardless of formatting or record-level
  metadata.
- Comparison (`internal/compare`) that explains *why* two documents differ.
- A CLI (`incidentdna`) exposing `init`, `validate`, `fingerprint`,
  `inspect`, and `compare`.
- A complete synthetic example: duplicate payment after message
  redelivery (`examples/duplicate-payment/`).
- A containerized development workflow (Docker + Makefile) and CI.

## Phase 2 scope

Phase 2 builds directly on Phase 1's foundation, without changing it: a
local, content-addressable **evidence store** and digest verification, so
`evidence[].digest` (defined and format-validated since Phase 1) can be
checked against real stored bytes. Concretely:

- A local filesystem evidence store, keyed by an object's own SHA-256
  digest (`internal/evidence`), default root
  `.incidentdna/evidence/objects`.
- Four CLI subcommands: `incidentdna evidence store`, `verify`, `list`,
  `inspect`.
- Path-traversal and symlink protections, atomic deduplicating writes, and
  three fixed resource limits (max object size, max evidence entries per
  command, max aggregate verification bytes per command).
- A second, fully fictional example (`examples/evidence-storage-demo/`)
  demonstrating the four commands end-to-end, entirely separate from
  `examples/duplicate-payment/`.

Full design in [`evidence-storage.md`](evidence-storage.md). Phase 2 does
not change `idir.Document`, the JSON Schema, `internal/validate`'s rules,
`internal/canonical`, or `internal/fingerprint` — the fingerprint payload
already excluded evidence entirely in Phase 1, and continues to.
Verifying a stored object's integrity is not the same as proving the
evidence is truthful or establishing who produced it — see
[`evidence-storage.md`](evidence-storage.md), "Integrity versus
authenticity," for that distinction.

## Phase 3 scope

Phase 3 builds directly on Phase 1 and Phase 2, without changing either: a
local, filesystem-backed **incident library** that persists validated
incident occurrences and groups recurring occurrences by the existing
deterministic failure fingerprint (`internal/fingerprint.Compute`,
unchanged), plus an offline lookup capability. This closes the gap named in
`README.md`'s prior "Future work" list — *"a shared incident library with
storage and retrieval"* — as a **local**, single-user library; a
shared/remote library remains future work (see "Explicitly out of scope for
Phase 1 through Phase 3" below). Concretely:

- A local filesystem incident library (`internal/library`), keyed at two
  levels: a fingerprint-derived directory per failure class, and within it,
  occurrence objects keyed by the SHA-256 digest of each occurrence's own
  canonical document bytes. Default root
  `.incidentdna/library/objects`, under the same `.incidentdna` root the
  Phase 2 evidence store already uses.
- Three CLI subcommands: `incidentdna library add`, `check`, `list`.
- Occurrence semantics: a new `incident.id` under an existing fingerprint is
  stored as another occurrence (a fingerprint identifies a failure *class*,
  not a unique occurrence); an exact or formatting-only repeat of an
  already-stored `incident.id` is idempotent; a same-`incident.id` document
  with materially different content is refused as a conflict, never
  silently overwritten; a corrupted stored occurrence is reported as such,
  never reused or repaired automatically.
- An author-declared privacy gate on `add`: `privacy.redacted == true`
  required by default, overridable only via an explicit
  `--allow-unredacted` flag that always emits a warning.
- Path-traversal and symlink protections, atomic writes, and four fixed
  resource limits: `MaxDocumentSize` (5 MiB, reused from `internal/idir`),
  `MaxLibraryEntries` (10,000 fingerprints), `MaxOccurrencesPerFingerprint`
  (100 occurrences), `MaxListResults` (1,000 entries per `list` call).
- A third, fully fictional example (`examples/incident-library-demo/`)
  demonstrating the three commands end-to-end, entirely separate from
  `examples/duplicate-payment/` and `examples/evidence-storage-demo/`.

Full design in [`incident-library.md`](incident-library.md). Phase 3 does
not change `idir.Document`, the JSON Schema, `internal/validate`'s rules,
`internal/canonical`, `internal/fingerprint`, `internal/compare`, or
`internal/evidence` — the library operates strictly on already-valid
documents and their already-computed fingerprints. As with evidence
integrity in Phase 2, a library occurrence's integrity check proves internal
self-consistency, not that the incident is truthful or who added it — see
[`incident-library.md`](incident-library.md), "Integrity versus
authenticity." The incident library is not an authorization or trust
system: it has no user accounts, no access control, and no concept of an
incident being "approved."

## Phase 4 scope

Phase 4 builds directly on Phases 1 through 3, without changing any of
them: a small, reviewable, versioned document format — a **regression
scenario** (IRS v0.1, "Incident Regression Scenario") — and a local,
bounded, offline **runner** that executes the one command a scenario
declares and classifies the result. This closes the gap this document
previously named as explicitly out of scope through Phase 3 — *"executable
regression scenarios"* — as a **local**, single-command-per-invocation
runner with no sandboxing and no release-gate wiring; a suite runner,
sandboxed execution, and CI/CD integration remain future work (see
"Explicitly out of scope for Phase 1 through Phase 4" below). Concretely:

- A new, dedicated document format (`internal/scenario`), loaded through
  its own size-capped loader — deliberately not an extension of
  `idir.Document` and not decoded through `internal/idir`, since a
  scenario describes an *execution*, not an *incident*.
- Two CLI subcommands: `incidentdna scenario verify`, `run`.
- A scenario declares which failure class it regression-tests via a
  `linked_fingerprint` field, mechanically checkable against a real
  incident file with `scenario verify --source` — reusing
  `internal/fingerprint.Compute` unchanged, never looking it up against
  `internal/library`.
- `scenario run` executes exactly one declared local command, with no
  shell interpretation and no implicit `$PATH` lookup, inside a freshly
  created, bounded, non-persistent workspace, under a timeout and output
  cap, and classifies the result into one of five outcomes: `PASS`,
  `FAIL`, `TIMEOUT`, `INVALID`, `INTERNAL_ERROR`.
- A machine-readable, deterministic JSON execution report (`--report`),
  alongside the human-readable stdout summary every other subcommand
  already produces.
- Eight fixed resource limits (scenario document size, workspace file
  count/size/aggregate size, output capture size, and timeout bounds) and
  the path-traversal/symlink protections `internal/evidence` and
  `internal/library` already established, extended to declared
  `workspace_files` entries.
- A fourth, fully fictional example (`examples/regression-scenario-demo/`)
  demonstrating all five outcomes end-to-end, entirely separate from
  `examples/duplicate-payment/`, `examples/evidence-storage-demo/`, and
  `examples/incident-library-demo/`.

Full design in [`regression-scenarios.md`](regression-scenarios.md). Phase
4 does not change `idir.Document`, the JSON Schema, `internal/validate`'s
rules, `internal/canonical`, `internal/fingerprint`, `internal/compare`,
`internal/evidence`, or `internal/library` — the scenario runner operates
strictly on a new, separate document format and, for the optional
`--source` check, an already-valid IDIR document's already-computed
fingerprint. Phase 4 introduces local process execution for the first
time in this codebase; the runner bounds its *own* behavior (workspace
containment, timeout, output capture) but does not sandbox the process it
launches — see [`regression-scenarios.md`](regression-scenarios.md), "A
new class of risk," for the full distinction between "bounded" and
"sandboxed."

## Phase 5 scope

Phase 5 builds directly on Phases 1 through 4, without changing any of
them: a small, reviewable, versioned manifest format — a **scenario suite**
(ISM v0.1, "Incident Scenario Manifest") — and a local, sequential, offline
**runner** that executes every scenario a manifest explicitly lists, once
each, via the unchanged Phase 4 runner (`internal/scenario.Run`), and
aggregates the five-outcome classification each scenario already produces
into one suite-level result. This closes the gap this document previously
named as explicitly out of scope through Phase 4 — a regression-scenario
**suite runner** — as a **local**, sequential, aggregation-only runner with
no directory/glob discovery, no parallel execution, and no automatic
coupling to the incident library; those remain future work (see
"Explicitly out of scope for Phase 1 through Phase 5" below). Concretely:

- A new, dedicated document format (`internal/suite`), loaded through its
  own size-capped loader — deliberately not an extension of
  `scenario.Document` and not decoded through `internal/scenario`, since a
  suite manifest describes an ordered *list* of scenarios, not an
  execution.
- Two CLI subcommands: `incidentdna suite verify`, `run`.
- A suite manifest lists scenario files explicitly, by declared relative
  path — never a directory walk or glob — mirroring the same
  "nothing is resolved implicitly, only what a reviewer can see in the
  file" discipline `execution.workspace_files` already established in IRS
  v0.1.
- `suite run` executes every listed scenario exactly once, sequentially, in
  declared order, by calling `internal/scenario.Run` unchanged — no new
  process-execution primitive, no parallelism — and aggregates the
  outcomes into one suite-level `PASS` (every listed scenario PASSed) or
  `FAIL` (otherwise) result, a pure function of the per-scenario outcomes.
- `--fail-fast` stops running further scenarios immediately after the first
  non-`PASS` outcome, recording every scenario never reached as `SKIPPED`
  rather than silently omitting it.
- A machine-readable, deterministic JSON aggregate report (`--report`),
  embedding each executed scenario's own unmodified `scenario.Report`,
  alongside the human-readable stdout summary every other subcommand
  already produces.
- Three fixed resource limits (suite manifest document size, maximum
  scenarios per suite, and a maximum aggregate suite timeout bound) and the
  path-traversal/symlink protections `internal/evidence`, `internal/library`,
  and `internal/scenario` already established, extended to declared
  `scenarios[].path` entries.
- A fifth, fully fictional example (`examples/regression-suite-demo/`)
  demonstrating both aggregate outcomes end-to-end, entirely separate from
  `examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
  `examples/incident-library-demo/`, and
  `examples/regression-scenario-demo/`.

Full design in [`scenario-suites.md`](scenario-suites.md). Phase 5 does not
change `idir.Document`, the JSON Schema, `internal/validate`'s rules,
`internal/canonical`, `internal/fingerprint`, `internal/compare`,
`internal/evidence`, `internal/library`, or `internal/scenario` — the suite
runner operates strictly on a new, separate manifest format and, for every
listed scenario, the unchanged Phase 4 loading/validation/execution
pipeline. Phase 5 introduces no new class of risk: `internal/suite` is the
first package in this codebase that never calls `exec.Command` itself,
directly or indirectly — every process a suite launches is launched by
`internal/scenario.Run`, unchanged — see
[`scenario-suites.md`](scenario-suites.md), "No new class of risk."

## Phase 6 scope

Phase 6 builds directly on Phases 1 through 5, without changing any of
them except one small, additive change to `internal/library`: a read-only,
purely informational **cross-reference** between a scenario's (or a
suite's) declared `linked_fingerprint` and the incident library's stored
occurrences. This closes the gap this document previously named as
explicitly out of scope through Phase 5 — *"automatic coupling between a
scenario's (or a suite's) `linked_fingerprint` and the incident library's
stored occurrences"* — as a **read-only, verify-only, non-gating** lookup;
release gating, sandboxed execution, remote storage, signing, and every
other item still named below remain future work. Concretely:

- One new exported function, `internal/library.CheckFingerprint`, and one
  new sentinel error, `ErrInvalidFingerprint` — the only change to
  `internal/library`, and the only package with any code change at all.
  `internal/scenario` and `internal/suite` are not modified and gain no new
  import.
- An optional `--library <dir>` flag on `incidentdna scenario verify`: after
  the existing structural/semantic (and optional `--source`) checks pass,
  look up the scenario's own `linked_fingerprint` against the named (or
  default) incident library and print whether it has recorded occurrences.
- An optional `--library <dir>` flag on `incidentdna suite verify`: after
  the existing checks pass, look up every *distinct* `linked_fingerprint`
  among the listed scenarios (first-occurrence order), annotate each
  scenario's own summary line, and print a one-line aggregate count.
- Strictly informational: a "no occurrences found" result never changes
  either command's existing 0/2 exit-code meaning; `scenario run` and `suite
  run` are entirely unchanged — no new flag, no new exit-code cause, no new
  report field.
- No new document field anywhere, no mutation of the library, and no new
  resource limit — bounded entirely by the existing
  `suite.MaxScenariosPerSuite` (100) and `internal/library`'s own limits,
  reused unchanged.

Full design in [`library-crossref.md`](library-crossref.md). Phase 6 does
not change `idir.Document`, the JSON Schema, `internal/validate`'s rules,
`internal/canonical`, `internal/fingerprint`, `internal/compare`,
`internal/evidence`, `internal/scenario`, or `internal/suite` — the
cross-reference composition lives entirely in `cmd/incidentdna`, which
already imported all three packages (`library`, `scenario`, `suite`)
before Phase 6.

## Phase 7 scope

Phase 7 builds directly on Phases 1 through 6, without changing any of
them: a small, reviewable, versioned policy document format — **IGP v0.1**
("Incident Gate Policy") — and a local, deterministic **evaluator**
(`incidentdna policy verify`, `evaluate`) that checks an already-produced
`scenario run --report`/`suite run --report` JSON artifact (Phase 4/5,
unchanged) against a declared policy and reports a `PASS`/`FAIL` verdict
with a meaningful exit code.

This closes the gap this document previously named as explicitly out of
scope through Phase 6 — a declared, reviewable statement of "what counts as
acceptable," evaluated automatically against an already-produced result —
**by deliberately splitting "release gating" into two distinct things**:

1. **Release-gate / CI-CD *integration*** — IncidentDNA itself calling out
   to, authenticating against, or blocking merges within some specific
   external CI/CD platform. **This remains fully out of scope.**
2. **Local, deterministic *policy evaluation* over already-produced
   results** — this is the Phase 7 increment. `policy evaluate` reads two
   local files (a policy document and an already-produced report artifact)
   and an optional fresh library lookup, prints a verdict, and sets an exit
   code — which some external system may then use to implement its own
   gate, exactly the same "makes results available to be consumed by
   something else, but IncidentDNA itself does not consume it into an
   external system" boundary already accepted for `scenario run`'s and
   `suite run`'s own exit codes.

Concretely:

- A new, dedicated policy document format (`internal/policy`, a new sixth
  parallel leaf package), loaded through its own size-capped loader —
  deliberately not an extension of `scenario.Document`/`suite.Document` and
  not decoded through either package.
- Two CLI subcommands: `incidentdna policy verify`, `evaluate`.
- Exactly two rule types in v0.1: `require_result` (the input report's own
  top-level `result` field must equal a declared value) and
  `require_library_occurrence` (every distinct `linked_fingerprint` named
  in the report must have at least one recorded incident-library
  occurrence, checked via the unchanged Phase 6 `library.CheckFingerprint`).
- `internal/policy` never calls `exec.Command`, never calls
  `scenario.Run`/`suite.Run`, and does not parse a scenario or suite
  *document* at all — it only parses an already-produced report *artifact*.
- `internal/policy` never imports `internal/library` and does not know the
  incident library exists — the `require_library_occurrence` rule is
  evaluated against a caller-supplied lookup result; the composition that
  opens the library and calls `library.CheckFingerprint` lives entirely in
  `cmd/incidentdna`, the identical discipline Phase 6 established.
- Strictly local and non-integrating: a `PASS`/`FAIL` verdict and a
  deterministic JSON verdict report (`--report`) are the only outputs;
  `policy evaluate` never calls a webhook, a status-check API, or any other
  external system.
- Four fixed resource limits (`MaxPolicyDocumentSize`,
  `MaxRulesPerPolicy`, `MaxReportDocumentSize`,
  `MaxDistinctFingerprintsPerEvaluation`) and no new resource limit on
  `internal/scenario`, `internal/suite`, or `internal/library`.
- One new checked-in example policy, `policies/release-gate-example.yaml`,
  used by the demo script — not a new `examples/` directory, since Phase 7
  introduces no new fixture content, only a small, reviewable config-shaped
  file.

Full design in [`policy-evaluation.md`](policy-evaluation.md). Phase 7 does
not change `idir.Document`, the JSON Schema, `internal/validate`'s rules,
`internal/canonical`, `internal/fingerprint`, `internal/compare`,
`internal/evidence`, `internal/library`, `internal/scenario`, or
`internal/suite` — the policy evaluator operates strictly on a new, separate
document format and, for the report artifact it evaluates, Phase 4/5's
already-existing, unchanged `Report` struct definitions.

## Explicitly out of scope for Phase 1 through Phase 7

This codebase, through the end of Phase 7, contains none of:

- React, FastAPI, or any web/API framework.
- Kubernetes or any cloud infrastructure.
- Kafka integration or any live event ingestion.
- OpenTelemetry ingestion or any observability pipeline integration.
- AI or LLM calls of any kind.
- SaaS authentication, billing, or customer management.
- Real fault injection against a running system.
- Any changes to, or reuse of, the separate OmniFlow repository.
- Release gating, release-gate integration, or CI/CD blocking of any kind —
  `incidentdna library check` is an offline lookup command, not a gate, and
  `incidentdna scenario run`'s, `incidentdna suite run`'s, and `incidentdna
  policy evaluate`'s exit codes/JSON reports are available to be consumed by
  something else but are not wired into any gate by this codebase. This is
  the explicit split Phase 7 made and restates rather than resolves: "local,
  deterministic policy evaluation over already-produced results" (built,
  see "Phase 7 scope" above) is a distinct thing from "IncidentDNA itself
  calling out to, authenticating against, or blocking merges within some
  specific external CI/CD platform" (still fully out of scope).
- **More than two policy rule types.** IGP v0.1 supports exactly
  `require_result` and `require_library_occurrence`; arbitrary boolean
  combinators, numeric thresholds on counts, and per-scenario (as opposed to
  per-report) rules are not built.
- **`policy evaluate` executing anything.** It reads an already-produced
  report file; it never itself invokes `scenario.Run`/`suite.Run` — an
  operator must run `scenario run --report`/`suite run --report` themselves
  first, as an explicit prior step.
- **Batch policy evaluation.** `policy evaluate` takes exactly one policy
  file and exactly one report file per invocation.
- **Directory-walk or glob-based scenario discovery**: `incidentdna suite`
  lists scenarios explicitly, by declared relative path; `incidentdna`
  itself never walks a directory tree looking for scenario files.
- **Parallel suite execution**: scenarios in a suite run strictly
  sequentially, in declared order — `incidentdna suite run` never runs two
  scenarios concurrently.
- **Sandboxed** scenario execution: `incidentdna scenario run` (called by
  `incidentdna suite run` once per listed scenario, unchanged) bounds
  argv/env/cwd/timeout/output for the process it launches, but does not
  isolate it with seccomp, cgroups, a container, or a VM — the reviewed
  command runs with the full OS-level permissions of the invoking user.
- **Release-gate use of the Phase 6 library cross-reference.** Phase 6 (see
  "Phase 6 scope" above) added an optional, read-only, informational
  `--library` lookup to `scenario verify`/`suite verify` — but neither
  `internal/scenario` nor `internal/suite` imports or queries
  `internal/library` themselves (the composition is entirely a
  `cmd/incidentdna`-layer concern), and nothing in this codebase turns a
  "no occurrences found" result into a blocking condition for `scenario
  run`, `suite run`, or any external CI/CD gate.
- Ingestion from observability/event systems — `library add` (like
  `evidence store` before it) takes a local file path given directly on the
  command line, never an ingested event; the same is true of `scenario
  verify`/`run` and `suite verify`/`run`, which take a scenario or suite
  manifest file path given directly on the command line.
- Remote/cloud storage for the evidence store, the incident library,
  regression scenarios, or scenario suites.
- Encryption at rest, for evidence, for library occurrences, for scenario
  or suite documents/reports, or for IDIR documents generally.
- Evidence or incident-library signing or authenticity proof.
- Multi-tenancy, or any multi-tenant/shared-store access control, for
  either the evidence store or the incident library.
- Garbage collection of orphaned evidence objects, or any
  retention/expiry/"remove" capability for library occurrences — an
  incident library is, by the product's own premise, meant to be a
  permanent record.
- Automatic scenario generation using AI — a scenario document is
  hand-authored; `incidentdna` only validates and runs one, never invents
  one.

These are all real future needs (see [`phase-1-report.md`](phase-1-report.md),
"Recommended Phase 2 scope", [`phase-2-plan.md`](phase-2-plan.md) §16,
[`phase-3-plan.md`](phase-3-plan.md) §4, [`phase-4-plan.md`](phase-4-plan.md)
§3/§27/§28, [`phase-5-plan.md`](phase-5-plan.md) §3/§25/§26, and
[`phase-7-plan.md`](phase-7-plan.md) §22), but each phase's job is to get
its own layer's guarantees right — Phase 1 the document representation,
Phase 2 local evidence integrity, Phase 3 local occurrence storage and
lookup, Phase 4 local, bounded, reviewed-command execution, Phase 5 local,
sequential, aggregated execution of many already-reviewed scenarios, Phase
6 read-only cross-reference between execution results and the incident
library, Phase 7 local, deterministic policy evaluation over an
already-produced result — before anything further is built on top.

## Why this order

A regression-scenario library is only trustworthy if two independently
authored incident records that describe the same failure are recognized as
the same, and if a record that has been tampered with or is structurally
incoherent is rejected before it ever reaches a release gate. Both of those
properties are pure data-representation problems, independent of how
incidents are ingested or how the library is eventually served — hence
building this layer first, deliberately without any transport, storage, or
UI dependency to get it wrong against.
