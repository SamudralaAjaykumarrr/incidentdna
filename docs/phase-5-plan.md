# Phase 5 Plan: Scenario Suites (Draft for Approval)

**Status: proposed for review. No implementation has started.** No line of
Go code, no schema, no example, and no test has been written against this
plan; nothing in this document has been merged into `Makefile`, CI, or any
existing package. It follows the same role `docs/phase-4-plan.md` played
before Phase 4 implementation began — a settled design to implement
against, not an implementer's inference — and has not yet been through a
decisions round-trip with the user. Everything below is proposed, pending
that review.

## 1. Problem statement

Phase 4 closed the "memory is never exercised" gap for exactly one
scenario at a time: `incidentdna scenario run <scenario-file>` runs one
declared command and classifies the result. That is deliberately how
Phase 4 was scoped (`docs/phase-4-plan.md` §3: "No scenario discovery,
aggregation, or 'run all' command... deliberately deferred"), but it means
today, checking "does this release still avoid every previously recorded
failure I've written a scenario for" requires an operator (or an external
script) to invoke `incidentdna scenario run` once per scenario file, by
hand, and combine the results themselves. There is no notion of a *set* of
scenarios, no aggregate pass/fail count, and no single command whose exit
code answers "did every scenario in this set pass."

`docs/phase-4-plan.md` §27/§28 and `docs/phase-4-report.md` §15 both name
this gap explicitly as the nearest unbuilt increment: a "suite/aggregation
layer" that discovers or is told about many scenarios, runs each, and
summarizes the aggregate result — named consistently, and separately from
release-gate integration, across every phase-4 document. Phase 5 closes
this gap narrowly, the same way Phase 4 closed "no execution" narrowly:
it adds a small, reviewable, versioned manifest format that lists
scenarios explicitly, and a local runner that executes each one (by
calling `internal/scenario.Run` unchanged, once per listed scenario) and
aggregates the results — without becoming a parallel execution engine, a
CI product, or a release gate.

## 2. Goals

1. Define a **deterministic, reviewable-as-text suite manifest format**
   (tentatively **ISM v0.1**, "Incident Scenario Manifest") that declares
   an explicit, ordered list of scenario files to run — no directory walk,
   no glob, no implicit discovery, mirroring the same "nothing is resolved
   implicitly from the environment, everything a reviewer needs is visible
   in the file" discipline `execution.command`, `workspace_files`, and
   `linked_fingerprint` already established in IRS v0.1.
2. Provide a **local runner** (`incidentdna suite run`) that executes each
   listed scenario, in declared order, by calling
   `internal/scenario.Run` unchanged once per entry — no new execution
   primitive, no parallelism, no new sandboxing surface — and aggregates
   the five-outcome classification each already produces into one suite-
   level result.
3. Provide a **local verifier** (`incidentdna suite verify`) that checks a
   suite manifest's structural validity and, for every listed scenario,
   the same structural/semantic checks `scenario verify` already performs
   — without ever executing anything, mirroring the existing
   `validate`/`evidence verify`/`library check`/`scenario verify` split
   between "is this trustworthy to run" and "did running it produce the
   expected result."
4. Emit a **machine-readable aggregate report** (JSON) that embeds each
   scenario's own report (reusing `scenario.Report` unchanged) alongside a
   suite-level summary, the same "always print a human-readable summary,
   optionally also write JSON" pattern every prior phase established.
5. Do all of this **without changing** `internal/scenario`,
   `internal/evidence`, or `internal/library`, and **without requiring**
   any of them to change — `internal/suite` is a fifth parallel package
   that imports `internal/scenario` (for `scenario.LoadFile`,
   `scenario.Validate`, and `scenario.Run`, all unchanged) the same way
   `internal/scenario` imports `internal/idir`/`internal/validate`/
   `internal/fingerprint` unchanged today.
6. Make the whole feature **testable without any external service**: no
   Docker daemon, no network, so `go test ./internal/suite/... -race
   -count=1` is self-contained the same way every existing package's test
   suite already is — using the same small, fast fixture commands
   (`/bin/true`, `/bin/false`, `/bin/sleep`) Phase 4's own tests use,
   never a dependency on the built `incidentdna` binary except at the CLI
   black-box test layer.

## 3. Explicit non-goals

- **No release gating.** `suite run`'s exit code is meaningful and could
  in principle be consumed by an external CI job — but IncidentDNA itself
  does not wire any suite result into a gate, a policy decision, a merge
  check, or any other blocking mechanism. This restates, unconditionally,
  the same standing exclusion every phase document has repeated since
  Phase 1 (`docs/product-scope.md`, "Explicitly out of scope for Phase 1
  through Phase 4"; `docs/phase-4-plan.md` §28).
- **No scenario discovery via directory walk or glob.** A suite manifest
  lists scenario files explicitly, by declared relative path, the same
  way `execution.workspace_files` lists staged files explicitly rather
  than accepting a glob pattern. An operator who wants "every scenario
  under `examples/`" writes (or generates, out of band) a manifest that
  names them; `incidentdna` itself never walks a directory tree looking
  for scenario files.
- **No parallel execution.** Scenarios in a suite run sequentially, in
  declared order. Parallelizing execution would multiply the process-
  management surface (concurrent workspaces, concurrent timeouts,
  concurrent output capture) for a feature whose primary purpose in v0.1
  is aggregation and reporting, not throughput; explicitly deferred (see
  §25).
- **No coupling to the incident library.** A suite manifest and its
  runner do not query `internal/library`. (This mirrors Phase 4's own
  restraint — `docs/phase-4-plan.md` §6/§28 named "cross-referencing a
  scenario's `linked_fingerprint` against library occurrences" as a
  distinct, separate, still-unbuilt future increment; Phase 5 does not
  fold it in, to keep this phase's own scope narrow and independently
  reviewable. See §25.)
- **No new process-execution primitive.** `internal/suite` never calls
  `exec.Command` itself; it calls `internal/scenario.Run` once per listed
  scenario, unchanged. Every safety guarantee and every limitation Phase
  4 already documented (bounded, not sandboxed; no shell; no implicit
  `$PATH`; fixed child environment) applies identically to a scenario run
  from inside a suite — Phase 5 introduces no new class of risk the way
  Phase 4 introduced process execution itself.
- **No suite-level timeout independent of its scenarios' own timeouts.**
  The suite's total wall-clock bound is the sum of its listed scenarios'
  own `execution.timeout_seconds` (each already bounded by
  `MaxScenarioTimeoutSeconds`), capped by one new fixed constant
  (§10) — not a separately-configurable suite timeout, keeping with the
  fixed-constants discipline every prior phase applied.
- **No mutation of any existing store or document.** `internal/suite`
  does not write into `.incidentdna/evidence`, `.incidentdna/library`, or
  any scenario file; its only durable, optional output is a caller-named
  `--report <file>`, mirroring `scenario run` exactly.
- **No change to fingerprint identity, IDIR, or IRS semantics.**
  `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library`, and `internal/scenario` are all untouched.
- **No signing, no encryption, no multi-tenancy, no network, no
  telemetry** — the same standing invariants every prior phase restates.
- **No configurable resource limits** — every bound in §10 is a fixed,
  named Go constant, not a config surface, matching Phases 2–4.

## 4. User workflows

**Workflow A — validate a suite manifest and every scenario it lists,
without running anything:**

```
$ incidentdna suite verify examples/regression-suite-demo/suite.yaml
Suite: core-regression-suite (schema suite/v0.1)
Scenarios: 4 listed
  [OK] duplicate-payment-refingerprint (scenario-pass.yaml)
  [OK] duplicate-payment-wrong-expectation (scenario-fail.yaml)
  [OK] duplicate-payment-timeout (scenario-timeout.yaml)
  [OK] duplicate-payment-bare-command (scenario-invalid.yaml)
OK: suite manifest and all 4 listed scenarios are structurally valid
```

**Workflow B — run every scenario in a suite and see the aggregate
result:**

```
$ incidentdna suite run examples/regression-suite-demo/suite.yaml
Suite: core-regression-suite
[1/4] duplicate-payment-refingerprint ... PASS
[2/4] duplicate-payment-wrong-expectation ... FAIL
[3/4] duplicate-payment-timeout ... TIMEOUT
[4/4] duplicate-payment-bare-command ... INVALID
Result: FAIL (1 PASS, 1 FAIL, 1 TIMEOUT, 1 INVALID, 0 INTERNAL_ERROR)
Duration: 1.31s
```

Exit code `2` (see §14). A `--report <file>` flag additionally writes the
full aggregate outcome, including every scenario's own report, as one
deterministic JSON document (§15).

**Workflow C — stop at the first failure** (`--fail-fast`), useful for a
human iterating locally: runs scenarios in declared order and stops
immediately after the first non-`PASS` outcome, reporting the scenarios
that were never reached as `SKIPPED` in the aggregate summary/report
rather than silently omitting them.

## 5. Suite manifest format (ISM v0.1)

A suite manifest is a small, standalone YAML or JSON document, decoded
through a new, dedicated, size-capped loader
(`internal/suite.LoadFile`, mirroring `scenario.LoadFile`'s discipline,
not reusing it — the two document types are unrelated, the same
"deliberately not an extension of an existing type" stance
`docs/regression-scenarios.md` already took for IRS v0.1 relative to
IDIR).

```yaml
schema_version: suite/v0.1

suite:
  id: core-regression-suite
  title: Core duplicate-payment regression coverage
  description: >
    Every regression scenario currently authored against the
    duplicate-payment failure class.

scenarios:
  - path: scenario-pass.yaml       # relative to this manifest's own directory
  - path: scenario-fail.yaml
  - path: scenario-timeout.yaml
  - path: scenario-invalid.yaml
```

Field notes:

- `schema_version` — fixed literal `suite/v0.1`, checked the same way
  `idir.Document.SchemaVersion` and `scenario.Document.SchemaVersion` are
  checked today: exact match, reject anything else.
- `suite.id` — stable, author-chosen identifier; present only for
  human/report readability, never used as a filesystem path component
  (no store, so nothing to key by it — the same rationale
  `scenario.id` already has).
- `scenarios` — a required, non-empty, ordered list, bounded by
  `MaxScenariosPerSuite` (§10). Each entry's `path` is resolved relative
  to the manifest's own directory; rejected if absolute, containing `..`,
  or resolving (after `filepath.Clean`) outside that directory — the
  identical discipline `workspace_files[].source` already applies. A
  `path` that is a symlink is rejected, not followed. Duplicate `path`
  values are rejected as invalid (a suite naming the same scenario file
  twice is almost certainly an authoring mistake, not a meaningful
  "run it twice" request) — if an operator genuinely wants a scenario run
  more than once, two distinct scenario files (or a copy) make that
  intent visible in the manifest, the same "explicit over implicit"
  reasoning already applied to `workspace_files[].destination` uniqueness
  in IRS v0.1.
- No suite-level `execution` block, `env`, or `timeout_seconds` override
  — each listed scenario's own `execution.timeout_seconds` (or its
  default) governs that scenario's own run, unchanged; a suite does not
  re-parameterize the scenarios it lists. This keeps a scenario file's
  meaning identical whether it is run standalone (`scenario run`) or as
  part of a suite (`suite run`) — no hidden suite-level override can
  change what a reviewer of the scenario file alone would expect.

## 6. Relationship between the artifacts

```
regression scenario (IRS v0.1 document, Phase 4, unchanged)
   |  scenario.LoadFile + scenario.Validate + scenario.Run
   v
PASS / FAIL / TIMEOUT / INVALID / INTERNAL_ERROR  (per scenario, unchanged)
   ^
   |  referenced by declared, explicit path
   |
suite manifest (ISM v0.1 document, Phase 5)
   |  suite.LoadFile + suite.Validate
   v
(valid) suite.Document
   |  suite.Run — calls scenario.Run once per listed scenario, in order
   v
aggregate outcome: PASS (all scenarios PASS) or FAIL
 (any scenario FAIL/TIMEOUT/INVALID/INTERNAL_ERROR)
   |  suite.BuildReport + suite.WriteReport (--report only)
   v
deterministic JSON suite report, embedding each scenario's own report
```

The load-bearing relationship is identical in shape to the one Phase 4
established between a scenario and its `linked_fingerprint`: a suite
*names* scenarios by declared path, exactly as reviewable and exactly as
free of implicit resolution as a scenario names its own `command`. Phase
5 does not require a scenario to belong to any suite, and does not modify
`internal/scenario` to know suites exist — a scenario file authored
before Phase 5 existed is listable in a suite manifest exactly as-is.

## 7. Determinism requirements

1. **Suite execution order is exactly the declared list order** — never
   alphabetical, never filesystem-iteration order, never reordered by
   size or estimated duration. Reruns of the same manifest against an
   unchanged environment execute scenarios in the identical order.
2. **Each scenario's own execution is exactly as deterministic as `scenario
   run` already is** (`docs/regression-scenarios.md` §"Determinism
   requirements") — `suite.Run` introduces no new source of
   nondeterminism around a scenario's own workspace, environment, or
   output capture, because it calls `scenario.Run` unchanged.
3. **A suite manifest's own validity depends only on its own bytes and
   the bytes of the scenario files it lists** — never on prior runs, the
   filesystem state of any workspace, or wall-clock time. This mirrors
   `scenario verify`'s existing determinism statement, extended to a list
   of scenarios instead of one.
4. **Aggregate classification is a pure function of the per-scenario
   outcomes** (§14) — the suite-level `PASS`/`FAIL` result and the
   per-outcome counts are recomputed identically from the same set of
   per-scenario results every time, with no suite-level judgment call
   beyond "did every scenario PASS."

## 8. Safety model

Phase 5 introduces **no new class of risk**. It is the first package that
does not call `exec.Command` at all, directly or indirectly through a new
code path — every process it ever launches is launched by
`internal/scenario.Run`, unchanged, under exactly the same "bounded, not
sandboxed" model `docs/regression-scenarios.md` already documents in
full. This document does not restate that model; it only states what is
different: a suite orchestrates *multiple* reviewed scenarios rather than
one, so the reviewability requirement Phase 4 already established
("a reviewer who approves a scenario file is approving exactly the
command that will run") extends to "a reviewer who approves a suite
manifest is approving exactly the ordered list of already-reviewed
scenario files that will run" — nothing about suite membership changes
what an already-reviewed scenario file does when executed.

## 9. Execution isolation boundaries

Inherited unchanged, per listed scenario, from `docs/regression-scenarios.md`
§"Execution isolation boundaries": no shell interpretation, no implicit
`$PATH` lookup, workspace containment, fixed child environment, process-
group timeout kill, bounded stream-separated output capture, workspace
removal unless `--keep-workspaces` is given. `internal/suite` adds exactly
one new boundary of its own:

| Boundary | Mechanism | Guarantee level |
|---|---|---|
| Suite manifest path resolution | Every listed `scenarios[].path` is `filepath.Clean`ed and re-checked to have the manifest's own directory as a prefix before `scenario.LoadFile` is ever called on it; a symlink at that path is rejected, not followed | Structural, mirroring `workspace_files[].source`'s existing discipline |
| One workspace per scenario, never shared | Each scenario in a suite gets its own fresh workspace (`os.MkdirTemp`, or a named subdirectory under `--workspace-root` if given), created and torn down exactly as it would be for a standalone `scenario run` — never reused across scenarios in the same suite run | Structural |
| Suite-level total timeout | `context.WithTimeout` wrapping the whole `suite.Run` call, bounded by `MaxSuiteTotalTimeoutSeconds` (§10) — a backstop in case the sum of individual scenario timeouts would otherwise exceed a sane bound, checked at verify time (summing declared/default `timeout_seconds` across all listed scenarios) so an over-budget suite is rejected before any scenario ever runs | Structural |

## 10. Resource limits

New, independent, named constants in `internal/suite/limits.go`, each
with its own distinct error and its own passing/failing boundary test
pair — the same discipline every prior phase's `limits.go` established:

| Constant | Value (proposed) | Applies to |
|---|---|---|
| `MaxSuiteDocumentSize` | 256 KiB | The suite manifest file's own on-disk size |
| `MaxScenariosPerSuite` | 100 | Number of `scenarios[]` entries a manifest may declare |
| `MaxSuiteTotalTimeoutSeconds` | 1800 (30 min) | Sum of declared/default `timeout_seconds` across every listed scenario; checked at verify time |

`MaxScenarioDocumentSize`, `MaxWorkspaceFiles`, `MaxWorkspaceFileSize`,
`MaxWorkspaceTotalBytes`, `MaxScenarioOutputBytes`, and the per-scenario
timeout bounds are all inherited unchanged, per listed scenario, from
`internal/scenario/limits.go` — Phase 5 does not redefine or widen any of
them.

## 11. Allowed and forbidden operations

**Allowed:** reading the suite manifest itself and each listed scenario
file, both resolved relative to fixed, validated roots; calling
`scenario.LoadFile`/`scenario.Validate`/`scenario.Run` once per listed
scenario, unchanged; writing exactly one aggregate report file, only if
`--report <path>` is given; removing each scenario's temporary workspace
on completion (default) or retaining all of them
(`--keep-workspaces`, plural, since a suite may run several).

**Forbidden (by construction):** any write by `internal/suite`'s own code
outside a resolved workspace or the user-named `--report` path (identical
to Phase 4's own forbidden list, since `internal/suite` performs no
filesystem writes of its own beyond delegating to `scenario.Run`); any
read of a `scenarios[].path` that is absolute, contains `..`, or resolves
outside the manifest's own directory; any symlink at a `scenarios[].path`
being followed; parallel or reordered execution of listed scenarios;
network access or telemetry from `internal/suite`'s own code
(`grep -rn '"net' internal/suite cmd/` must stay empty, the same
invariant every prior phase's acceptance criteria checked).

## 12. Local filesystem layout

Like Phase 4, Phase 5 introduces **no new permanent local store** — a
suite has nothing durable to persist beyond its optional `--report`. At
run time:

```
<workspace root>/                 os.MkdirTemp() by default, or an
                                    explicitly empty --workspace-root <dir>
  <scenario-1-id or index>/       one subdirectory per listed scenario,
    <staged workspace_files>       each isolated exactly as a standalone
                                    `scenario run` would create it
  <scenario-2-id or index>/
  ...
```

removed after the run unless `--keep-workspaces` is given. No new
`.gitignore` entry is needed, for the same reason Phase 4 needed none.

## 13. CLI commands

```
incidentdna suite verify <suite-file>
    Load and structurally/semantically validate <suite-file> against the
    ISM v0.1 rules (schema_version, non-empty scenarios list, path safety,
    duplicate-path rejection, aggregate timeout bound), then load and
    validate every listed scenario file against the existing IRS v0.1
    rules (scenario.Validate, unchanged). Never executes anything.

incidentdna suite run [--workspace-root <dir>] [--report <file>]
                       [--keep-workspaces] [--fail-fast] <suite-file>
    Run the same checks as `suite verify`. If they pass, run each listed
    scenario in declared order via scenario.Run (unchanged), aggregate the
    outcomes, print a per-scenario and summary line, and (if --report is
    given) write the aggregate outcome as one deterministic JSON report
    file (§15).
```

`main.go`'s `commands` table gains one entry, `{"suite", runSuite}`,
following the exact `{"scenario", runScenario}` precedent. `runSuite`
self-dispatches `verify`/`run` using the same `flag.NewFlagSet` idiom
every existing command already uses. `main.go` gains one more conditional
branch in its per-command context construction, extending the existing
`c.name != "scenario"` special-case to also exclude `"suite"` from the
fixed 30-second `commandTimeout` wrap, for the identical reason
`"scenario"` was excepted in Phase 4: `suite run`'s workload (potentially
many bounded child-process executions in sequence) needs its own budget,
derived from the sum of its listed scenarios' timeouts and bounded by
`MaxSuiteTotalTimeoutSeconds`, not the fixed budget sized for
parsing/hashing a document. `runSuite` re-applies `commandTimeout` itself
for the `verify` subcommand, which never executes anything, exactly as
`runScenario` already does.

`init`, `validate`, `fingerprint`, `inspect`, `compare`, all four
`evidence` subcommands, all three `library` subcommands, and both
`scenario` subcommands remain byte-for-byte unchanged in behavior.

## 14. Exit-code contract

**`suite verify`** (mirrors `scenario verify`'s existing 0/1/2 split):

| Exit | Meaning |
|---|---|
| `0` | Suite manifest and every listed scenario are structurally/semantically valid |
| `1` | I/O/parse/usage error — can't read the suite file or a listed scenario file, bad CLI usage |
| `2` | Suite manifest decodes but fails a semantic ISM v0.1 rule, or a listed scenario fails `scenario.Validate` |

**`suite run`**:

| Outcome | Exit | Meaning |
|---|---|---|
| Aggregate `PASS` | `0` | Every listed scenario's own outcome was `PASS` |
| Aggregate `FAIL` | `2` | At least one listed scenario's outcome was `FAIL`, `TIMEOUT`, or (with `--fail-fast`) `SKIPPED` |
| Aggregate `INVALID` | `1` | The suite manifest itself, or any listed scenario, failed pre-execution checks — nothing was ever executed |
| Aggregate `INTERNAL_ERROR` | `1` | An environmental failure prevented determining the aggregate result (e.g. `--report` path unwritable, `--workspace-root` pre-existing and non-empty) |

This reuses, rather than reinvents, the same exit-code space Phase 4
already established: `2` means "well-formed input, materially negative
finding," `1` means "environmental/usage failure" — the project-wide
convention `docs/incident-library.md` and `docs/regression-scenarios.md`
both already document as intentionally reused per command, not a new
code space per phase.

## 15. Result and report format

```json
{
  "schema_version": "suite-report/v0.1",
  "suite_id": "core-regression-suite",
  "suite_file": "examples/regression-suite-demo/suite.yaml",
  "result": "FAIL",
  "counts": {"pass": 1, "fail": 1, "timeout": 1, "invalid": 1, "internal_error": 0, "skipped": 0},
  "duration_ms": 1310,
  "scenarios": [
    {"path": "scenario-pass.yaml", "report": { /* full scenario.Report, unchanged shape */ }},
    {"path": "scenario-fail.yaml", "report": { /* ... */ }},
    {"path": "scenario-timeout.yaml", "report": { /* ... */ }},
    {"path": "scenario-invalid.yaml", "report": { /* ... */ }}
  ],
  "error": null
}
```

Each `scenarios[].report` is exactly the JSON object `scenario.BuildReport`
already produces for that scenario, unchanged — the suite report is a
deterministic aggregation, not a reinterpretation, of Phase 4's existing
per-scenario report shape. `--report` overwrites unconditionally on each
run, matching `scenario run --report`'s existing behavior.

## 16. Idempotency behavior

Identical in kind to `scenario run`'s existing idempotency statement
(`docs/regression-scenarios.md` §"Idempotency behavior"): no persistent
side effects on the repository or any store; a fresh set of workspaces
every run; `--report`, if given, is overwritten each run, never appended
or versioned; the aggregate result is repeatable only to the extent every
listed scenario's own declared command is itself deterministic.

## 17. Corruption and malformed-manifest handling

- **Suite manifest fails to parse** (bad YAML/JSON, exceeds
  `MaxSuiteDocumentSize`): `suite verify` exits `1`; `suite run` reports
  aggregate `INTERNAL_ERROR` and exits `1`.
- **Manifest decodes but fails a semantic rule** (missing/wrong
  `schema_version`, empty `scenarios`, a `path` with a
  traversal/absolute/symlinked/duplicate value, declared scenario count
  exceeding `MaxScenariosPerSuite`, summed timeouts exceeding
  `MaxSuiteTotalTimeoutSeconds`): `suite verify` exits `2`; `suite run`
  reports aggregate `INVALID` and exits `1` — nothing is ever executed,
  the same "pre-execution rejection is a usage-class outcome for `run`"
  reasoning `docs/regression-scenarios.md` §"Corruption and
  malformed-scenario handling" already applies to a single scenario,
  extended here to the whole listed set: if any one listed scenario is
  itself invalid, the suite run does not partially execute the valid ones
  and skip the invalid one silently — the entire suite is rejected at
  verify time, so a reviewer's approval of "this suite is valid" always
  means "every scenario in it is valid," never "most of them are."
- **A listed scenario file is missing at verify/run time**: reported
  distinctly (`ErrSuiteScenarioMissing`), `INVALID` for `run` if caught at
  the pre-execution check, `INTERNAL_ERROR` if a TOCTOU race caused it to
  disappear between check and load — the same documented residual gap
  class Phase 4 already accepted for `workspace_files`.
- **A listed scenario itself fails mid-suite for an environmental reason**
  (its own `INTERNAL_ERROR`, e.g. its `command[0]` disappeared between
  `suite verify` and `suite run`): recorded as that scenario's own
  `INTERNAL_ERROR` in the aggregate counts and report; the suite continues
  to the next listed scenario unless `--fail-fast` was given, and the
  suite's own aggregate result is `FAIL` (§14) — one scenario's
  environmental failure does not abort the whole suite's report
  generation.
- **`--report` cannot be written**: reported to stderr, exit `1`, even if
  every listed scenario PASSed — the per-scenario and summary lines are
  still printed to stdout first, mirroring `scenario run`'s existing
  behavior exactly.

## 18. Privacy implications

Identical in kind to Phase 4's own statement
(`docs/privacy-model.md` §"Regression scenario privacy implications"): a
suite manifest carries no dedicated privacy/redaction block (it is
short-lived, reviewable execution metadata, not a permanent incident
record); the aggregate report's embedded per-scenario reports are exactly
as bounded (`MaxScenarioOutputBytes` per stream, per scenario) as a
standalone scenario report already is; nothing scans manifest or report
content for sensitive material; the child processes' own privacy
behavior remains entirely outside this tool's control, restated rather
than newly introduced.

## 19. Threat model additions

New subsection proposed for `docs/threat-model.md`, "Scenario suites:
aggregated local process-execution risk (Phase 5)," stating plainly that
Phase 5 introduces no new category of risk beyond what
`docs/threat-model.md` already documents for Phase 4
("Executable regression scenarios: local process-execution risks"): every
process a suite launches is launched by `internal/scenario.Run`,
unchanged, so every one of that section's bullets (malicious/careless
scenario authorship, workspace path-traversal, resource exhaustion,
`command[0]` being a shell, TOCTOU, detached-process timeout evasion, no
network/telemetry from the runner's own code) applies identically, per
listed scenario. The one genuinely new consideration:

- **A malicious or careless suite manifest can name scenario files whose
  content a reviewer of the manifest alone has not necessarily read.**
  Mitigated the same way `--source` cross-checking is mitigated in Phase
  4: nothing about suite membership hides or mutates a scenario file's
  own content — `suite verify` prints every listed scenario's path and
  its own validation result, so a reviewer approving a suite manifest is
  explicitly shown which scenario files it will run, in what order,
  before ever running `suite run`. As with every other "review before
  running" control in this project, IncidentDNA does not verify that a
  human actually read each one — it makes doing so straightforward and
  the alternative (running an unreviewed suite) an explicit, visible
  choice.

## 20. Backward compatibility with Phases 1–4

- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library`, and `internal/scenario` are **untouched** —
  `internal/suite` imports only `internal/scenario` (for `LoadFile`,
  `Validate`, and `Run`, all unchanged), and is imported by nothing else
  in `internal/`.
- `examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
  `examples/incident-library-demo/`, and
  `examples/regression-scenario-demo/` are **not modified** — the new
  example (§24) is a new, separate directory that only *references*
  copies of existing scenario-shaped fixtures, never edits the originals.
- All existing CLI subcommands keep their exact current exit-code and
  output behavior — proven the same way every prior phase proved it: the
  full existing test suite passes unmodified, and no file under any
  existing package appears in the implementation diff except the one
  narrow, additive `main.go` touch described in §13 (extending the
  existing `c.name != "scenario"` special-case, following the exact
  precedent that same line already set in Phase 4).
- No `schema_version` bump for IDIR or IRS — Phase 5 does not touch
  either format. ISM v0.1 is a wholly new, separate format with its own
  version string.
- A scenario file authored under Phase 4 (with no awareness Phase 5
  exists) is listable in a suite manifest exactly as-is — no
  re-authoring required.

## 21. Implementation slices in exact order

Following the exact "each slice independently mergeable and testable"
discipline `docs/phase-4-plan.md` §21 established:

1. `internal/suite/types.go` + `load.go` + `load_test.go` — ISM v0.1
   struct definitions, size-capped loader, no validation logic yet.
2. `internal/suite/validate.go` + `validate_test.go` — every semantic
   rule in §5/§17 (schema version, non-empty `scenarios`, path safety,
   duplicate-path rejection, `MaxScenariosPerSuite`, aggregate timeout
   bound against `MaxSuiteTotalTimeoutSeconds`), returning a
   `Result`/`Issue` shape mirroring `internal/scenario`'s existing
   pattern; also validates every listed scenario by calling
   `scenario.LoadFile` + `scenario.Validate` unchanged.
3. `internal/suite/limits.go` + `errors.go` + tests — the three §10
   constants, each with its own passing/failing boundary test pair.
4. `internal/suite/run.go` + `run_test.go` — sequential per-scenario
   execution via `scenario.Run` (unchanged), aggregate outcome
   classification, `--fail-fast` short-circuiting, tested with the same
   small fixture-command style Phase 4's own `run_test.go` uses.
5. `internal/suite/report.go` + `report_test.go` — the JSON report type
   (§15) embedding per-scenario `scenario.Report` values, deterministic
   field order, `--report` file-write behavior.
6. `cmd/incidentdna/cmd_suite.go` + `cli_suite_test.go` — CLI wiring for
   `verify`/`run`, `--workspace-root`/`--report`/`--keep-workspaces`/
   `--fail-fast` flags, exit codes from §14, black-box tests against the
   real compiled binary.
7. `examples/regression-suite-demo/` — the new example (§24), reusing
   copies of Phase 4's four demo scenarios (adapted the same
   placeholder-binary-path way `regression-scenario-demo` already is),
   covering aggregate PASS and aggregate FAIL.
8. `docs/scenario-suites.md` — new dedicated design doc, the Phase 5
   analog of `docs/regression-scenarios.md`; updates to `README.md`,
   `docs/architecture.md`, `docs/product-scope.md`, `docs/threat-model.md`,
   `docs/privacy-model.md` — done after the code so they describe actual
   behavior, not the plan.
9. `Makefile` + `.github/workflows/ci.yml` — new `example-suite` target
   (mirroring `example-scenario`) wired into `verify`; a new
   `scripts/verify-suite-demo.sh`; a new CI step.
10. `docs/phase-5-report.md` — written last, after real command
    transcripts exist to record, mirroring `docs/phase-4-report.md`'s
    role.

Each slice through 6 should independently pass `go test ./... -race
-count=1`; slices 7–9 should independently pass `make verify` once slice
9 lands.

## 22. Files expected to be added or modified

**New:**

```
internal/suite/
  types.go, load.go, validate.go, run.go, report.go, limits.go, errors.go
  load_test.go, validate_test.go, run_test.go, report_test.go, limits_test.go

cmd/incidentdna/
  cmd_suite.go
  cli_suite_test.go

examples/regression-suite-demo/
  README.md
  suite-all-pass.yaml           — aggregate PASS outcome
  suite-mixed.yaml              — aggregate FAIL outcome (mixed results)
  scenarios/                    — copies of scenario-pass.yaml/
                                   scenario-fail.yaml/scenario-timeout.yaml/
                                   scenario-invalid.yaml adapted from
                                   examples/regression-scenario-demo/,
                                   never a symlink or reference to the
                                   originals
  fixtures/incident.yaml        — a copy of the duplicate-payment content

docs/
  scenario-suites.md
  phase-5-report.md             (written last, per §21 slice 10)

scripts/
  verify-suite-demo.sh
```

**Modified:**

```
cmd/incidentdna/main.go          — one new {"suite", runSuite} entry;
                                    extend the existing scenario-only
                                    context-timeout special-case to also
                                    exclude "suite"
README.md                        — CLI commands list, roadmap, known-limitations
docs/architecture.md             — package layout table, dependency direction, new data-flow diagram
docs/product-scope.md            — new "Phase 5 scope" section; out-of-scope list updated
docs/threat-model.md             — new "Scenario suites" section (§19)
docs/privacy-model.md            — new subsection (§18)
Makefile                         — example-suite target, wired into verify
.github/workflows/ci.yml         — new "Scenario suite demo validation" step
.gitignore                       — no new entry expected (§12); confirmed during implementation
```

**Untouched (explicit):** `internal/idir/`, `internal/validate/`,
`internal/canonical/`, `internal/fingerprint/`, `internal/compare/`,
`internal/evidence/`, `internal/library/`, `internal/scenario/`,
`examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
`examples/incident-library-demo/`, `examples/regression-scenario-demo/`,
`testdata/golden/`.

## 23. Unit, integration, CLI, race, corruption, and boundary tests

Following the exact table-driven, real-filesystem (`t.TempDir()`),
no-mocking style every prior phase established:

- **Unit — `internal/suite/validate_test.go`**: one table-driven test per
  §5/§17 semantic rule (missing/wrong `schema_version`, empty
  `scenarios`, `path` traversal/absolute/symlink/duplicate rejection,
  over-limit scenario count, over-limit aggregate timeout, a listed
  scenario that itself fails `scenario.Validate`).
- **Unit — `internal/suite/run_test.go`** (the highest-value new coverage
  in this phase): aggregate `PASS` (every listed scenario PASSes);
  aggregate `FAIL` from one `FAIL`, one `TIMEOUT`, and one
  `INTERNAL_ERROR` scenario, independently; declared-order execution
  (asserted via a fixture command that appends its own identity to a
  shared file, confirming append order matches declared order);
  `--fail-fast` short-circuiting (asserting later scenarios are recorded
  `SKIPPED` and never actually executed — via the same "marker file
  would have been created" technique Phase 4's own `INVALID` test uses);
  workspace isolation across scenarios in the same suite run (two
  scenarios each writing a same-named relative file, asserting neither
  overwrites the other because each has its own workspace).
- **Unit — `internal/suite/report_test.go`**: deterministic JSON field
  order across repeated marshals; correct embedding of each scenario's
  own unmodified `scenario.Report` shape; `--report` overwrite behavior.
- **Unit — `internal/suite/limits_test.go`**: passing/failing boundary
  pair for each of the three §10 constants.
- **CLI — `cmd/incidentdna/cli_suite_test.go`** (black-box against the
  real compiled binary, the style `cli_scenario_test.go` already uses):
  `suite verify` exit 0/1/2 paths; `suite run` exit 0/1/2 across
  aggregate PASS/FAIL/INVALID/INTERNAL_ERROR; `--report` file contents
  match the printed summary; `--keep-workspaces` leaves real, inspectable
  per-scenario directories; `--fail-fast` behavior; a suite whose listed
  scenarios include one pointing at the real `bin/incidentdna` binary,
  demonstrating the same self-referential regression check the Phase 4
  demo already uses.
- **Race** — `go test ./... -race -count=1` must stay green with
  `internal/suite` included.
- **Regression discipline**: the full existing suite must pass with zero
  modification to any existing test file's expectations; the golden
  fingerprint test/script must produce the unchanged Phase 1 value.

## 24. Runnable demo requirements

`examples/regression-suite-demo/` — a new, self-contained directory
demonstrating both aggregate outcomes end-to-end via
`scripts/verify-suite-demo.sh` (mirroring
`scripts/verify-scenario-demo.sh`'s structure: build the binary once,
substitute the placeholder binary path, then drive `suite verify`/
`suite run` against a temporary `--workspace-root`/`--report` location,
never leaving generated output in the repository):

1. **Aggregate PASS** — `suite-all-pass.yaml` lists only scenarios that
   individually PASS, demonstrating exit `0` and every per-scenario line
   reporting `PASS`.
2. **Aggregate FAIL** — `suite-mixed.yaml` lists a mix (at least one
   PASS, one FAIL, one TIMEOUT, one INVALID scenario), demonstrating exit
   `2`, the correct aggregate counts, and that a suite continues past a
   non-`PASS` scenario when `--fail-fast` is not given.
3. **`--fail-fast`** — running `suite-mixed.yaml` with `--fail-fast`,
   demonstrating early termination and `SKIPPED` entries in the report
   for scenarios never reached.

`examples/regression-suite-demo/README.md` documents the exact commands
and expected output, the same documentation discipline
`examples/regression-scenario-demo/README.md` already follows. No real
credentials, personal information, or customer data anywhere in the
directory.

## 25. Known limitations (anticipated up front)

- **No suite-level parallelism.** Scenarios run strictly sequentially;
  a suite with many slow scenarios takes as long as their sum.
- **No coupling to the incident library**, restated deliberately as still
  out of scope for this phase too — `docs/phase-4-plan.md` §6/§28 named
  "cross-referencing a scenario's `linked_fingerprint` against library
  occurrences" as a distinct, separate future increment; Phase 5 does not
  build it, to keep this phase's own review surface narrow. It remains a
  plausible, still-unbuilt Phase 6 candidate (§26).
- **No directory-walk/glob-based scenario discovery** — a suite lists
  scenarios explicitly; keeping every scenario a suite will run visible
  in the manifest's own text is treated as more valuable than convenience
  discovery, consistent with every other "no implicit resolution" design
  choice in this codebase.
- **No release gating**, restated as unconditional.
- **Fixed, non-configurable resource limits** (§10).
- **No new sandboxing** — every limitation `docs/regression-scenarios.md`
  already states about a single scenario's execution applies identically,
  per listed scenario, inside a suite.

## 26. Future release-gate integration boundary

Named explicitly, the same way `docs/phase-4-plan.md` §28 named this
phase's own boundary before it existed: Phase 5 makes `suite run`'s exit
code and aggregate JSON report *available* to be consumed by something
else, but IncidentDNA itself does not consume it. Plausible, deliberately
**unbuilt** future increments, named here rather than started:

- Cross-referencing a scenario's (or a suite's) `linked_fingerprint`
  set against `internal/library`'s stored occurrences — a pure read-only
  composition of already-existing, unmodified pieces, carried forward
  unbuilt from `docs/phase-4-plan.md` §6/§28.
- Parallel suite execution.
- Wiring a suite's aggregate result into an actual CI/CD gate or merge
  check — explicitly excluded from this phase and from every phase
  before it, restated rather than lifted.

None of this is claimed to be started, scoped in code, or implicitly
implied by anything in this plan.
