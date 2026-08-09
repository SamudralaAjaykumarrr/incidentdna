# IncidentDNA

**Production Failure Memory** — IncidentDNA converts a production incident
into a permanent, executable regression scenario, so future releases can be
tested against failures that have already happened instead of relying on
whoever remembers the postmortem.

[![CI](https://github.com/SamudralaAjaykumarrr/incidentdna/actions/workflows/ci.yml/badge.svg)](https://github.com/SamudralaAjaykumarrr/incidentdna/actions/workflows/ci.yml)

## The problem

Production incidents produce postmortems, tickets, and tribal knowledge —
but rarely a durable, machine-checkable artifact. The specific causal
sequence that produced a duplicate charge, a data race, or a cascading
timeout is easy to describe in prose and easy to reintroduce eighteen months
later, once the engineers who lived through it have moved on or forgotten
the details. Nothing about "we fixed this in March" is checked automatically
against "did we just reintroduce it in November."

## Why incidents should become permanent regression scenarios

If an incident is captured as a structured, versionable record instead of a
paragraph in a wiki, it can be treated the same way as any other regression
test: run automatically, compared deterministically, and never silently
forgotten. Two independently authored records that describe the same
underlying failure — different app, different release, reworded
descriptions — should be recognized as *the same failure class*. A record
that has been tampered with, or is internally incoherent (a dangling
reference, a causal cycle, an unredacted field that claims to be redacted),
should be rejected before it can ever be trusted as a regression gate. Both
of these are pure data-representation problems, which is why IncidentDNA
starts there.

## Phase 1: the foundation

This repository is Phase 1 of IncidentDNA. It builds *only* the
vendor-neutral foundation — a format for representing incidents and a CLI to
work with them — deliberately before any storage, transport, UI, or release
integration is built on top of it. See
[`docs/product-scope.md`](docs/product-scope.md) for the full scope
statement, including what is explicitly out of scope for this phase.

What Phase 1 implements:

- **IDIR v0.1** — a JSON/YAML document format for representing one incident
  as a permanent, executable regression scenario
  ([`docs/idir-specification-v0.1.md`](docs/idir-specification-v0.1.md)).
- **Semantic validation** — rejects incoherent or unsafe documents: causal
  cycles, dangling event/evidence references, malformed evidence digests,
  incomplete redaction, and more.
- **Deterministic canonicalization and fingerprinting** — two documents
  describing the same underlying failure pattern hash identically,
  regardless of formatting, record identity, or which application/release
  they were observed in.
- **Comparison** — explains *why* two documents differ, dimension by
  dimension, when their fingerprints don't match.
- **A CLI** (`incidentdna`) exposing `init`, `validate`, `fingerprint`,
  `inspect`, and `compare`.
- **A complete synthetic example** — duplicate payment after message
  redelivery — used by tests, `make example`, and the golden fingerprint
  check.
- **A containerized development workflow** (Docker + Makefile) and CI, so
  the toolchain is identical for every contributor and for CI itself.

## Phase 2: evidence storage and digest verification

Phase 2 adds a local, content-addressable evidence store and four
`incidentdna evidence` subcommands (`store`, `verify`, `list`, `inspect`) so
an `evidence[].digest` declared in an IDIR document can be checked against
real stored bytes, not just format-validated. See
[`docs/evidence-storage.md`](docs/evidence-storage.md) for the full design —
store layout, resource limits, path-traversal/symlink protections, and what
is explicitly *not* covered (no encryption at rest, no signing/authenticity
proof, no remote storage). A separate, fully fictional example,
[`examples/evidence-storage-demo/`](examples/evidence-storage-demo/), exists
to demonstrate the four commands end-to-end without touching
`examples/duplicate-payment/` or its golden fingerprint.

## Phase 3: local incident library

Phase 3 adds a local, filesystem-backed library of validated incident
*occurrences*, grouped by the existing deterministic failure fingerprint
(`internal/fingerprint.Compute`, unchanged), plus three `incidentdna library`
subcommands (`add`, `check`, `list`) to write to and read from it. The
fingerprint identifies a *failure class*, not a unique occurrence, so the
library stores multiple distinct occurrences under one fingerprint rather
than deduplicating to a single entry: a new `incident.id` under an existing
fingerprint is stored as another occurrence, a repeated (or
formatting-reformatted) `incident.id` is idempotent, and a same-`incident.id`
document with materially different content is refused as a conflict. See
[`docs/incident-library.md`](docs/incident-library.md) for the full design —
store layout, resource limits, the privacy gate, path-traversal/symlink
protections, and what is explicitly *not* covered (no encryption at rest, no
signing/authenticity proof, no remote storage, no release gating, no
executable regression scenarios). A third, fully fictional example,
[`examples/incident-library-demo/`](examples/incident-library-demo/), exists
to demonstrate `add`/`check`/`list` end-to-end without touching
`examples/duplicate-payment/` or `examples/evidence-storage-demo/`.

## Phase 4: deterministic executable regression scenarios

Phase 4 adds a small, reviewable, versioned document format — a **regression
scenario** (IRS v0.1) — and a local, bounded, offline runner
(`incidentdna scenario verify|run`) that executes the one command a scenario
declares and classifies the result as `PASS`, `FAIL`, `TIMEOUT`, `INVALID`,
or `INTERNAL_ERROR`. A scenario declares which failure class it
regression-tests by referencing an existing IDIR fingerprint
(`internal/fingerprint.Compute`, unchanged), optionally checked against a
real incident file with `scenario verify --source`. The runner never invokes
a shell and never performs an implicit `$PATH` lookup, runs the declared
command inside a freshly created, bounded, non-persistent workspace under a
timeout and output cap, and writes a deterministic JSON execution report on
request. See [`docs/regression-scenarios.md`](docs/regression-scenarios.md)
for the full design — the scenario document format, the safety model
("bounded, not sandboxed" — this is the first phase to execute local
processes, and the runner does not sandbox what it launches), resource
limits, and what is explicitly *not* covered (no release gating, no suite
runner, no coupling to the incident library). A fourth, fully fictional
example, [`examples/regression-scenario-demo/`](examples/regression-scenario-demo/),
demonstrates all five outcomes end-to-end without touching
`examples/duplicate-payment/`, `examples/evidence-storage-demo/`, or
`examples/incident-library-demo/`.

## Phase 5: scenario suites

Phase 5 adds a small, reviewable, versioned manifest format — a **scenario
suite** (ISM v0.1) — and a local, sequential, offline runner
(`incidentdna suite verify|run`) that executes every scenario a manifest
lists, once each, in declared order, via the unchanged Phase 4 runner
(`internal/scenario.Run`), and aggregates the five-outcome classification
each scenario already produces into one suite-level `PASS`/`FAIL` result. A
suite manifest lists scenario files explicitly, by declared relative path —
never a directory walk or glob. The runner never introduces a new
process-execution primitive: every command a suite runs is launched by
`internal/scenario.Run`, unchanged, exactly as if that scenario had been run
standalone. See [`docs/scenario-suites.md`](docs/scenario-suites.md) for the
full design — the ISM v0.1 manifest format, the safety model (identical to
Phase 4's "bounded, not sandboxed," extended to multiple already-reviewed
scenarios), resource limits, `--fail-fast` behavior, and what is explicitly
*not* covered (no release gating, no directory/glob discovery, no parallel
execution, no coupling to the incident library). A fifth, fully fictional
example, [`examples/regression-suite-demo/`](examples/regression-suite-demo/),
demonstrates both the aggregate PASS and aggregate FAIL outcomes end-to-end
without touching `examples/duplicate-payment/`,
`examples/evidence-storage-demo/`, `examples/incident-library-demo/`, or
`examples/regression-scenario-demo/`.

## Phase 6: library cross-reference

Phase 6 adds a read-only, purely informational cross-reference between a
scenario's (or a suite's) declared `linked_fingerprint` and the incident
library's stored occurrences, exposed as an optional `--library <dir>` flag
on `incidentdna scenario verify` and `incidentdna suite verify`. This closes
the gap named as explicitly out of scope through Phase 5 — automatic
coupling between a scenario's/suite's fingerprint and library
occurrences — as a **read-only, verify-only, non-gating** lookup: a "no
occurrences found" result is printed, never enforced, and `scenario
run`/`suite run` are entirely unchanged. The only code change to an
existing package is one new exported function,
`internal/library.CheckFingerprint`; `internal/scenario` and
`internal/suite` are not modified and gain no new import — the composition
lives entirely in `cmd/incidentdna`. See
[`docs/library-crossref.md`](docs/library-crossref.md) for the full
design — the exact CLI output, the exit-code contract, suite-level
fingerprint deduplication, and what is explicitly *not* covered (no release
gating, no new document field, no mutation of the library).

## Phase 7: local policy evaluation

Phase 7 adds a small, reviewable, versioned policy document format — IGP
v0.1 ("Incident Gate Policy") — and a local, deterministic evaluator
(`incidentdna policy verify|evaluate`) that checks an already-produced
`scenario run --report`/`suite run --report` JSON artifact (Phase 4/5,
unchanged) against a declared policy and reports a `PASS`/`FAIL` verdict
with a meaningful exit code. This closes the gap named as explicitly out of
scope through Phase 6 — a declared, reviewable statement of "what counts as
acceptable," evaluated automatically against an already-produced result — as
a **local, deterministic, non-executing** evaluator: `policy evaluate` reads
two local files and prints a verdict plus exit code; it never calls a
webhook, a status-check API, or any other external system, and it never runs
`scenario.Run`/`suite.Run` itself. Exactly two rule types exist in v0.1:
`require_result` (the report's own top-level `result` field must equal a
declared value) and `require_library_occurrence` (every distinct
`linked_fingerprint` named in the report must have at least one recorded
incident-library occurrence, via the unchanged Phase 6
`library.CheckFingerprint`). `internal/policy` is a new, sixth parallel leaf
package: it imports `internal/scenario`/`internal/suite` only for their
existing `Report` struct definitions, and does not import `internal/library`
at all — the library composition lives entirely in `cmd/incidentdna`, the
identical discipline Phase 6 established. See
[`docs/policy-evaluation.md`](docs/policy-evaluation.md) for the full
design — the exact CLI output, the exit-code contract, the JSON verdict
report format, and what is explicitly *not* covered (no CI/CD integration,
no fresh execution, no boolean combinators, no batch evaluation).

## Phase 8: release readiness

Phase 8, the final planned phase, builds no new product capability on top
of Phases 1-7 — every `internal/` package they built is byte-for-byte
unchanged (except four new benchmark test files, plus a small,
byte-preserving-on-Unix OS split inside `internal/scenario` described
below). Instead it makes the seven already-built layers installable,
verifiable, and releasable as a versioned artifact: a
`version`/`--version`/`-v` CLI contract, the Apache License 2.0 at the
repository root, a reproducible five-platform release build with SHA-256
checksums and a minimal SBOM, informational non-gating benchmarks, a
vendor-neutral CI-consumption example (`examples/ci-consumption-example/`),
a written v0.1.0 compatibility contract, and a flagship end-to-end
acceptance script proving the whole stack — incident → fingerprint →
evidence → library → scenario → suite → cross-reference → policy →
deterministic release decision — composes correctly using only
already-shipped commands and fixtures. See
[`docs/release-process.md`](docs/release-process.md) for the full design,
including the Windows portability fix that lets `windows/amd64` build
alongside the other four platforms (`internal/scenario/run_unix.go` and
`run_windows.go`, an OS-specific split of the process-group timeout/kill
mechanism only — see that document's "Windows portability fix").

## CLI commands

```
incidentdna init [--force]
    Create a starter .incidentdna/incident.yaml in the current directory.

incidentdna validate <file>
    Validate an IDIR document against the supported schema version and
    semantic rules.

incidentdna fingerprint <file>
    Print the deterministic sha256: incident fingerprint of an IDIR
    document.

incidentdna inspect <file>
    Print a human-readable summary of an IDIR document.

incidentdna compare <file-a> <file-b>
    Report whether two IDIR documents share the same normalized incident
    fingerprint, and explain material differences if they do not.

incidentdna evidence store [--store <dir>] <evidence-file>
    Compute the SHA-256 digest of <evidence-file> and copy its bytes into a
    local content-addressed evidence store. Never modifies any incident
    document.

incidentdna evidence verify [--store <dir>] <incident-file>
    Validate <incident-file> and check every evidence[].digest against the
    store: presence and full content-integrity re-hash.

incidentdna evidence list [--store <dir>] <incident-file>
    Show each evidence[] entry and whether an object is present in the
    store (presence only — not an integrity guarantee; use verify for that).

incidentdna evidence inspect [--store <dir>] <digest>
    Report metadata (presence, size, integrity) about one stored object,
    addressed directly by digest. Never prints its content.

incidentdna library add [--library <dir>] [--allow-unredacted] <file>
    Validate and fingerprint <file>, enforce the privacy gate
    (privacy.redacted == true by default), and store it as a library
    occurrence: a new incident.id under an existing fingerprint is stored as
    another occurrence, a repeated (or reformatted) incident.id is
    idempotent, and a same-incident.id document with materially different
    content is refused as a conflict.

incidentdna library check [--library <dir>] <file>
    Validate and fingerprint <file>, and report whether the library holds
    one or more intact occurrences under that fingerprint.

incidentdna library list [--library <dir>]
    Enumerate every fingerprint currently stored, each with its occurrence
    count and a bounded per-occurrence summary (incident id, title,
    application/service, occurred timestamp) — never the full stored
    document.

incidentdna scenario verify [--source <incident-file>] [--library <dir>] <scenario-file>
    Structurally/semantically validate <scenario-file> against the IRS v0.1
    rules. Never executes anything. If --source is given, additionally
    validate and fingerprint <incident-file> through the unchanged Phase 1
    pipeline and assert the result equals the scenario's declared
    linked_fingerprint. If --library is given, after the above checks pass,
    look up the scenario's own linked_fingerprint against the incident
    library and print whether it has recorded occurrences (informational
    only; never affects the exit code).

incidentdna scenario run [--workspace <dir>] [--report <file>]
                          [--keep-workspace] <scenario-file>
    Run the same checks as scenario verify. If they pass, create a bounded,
    non-persistent workspace, stage execution.workspace_files, execute
    execution.command under its declared/default timeout with output capped
    per stream, and compare the result against expected. Reports PASS, FAIL,
    TIMEOUT, INVALID, or INTERNAL_ERROR, and optionally writes a
    deterministic JSON execution report to --report.

incidentdna suite verify [--library <dir>] <suite-file>
    Structurally/semantically validate <suite-file> against the ISM v0.1
    rules, then load and validate every listed scenario file against the
    existing IRS v0.1 rules (scenario.Validate, unchanged). Never executes
    anything. If --library is given, after the above checks pass, look up
    every distinct linked_fingerprint among the listed scenarios against
    the incident library, annotate each scenario line, and print an
    aggregate count (informational only; never affects the exit code).

incidentdna suite run [--workspace-root <dir>] [--report <file>]
                       [--keep-workspaces] [--fail-fast] <suite-file>
    Run the same checks as suite verify. If they pass, run each listed
    scenario in declared order via scenario.Run (unchanged), aggregate the
    outcomes, print a per-scenario and summary line, and optionally write
    the aggregate outcome as one deterministic JSON report to --report.
    --fail-fast stops after the first non-PASS scenario outcome, recording
    every scenario never reached as SKIPPED.

incidentdna policy verify <policy-file>
    Structurally/semantically validate <policy-file> against the IGP v0.1
    rules (schema_version, non-empty rules, known rule type,
    required/forbidden value per rule type, MaxRulesPerPolicy). Never
    evaluates anything.

incidentdna policy evaluate --policy <policy-file>
                             (--scenario-report <file> | --suite-report <file>)
                             [--library <dir>] [--report <file>]
    Run the same checks as policy verify. If they pass, load the named
    scenario/suite report file (unchanged Phase 4/5 JSON shape), evaluate
    every declared rule against it, print a per-rule and overall verdict
    line, and optionally write a deterministic JSON verdict report to
    --report. require_library_occurrence is evaluated only if --library is
    given; otherwise it is reported SKIP (never silently treated as
    satisfied).

incidentdna version
incidentdna --version
incidentdna -v
    Print the version this binary was built with ("dev" for an ordinary
    `make build`; a real version string for a release build, see
    "Installing a release" below), then exit 0. Never fails.
```

Exit codes are meaningful and relied on by CI: `0` success, `1`
I/O/parse/usage error, `2` semantic validation failure (or, for `evidence
verify`/`evidence inspect`, a MISSING/CORRUPTED finding; for `library
check`, no matching fingerprint found; for `scenario run`, a FAIL or
TIMEOUT outcome; for `suite run`, an aggregate FAIL outcome; or, for
`policy evaluate`, verdict FAIL). See
[`docs/evidence-storage.md`](docs/evidence-storage.md),
[`docs/incident-library.md`](docs/incident-library.md),
[`docs/regression-scenarios.md`](docs/regression-scenarios.md),
[`docs/scenario-suites.md`](docs/scenario-suites.md),
[`docs/library-crossref.md`](docs/library-crossref.md), and
[`docs/policy-evaluation.md`](docs/policy-evaluation.md) for the full
`evidence`, `library`, `scenario`, `suite`, `--library` cross-reference, and
`policy` command references.

`incidentdna` performs no network access and collects no telemetry —
every subcommand is pure local file I/O.

## Quick start (Docker)

The host machine is not assumed to have Go installed. Every command runs
through Docker via the Makefile:

```
make format   # gofmt -w .
make lint     # gofmt -l check + go vet
make test     # go test ./... -race -count=1
make build    # go build -o bin/incidentdna ./cmd/incidentdna
make example  # build, then validate + fingerprint examples/duplicate-payment/incident.yaml
make verify   # lint + test + build + example + example-evidence + example-library + example-scenario + example-suite + example-library-crossref + example-policy + golden fingerprint check
make clean    # rm -rf bin
```

If Go is available directly (e.g. a local toolchain install), drop the
`docker compose run --rm dev` prefix and run the same `go`/`gofmt` commands
yourself — the Makefile is a thin wrapper, not a requirement.

## Installing a release

Building from source via Docker (above) remains the documented path for
contributors. Someone who only wants to *run* `incidentdna` can instead
download a checksummed, platform-appropriate release archive:

```
# 1. Download (pick your platform) and verify integrity
curl -LO https://github.com/SamudralaAjaykumarrr/incidentdna/releases/download/v0.1.0/incidentdna-v0.1.0-linux-amd64.tar.gz
curl -LO https://github.com/SamudralaAjaykumarrr/incidentdna/releases/download/v0.1.0/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing

# 2. Extract and run
tar xzf incidentdna-v0.1.0-linux-amd64.tar.gz
./incidentdna version
# incidentdna v0.1.0

# 3. Try it against this repository's checked-in example (five-minute path)
./incidentdna validate examples/duplicate-payment/incident.yaml
./incidentdna fingerprint examples/duplicate-payment/incident.yaml
./incidentdna scenario run examples/regression-scenario-demo/scenario-pass.yaml
./incidentdna policy evaluate --policy policies/release-gate-example.yaml \
    --scenario-report <report-from-previous-step>
```

Step 3 requires cloning (or downloading) this repository's `examples/` and
`policies/` directories alongside the binary — deliberately not bundled
inside the release archive (each archive contains exactly the binary and a
copy of `LICENSE`), since these are demonstration fixtures, not part of the
product. **This flow is not aspirational prose** — it is the literal
sequence `scripts/verify-release-readiness.sh` executes against a freshly
extracted archive as part of an automated acceptance check
(`VERSION=v0.1.0 make release-verify`), so the documented quick start and
the tested quick start are the same steps. See
[`docs/release-process.md`](docs/release-process.md) for the full release
build/checksum/SBOM/benchmark design, the supported platform matrix (five
platforms — `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
`windows/amd64`), and the v0.1.0 compatibility contract.

## Try it: the duplicate-payment example

[`examples/duplicate-payment/incident.yaml`](examples/duplicate-payment/incident.yaml)
is a complete, synthetic IDIR document: a payment worker crashes after
committing a charge but before acknowledging the triggering message, the
message broker redelivers it, and a second charge is committed for the same
order. Run it directly, once you have a built binary (`make build`):

```
./bin/incidentdna validate examples/duplicate-payment/incident.yaml
./bin/incidentdna fingerprint examples/duplicate-payment/incident.yaml
./bin/incidentdna inspect examples/duplicate-payment/incident.yaml
```

Or via Docker, with no local build step:

```
make example
```

`make verify` additionally checks that this example's fingerprint matches
the pinned golden value in
[`testdata/golden/duplicate-payment.fingerprint`](testdata/golden/duplicate-payment.fingerprint),
guarding the fingerprint algorithm against silent drift.

## Architecture

```
cmd/incidentdna/       CLI entrypoint and one file per subcommand
internal/idir/         IDIR v0.1 Go types + size-capped JSON/YAML loader
internal/validate/     Semantic rule engine (cycles, dangling refs, digest format, redaction, ...)
internal/canonical/    Deterministic ("canonical") JSON re-encoder
internal/fingerprint/  Identity-payload extraction -> canonical -> sha256
internal/compare/      Fingerprint comparison + per-dimension diff
internal/evidence/     Local content-addressed evidence store + digest verification
internal/library/      Local incident library: occurrences grouped by fingerprint
internal/scenario/     Local, bounded, offline regression-scenario runner (IRS v0.1)
internal/suite/        Local, sequential, offline scenario suite runner (ISM v0.1)
internal/policy/       Local, deterministic policy evaluation over an already-produced report (IGP v0.1)
schemas/idir/v0.1/     Documentation-grade JSON Schema for the format
examples/duplicate-payment/     Synthetic example used by tests and `make example`
examples/evidence-storage-demo/ Separate synthetic example for `incidentdna evidence`
examples/incident-library-demo/ Separate synthetic example for `incidentdna library`
examples/regression-scenario-demo/ Separate synthetic example for `incidentdna scenario`
examples/regression-suite-demo/ Separate synthetic example for `incidentdna suite`
testdata/golden/       Golden fingerprint + one fixture per rejected validation case
```

Data flow:

```
file bytes (JSON or YAML)
   │  idir.LoadFile — size cap, format sniff, typed decode
   ▼
idir.Document
   │  validate.Validate — semantic rules
   ▼
(valid) idir.Document
   │  fingerprint.Compute — identity-payload extraction → canonicalize → sha256
   ▼
sha256:... fingerprint string
```

`compare.Documents` runs this pipeline for two documents and, if their
fingerprints differ, builds a human-readable diff across the
fingerprint-relevant dimensions (trigger, causal structure, invariants, side
effect types, recovery behavior, technology categories). See
[`docs/architecture.md`](docs/architecture.md) for the full package layout,
dependency direction, and rejected alternatives (why stdlib `flag` instead
of a CLI framework, why a hand-written validator instead of pure JSON
Schema, etc.).

The only non-stdlib dependency in the module is `gopkg.in/yaml.v3`.

## IDIR and deterministic fingerprinting

**IDIR** (Incident Deterministic Intermediate Representation) is a
JSON- or YAML-encoded document describing one production incident: its
trigger, an ordered/causal graph of events, implicated services, observable
side effects, the business invariant it violated, the corrected behavior
expected once fixed, evidence references, a reproduction sequence, and a
privacy/redaction block. The full field-by-field reference is
[`docs/idir-specification-v0.1.md`](docs/idir-specification-v0.1.md).

The **fingerprint** is a `sha256:`-prefixed hash computed over a reduced
*identity payload* extracted from a valid document — not the whole document.
It deliberately includes only what defines the *class* of failure (trigger,
causal structure of events by type, violated invariants, side-effect types,
expected corrected behavior, technology categories) and excludes what
identifies the *record* (incident id/title/timestamps, application/release
identity, event ids and free-text descriptions, evidence storage locations,
privacy metadata). The result: a reformatted copy of a document, or the same
failure observed in a different application or release, fingerprints
identically; a document with a materially different causal structure does
not. See [`docs/fingerprint-design.md`](docs/fingerprint-design.md) for the
exact field-by-field inclusion table and the reasoning behind it.

## Security and privacy principles

- **No network access, no telemetry, anywhere.** Every subcommand is pure
  local file I/O. This is a stated invariant — see
  [`docs/threat-model.md`](docs/threat-model.md).
- **Evidence locations are never dereferenced.** `evidence[].location` is
  free-text metadata; the CLI never opens, fetches, or otherwise interprets
  it.
- **Evidence digest verification is integrity-only, not authenticity.**
  `incidentdna evidence verify`/`inspect` prove that stored bytes hash to
  their declared digest — not that the evidence is truthful, or who stored
  it. Stored evidence is not encrypted at rest. See
  [`docs/evidence-storage.md`](docs/evidence-storage.md).
- **Library occurrence verification is integrity-only, not authenticity,
  either.** `incidentdna library check`/`list` prove a stored occurrence's
  bytes re-hash to the digest recorded for it — not that the incident is
  truthful, or who added it. Stored occurrences are not encrypted at rest.
  `library add` requires `privacy.redacted == true` by default
  (`--allow-unredacted` overrides it with a mandatory warning); that flag is
  author-declared, not independently verified. See
  [`docs/incident-library.md`](docs/incident-library.md).
- **Regression scenario execution is bounded, not sandboxed.** `incidentdna
  scenario run` never invokes a shell, never performs an implicit `$PATH`
  lookup, and never writes outside its resolved workspace and the
  user-named `--report` path — but it does not sandbox the process it
  launches: a reviewed scenario's command runs with the full OS-level
  permissions of the invoking user. The safety mechanism is human review of
  the scenario file before running, not runtime containment. See
  [`docs/regression-scenarios.md`](docs/regression-scenarios.md).
- **Scenario suite execution introduces no new class of risk.**
  `incidentdna suite run` never calls `exec.Command` itself — it executes
  every listed scenario by calling `internal/scenario.Run`, unchanged, once
  each, in declared order, never in parallel. `suite verify` prints every
  listed scenario's path and validation result before `suite run` ever
  executes anything. See
  [`docs/scenario-suites.md`](docs/scenario-suites.md).
- **Documents decode into a fixed typed struct**, never
  `interface{}`/`map[string]interface{}`, closing off a class of YAML-parser
  abuse.
- **Size-capped, timeout-bounded input handling.** Documents are capped at 5
  MiB before parsing, and every subcommand runs under a 30-second context
  timeout.
- **Redaction is a mechanically-checked assertion, not automatic PII
  removal.** When `privacy.redacted` is `true`, validation requires a fixed,
  documented list of "known sensitive locations" to be empty, plus a small
  regex backstop over free-text description fields. This is explicitly a
  tripwire, not general-purpose PII detection — see
  [`docs/privacy-model.md`](docs/privacy-model.md) for the exact scope and
  its limitations.
- **Minimal dependency surface.** `gopkg.in/yaml.v3` is the only non-stdlib
  Go dependency in the module.

Full details, including residual risks explicitly accepted for this phase,
are in [`docs/threat-model.md`](docs/threat-model.md) and
[`docs/privacy-model.md`](docs/privacy-model.md).

## Repository structure

```
cmd/incidentdna/    CLI entrypoint and subcommands
internal/           idir, validate, canonical, fingerprint, compare, evidence, library, scenario, suite, policy packages
schemas/idir/v0.1/  Documentation-grade JSON Schema for IDIR v0.1
examples/           Synthetic example incident(s), including ci-consumption-example/ (Phase 8)
testdata/golden/    Golden fingerprint and validation-rejection fixtures
scripts/            Golden-fingerprint, demo, and release verification scripts
docs/               Architecture, IDIR spec, fingerprint design, evidence storage, incident library, regression scenarios, scenario suites, library cross-reference, policy evaluation, release process, threat model, privacy model, product scope
dist/               Release build output (Phase 8; git-ignored, generated by `make dist`/`make release-verify` — not checked in)
Dockerfile.dev, compose.yaml, Makefile   Containerized dev/build/test workflow
LICENSE             Apache License 2.0 (Phase 8)
```

## Status and known limitations

Phase 1 is complete for its own stated scope: representing, validating,
fingerprinting, and comparing IDIR documents through a CLI, with no
dependency on any storage, transport, or UI layer. Phase 2 adds a local
evidence store and digest verification on top of that foundation, without
changing it. Phase 3 adds a local incident library — validated occurrences
grouped by fingerprint, with lookup — on top of both, again without changing
either. Phase 4 adds a local, bounded, offline regression-scenario runner on
top of all three, again without changing any of them. Phase 5 adds a local,
sequential, offline scenario-suite runner on top of all four, again without
changing any of them. Phase 6 adds a read-only, informational
`--library` cross-reference on top of all five, changing only one function
in `internal/library` and nothing else. Phase 7 adds a local, deterministic
policy evaluator (`internal/policy`, a new sixth parallel leaf package) on
top of all six, again without changing any of them. Phase 8, the final
planned phase, adds no product capability on top of any of them — every
`internal/` package above is byte-for-byte unchanged (except four new
benchmark test files) — and instead makes the whole stack installable,
verifiable, and releasable as a versioned artifact (`incidentdna version`,
a reproducible multi-platform build, SHA-256 checksums, an SBOM,
informational benchmarks, and a flagship end-to-end acceptance script). See
[`docs/release-process.md`](docs/release-process.md). Within that combined
scope, the following limitations are by design — see
[`docs/threat-model.md`](docs/threat-model.md),
[`docs/privacy-model.md`](docs/privacy-model.md),
[`docs/evidence-storage.md`](docs/evidence-storage.md),
[`docs/incident-library.md`](docs/incident-library.md),
[`docs/regression-scenarios.md`](docs/regression-scenarios.md),
[`docs/scenario-suites.md`](docs/scenario-suites.md),
[`docs/library-crossref.md`](docs/library-crossref.md), and
[`docs/policy-evaluation.md`](docs/policy-evaluation.md):

- **No semantic tamper detection.** Validation checks internal coherence
  (no dangling refs, no cycles, required fields present), not whether a
  document truthfully represents what actually happened.
- **Evidence digest verification is integrity-only, not authenticity.**
  `evidence verify`/`evidence inspect` prove stored bytes hash to their
  declared digest; they cannot prove the evidence is truthful, or who stored
  it, or when. There is no signing or provenance chain.
- **Redaction and sensitivity classification are self-declared and
  mechanically checked against a fixed field list**, not automatically
  detected or enforced across the whole document.
- **No multi-tenant or access-control model.** Every invocation operates on
  files the invoking user already has filesystem access to.
- **No encryption at rest for stored evidence, no remote/cloud storage, no
  garbage collection, and fixed (non-configurable) resource limits** on
  evidence size/count per command. See
  [`docs/evidence-storage.md`](docs/evidence-storage.md) for the full list.
- **The incident library groups occurrences, it does not gate or execute
  anything.** `library check` is an offline lookup command; it does not wire
  into any CI/CD pipeline, and the library does not generate, run, or replay
  any test or fault-injection scenario. Library occurrences are not
  encrypted at rest, not signed, not stored remotely, and not
  garbage-collected. The library's four resource limits
  (`MaxLibraryEntries`, `MaxOccurrencesPerFingerprint`, `MaxListResults`,
  and the reused `MaxDocumentSize`) are fixed, not configurable. See
  [`docs/incident-library.md`](docs/incident-library.md) for the full list.
- **The incident library is not an authorization or trust system.** Its
  integrity checks prove a stored occurrence's bytes match the digest
  recorded for it — not that the incident is truthful, that its author
  reviewed it, or who added it. It has no user accounts, no access control,
  and no concept of an incident being "approved."
- **Regression scenario execution is bounded, not sandboxed, and is not a
  release gate.** `incidentdna scenario run` executes exactly one declared
  local command per invocation with no shell interpretation and no implicit
  `$PATH` lookup, inside a freshly created, bounded, non-persistent
  workspace — but it does not sandbox the process it launches (no seccomp,
  cgroups, container, or VM isolation), and its exit code/JSON report are
  not wired into any CI/CD pipeline or merge check by this codebase. There
  is no suite runner (exactly one scenario per invocation) and no automatic
  coupling to the incident library. The eight resource limits in
  `internal/scenario/limits.go` are fixed, not configurable. See
  [`docs/regression-scenarios.md`](docs/regression-scenarios.md) for the
  full list.
- **Scenario suites aggregate, they do not gate, discover, or parallelize.**
  `incidentdna suite run` executes every scenario a manifest explicitly
  lists, sequentially, in declared order, via the unchanged Phase 4 runner
  — there is no directory-walk or glob-based discovery, no parallel
  execution, and no automatic coupling to the incident library as part of
  `suite run` itself. Its exit code and JSON report are not wired into any
  CI/CD pipeline or merge check by this codebase. The three resource limits
  in `internal/suite/limits.go` are fixed, not configurable. See
  [`docs/scenario-suites.md`](docs/scenario-suites.md) for the full list.
- **The library cross-reference is read-only, verify-only, and non-gating.**
  `scenario verify --library`/`suite verify --library` report whether the
  incident library holds occurrences under a declared `linked_fingerprint`,
  but never enforce anything: a "no occurrences found" result never changes
  either command's exit code, `scenario run`/`suite run` are entirely
  unchanged, and `internal/scenario`/`internal/suite` still do not import or
  query `internal/library` — the lookup is composed entirely in
  `cmd/incidentdna`. It reveals only a fingerprint (already visible in the
  scenario/suite's own output) and an occurrence count, never any stored
  occurrence's content. See
  [`docs/library-crossref.md`](docs/library-crossref.md) for the full list.
- **Policy evaluation is local, deterministic, and non-integrating.**
  `policy evaluate` reads an already-produced `scenario run --report`/
  `suite run --report` JSON file and an optional fresh library lookup,
  prints a per-rule and overall `PASS`/`FAIL` verdict, and sets a meaningful
  exit code — it never calls a webhook, a status-check API, or any other
  external system, and never runs `scenario.Run`/`suite.Run` itself. Only
  two rule types exist (`require_result`, `require_library_occurrence`); no
  boolean combinators, numeric thresholds, or batch evaluation. The four
  resource limits in `internal/policy/limits.go` are fixed, not
  configurable. See [`docs/policy-evaluation.md`](docs/policy-evaluation.md)
  for the full list.
- **Release engineering is productization, not a new trust boundary.**
  Release archives are integrity-checked (SHA-256), not signed — no GPG,
  no cosign, no key management. All five platforms build, including
  `windows/amd64`: `internal/scenario`'s process-group timeout/kill
  mechanism is now split by `//go:build` tag into `run_unix.go` (the
  pre-existing Setpgid/`Kill(-pid, ...)` behavior, unchanged) and
  `run_windows.go` (a Job-Object-based equivalent) — see
  `docs/release-process.md`, "Windows portability fix," for detail,
  including the one honestly-stated residual gap: the Windows path is
  verified by successful compilation and archive inspection, not by
  execution on a real Windows host (no such host exists in this project's
  CI). The SBOM covers only this module's own two-dependency Go graph, not
  the base build-container OS. Benchmarks are a point-in-time snapshot with
  no historical comparison and never gate CI or a release. See
  [`docs/release-process.md`](docs/release-process.md) for the full list.

This codebase contains no React/web framework, no Kubernetes or cloud
infrastructure, no Kafka or event-ingestion integration, no OpenTelemetry or
observability-pipeline integration, no AI/LLM calls, no SaaS
authentication/billing, and no real fault injection against a running
system. It does not perform release blocking or telemetry ingestion, and it
is not integrated with any other repository.

## License

IncidentDNA is licensed under the [Apache License 2.0](LICENSE).

## Roadmap

**Implemented (Phase 1):**

- IDIR v0.1 format, Go types, and size-capped loader
- Semantic validation engine
- Deterministic canonicalization and fingerprinting
- Fingerprint comparison and dimension-level diffing
- `incidentdna` CLI (`init`, `validate`, `fingerprint`, `inspect`, `compare`)
- Duplicate-payment synthetic example
- Containerized dev workflow and CI

**Implemented (Phase 2, this repository):**

- A local, content-addressable evidence store
  (`.incidentdna/evidence/objects` by default)
- `incidentdna evidence store` / `verify` / `list` / `inspect`
- Path-traversal and symlink protections, atomic deduplicating writes, and
  fixed resource limits (50 MiB/object, 100 entries/command, 500 MiB
  aggregate verify bytes)
- A separate, fully fictional `examples/evidence-storage-demo/` example
- See [`docs/evidence-storage.md`](docs/evidence-storage.md) for the full
  design and its explicit limitations.

**Implemented (Phase 3, this repository):**

- A local, filesystem-backed incident library
  (`.incidentdna/library/objects` by default), grouping validated
  occurrences by the existing deterministic failure fingerprint
- `incidentdna library add` / `check` / `list`
- Exact-document idempotency, multiple occurrences per fingerprint,
  conflict detection on same-`incident.id`/different-content documents, and
  corrupted-occurrence detection
- A privacy gate (`privacy.redacted == true` required by default,
  `--allow-unredacted` override with a mandatory warning) enforced before
  any occurrence is persisted
- Path-traversal and symlink protections, atomic writes, and four fixed
  resource limits (`MaxDocumentSize`, `MaxLibraryEntries`,
  `MaxOccurrencesPerFingerprint`, `MaxListResults`)
- A third, fully fictional `examples/incident-library-demo/` example
- See [`docs/incident-library.md`](docs/incident-library.md) for the full
  design and its explicit limitations.

**Implemented (Phase 4, this repository):**

- A local, bounded, offline regression-scenario runner (IRS v0.1 document
  format, `internal/scenario/`)
- `incidentdna scenario verify` / `run`, including the `--source`
  fingerprint-linkage cross-check
- Five-outcome classification (`PASS`/`FAIL`/`TIMEOUT`/`INVALID`/`INTERNAL_ERROR`)
  with a deterministic JSON execution report (`--report`)
- No shell interpretation, no implicit `$PATH` lookup, workspace
  containment, timeout enforcement (process-group kill), bounded
  stream-separated output capture, and eight fixed resource limits
- A fourth, fully fictional `examples/regression-scenario-demo/` example
- See [`docs/regression-scenarios.md`](docs/regression-scenarios.md) for the
  full design, the safety model, and its explicit limitations.

**Implemented (Phase 5, this repository):**

- A local, sequential, offline scenario-suite runner (ISM v0.1 document
  format, `internal/suite/`), executing every listed scenario via the
  unchanged Phase 4 `internal/scenario.Run`
- `incidentdna suite verify` / `run`, including `--workspace-root`,
  `--report`, `--keep-workspaces`, and `--fail-fast`
- Aggregate `PASS`/`FAIL` classification (a pure function of each listed
  scenario's own five-outcome result) with a deterministic JSON aggregate
  report (`--report`), embedding every executed scenario's own unmodified
  `scenario.Report`
- No directory-walk or glob-based scenario discovery, no parallel
  execution, no coupling to the incident library, and three fixed resource
  limits
- A fifth, fully fictional `examples/regression-suite-demo/` example
- See [`docs/scenario-suites.md`](docs/scenario-suites.md) for the full
  design, the safety model, and its explicit limitations.

**Implemented (Phase 6, this repository):**

- One new exported function, `internal/library.CheckFingerprint` (plus one
  new sentinel error, `ErrInvalidFingerprint`) — the only change to an
  existing package; `internal/scenario` and `internal/suite` are unmodified
- `incidentdna scenario verify --library <dir>` and
  `incidentdna suite verify --library <dir>`: a read-only, purely
  informational lookup of a declared `linked_fingerprint` against the
  incident library, printed but never affecting either command's exit code
- Suite-level deduplication: distinct `linked_fingerprint` values among a
  suite's listed scenarios are looked up at most once each, in
  first-occurrence declared order
- `scenario run` and `suite run` entirely unchanged — no new flag, no new
  exit-code cause, no new report field
- See [`docs/library-crossref.md`](docs/library-crossref.md) for the full
  design and its explicit limitations.

**Implemented (Phase 7, this repository):**

- A new, dedicated policy document format (IGP v0.1, `internal/policy/`,
  a new sixth parallel leaf package) — loaded through its own size-capped
  loader, deliberately not an extension of `scenario.Document`/
  `suite.Document`
- `incidentdna policy verify` / `evaluate`
- Two rule types: `require_result` (the input report's own top-level
  `result` field must equal a declared value) and
  `require_library_occurrence` (every distinct `linked_fingerprint` in the
  report must have a recorded incident-library occurrence, via the
  unchanged Phase 6 `library.CheckFingerprint`)
- A deterministic `PASS`/`FAIL` verdict with a meaningful exit code, and a
  deterministic JSON verdict report (`--report`)
- `internal/policy` never opens a library store itself — the
  `require_library_occurrence` composition lives entirely in
  `cmd/incidentdna`, mirroring Phase 6's discipline exactly; four fixed
  resource limits
- One new checked-in example policy, `policies/release-gate-example.yaml`
- See [`docs/policy-evaluation.md`](docs/policy-evaluation.md) for the full
  design and its explicit limitations.

**Implemented (Phase 8, this repository):**

- A minimal CLI version contract: `incidentdna version` /
  `--version` / `-v`, backed by a build-time-injected `-ldflags -X
  main.version=$VERSION` string, default `dev` for every ordinary build
- A root `LICENSE` file: the standard, unmodified Apache License 2.0 text
- A reproducible, five-platform release build (`scripts/build-release.sh`)
  — `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, and
  `windows/amd64`, each built twice and SHA-256-compared before archiving.
  `windows/amd64` required extracting `internal/scenario`'s process-group
  timeout/kill mechanism into a `//go:build`-tagged OS split
  (`run_unix.go`/`run_windows.go`) — see
  [`docs/release-process.md`](docs/release-process.md), "Windows
  portability fix"
- SHA-256 checksum generation and verification (`scripts/generate-checksums.sh`,
  `dist/SHA256SUMS`)
- A minimal, dependency-free SBOM (`scripts/generate-sbom.sh`,
  `incidentdna-sbom/v1`), generated from `go list -m -json all` with no
  network access
- Informational, non-gating benchmarks for `internal/canonical`,
  `internal/fingerprint`, `internal/evidence`, and `internal/validate`
  (`scripts/run-benchmarks.sh`) — never wired into `make verify` or any CI
  gate
- A vendor-neutral CI-consumption example
  (`examples/ci-consumption-example/`), demonstrating `suite run` +
  `policy evaluate` exit-code consumption from an external CI system,
  without any CI-vendor dependency inside `cmd/incidentdna`
- A written v0.1.0 compatibility contract and a flagship end-to-end
  acceptance script (`scripts/verify-release-readiness.sh`) proving the
  full ten-stage workflow this README has described since Phase 1
- New Makefile targets (`dist`, `checksums`, `sbom`, `bench`,
  `release-verify`), all additive and outside `make verify`'s existing,
  unchanged dependency chain
- Every `internal/` package above is byte-for-byte unchanged by Phase 8
  except four new, additive `_bench_test.go` files, plus
  `internal/scenario`'s process-group timeout/kill mechanism, which was
  relocated (unchanged in behavior on Unix) into `run_unix.go` and given a
  new, equivalent Windows implementation in `run_windows.go` so
  `cmd/incidentdna` compiles under `GOOS=windows`
- See [`docs/release-process.md`](docs/release-process.md) for the full
  design and its explicit limitations.

**Future work: none currently planned.** Phase 8 is the final phase in
this project's planned roadmap. Every item below was already named as a
real future need across Phases 1-7's own planning documents, and remains
exactly that — named, not built, and not currently scheduled: ingestion
from observability/event systems, integration into release gating
(including wiring the Phase 6 cross-reference's "no occurrences found"
result, or the Phase 7 policy evaluator's own exit code/verdict, into an
actual CI/CD gate or merge check), sandboxed scenario execution, parallel
suite execution, evidence/library/release-artifact signing or authenticity
proof (cryptographic or otherwise), remote/cloud storage for the evidence
store, incident library, scenarios, or suites, additional policy rule
types (boolean combinators, numeric thresholds, per-scenario rules),
package-manager distribution, additional release platforms beyond the five
already built, and any of the other items listed as explicitly out of
scope in [`docs/product-scope.md`](docs/product-scope.md). None of this
exists
yet; treat any description of it as forward-looking, not current
capability. A future maintainer choosing to pursue any of it starts a
genuinely new, unscoped planning effort — the same discipline this project
applied to every phase from Phase 1 onward.

## Contributing and development

```
make lint     # gofmt -l check + go vet
make test     # go test ./... -race -count=1
make verify   # full local == CI check: lint + test + build + examples + golden fingerprint
```

Run a single test:

```
docker compose run --rm dev go test ./internal/validate/... -run TestValidate_RejectsEachRequiredCase -v
```

CI (`.github/workflows/ci.yml`) runs the same `make` targets against the
same Docker image, so a passing `make verify` locally means CI will pass
too. Before changing what the fingerprint payload includes or excludes,
read [`docs/fingerprint-design.md`](docs/fingerprint-design.md) — the
golden tests are designed to fail loudly on any such change, and
`testdata/golden/duplicate-payment.fingerprint` must be updated
deliberately, not silently, when that happens.
