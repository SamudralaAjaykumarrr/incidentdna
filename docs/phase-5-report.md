# Phase 5 Report: Scenario Suites

This report documents the state of Phase 5 as implemented in the working
tree of branch `phase-5-scenario-suites`, after the final verification pass
described below. It follows the same role `docs/phase-4-report.md` served
for Phase 4: a record of what was built, how it was verified, and what is
explicitly not yet true. `docs/phase-5-plan.md` is the approved plan this
implements.

## 1. Phase 5 goals

Phase 4 closed "memory is never exercised" for exactly one scenario at a
time: `incidentdna scenario run <scenario-file>` runs one declared command
and classifies the result, with no discovery, aggregation, or "run all"
command — deliberately deferred (`docs/phase-4-plan.md` §3/§27/§28). Phase
5's entire scope is closing exactly that gap, and nothing else:

1. A new `internal/suite` package: ISM v0.1 (Incident Scenario Manifest)
   document types, a size-capped loader, a semantic rule engine (including
   per-listed-scenario validation via the unchanged Phase 4
   `internal/scenario` package), a local sequential runner, and
   deterministic JSON aggregate report generation.
2. Two CLI subcommands (`incidentdna suite verify|run`).
3. Fixed resource limits (three, independently enforced), and the
   filesystem-security properties (path-traversal safety, symlink
   rejection) `internal/evidence`, `internal/library`, and
   `internal/scenario` already established, extended to declared
   `scenarios[].path` entries.
4. **No new class of risk**: `internal/suite` is the first package in this
   codebase that never calls `exec.Command` itself, directly or indirectly
   — every process a suite launches is launched by the unchanged Phase 4
   `internal/scenario.Run`, once per listed scenario.
5. Documentation updates so `docs/scenario-suites.md`, `README.md`,
   `docs/architecture.md`, `docs/product-scope.md`, `docs/threat-model.md`,
   and `docs/privacy-model.md` accurately describe what Phase 5 covers.
6. A Makefile/CI verification path exercising the above end-to-end.

Explicitly not part of Phase 5 (see `docs/phase-5-plan.md` §3/§25/§26 for
the full list): directory-walk or glob-based scenario discovery, parallel
execution, coupling to the incident library, release gating or CI/CD
blocking, a new sandbox, or any change to `idir.Document`,
`internal/validate`, `internal/canonical`, `internal/fingerprint`,
`internal/compare`, `internal/evidence`, `internal/library`, or
`internal/scenario`.

## 2. Implementation summary

`internal/suite` is a new, fifth parallel leaf package, one level further
from the shared primitives than `internal/scenario` itself: it imports only
`internal/scenario` (`LoadFile`, `Validate`, and `Run`, all unchanged), and
is imported by nothing else in `internal/`.

```
ISM v0.1 suite manifest (suite verify/run)
   |  suite.LoadFile — size cap, format sniff, typed decode
   v
suite.Document
   |  suite.Validate — schema_version, non-empty scenarios, path safety,
   |  duplicate-path rejection, MaxScenariosPerSuite, aggregate timeout
   |  bound — and, per listed scenario, scenario.LoadFile + scenario.Validate
   v
(valid) suite.Document, every listed scenario also valid
   |  suite.Run — sequential, declared-order iteration
   v
   for each listed scenario: scenario.LoadFile + scenario.Run (unchanged,
   one fresh workspace each) -> PASS/FAIL/TIMEOUT/INVALID/INTERNAL_ERROR
   |
   v
aggregate PASS (every scenario PASSed) or FAIL (otherwise)
   |  suite.BuildReport + suite.WriteReport (--report only)
   v
deterministic JSON aggregate report, embedding each executed scenario's own
unmodified scenario.Report
```

`cmd/incidentdna/cmd_suite.go` adds a single new top-level command,
`suite`, registered in `main.go`'s `commands` table, self-dispatching to
`verify`/`run` using the exact `flag.NewFlagSet` idiom every existing
subcommand already established — no CLI framework introduced. `main.go`'s
existing `"scenario"`-only context-timeout special-case is extended to also
exclude `"suite"`: `suite run`'s workload (potentially many bounded
child-process executions in sequence) needs its own budget, so `main.go`
skips the fixed 30-second `commandTimeout` wrap for it, and `runSuite`
re-applies that same budget itself for the `verify` subcommand, which never
executes anything — the identical pattern `runScenario` already established
in Phase 4.

Full design writeup: `docs/scenario-suites.md`.

## 3. Architecture and the ISM v0.1 format

A suite manifest is deliberately not an extension of `scenario.Document`
and is not decoded through `internal/scenario` — it describes an ordered
*list* of scenarios, not an execution. Its top-level fields:
`schema_version` (fixed `suite/v0.1`), `suite.id` (required, non-empty),
`suite.title`/`suite.description` (optional, human-readable only), and
`scenarios` (required, non-empty, ordered list of `{path}` entries, bounded
by `MaxScenariosPerSuite`). There is deliberately no suite-level
`execution`/`env`/`timeout_seconds` override — each listed scenario's own
declared timeout governs its own run, unchanged, so a scenario file's
meaning is identical whether run standalone or as part of a suite.

`scenarios[].path` is resolved relative to the suite manifest's own
directory, `filepath.Clean`ed and rejected if absolute, containing `..`, or
resolving outside that directory; a `path` that is a symlink is rejected,
not followed — the identical discipline `execution.workspace_files` already
established in Phase 4, applied here to a declared scenario-file path
instead. Duplicate `path` values are rejected as invalid (proven by
`TestValidate_RejectsDuplicateScenarioPath`).

## 4. Determinism, aggregation, and safety model

`internal/suite/run.go` implements the runner:

- **No new process-execution primitive.** `internal/suite` never calls
  `exec.Command`; `suite.Run` calls `scenario.LoadFile` +  `scenario.Run`
  once per listed scenario, unchanged.
- **Declared-order, sequential execution.** Scenarios run strictly in the
  order they appear in the manifest, never reordered, never parallel.
  Verified directly by `TestRun_DeclaredOrderExecution`, which asserts a
  shared marker file's append order matches declared order exactly.
- **One fresh workspace per scenario, never shared.** Each scenario gets
  its own workspace — an independent `os.MkdirTemp` by default, or its own
  subdirectory under a shared `--workspace-root` if given — created and
  torn down exactly as a standalone `scenario run` would. Verified by
  `TestRun_WorkspaceIsolationAcrossScenariosInSameSuiteRun`, which has two
  scenarios each write a same-named relative file and confirms neither
  overwrites the other.
- **Aggregate classification is a pure function of the per-scenario
  outcomes.** Aggregate `PASS` only if every listed scenario's own outcome
  is `PASS`; `FAIL` otherwise (`FAIL`, `TIMEOUT`, `INTERNAL_ERROR`, or
  `SKIPPED` all count as a materially negative finding for the suite as a
  whole) — no suite-level judgment call beyond "did every scenario PASS."
  Verified independently for each non-PASS outcome by
  `TestRun_AggregateFail_MixedOutcomesIndependently`.
- **`--fail-fast` short-circuits without executing skipped scenarios.**
  Verified by `TestRun_FailFast_SkipsLaterScenariosWithoutExecutingThem`,
  which uses the same "marker file would have been created" technique
  Phase 4's own `INVALID` test uses, confirming the marker is absent.
- **A suite listing even one structurally invalid scenario is rejected as
  a whole, at verify time.** `suite.Validate` calls `scenario.Validate` on
  every resolved listed scenario; if any one fails, the whole suite is
  `INVALID` and `suite.Run` executes nothing — proven by
  `TestRun_AggregateInvalid_NothingExecutes`, and demonstrated end-to-end
  by `scripts/verify-suite-demo.sh` (§10).

## 5. The two commands and exit-code contract

```
incidentdna suite verify <suite-file>
incidentdna suite run [--workspace-root <dir>] [--report <file>] [--keep-workspaces] [--fail-fast] <suite-file>
```

**`suite verify`**: `0` valid (and every listed scenario valid); `1`
I/O/parse/usage error; `2` semantic rule failure, or any listed scenario
itself failing `scenario.Validate`.

**`suite run`**: aggregate `PASS` → `0`; aggregate `FAIL` → `2`; aggregate
`INVALID`/`INTERNAL_ERROR` → `1` — mirroring the existing project-wide
convention that `2` means "well-formed input, materially negative finding"
and `1` means "environmental/usage failure," reused for a *different
specific condition* per command rather than a new exit-code space.

## 6. Resource limits

Three independent, fixed constants (`internal/suite/limits.go`), each with
its own distinct error and its own passing/failing boundary test pair:

| Constant | Value | Applies to |
|---|---|---|
| `MaxSuiteDocumentSize` | 256 KiB | The suite manifest file's own on-disk size |
| `MaxScenariosPerSuite` | 100 | Number of `scenarios[]` entries a manifest may declare |
| `MaxSuiteTotalTimeoutSeconds` | 1800 (30 min) | Sum of declared/default `timeout_seconds` across every listed scenario, checked at verify time |

All three are fixed, not user-configurable, consistent with Phases 2
through 4's resource-limit design. Every per-scenario limit from
`internal/scenario/limits.go` (document size, workspace file
count/size/aggregate size, output capture size, timeout bounds) is
inherited unchanged, per listed scenario — Phase 5 does not redefine or
widen any of them.

## 7. Corruption and malformed-manifest handling

- A suite manifest that fails to parse, or exceeds `MaxSuiteDocumentSize`:
  `suite verify` exits `1`; `suite run` reports aggregate `INTERNAL_ERROR`
  and exits `1`.
- A manifest that decodes but fails a semantic rule (missing/wrong
  `schema_version`, empty `suite.id`, empty `scenarios`, a `path` with a
  traversal/absolute/symlinked/duplicate value, declared scenario count
  exceeding `MaxScenariosPerSuite`, summed timeouts exceeding
  `MaxSuiteTotalTimeoutSeconds`, or any one listed scenario itself failing
  `scenario.Validate`): `suite verify` exits `2`; `suite run` reports
  aggregate `INVALID` and exits `1` — nothing is ever executed.
- A listed scenario file missing at verify/run time: reported distinctly
  (`ErrSuiteScenarioMissing`).
- A pre-existing, non-empty `--workspace-root <dir>`: refused immediately,
  aggregate `INTERNAL_ERROR`, exit `1`.
- The `--report` path cannot be written: reported to stderr, exit `1`, even
  if every listed scenario PASSed — the summary is still printed to stdout
  first.

## 8. Result and report format

`suite run` always prints a human-readable stdout summary (a `Suite:`
line, one `[i/N] <scenario-id> ... <OUTCOME>` progress line per listed
scenario, and a `Result:` line with per-outcome counts), and (with
`--report <path>`) writes a deterministic JSON aggregate report — fixed Go
struct, no map iteration, so field order never varies across repeated
marshals of the same value (`TestReport_DeterministicFieldOrderAcrossMarshals`
asserts both byte-identical repeated marshals and the exact documented
field order). Each `scenarios[].report` is exactly the JSON object
`scenario.BuildReport` already produces for that scenario, unchanged
(`TestBuildReport_CountsAndEmbeddedScenarioReports`); a `SKIPPED` scenario's
`report` field is `null`. `--report` overwrites unconditionally on each run
(`TestWriteReport_OverwritesOnRepeatedRuns`).

## 9. Test coverage

- **`internal/suite`**: 38 top-level `Test*` functions across 5 test files
  (`validate_test.go` 12, `run_test.go` 10, `load_test.go` 7,
  `report_test.go` 6, `limits_test.go` 3 — `validate_test.go`'s
  `TestValidate_RejectsPathTraversal` additionally runs 4 table-driven
  subtests), table-driven, real filesystem via `t.TempDir()`, real child
  processes via `/bin/true`/`/bin/false`/`/bin/sleep`/`/bin/sh` (never the
  built `incidentdna` binary, per `docs/phase-5-plan.md` §6's guidance).
  Covers: every semantic validation rule including per-listed-scenario
  validation and duplicate/traversal/symlink rejection; aggregate
  `PASS`/`FAIL` from every combination of per-scenario outcomes
  independently; declared-order execution; `--fail-fast` short-circuiting
  (with proof that skipped scenarios never execute); workspace isolation
  across scenarios in the same suite run; aggregate `INVALID` (nothing
  executes) and aggregate `INTERNAL_ERROR` (pre-existing non-empty
  `--workspace-root`); deterministic report field order and overwrite
  behavior; and a passing/failing boundary test pair for each of the three
  resource limits.
- **`cmd/incidentdna/cli_suite_test.go`**: 19 top-level `Test*` functions,
  black-box against the real compiled binary, the same style
  `cli_scenario_test.go` already uses — `suite verify` exit 0/1/2 paths
  including a listed-invalid-scenario case; `suite run` exit 0/1/2 across
  aggregate PASS/FAIL/INVALID/INTERNAL_ERROR; `--report` file contents
  match the printed summary and embed each scenario's own report;
  `--keep-workspaces`/`--workspace-root` leave real, inspectable
  per-scenario directories; a pre-existing non-empty `--workspace-root` is
  refused; `--fail-fast` behavior; a suite whose listed scenario points at
  the real `bin/incidentdna` binary, demonstrating the same self-referential
  regression check the Phase 4 demo already uses; help text and
  unknown/missing-subcommand handling.
- **Race**: `go test ./... -race -count=1` is green across all 10 packages,
  `internal/suite` included.
- **Regression discipline**: the full existing suite (81 pre-existing
  top-level tests across `cmd/incidentdna` and 8 `internal/*` packages)
  passed unmodified; the golden fingerprint test/script produced the
  unchanged Phase 1 value.

## 10. Demo coverage

`examples/regression-suite-demo/` (`suite-all-pass.yaml`, `suite-mixed.yaml`,
`scenarios/scenario-{pass,trivial-pass,fail,timeout,invalid}.yaml`, plus
`scenarios/fixtures/incident.yaml`, a frozen copy — not a symlink — of
`examples/duplicate-payment/incident.yaml`'s content) demonstrates both
aggregate outcomes, `--fail-fast`, aggregate `INVALID`, and
`--keep-workspaces`/`--workspace-root` end to end via
`scripts/verify-suite-demo.sh`.

**A deliberate, documented deviation from `docs/phase-5-plan.md` §22's
literal file-tree diagram**: the plan illustrates `fixtures/incident.yaml`
as a sibling of `scenarios/`, but an IRS v0.1 scenario's
`workspace_files[].source` is resolved relative to *that scenario file's
own directory* and is rejected outright if it contains `".."`
(`internal/scenario`'s existing, unmodified path-safety rule). A scenario
file living in `scenarios/` cannot reference `../fixtures/...` without
tripping that rule, so the fixture is nested at
`scenarios/fixtures/incident.yaml` instead — the same shape
`examples/regression-scenario-demo/fixtures/incident.yaml` already uses
relative to its own scenario files. This is documented in
`examples/regression-suite-demo/README.md`, "Why `fixtures/` is nested
under `scenarios/`, not a sibling of it."

**A second judgment call, resolving an internal inconsistency in the
approved plan**: `docs/phase-5-plan.md` §4 Workflow B's illustrative
transcript shows one suite executing all four Phase-4-style outcomes
(`PASS`/`FAIL`/`TIMEOUT`/`INVALID`) in a single successful `suite run`, but
§17 states explicitly and in detail that "if any one listed scenario is
itself invalid, the suite run does not partially execute the valid ones and
skip the invalid one silently — the entire suite is rejected at verify
time." These two statements cannot both be literally true: a scenario that
is genuinely `INVALID` (fails `scenario.Validate`, e.g. a bare `command[0]`)
makes the *whole* suite `INVALID` at verify time under §17's rule, so it can
never appear as one outcome alongside `PASS`/`FAIL`/`TIMEOUT` inside a
suite that otherwise executes. This implementation follows §17 (the more
detailed, acceptance-criteria-grade section, and the one that defines the
exit-code contract), and demonstrates it directly: `suite-mixed.yaml` uses
only `PASS`/`FAIL`/`TIMEOUT` (all structurally valid scenarios that differ
only in runtime outcome), and a separate, dedicated demonstration (a
throwaway manifest generated by `scripts/verify-suite-demo.sh` at
verification time, listing only `scenarios/scenario-invalid.yaml`) proves
the §17 behavior: `suite verify` exits `2`, `suite run` reports aggregate
`INVALID` and exits `1`, having executed nothing. See
`examples/regression-suite-demo/README.md`, "Demonstrating aggregate
INVALID," and `docs/scenario-suites.md`'s corruption-handling section (which
documents §17's rule, not §4's transcript, as the implemented behavior).

Real transcript from this verification pass (`scripts/verify-suite-demo.sh`,
run standalone and via `make example-suite`/`make verify` inside the Docker
dev container):

```
verify-suite-demo: suite verify suite-all-pass.yaml (expect: valid, exit 0)
Suite: suite-demo-all-pass (schema suite/v0.1)
Scenarios: 2 listed
  [OK] suite-demo-pass (scenarios/scenario-pass.yaml)
  [OK] suite-demo-trivial-pass (scenarios/scenario-trivial-pass.yaml)
OK: suite manifest and all 2 listed scenarios are structurally valid

verify-suite-demo: suite verify suite-mixed.yaml (expect: valid, exit 0)
Suite: suite-demo-mixed (schema suite/v0.1)
Scenarios: 3 listed
  [OK] suite-demo-pass (scenarios/scenario-pass.yaml)
  [OK] suite-demo-fail (scenarios/scenario-fail.yaml)
  [OK] suite-demo-timeout (scenarios/scenario-timeout.yaml)
OK: suite manifest and all 3 listed scenarios are structurally valid

verify-suite-demo: suite run suite-all-pass.yaml (expect: aggregate PASS, exit 0)
Suite: suite-demo-all-pass
[1/2] suite-demo-pass ... PASS
[2/2] suite-demo-trivial-pass ... PASS
Result: PASS (2 PASS, 0 FAIL, 0 TIMEOUT, 0 INVALID, 0 INTERNAL_ERROR)
Duration: 9ms

verify-suite-demo: suite run suite-mixed.yaml (expect: aggregate FAIL, exit 2)
Suite: suite-demo-mixed
[1/3] suite-demo-pass ... PASS
[2/3] suite-demo-fail ... FAIL
[3/3] suite-demo-timeout ... TIMEOUT
Result: FAIL (1 PASS, 1 FAIL, 1 TIMEOUT, 0 INVALID, 0 INTERNAL_ERROR)
Duration: 1.01s

verify-suite-demo: suite run --fail-fast suite-mixed.yaml (expect: aggregate FAIL, exit 2, one SKIPPED)
Suite: suite-demo-mixed
[1/3] suite-demo-pass ... PASS
[2/3] suite-demo-fail ... FAIL
[3/3] scenarios/scenario-timeout.yaml ... SKIPPED
Result: FAIL (1 PASS, 1 FAIL, 0 TIMEOUT, 0 INVALID, 0 INTERNAL_ERROR, 1 SKIPPED)
Duration: 12ms

verify-suite-demo: suite verify suite-invalid-listed.yaml (expect: exit 2)
Suite: suite-demo-invalid-listed (schema suite/v0.1)
Scenarios: 1 listed
  [FAIL] suite-demo-invalid (scenario-invalid.yaml)
incidentdna: .../suite-invalid-listed.yaml is not a valid ISM v0.1 suite:
  - scenarios[0]: scenario is invalid: execution.command[0]: must be an absolute path ...

verify-suite-demo: suite run suite-invalid-listed.yaml (expect: aggregate INVALID, exit 1, nothing executed)
Suite: suite-demo-invalid-listed
Result: INVALID (0 PASS, 0 FAIL, 0 TIMEOUT, 0 INVALID, 0 INTERNAL_ERROR)
Duration: 0s
Error: scenarios[0]: scenario is invalid: execution.command[0]: must be an absolute path ...

verify-suite-demo: suite run --workspace-root --keep-workspaces (expect: per-scenario directories retained on disk)
verify-suite-demo: suite run against a pre-existing non-empty --workspace-root (expect: aggregate INTERNAL_ERROR, exit 1)
Suite: suite-demo-all-pass
Result: INTERNAL_ERROR (0 PASS, 0 FAIL, 0 TIMEOUT, 0 INVALID, 0 INTERNAL_ERROR)
Duration: 0s
Error: suite: --workspace-root directory already exists and is not empty: "..."
verify-suite-demo: OK
```

## 11. Commands run and real results

All commands below were run in this verification pass, from the repository
root, both directly (Go 1.26.5 installed locally) and inside the same
Docker dev container CI uses (`docker compose run --rm --remove-orphans dev
...`, via `make verify`).

| # | Command | Result |
|---|---|---|
| 1 | `git diff --check` | Exit 0 — no whitespace errors, no conflict markers |
| 2 | `gofmt -l .` | Exit 0 — empty output, no unformatted files |
| 3 | `go vet ./...` | Exit 0 — no findings |
| 4 | `go test ./... -race -count=1` | Exit 0 — all 10 packages `ok`: `cmd/incidentdna` (~21.6s), `internal/canonical`, `internal/compare`, `internal/evidence`, `internal/fingerprint`, `internal/idir`, `internal/library` (~29s), `internal/scenario` (~2.1s), `internal/suite` (~2.1s), `internal/validate` |
| 5 | `go build -buildvcs=false -o bin/incidentdna ./cmd/incidentdna` | Exit 0 — binary built |
| 6 | `scripts/verify-golden-fingerprint.sh` | Exit 0 — `verify-golden-fingerprint: OK (sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e)` |
| 7 | `scripts/verify-evidence-demo.sh` | Exit 0 — `verify-evidence-demo: OK`, all 3 evidence entries verified |
| 8 | `scripts/verify-library-demo.sh` | Exit 0 — `verify-library-demo: OK` |
| 9 | `scripts/verify-scenario-demo.sh` | Exit 0 — `verify-scenario-demo: OK` |
| 10 | `scripts/verify-suite-demo.sh` | Exit 0 — `verify-suite-demo: OK` (full transcript in §10) |
| 11 | `make example-suite` | Exit 0 — builds binary (if needed), runs `verify-suite-demo.sh` |
| 12 | `make verify` (full chain, in Docker: lint → test → build → example → example-evidence → example-library → example-scenario → example-suite → golden fingerprint script) | Exit 0 throughout, run twice in this session |
| 13 | `git diff --stat` | Confirmed: only `.github/workflows/ci.yml`, `Makefile`, `README.md`, `cmd/incidentdna/main.go`, `docs/architecture.md`, `docs/privacy-model.md`, `docs/product-scope.md`, `docs/threat-model.md` modified; new (untracked): `cmd/incidentdna/{cli_suite_test.go,cmd_suite.go}`, `docs/scenario-suites.md`, `examples/regression-suite-demo/`, `internal/suite/`, `scripts/verify-suite-demo.sh` |
| 14 | `git diff --stat -- testdata/golden` | Empty — no output |
| 15 | `git diff --stat -- examples/duplicate-payment` | Empty — no output |
| 16 | `git diff --stat -- examples/evidence-storage-demo` | Empty — no output |
| 17 | `git diff --stat -- examples/incident-library-demo` | Empty — no output |
| 18 | `git diff --stat -- examples/regression-scenario-demo` | Empty — no output |
| 19 | `git diff --stat -- internal/idir internal/validate internal/canonical internal/fingerprint internal/compare internal/evidence internal/library internal/scenario` | Empty — no output |
| 20 | `grep -rn '"net' cmd/ internal/` | Empty — no network package imported anywhere |
| 21 | `find . -type d -name .incidentdna` | Empty — no evidence/library store artifacts left anywhere in the repository |

## 12. Phase 1 through Phase 4 regression status

- `examples/duplicate-payment/incident.yaml` and
  `testdata/golden/duplicate-payment.fingerprint`: confirmed byte-for-byte
  unchanged; golden fingerprint reproduced identically at
  `sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e`.
- `examples/evidence-storage-demo/`, `examples/incident-library-demo/`, and
  `examples/regression-scenario-demo/`: confirmed unchanged (`git diff
  --stat` produced no output for any of the three);
  `scripts/verify-evidence-demo.sh`, `scripts/verify-library-demo.sh`, and
  `scripts/verify-scenario-demo.sh` all passed in full, unmodified.
- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library`, `internal/scenario`: no files modified — none appear
  anywhere in `git status`/`git diff --stat`.
- All pre-existing CLI subcommands and their tests
  (`cmd/incidentdna/cli_test.go`, `cli_evidence_test.go`,
  `cli_library_test.go`, `cli_scenario_test.go`) are unmodified and pass
  under `go test ./... -race -count=1`.
- The one deliberate touch to a pre-existing file's *behavior*,
  `cmd/incidentdna/main.go`'s context-construction special-case, is
  additive and conditional: it already excepted `"scenario"`; this phase
  extends the same condition to also except `"suite"` — every other
  command name (including `"scenario"` itself) takes the exact code path
  it always did. Confirmed both by the existing test suite passing
  unmodified and by inspection of the diff itself.

## 13. Files added and modified

**New package**, `internal/suite/` (7 implementation files + 5 test files,
12 total): `types.go`, `load.go`, `validate.go`, `run.go`, `report.go`,
`limits.go`, `errors.go`, `load_test.go`, `validate_test.go`, `run_test.go`,
`report_test.go`, `limits_test.go`.

**New CLI files**: `cmd/incidentdna/cmd_suite.go`,
`cmd/incidentdna/cli_suite_test.go`. **Modified**:
`cmd/incidentdna/main.go` (one new `{"suite", runSuite}` entry, plus
extending the existing scenario-only context-timeout special-case to also
except `"suite"`).

**New example**: `examples/regression-suite-demo/` — `README.md`,
`suite-all-pass.yaml`, `suite-mixed.yaml`,
`scenarios/scenario-{pass,trivial-pass,fail,timeout,invalid}.yaml`,
`scenarios/fixtures/incident.yaml`.

**New docs**: `docs/scenario-suites.md`, this report
(`docs/phase-5-report.md`).

**Modified docs**: `README.md`, `docs/architecture.md`,
`docs/product-scope.md`, `docs/threat-model.md`, `docs/privacy-model.md`.

**New**: `scripts/verify-suite-demo.sh` — CLI-binary-level end-to-end check
of the suite demo, same style as `scripts/verify-scenario-demo.sh`.

**Modified**: `Makefile` — added `example-suite` target (mirrors
`example-scenario`), wired into `verify`'s dependency chain and `.PHONY`
list. `.github/workflows/ci.yml` — added a "Regression suite demo
validation" step (`make example-suite`) between the regression-scenario-demo
step and the golden-fingerprint step.

No file under `internal/idir/`, `internal/validate/`, `internal/canonical/`,
`internal/fingerprint/`, `internal/compare/`, `internal/evidence/`,
`internal/library/`, `internal/scenario/`, `examples/duplicate-payment/`,
`examples/evidence-storage-demo/`, `examples/incident-library-demo/`,
`examples/regression-scenario-demo/`, or `testdata/golden/` was touched at
any point in Phase 5.

## 14. Known limitations

Restated from `docs/scenario-suites.md` for completeness:

- **No parallel execution.** Scenarios run strictly sequentially; a suite
  with many slow scenarios takes as long as their sum.
- **No coupling to the incident library.** A suite manifest and its runner
  do not query `internal/library`.
- **No directory-walk/glob-based scenario discovery** — a suite lists
  scenarios explicitly.
- **No release gating** — `suite run`'s exit code and JSON report are
  available to be consumed by something else, but nothing in this codebase
  consumes them.
- **Not a sandbox** — every limitation `docs/regression-scenarios.md`
  already states about a single scenario's execution applies identically,
  per listed scenario, inside a suite.
- **Fixed, non-configurable resource limits.**
- **A suite listing one invalid scenario cannot partially run** — this is
  documented as deliberate (§10 above), not a gap, but is worth restating
  as a real operational consequence: authors must fix every listed
  scenario before any of them can run as part of that suite.

## 15. Acceptance criteria status

Every criterion named in `docs/phase-5-plan.md`'s implementation slices
(§21), file list (§22), test requirements (§23), demo requirements (§24),
and known-limitations statement (§25) is satisfied:

1. `internal/suite` implements load, validate (including per-listed-scenario
   validation), run (sequential, `--fail-fast`, aggregate classification),
   and report generation, each covered by table-driven tests. ✅
2. `suite verify` and `suite run` work end-to-end against real suite
   manifests, with real command transcripts for both aggregate outcomes,
   `--fail-fast`, aggregate `INVALID`, and `--workspace-root`/
   `--keep-workspaces` recorded in this report (§10, §11). ✅
3. Full existing suite still passes unmodified: `gofmt -l` clean, `go vet
   ./...` clean, `go test ./... -race -count=1` green, and the golden
   fingerprint value is byte-for-byte unchanged. ✅
4. `internal/idir`, `internal/validate`, `internal/canonical`,
   `internal/fingerprint`, `internal/compare`, `internal/evidence`,
   `internal/library`, `internal/scenario` are untouched. ✅
5. No `scenarios[].path`, and no report-file path, can escape its resolved
   root — proven the same way Phases 2 through 4 proved their own
   path-containment guarantees. ✅
6. Each of the three resource limits is enforced by its own dedicated
   constant, produces a distinct clear error, and is covered by an
   independent passing/failing test pair. ✅
7. Both aggregate outcomes (`PASS`, `FAIL`), plus `INVALID` and
   `INTERNAL_ERROR`, are independently, deterministically reproducible by a
   dedicated test and a dedicated demo path. ✅
8. `internal/suite` never calls `exec.Command` itself — proven by
   inspection (`grep -rn 'exec\.' internal/suite` finds no such call) and by
   every execution test passing through `scenario.Run` unchanged. ✅
9. `examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
   `examples/incident-library-demo/`, and
   `examples/regression-scenario-demo/` are confirmed byte-for-byte
   unchanged by `git diff --stat`. ✅
10. No network access, no telemetry introduced by the runner's own code
    (`grep -rn '"net' cmd/ internal/` stays empty). ✅
11. The new example contains no credentials, personal information,
    customer data, or real incident material. ✅
12. `suite run` writes no file outside its resolved workspace(s) and the
    user-named `--report` path; every scenario's workspace is removed by
    default and only retained when `--keep-workspaces` is explicitly
    given. ✅
13. Documentation updates are made, and no doc makes a now-false claim
    about what is/isn't implemented. ✅
14. Nothing was committed, staged, or pushed during implementation. ✅

## 16. Phase 6 boundary (explicit)

Everything listed as explicitly out of scope in `docs/product-scope.md` and
`docs/phase-5-plan.md` §25/§26 remains deferred, unstarted, and unscoped in
this codebase: automatic cross-referencing of a scenario's (or a suite's)
`linked_fingerprint` against `internal/library`'s stored occurrences,
parallel suite execution, release-gate/CI-CD integration, remote/cloud
storage, encryption at rest, signing/authenticity proof, multi-tenancy, and
any change to `idir.Document`, the JSON Schema, or the fingerprint
algorithm. **Phase 6 has not been started or scoped as part of this work.**
This report makes no claim about what a future Phase 6 would contain beyond
what `docs/product-scope.md`'s existing "future work" list already names.

## 17. GitHub Actions status

`.github/workflows/ci.yml` has been updated to add a "Regression suite demo
validation" step (`make example-suite`) to the existing job, so the
workflow-as-configured now covers the complete Phase 5 verification path
(lint, full test suite including `internal/suite` and the CLI suite
integration tests, build, the duplicate-payment example, the
evidence-storage demo, the incident-library demo, the regression-scenario
demo, the regression-suite demo, and the golden fingerprint check).

**This configuration has not yet been exercised by the GitHub-hosted
runner.** Every command in §11 was run locally, and via `docker compose run
--rm --remove-orphans dev` (the same image CI builds), with results recorded
above — but the actual hosted GitHub Actions workflow run has not been
confirmed, because these changes have not been pushed. This report makes no
claim that the hosted CI workflow has passed.

## 18. Final completion checklist

- [x] `internal/suite` implements load, validate, run (all classifications
  including `--fail-fast`), and report generation, each covered by
  table-driven tests.
- [x] `suite verify` and `suite run` work end-to-end against real suite
  manifests, with real command transcripts recorded in this report.
- [x] Full existing suite still passes unmodified: `gofmt -l` clean, `go vet
  ./...` clean, `go test ./... -race -count=1` green (all 10 packages), and
  the golden fingerprint value is byte-for-byte unchanged.
- [x] `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library`, `internal/scenario` are untouched.
- [x] `docs/scenario-suites.md`, `README.md`, `docs/architecture.md`,
  `docs/product-scope.md`, `docs/threat-model.md`, `docs/privacy-model.md`
  updated to match implemented behavior.
- [x] `scripts/verify-suite-demo.sh` created and passes standalone and as
  part of `make verify`.
- [x] `Makefile` gained `example-suite`, wired into `verify` and `.PHONY`.
- [x] `.github/workflows/ci.yml` updated to run the suite demo step,
  preserving every existing Phase 1–4 CI step unchanged.
- [x] `git diff --check`, `gofmt -l .`, `go vet ./...`, `go test ./...
  -race -count=1`, `go build`, `scripts/verify-golden-fingerprint.sh`,
  `scripts/verify-evidence-demo.sh`, `scripts/verify-library-demo.sh`,
  `scripts/verify-scenario-demo.sh`, `scripts/verify-suite-demo.sh`, and
  `make verify` all passed locally and in the Docker dev container (exit 0
  throughout, §11, `make verify` run twice).
- [x] No `.incidentdna` directory or generated evidence/library-store
  artifacts remain anywhere in the repository.
- [x] `examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
  `examples/incident-library-demo/`, and
  `examples/regression-scenario-demo/` confirmed byte-for-byte unchanged.
- [x] `internal/suite` never calls `exec.Command` — proven by inspection
  and by every execution test passing through `scenario.Run` unchanged.
- [x] `suite run` writes no file outside its resolved workspace(s) and the
  user-named `--report` path; every workspace is removed by default and
  only retained when `--keep-workspaces` is explicitly given.
- [x] The new example contains no credentials, personal information,
  customer data, or real incident material.
- [x] Diff inspected for security regressions, incorrect documentation
  claims, incomplete CLI wiring, unsafe filesystem behavior, test gaps,
  accidental Phase 6 scope, sensitive/real data, and generated artifacts —
  none found.
- [x] Two deliberate judgment calls made where the approved plan was
  ambiguous or internally inconsistent are documented explicitly rather
  than silently resolved: the nested `scenarios/fixtures/` layout (§10),
  and following §17's detailed "one invalid scenario invalidates the whole
  suite" rule over §4's illustrative (and, on this point, inconsistent)
  transcript (§10).
- [ ] Hosted GitHub Actions run — **not yet confirmed** (requires pushing
  this branch, not done as part of this task).
- [ ] Commit / push / merge — **not done**, per explicit instruction for
  this task.

No production adoption, deployment, customer usage, or performance
benchmarking is claimed anywhere in this report or in Phase 5 generally.
