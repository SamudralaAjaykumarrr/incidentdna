# Scenario Suites

Phase 5 adds a small, reviewable, versioned manifest format — a **scenario
suite** (ISM v0.1, "Incident Scenario Manifest") — and a local, sequential,
offline runner that executes every scenario a manifest lists, once each, via
the unchanged Phase 4 runner, and aggregates the result. This document
describes what that runner is, what it guarantees, and what it does not.
`docs/phase-5-plan.md` is the approved plan this implements; this document
describes the resulting design as built, for the same audience
`docs/regression-scenarios.md` serves for Phase 4.

Implementation: `internal/suite/` (`types.go`, `load.go`, `validate.go`,
`run.go`, `report.go`, `limits.go`, `errors.go`) and
`cmd/incidentdna/cmd_suite.go`.

## What problem this closes

Phase 4 closed "memory is never exercised" for exactly one scenario at a
time: `incidentdna scenario run <scenario-file>` runs one declared command
and classifies the result. That was deliberately how Phase 4 was scoped
(`docs/phase-4-plan.md` §3: "No scenario discovery, aggregation, or 'run
all' command... deliberately deferred"), but it meant checking "does this
release still avoid every previously recorded failure I've written a
scenario for" required an operator to invoke `scenario run` once per
scenario file, by hand, and combine the results themselves. Phase 5 closes
this gap narrowly, the same way Phase 4 closed "no execution" narrowly: a
suite manifest lists scenarios explicitly, in order, and `incidentdna suite
run` executes each one (by calling `internal/scenario.Run` unchanged, once
per listed scenario) and aggregates the five-outcome classification each
already produces into one suite-level result.

Phase 5 does not change `idir.Document`, the JSON Schema,
`internal/validate`, `internal/canonical`, `internal/fingerprint`,
`internal/compare`, `internal/evidence`, `internal/library`, or
`internal/scenario`. `internal/suite` imports `internal/scenario` only —
`LoadFile`, `Validate`, and `Run`, all unchanged — and is imported by
nothing else in `internal/`; it does not import `internal/idir`,
`internal/validate`, `internal/fingerprint`, `internal/evidence`, or
`internal/library` directly, and it does not query `internal/library` at
all — a suite has no automatic coupling to the incident library, exactly as
a scenario's own `linked_fingerprint` does not.

## No new class of risk

`internal/suite` is the first package in this codebase that does not call
`exec.Command` at all, directly or indirectly through a new code path —
every process it ever launches is launched by `internal/scenario.Run`,
unchanged, under exactly the same "bounded, not sandboxed" model
`docs/regression-scenarios.md` already documents in full. A suite
orchestrates *multiple* already-reviewed scenarios rather than one, so the
reviewability requirement Phase 4 already established ("a reviewer who
approves a scenario file is approving exactly the command that will run")
extends here to: **a reviewer who approves a suite manifest is approving
exactly the ordered list of already-reviewed scenario files that will
run** — nothing about suite membership changes what an already-reviewed
scenario file does when executed. The one genuinely new consideration is
that a suite manifest can name scenario files a reviewer of the manifest
alone has not necessarily read; `suite verify` mitigates this the same way
`scenario verify --source` cross-checking does for Phase 4 — it prints
every listed scenario's path and its own validation result before
`suite run` ever executes anything, making that visible rather than hidden.

## The suite manifest format (ISM v0.1)

A suite manifest is a small, standalone YAML or JSON document, deliberately
not an extension of `scenario.Document` and not decoded through
`internal/scenario` at all — it describes an ordered *list* of scenarios to
run, not an execution. New, dedicated types in `internal/suite`, loaded
through a size-capped loader (`suite.LoadFile`, mirroring
`scenario.LoadFile`'s size-cap-then-typed-decode discipline, not reusing it
— the two document types are unrelated).

```yaml
schema_version: suite/v0.1

suite:
  id: suite-demo-mixed
  title: A mix of PASS, FAIL, and TIMEOUT scenario outcomes
  description: >
    Demonstrates the aggregate FAIL outcome.

scenarios:
  - path: scenarios/scenario-pass.yaml     # relative to this manifest's own directory
  - path: scenarios/scenario-fail.yaml
  - path: scenarios/scenario-timeout.yaml
```

Field notes:

- **`schema_version`** — fixed literal `suite/v0.1`; checked the same way
  `scenario.Document.SchemaVersion` is: exact match, reject anything else.
- **`suite.id`** — required, non-empty, stable, author-chosen identifier;
  used only for human/report readability, never as a filesystem path
  component (there is no store, so nothing to key by it).
- **`scenarios`** — a required, non-empty, ordered list, bounded by
  `MaxScenariosPerSuite`. Each entry's `path` is resolved relative to the
  manifest's own directory; rejected if absolute, containing `..`, or
  resolving (after `filepath.Clean`) outside that directory. A `path` that
  is a symlink is rejected, not followed. Duplicate `path` values are
  rejected as invalid — a suite naming the same scenario file twice is
  almost certainly an authoring mistake, not a meaningful "run it twice"
  request.
- **No suite-level `execution` block, `env`, or `timeout_seconds`
  override** — each listed scenario's own `execution.timeout_seconds` (or
  its default) governs that scenario's own run, unchanged; a suite does not
  re-parameterize the scenarios it lists. A scenario file's meaning is
  identical whether run standalone (`scenario run`) or as part of a suite
  (`suite run`).

## The two commands

```
incidentdna suite verify <suite-file>
incidentdna suite run [--workspace-root <dir>] [--report <file>]
                       [--keep-workspaces] [--fail-fast] <suite-file>
```

**`suite verify <suite-file>`** — loads and structurally/semantically
validates `<suite-file>` against every ISM v0.1 rule above, then loads and
validates every listed scenario file against the existing IRS v0.1 rules
(`scenario.Validate`, unchanged). **Never executes anything.** Prints one
`[OK]`/`[FAIL]` line per listed scenario before the overall result, so a
reviewer sees exactly which scenario files a suite will run, and whether
each is itself valid, before ever running `suite run`.

**`suite run <suite-file>`** — runs the same pre-execution checks
`suite verify` performs. If they pass, runs each listed scenario in
declared order via `scenario.Run` (unchanged), aggregates the outcomes,
prints a per-scenario progress line and a summary line, and (with
`--report <file>`) writes the aggregate outcome as one deterministic JSON
report.

Sample transcript (see `examples/regression-suite-demo/` for the full,
runnable version):

```
$ incidentdna suite verify examples/regression-suite-demo/suite-mixed.yaml
Suite: suite-demo-mixed (schema suite/v0.1)
Scenarios: 3 listed
  [OK] suite-demo-pass (scenarios/scenario-pass.yaml)
  [OK] suite-demo-fail (scenarios/scenario-fail.yaml)
  [OK] suite-demo-timeout (scenarios/scenario-timeout.yaml)
OK: suite manifest and all 3 listed scenarios are structurally valid

$ incidentdna suite run examples/regression-suite-demo/suite-mixed.yaml
Suite: suite-demo-mixed
[1/3] suite-demo-pass ... PASS
[2/3] suite-demo-fail ... FAIL
[3/3] suite-demo-timeout ... TIMEOUT
Result: FAIL (1 PASS, 1 FAIL, 1 TIMEOUT, 0 INVALID, 0 INTERNAL_ERROR)
Duration: 1.01s
```

### Exit codes

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
| Aggregate `FAIL` | `2` | At least one listed scenario's outcome was `FAIL`, `TIMEOUT`, `INTERNAL_ERROR`, or (with `--fail-fast`) `SKIPPED` |
| Aggregate `INVALID` | `1` | The suite manifest itself, or any listed scenario, failed pre-execution checks — nothing was ever executed |
| Aggregate `INTERNAL_ERROR` | `1` | An environmental failure prevented determining the aggregate result (e.g. `--report` path unwritable, `--workspace-root` pre-existing and non-empty) |

This reuses, rather than reinvents, the same exit-code space every prior
phase already established: `2` means "well-formed input, materially
negative finding," `1` means "environmental/usage failure."

## Execution isolation boundaries

Inherited unchanged, per listed scenario, from `docs/regression-scenarios.md`
— no shell interpretation, no implicit `$PATH` lookup, workspace
containment, fixed child environment, process-group timeout kill, bounded
stream-separated output capture. `internal/suite` adds exactly these new
boundaries of its own:

| Boundary | Mechanism | Guarantee level |
|---|---|---|
| Suite manifest path resolution | Every listed `scenarios[].path` is `filepath.Clean`ed and re-checked to have the manifest's own directory as a prefix before `scenario.LoadFile` is ever called on it; a symlink at that path is rejected, not followed | Structural |
| One workspace per scenario, never shared | Each scenario in a suite gets its own fresh workspace (`os.MkdirTemp`, or a named subdirectory under `--workspace-root` if given), created and torn down exactly as it would be for a standalone `scenario run` — never reused across scenarios in the same suite run | Structural |
| Suite-level total timeout | `suite.Validate` rejects, at verify time, any suite whose summed listed-scenario `timeout_seconds` exceeds `MaxSuiteTotalTimeoutSeconds` — before any scenario ever runs. `suite run`'s CLI layer additionally wraps the whole run in a `context.WithTimeout` bounded by that same fixed constant, as a backstop | Structural |

## Resource limits

Three independent, fixed constants (`internal/suite/limits.go`), each with
its own distinct error and its own passing/failing boundary test pair:

| Constant | Value | Applies to |
|---|---|---|
| `MaxSuiteDocumentSize` | 256 KiB | The suite manifest file's own on-disk size |
| `MaxScenariosPerSuite` | 100 | Number of `scenarios[]` entries a manifest may declare |
| `MaxSuiteTotalTimeoutSeconds` | 1800 (30 min) | Sum of declared/default `timeout_seconds` across every listed scenario; checked at verify time |

`MaxScenarioDocumentSize`, `MaxWorkspaceFiles`, `MaxWorkspaceFileSize`,
`MaxWorkspaceTotalBytes`, `MaxScenarioOutputBytes`, and the per-scenario
timeout bounds are all inherited unchanged, per listed scenario, from
`internal/scenario/limits.go` — Phase 5 does not redefine or widen any of
them.

`suite run` is **not** run under `main.go`'s usual 30-second overall command
context — that timeout was sized for parsing/hashing a document. `suite
run` instead uses its own context, bounded by `suite.MaxSuiteTotalTimeoutSeconds`
(a fixed backstop, not a per-suite computed budget: `suite.Validate` already
rejects any suite whose summed scenario timeouts exceed that same constant
before anything executes, so using it directly as the context's upper bound
is always safe). `suite verify` (which never executes anything) continues
to run under the existing 30-second command context, unchanged — `main.go`
special-cases the `suite` command's dispatch to skip its usual context
wrapping, and `runSuite` applies the 30-second budget itself only for the
`verify` subcommand.

## Result and report format

Two outputs, always produced together by `suite run`:

1. **Human-readable stdout summary** (always printed) — a `Suite:` line, one
   `[i/N] <scenario-id> ... <OUTCOME>` progress line per listed scenario,
   and a `Result:` summary line with per-outcome counts.
2. **Machine-readable JSON report**, written to `--report <path>` only if
   given; deterministic field order via a fixed Go struct (no map
   iteration), one file per run, **overwritten unconditionally** on each run:

```json
{
  "schema_version": "suite-report/v0.1",
  "suite_id": "suite-demo-mixed",
  "suite_file": "examples/regression-suite-demo/suite-mixed.yaml",
  "result": "FAIL",
  "counts": {"pass": 1, "fail": 1, "timeout": 1, "invalid": 0, "internal_error": 0, "skipped": 0},
  "duration_ms": 1013,
  "scenarios": [
    {"path": "scenarios/scenario-pass.yaml", "report": { /* full scenario.Report, unchanged shape */ }},
    {"path": "scenarios/scenario-fail.yaml", "report": { /* ... */ }},
    {"path": "scenarios/scenario-timeout.yaml", "report": { /* ... */ }}
  ],
  "error": null
}
```

Each `scenarios[].report` is exactly the JSON object `scenario.BuildReport`
already produces for that scenario, unchanged — the suite report is a
deterministic aggregation, not a reinterpretation, of Phase 4's existing
per-scenario report shape. A scenario recorded `SKIPPED` (because
`--fail-fast` stopped the suite before reaching it) has a `null` `report`,
since it was never executed. `--report` overwrites unconditionally on each
run, matching `scenario run --report`'s existing behavior.

## Determinism and aggregate classification

1. **Suite execution order is exactly the declared list order** — never
   alphabetical, never filesystem-iteration order. Reruns of the same
   manifest against an unchanged environment execute scenarios in the
   identical order.
2. **Each scenario's own execution is exactly as deterministic as
   `scenario run` already is** — `suite.Run` introduces no new source of
   nondeterminism around a scenario's own workspace, environment, or output
   capture, because it calls `scenario.Run` unchanged.
3. **A suite manifest's own validity depends only on its own bytes and the
   bytes of the scenario files it lists** — never on prior runs or
   wall-clock time.
4. **Aggregate classification is a pure function of the per-scenario
   outcomes**: aggregate `PASS` only if every listed scenario's own outcome
   was `PASS`; aggregate `FAIL` otherwise (a `FAIL`, `TIMEOUT`,
   `INTERNAL_ERROR`, or `SKIPPED` scenario outcome all count as a
   materially negative finding for the suite as a whole) — with no
   suite-level judgment call beyond "did every scenario PASS."

## Idempotency behavior

Identical in kind to `scenario run`'s existing idempotency statement: no
persistent side effects on the repository or any store; a fresh set of
workspaces every run; `--report`, if given, is overwritten each run, never
appended or versioned; the aggregate result is repeatable only to the
extent every listed scenario's own declared command is itself
deterministic.

## Corruption and malformed-manifest handling

- **Suite manifest fails to parse** (bad YAML/JSON, exceeds
  `MaxSuiteDocumentSize`): `suite verify` exits `1`; `suite run` reports
  aggregate `INTERNAL_ERROR` and exits `1`.
- **Manifest decodes but fails a semantic rule** (missing/wrong
  `schema_version`, empty `suite.id`, empty `scenarios`, a `path` with a
  traversal/absolute/symlinked/duplicate value, declared scenario count
  exceeding `MaxScenariosPerSuite`, summed timeouts exceeding
  `MaxSuiteTotalTimeoutSeconds`, or **any one listed scenario itself failing
  `scenario.Validate`**): `suite verify` exits `2`; `suite run` reports
  aggregate `INVALID` and exits `1` — nothing is ever executed. If any one
  listed scenario is itself invalid, the suite run does not partially
  execute the valid ones and skip the invalid one silently — the entire
  suite is rejected at verify time, so a reviewer's approval of "this suite
  is valid" always means "every scenario in it is valid," never "most of
  them are." See `examples/regression-suite-demo/README.md`,
  "Demonstrating aggregate INVALID," for a runnable example.
- **A listed scenario file is missing at verify/run time**: reported
  distinctly (`ErrSuiteScenarioMissing`), caught at the pre-execution check
  in the common case (aggregate `INVALID`), or `INTERNAL_ERROR` if a TOCTOU
  race caused it to disappear between check and load — the same documented
  residual gap class Phase 4 already accepted for `workspace_files`.
- **A listed scenario itself fails mid-suite for an environmental reason**
  (its own `INTERNAL_ERROR`, e.g. its `command[0]` disappeared between
  `suite verify` and `suite run`): recorded as that scenario's own
  `INTERNAL_ERROR` in the aggregate counts and report; the suite continues
  to the next listed scenario unless `--fail-fast` was given, and the
  suite's own aggregate result is `FAIL` — one scenario's environmental
  failure does not abort the whole suite's report generation.
- **A pre-existing, non-empty `--workspace-root <dir>`**: refused
  immediately, aggregate `INTERNAL_ERROR`, exit `1` — the runner never
  writes into a directory it did not create empty.
- **`--report` cannot be written**: reported to stderr, exit `1`, even if
  every listed scenario PASSed — the per-scenario and summary lines are
  still printed to stdout first, mirroring `scenario run`'s existing
  behavior exactly.

## `--fail-fast`

By default, `suite run` executes every listed scenario regardless of
earlier outcomes. `--fail-fast` stops running further scenarios immediately
after the first non-`PASS` outcome; every scenario never reached is
recorded `SKIPPED` in both the printed summary and the JSON report (with a
`null` embedded `report`), rather than silently omitted — so a suite report
always accounts for every listed scenario, whether it ran, passed, failed,
or was skipped.

## Library cross-reference (Phase 6)

`suite verify` accepts an optional `--library <dir>` flag: after the
existing checks pass, it looks up every *distinct* `linked_fingerprint`
among the listed scenarios (in first-occurrence declared order) against the
named (or default) incident library, annotates each scenario's own summary
line, and prints a one-line aggregate count — purely informational, never
affecting `suite verify`'s own exit code, and never touching `suite run` at
all.

```
$ incidentdna suite verify --library .incidentdna/library/objects \
    examples/regression-suite-demo/suite-mixed.yaml
Suite: suite-demo-mixed (schema suite/v0.1)
Scenarios: 3 listed
  [OK] suite-demo-pass (scenarios/scenario-pass.yaml) — library: 1 occurrence(s)
  [OK] suite-demo-fail (scenarios/scenario-fail.yaml) — library: 1 occurrence(s)
  [OK] suite-demo-timeout (scenarios/scenario-timeout.yaml) — library: 1 occurrence(s)
Library cross-reference: 1 of 1 distinct linked fingerprint(s) have library occurrences
OK: suite manifest and all 3 listed scenarios are structurally valid
```

All three of `suite-mixed.yaml`'s listed scenarios happen to share the same
checked-in `linked_fingerprint`, so this transcript directly demonstrates
deduplication: three listed scenarios sharing one fingerprint trigger
exactly **one** library lookup, not three — the aggregate line counts
**distinct** fingerprints. This
bounds `suite verify --library`'s library I/O to at most one lookup per
distinct fingerprint, never per scenario entry, and is bounded overall by
the existing `MaxScenariosPerSuite` (100). `--library` omitted is
byte-for-byte identical to pre-Phase-6 output. This is implemented entirely
at the `cmd/incidentdna` layer, calling the new `library.CheckFingerprint`
directly, once per distinct fingerprint — `internal/suite` itself is not
modified and gains no new import. See
[`library-crossref.md`](library-crossref.md) for the full design, including
the `scenario verify --library` equivalent.

## Privacy implications

Identical in kind to Phase 4's own statement: a suite manifest carries no
dedicated privacy/redaction block (it is short-lived, reviewable execution
metadata, not a permanent incident record); the aggregate report's embedded
per-scenario reports are exactly as bounded (`MaxScenarioOutputBytes` per
stream, per scenario) as a standalone scenario report already is; nothing
scans manifest or report content for sensitive material; the child
processes' own privacy behavior remains entirely outside this tool's
control, restated rather than newly introduced.

## What Phase 5 explicitly does not provide

- **No release gating.** `suite run`'s exit code and JSON report are
  *available* to be consumed by something else, but `incidentdna` itself
  does not wire any suite result into a gate, a policy decision, a merge
  check, or any other blocking mechanism.
- **No scenario discovery via directory walk or glob.** A suite manifest
  lists scenarios explicitly, by declared relative path; `incidentdna`
  itself never walks a directory tree looking for scenario files.
- **No parallel execution.** Scenarios in a suite run strictly sequentially,
  in declared order.
- **No automatic coupling to the incident library.** A suite manifest and
  its runner do not query `internal/library` as part of `suite run`, and
  `internal/suite` itself still does not import or query `internal/library`.
  As of Phase 6, `suite verify --library` offers an optional, read-only,
  informational lookup for this — see "Library cross-reference (Phase 6)"
  above — but it remains `verify`-only, non-gating, and implemented
  entirely outside `internal/suite`.
- **No new process-execution primitive.** `internal/suite` never calls
  `exec.Command` itself; it calls `internal/scenario.Run` once per listed
  scenario, unchanged.
- **No suite-level timeout independent of its scenarios' own timeouts** —
  the fixed `MaxSuiteTotalTimeoutSeconds` backstop is the only suite-level
  time bound, not a separately-configurable suite timeout.
- **No mutation of any existing store or document.** `internal/suite` does
  not write into `.incidentdna/evidence`, `.incidentdna/library`, or any
  scenario file; its only durable, optional output is a caller-named
  `--report <file>`.
- **No new sandboxing** — every limitation `docs/regression-scenarios.md`
  already states about a single scenario's execution applies identically,
  per listed scenario, inside a suite.
- **Fixed, non-configurable resource limits.**

None of these are claimed to be solved, partially solved, or planned for a
specific near-term release — they are simply out of scope for Phase 5, the
same way analogous items were out of scope for Phases 1 through 4.

## The `regression-suite-demo` example

`examples/regression-suite-demo/` is a fifth, self-contained example,
entirely separate from `examples/duplicate-payment/`,
`examples/evidence-storage-demo/`, `examples/incident-library-demo/`, and
`examples/regression-scenario-demo/`. Two suite manifests
(`suite-all-pass.yaml`, `suite-mixed.yaml`) list scenario files under
`scenarios/`, which in turn stage a frozen copy (not a symlink or
reference) of `examples/duplicate-payment/incident.yaml`'s content from
`scenarios/fixtures/incident.yaml`. See
`examples/regression-suite-demo/README.md` for the exact commands, expected
output, and why `fixtures/` is nested under `scenarios/` rather than a
sibling of it. No real credentials, personal information, or customer data
appear anywhere in the directory.

## Current Phase 5 status and remaining limitations

Implemented, per `internal/suite/` and `cmd/incidentdna/cmd_suite.go`: ISM
v0.1 document loading and semantic validation (including per-listed-scenario
validation via unchanged `internal/scenario` calls), the bounded sequential
runner (aggregate `PASS`/`FAIL`/`INVALID`/`INTERNAL_ERROR`, `--fail-fast`
short-circuiting), and deterministic JSON aggregate report generation. This
document describes that implementation as it exists in the code today.

Known, accepted limitations, restated from the sections above so they are
findable in one place:

- No parallel execution; a suite with many slow scenarios takes as long as
  their sum.
- No coupling to the incident library.
- No directory-walk/glob-based scenario discovery.
- No release gating.
- Not a sandbox — every Phase 4 limitation applies identically, per listed
  scenario.
- Fixed, non-configurable resource limits.

This document intentionally does not claim CI results, production usage, or
performance benchmarks for this phase beyond what is recorded in
`docs/phase-5-report.md`.
