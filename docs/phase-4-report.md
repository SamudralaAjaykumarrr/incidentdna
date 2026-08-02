# Phase 4 Report: Deterministic Executable Regression Scenarios

This report documents the state of Phase 4 as implemented in the working
tree of branch `phase-4-regression-scenarios`, after the final verification
pass described below. It follows the same role `docs/phase-3-report.md`
served for Phase 3: a record of what was built, how it was verified, and
what is explicitly not yet true. `docs/phase-4-plan.md` is the approved plan
this implements.

## 1. Phase 4 goals

Phases 1–3 built a complete *memory* system: incidents can be represented
(IDIR), validated, deterministically fingerprinted by failure class,
integrity-checked against stored evidence, and accumulated in a local
library grouped by fingerprint. None of that memory was ever *exercised*.
Phase 4's entire scope is closing exactly that gap, and nothing else:

1. A new `internal/scenario` package: IRS v0.1 (Incident Regression
   Scenario) document types, a size-capped loader, a semantic rule engine,
   an optional `--source` fingerprint-linkage cross-check
   (`internal/fingerprint.Compute`, unchanged), a local bounded runner, and
   deterministic JSON report generation.
2. Two CLI subcommands (`incidentdna scenario verify|run`).
3. Fixed resource limits (eight, independently enforced), and the
   filesystem-security properties (path-traversal safety, symlink
   rejection) `internal/evidence` and `internal/library` already
   established, extended to declared `workspace_files` entries.
4. Local process execution — a new class of risk this codebase had not
   previously carried — bounded (timeout, output caps, workspace
   containment, no shell, no implicit `$PATH` lookup) but explicitly not
   sandboxed, with that distinction documented rather than implied away.
5. Documentation updates so `docs/regression-scenarios.md`, `README.md`,
   `docs/architecture.md`, `docs/product-scope.md`, `docs/threat-model.md`,
   and `docs/privacy-model.md` accurately describe what Phase 4 covers.
6. A Makefile/CI verification path exercising the above end-to-end.

Explicitly not part of Phase 4 (see `docs/phase-4-plan.md` §3/§27/§28 for
the full list): a sandbox (seccomp/cgroups/container/VM isolation), a suite
runner (discovering/aggregating/parallelizing multiple scenarios), coupling
to the incident library, release gating or CI/CD blocking, AI-generated
scenarios, signing, encryption, multi-tenancy, or any change to
`idir.Document`, `internal/validate`, `internal/canonical`, or
`internal/fingerprint`.

## 2. Implementation summary

`internal/scenario` is a new, parallel leaf package following the same
dependency-direction precedent `internal/evidence` and `internal/library`
set: it imports `internal/idir`, `internal/validate`, and
`internal/fingerprint` (the three needed for the optional `--source`
cross-check, reusing them unchanged), but nothing in those packages, or in
`internal/evidence`/`internal/library`, imports it or is changed by it.

```
IRS v0.1 scenario document (scenario verify/run)     linked_fingerprint (declared in the document)
   |  scenario.LoadFile — size cap, format sniff, typed decode
   v
scenario.Document
   |  scenario.Validate — semantic rules
   v
(valid) scenario.Document ---> scenario.CheckSourceFingerprint (--source only)
   |  scenario.Run                    |  idir.LoadFile + validate.Validate +
   |  (re-validates, then workspace   |  fingerprint.Compute (unchanged)
   |   + exec)                        v
   v                            match/mismatch vs. linked_fingerprint
bounded workspace + exec.Command,
 no shell, own process group
   |  timeout-bounded, output-capped
   v
PASS / FAIL / TIMEOUT / INVALID / INTERNAL_ERROR
   |  scenario.BuildReport + scenario.WriteReport (--report only)
   v
deterministic JSON execution report
```

`cmd/incidentdna/cmd_scenario.go` adds a single new top-level command,
`scenario`, registered in `main.go`'s `commands` table, self-dispatching to
`verify`/`run` using the exact `flag.NewFlagSet` idiom every existing
subcommand already established — no CLI framework introduced, and no
existing subcommand's dispatch changed except one deliberate, narrow
addition: `main.go`'s per-command context construction now special-cases
`"scenario"` to skip its usual fixed 30-second `context.WithTimeout`
wrapping, because `scenario run` needs its own, larger,
document-declared timeout budget (up to `MaxScenarioTimeoutSeconds`, 300s)
for the child process it executes — a categorically different workload from
parsing/hashing a document. `runScenario` re-applies the standard 30-second
budget itself for the `verify` subcommand, which never executes anything,
so `scenario verify`'s timeout behavior is unchanged from every other
read-only command.

Full design writeup: `docs/regression-scenarios.md`.

## 3. Architecture and the IRS v0.1 format

A scenario document is deliberately not an extension of `idir.Document` and
is not decoded through `internal/idir` — it describes an *execution*, not an
*incident*. Its top-level fields: `schema_version` (fixed `irs/v0.1`),
`scenario.id` (required, non-empty), `linked_fingerprint`
(`sha256:`-prefixed, 64-lowercase-hex, format-checked exactly like
`evidence[].digest`), `execution` (`workspace_files`, `command`, `env`,
`timeout_seconds`), and `expected` (`exit_code`, optional `stdout`/`stderr`
assertions). `expected.exit_code` and `execution.timeout_seconds` are Go
pointer fields specifically so "not declared" (`nil`) is distinguishable
from an explicitly declared `0` — both are meaningfully different states
for a required-vs-optional-with-a-default field.

`command[0]` must be either an absolute path or contain a `/` (interpreted
as workspace-relative); a bare command name is rejected at verify time,
before any process is ever started, and this rejection is proven by a
dedicated test (`TestValidate_RejectsBareCommand`,
`TestCLI_ScenarioRun_InvalidReturnsExitOne`) rather than merely documented.

## 4. Determinism and safety model

`internal/scenario/run.go` implements the runner:

- **No shell interpretation.** `exec.Command(path, args...)` is called
  directly; there is no code path that constructs a shell command string.
- **No implicit `$PATH` lookup.** `command[0]` is resolved to an absolute
  path (used as-is if already absolute, or joined against the workspace
  root) and existence/executable-checked before the process starts;
  `exec.LookPath` is never called.
- **Fixed, non-inherited child environment.** The child's environment is
  `HOME=<workspace>`, `TZ=UTC`, plus `execution.env` — `os.Environ()` is
  never passed through. Verified directly by
  `TestRun_EnvironmentIsolation_ParentEnvNotInherited`, which sets a marker
  environment variable in the test process and confirms it does not reach
  the child.
- **Timeout enforcement kills the whole process group.** The child runs
  with `Setpgid: true`; on timeout, `syscall.Kill(-pid, SIGKILL)` is sent to
  the negative PID (the process group), not just the direct child.
  `TestRun_Timeout_KillsProcessWithinBoundedWallClock` asserts the run
  completes in well under the fixture command's 30-second sleep duration.
- **Bounded, stream-separated output capture.** stdout and stderr are
  captured through independent `boundedWriter` values whose internal buffer
  never exceeds `MaxScenarioOutputBytes`, even under memory pressure — not
  merely truncated after the fact.
  `TestRun_OutputCapTruncatesAndBoundsMemory` writes 2 MiB to stdout and
  asserts the captured excerpt is exactly `MaxScenarioOutputBytes` long with
  `Truncated: true`.
- **Workspace containment.** Every `workspace_files` destination is
  `filepath.Clean`ed and re-checked to have the workspace root as a prefix
  immediately before the write, mirroring `internal/library.Store`'s
  `checkContained` precedent applied to declared relative paths.
- **cwd isolation.** The child's working directory is the workspace root;
  `TestRun_CwdIsolation_RelativeWritesLandInWorkspace` confirms a relative
  write from the child lands inside the workspace, not the test process's
  own working directory.

## 5. The two commands and exit-code contract

```
incidentdna scenario verify [--source <incident-file>] <scenario-file>
incidentdna scenario run [--workspace <dir>] [--report <file>] [--keep-workspace] <scenario-file>
```

**`scenario verify`**: `0` valid (and, with `--source`, fingerprint
matches); `1` I/O/parse/usage error; `2` semantic rule failure or `--source`
mismatch.

**`scenario run`**: `PASS` → `0`; `FAIL`/`TIMEOUT` → `2`;
`INVALID`/`INTERNAL_ERROR` → `1` — mirroring the existing project-wide
convention (already documented in `docs/incident-library.md`) that `2`
means "well-formed input, materially negative finding" and `1` means
"environmental/usage failure," reused for a *different specific condition*
per command rather than a new exit-code space.

## 6. Resource limits

Eight independent, fixed constants (`internal/scenario/limits.go`), each
with its own distinct error and its own passing/failing boundary test pair:

| Constant | Value | Applies to |
|---|---|---|
| `MaxScenarioDocumentSize` | 1 MiB | The scenario file's own on-disk size |
| `MinScenarioTimeoutSeconds` | 1 | Lower bound on a declared `timeout_seconds` |
| `DefaultScenarioTimeoutSeconds` | 30 | Used when `timeout_seconds` is omitted |
| `MaxScenarioTimeoutSeconds` | 300 | Upper bound a declared `timeout_seconds` must not exceed |
| `MaxWorkspaceFiles` | 50 | Number of `workspace_files` entries a scenario may declare |
| `MaxWorkspaceFileSize` | 10 MiB | Size of any single declared `workspace_files` source file |
| `MaxWorkspaceTotalBytes` | 50 MiB | Sum of all staged `workspace_files` sizes |
| `MaxScenarioOutputBytes` | 1 MiB | Captured bytes per stream (stdout, stderr independently) before truncation |

All eight are fixed, not user-configurable, consistent with Phases 2 and 3's
resource-limit design.

## 7. Corruption and malformed-scenario handling

- A scenario file that fails to parse, or exceeds `MaxScenarioDocumentSize`:
  `scenario verify` exits `1`; `scenario run` reports `INTERNAL_ERROR` and
  exits `1`, and still writes a best-effort `--report` (with `scenario_id`/
  `linked_fingerprint` left empty, since nothing was decoded) — verified by
  `TestCLI_ScenarioRun_LoadFailureStillWritesReport`.
- A scenario that decodes but fails a semantic rule (missing/wrong
  `schema_version`, empty `scenario.id`, empty `command`, bare `command[0]`,
  out-of-range `timeout_seconds`, a `workspace_files` traversal/absolute/
  symlinked/missing/duplicate-destination entry, malformed
  `linked_fingerprint`, missing `expected.exit_code`, an unknown
  `stdout`/`stderr` `mode`, or an unparseable `regex` value): `scenario
  verify` exits `2`; `scenario run` reports `INVALID` and exits `1` —
  nothing is ever executed, proven by
  `TestRun_Invalid_NeverExecutesAndReportsNoWorkspace`, which asserts a
  marker file the fixture command would have created is absent.
- `command[0]` does not exist or is not executable at run time:
  `INTERNAL_ERROR`, exit `1`, caught by a pre-flight check immediately
  after workspace staging and before the process is ever started.
- A pre-existing, non-empty `--workspace <dir>`: refused immediately,
  `INTERNAL_ERROR`, exit `1`.
- A `workspace_files` source that disappears between verify and staging
  (TOCTOU): reported distinctly via `ErrWorkspaceFileMissing`
  (`TestStageWorkspaceFiles_MissingSourceIsReportedDistinctly`).
- The `--report` path cannot be written: reported to stderr, exit `1`, even
  if the run itself PASSed — the run's own summary is still printed to
  stdout first.

## 8. Result and report format

`scenario run` always prints a human-readable stdout summary, and (with
`--report <path>`) writes a deterministic JSON report — fixed Go struct, no
map iteration, so field order never varies across repeated marshals of the
same value (`TestReport_DeterministicFieldOrderAcrossMarshals` asserts both
byte-identical repeated marshals and the exact documented field order).
`--report` overwrites unconditionally on each run
(`TestWriteReport_OverwritesOnRepeatedRuns`).

## 9. Test coverage

- **`internal/scenario`**: 64 top-level `Test*` functions across 6 test
  files (`validate_test.go` 25, `run_test.go` 16, `load_test.go` 8,
  `limits_test.go` 6, `report_test.go` 5, `fingerprint_link_test.go` 4),
  table-driven, real filesystem via `t.TempDir()`, real child processes via
  `/bin/sh`/`/bin/sleep`/`/bin/true`/`/bin/false` (never the built
  `incidentdna` binary, per `docs/phase-4-plan.md` §21 slice 5's guidance).
  Covers: every semantic validation rule; the `--source` cross-check match/
  mismatch/source-invalid paths; all five run outcomes including output-cap
  truncation, environment isolation, cwd isolation, and workspace staging;
  deterministic report field order and overwrite behavior; and a
  passing/failing boundary test pair for each of the eight resource limits.
- **`cmd/incidentdna/cli_scenario_test.go`**: 22 top-level `Test*`
  functions, black-box against the real compiled binary, the same style
  `cli_library_test.go`/`cli_evidence_test.go` already use — `scenario
  verify` exit 0/1/2 paths including `--source` match/mismatch; `scenario
  run` exit 0/1/2 across all five outcomes; `--report` file contents;
  `--keep-workspace`/`--workspace` behavior; a scenario whose `command[0]`
  points at the real, freshly built `bin/incidentdna` binary
  (`TestCLI_ScenarioRun_WorkspaceRelativeCommandAgainstRealBinary`),
  demonstrating the self-referential regression check the demo uses; help
  text and unknown/missing-subcommand handling.
- **Race**: `go test ./... -race -count=1` is green across all 9 packages,
  `internal/scenario` included — the output-capture goroutines os/exec
  manages internally for a non-`*os.File` `Stdout`/`Stderr` are joined by
  `cmd.Wait()` before `Run` returns, so no additional synchronization was
  needed in `internal/scenario`'s own code.
- **Regression discipline**: the full existing suite passed unmodified; the
  golden fingerprint test/script produced the unchanged Phase 1 value.

## 10. Demo coverage

`examples/regression-scenario-demo/` (four scenario files:
`scenario-pass.yaml`, `scenario-fail.yaml`, `scenario-timeout.yaml`,
`scenario-invalid.yaml`, plus `fixtures/incident.yaml`, a frozen copy — not
a symlink — of `examples/duplicate-payment/incident.yaml`'s content)
demonstrates all five outcomes end to end via
`scripts/verify-scenario-demo.sh`. `scenario-pass.yaml` and
`scenario-fail.yaml` declare `command[0]` as the placeholder
`/INCIDENTDNA_BIN_PLACEHOLDER`, substituted by the verification script for
the real, checkout-specific absolute path to the built binary before
`scenario run` — `scenario verify` (structural, and the `--source`
cross-check) always runs against the original, unmodified checked-in files.
See `examples/regression-scenario-demo/README.md`, "Why this demo uses an
absolute command path," and `docs/regression-scenarios.md`'s identically
titled section, for why: a portable *relative* `command[0]` back to the
repository's `bin/` directory would need to know the workspace's depth
below the repository root, which is neither fixed nor predictable for a
default `os.MkdirTemp` workspace across machines and CI runners.

Exact transcript from this verification pass, run via
`scripts/verify-scenario-demo.sh` (full output in this session's tool
transcript; representative excerpts below):

```
verify-scenario-demo: scenario verify scenario-pass.yaml (expect: valid, exit 0)
Scenario: duplicate-payment-refingerprint (schema irs/v0.1)
Linked fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
OK: scenario is structurally valid

verify-scenario-demo: scenario verify scenario-invalid.yaml (expect: INVALID at verify time, exit 2)
incidentdna: .../scenario-invalid.yaml is not a valid IRS v0.1 scenario:
  - execution.command[0]: must be an absolute path or a path relative to the workspace root (containing a "/"), not a bare command name "echo" that would require an implicit $PATH lookup

verify-scenario-demo: scenario verify --source (expect: match, exit 0)
Source fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
OK: scenario is structurally valid and its linked_fingerprint matches --source

verify-scenario-demo: scenario run scenario-pass.yaml (expect: PASS, exit 0)
Result: PASS
Command exited 0 (expected 0)
Workspace: removed (pass --keep-workspace to retain it)

verify-scenario-demo: scenario run scenario-fail.yaml (expect: FAIL, exit 2)
Result: FAIL
Command exited 0 (expected 1)

verify-scenario-demo: scenario run scenario-timeout.yaml (expect: TIMEOUT, exit 2, bounded wall-clock)
Result: TIMEOUT
Command timed out (expected exit 0)
Duration: 1.002s

verify-scenario-demo: scenario run scenario-invalid.yaml (expect: INVALID, exit 1)
Result: INVALID
Error: execution.command[0]: must be an absolute path or a path relative to the workspace root ...

verify-scenario-demo: scenario run with a nonexistent command[0] (expect: INTERNAL_ERROR, exit 1)
Result: INTERNAL_ERROR
Error: scenario: execution.command[0] does not exist or is not executable: /no/such/executable-xyz: ...

verify-scenario-demo: scenario run --keep-workspace (expect: workspace retained on disk)
Result: PASS
Workspace: kept at /tmp/tmp.XXXXXXXXXX/kept-workspace

verify-scenario-demo: OK
```

Every fingerprint above (`fc3dac01...bca2a8e`) is the unchanged Phase 1
golden fingerprint, confirmed byte-for-byte identical to
`testdata/golden/duplicate-payment.fingerprint`.

## 11. Commands run and real results

All commands below were run in this verification pass, from the repository
root, on branch `phase-4-regression-scenarios`, both directly (Go 1.26.5
installed locally) and inside the same Docker dev container CI uses
(`docker compose run --rm --remove-orphans dev ...`, via `make verify`).

| # | Command | Result |
|---|---|---|
| 1 | `git diff --check` | Exit 0 — no whitespace errors, no conflict markers |
| 2 | `gofmt -l .` | Exit 0 — empty output, no unformatted files |
| 3 | `go vet ./...` | Exit 0 — no findings |
| 4 | `go test ./... -race -count=1` | Exit 0 — all 9 packages `ok`: `cmd/incidentdna` (17.7s), `internal/canonical`, `internal/compare`, `internal/evidence`, `internal/fingerprint`, `internal/idir`, `internal/library` (28.6s), `internal/scenario` (2.1s), `internal/validate` |
| 5 | `go build -buildvcs=false -o bin/incidentdna ./cmd/incidentdna` | Exit 0 — binary built |
| 6 | `scripts/verify-golden-fingerprint.sh` | Exit 0 — `verify-golden-fingerprint: OK (sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e)` |
| 7 | `scripts/verify-evidence-demo.sh` | Exit 0 — `verify-evidence-demo: OK`, all 3 evidence entries verified |
| 8 | `scripts/verify-library-demo.sh` | Exit 0 — `verify-library-demo: OK` |
| 9 | `scripts/verify-scenario-demo.sh` | Exit 0 — `verify-scenario-demo: OK` (full transcript in §10) |
| 10 | `make example-scenario` | Exit 0 — builds binary (if needed), runs `verify-scenario-demo.sh` |
| 11 | `make verify` (full chain, in Docker: lint → test → build → example → example-evidence → example-library → example-scenario → golden fingerprint script) | Exit 0 throughout |
| 12 | `git diff --stat` | Confirmed: only `.github/workflows/ci.yml`, `Makefile`, `README.md`, `cmd/incidentdna/main.go`, `docs/architecture.md`, `docs/privacy-model.md`, `docs/product-scope.md`, `docs/threat-model.md` modified; new (untracked): `cmd/incidentdna/{cli_scenario_test.go,cmd_scenario.go}`, `docs/regression-scenarios.md`, `examples/regression-scenario-demo/`, `internal/scenario/`, `scripts/verify-scenario-demo.sh` |
| 13 | `git diff -- testdata/golden` | Empty — no output |
| 14 | `git diff -- examples/duplicate-payment` | Empty — no output |
| 15 | `git diff -- examples/evidence-storage-demo` | Empty — no output |
| 16 | `git diff -- examples/incident-library-demo` | Empty — no output |
| 17 | `git diff -- internal/idir internal/validate internal/canonical internal/fingerprint internal/compare internal/evidence internal/library` | Empty — no output |
| 18 | `grep -rn '"net' cmd/ internal/` | Empty — no network package imported anywhere |
| 19 | `find . -type d -name .incidentdna` | Empty — no evidence/library store artifacts left anywhere in the repository |

## 12. Phase 1/2/3 regression status

- `examples/duplicate-payment/incident.yaml` and
  `testdata/golden/duplicate-payment.fingerprint`: confirmed byte-for-byte
  unchanged; golden fingerprint reproduced identically at
  `sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e`.
- `examples/evidence-storage-demo/` and `examples/incident-library-demo/`:
  confirmed unchanged (`git diff` produced no output for either);
  `scripts/verify-evidence-demo.sh` and `scripts/verify-library-demo.sh`
  both passed in full.
- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library`: no files modified — none appear anywhere in `git
  status`/`git diff --stat`.
- All pre-existing CLI subcommands and their tests
  (`cmd/incidentdna/cli_test.go`, `cli_evidence_test.go`,
  `cli_library_test.go`) are unmodified and pass under `go test ./...
  -race -count=1`.
- The one deliberate touch to a pre-existing file's *behavior*,
  `cmd/incidentdna/main.go`'s context-construction special-case for the
  `"scenario"` command, is additive and conditional: every other command
  name takes the exact code path it always did (`ctx, cancelTimeout :=
  context.WithTimeout(ctx, commandTimeout)`), unchanged. This is confirmed
  both by the existing test suite passing unmodified and by inspection of
  the diff itself (`git diff -- cmd/incidentdna/main.go`).

## 13. Files added and modified

**New package**, `internal/scenario/` (8 implementation files + 6 test
files, 14 total): `types.go`, `load.go`, `validate.go`,
`fingerprint_link.go`, `run.go`, `report.go`, `limits.go`, `errors.go`,
`load_test.go`, `validate_test.go`, `fingerprint_link_test.go`,
`run_test.go`, `report_test.go`, `limits_test.go`.

**New CLI files**: `cmd/incidentdna/cmd_scenario.go`,
`cmd/incidentdna/cli_scenario_test.go`. **Modified**:
`cmd/incidentdna/main.go` (one new `{"scenario", runScenario}` entry, plus
the context-timeout special-case described in §2/§12).

**New example**: `examples/regression-scenario-demo/` — `README.md`,
`scenario-pass.yaml`, `scenario-fail.yaml`, `scenario-timeout.yaml`,
`scenario-invalid.yaml`, `fixtures/incident.yaml`.

**New docs**: `docs/regression-scenarios.md`, this report
(`docs/phase-4-report.md`).

**Modified docs**: `README.md`, `docs/architecture.md`,
`docs/product-scope.md`, `docs/threat-model.md`, `docs/privacy-model.md`.

**New**: `scripts/verify-scenario-demo.sh` — CLI-binary-level end-to-end
check of the scenario demo, same style as `scripts/verify-library-demo.sh`.

**Modified**: `Makefile` — added `example-scenario` target (mirrors
`example-library`), wired into `verify`'s dependency chain and `.PHONY`
list. `.github/workflows/ci.yml` — added a "Regression scenario demo
validation" step (`make example-scenario`) between the incident-library-demo
step and the golden-fingerprint step.

No file under `internal/idir/`, `internal/validate/`, `internal/canonical/`,
`internal/fingerprint/`, `internal/compare/`, `internal/evidence/`,
`internal/library/`, `examples/duplicate-payment/`,
`examples/evidence-storage-demo/`, `examples/incident-library-demo/`, or
`testdata/golden/duplicate-payment.fingerprint` was touched at any point in
Phase 4.

## 14. Known limitations

Restated from `docs/regression-scenarios.md` for completeness:

- **Not a sandbox.** Process isolation is argv/env/cwd/timeout/output
  bounding only — no seccomp, cgroups, container, or VM isolation. A
  reviewed scenario's command runs with the full permissions of the
  invoking user.
- **No suite runner.** Exactly one scenario per invocation.
- **No coupling to the incident library.** A scenario's
  `linked_fingerprint` is never checked against stored library occurrences
  automatically — only against a directly-named `--source` incident file.
- **Detached-process timeout evasion is a known, accepted gap** — a
  grandchild process that double-forks out of its process group can
  survive past the timeout.
- **No enforcement that a reviewed command's own filesystem/network
  behavior stays within the workspace** — only the runner's own operations
  are bounded.
- **`--report`'s output path has no overwrite protection** — always
  overwrites the named path unconditionally.
- **Fixed, non-configurable resource limits.**
- **No release gating** — `scenario run`'s result is available to be
  consumed by something else, but nothing in this codebase consumes it.

## 15. Phase 5 boundary (explicit)

Everything listed as explicitly out of scope in `docs/product-scope.md` and
`docs/phase-4-plan.md` §27/§28 remains deferred, unstarted, and unscoped in
this codebase: a suite/aggregation layer, sandboxed execution, automatic
cross-referencing of a scenario's `linked_fingerprint` against
`internal/library`'s stored occurrences, release-gate/CI-CD integration,
remote/cloud storage, encryption at rest, signing/authenticity proof,
multi-tenancy, and any change to `idir.Document`, the JSON Schema, or the
fingerprint algorithm. **Phase 5 has not been started or scoped as part of
this work.** This report makes no claim about what a future Phase 5 would
contain beyond what `docs/product-scope.md`'s existing "future work" list
already names.

## 16. GitHub Actions status

`.github/workflows/ci.yml` has been updated to add a "Regression scenario
demo validation" step (`make example-scenario`) to the existing job, so the
workflow-as-configured now covers the complete Phase 4 verification path
(lint, full test suite including `internal/scenario` and the CLI scenario
integration tests, build, the duplicate-payment example, the
evidence-storage demo, the incident-library demo, the regression-scenario
demo, and the golden fingerprint check).

**This configuration has not yet been exercised by the GitHub-hosted
runner.** Every command in §11 was run locally, and via `docker compose run
--rm --remove-orphans dev` (the same image CI builds), with results recorded
above — but the actual hosted GitHub Actions workflow run has not been
confirmed, because these changes have not been pushed. CI status for this
branch will only be known once it is pushed and the hosted workflow runs.
This report makes no claim that the hosted CI workflow has passed.

## 17. Final completion checklist

- [x] `internal/scenario` implements load, validate, the `--source`
  fingerprint cross-check, run (with all five outcomes), and report
  generation, each covered by table-driven tests.
- [x] `scenario verify` and `scenario run` work end-to-end against real
  scenario files, with real command transcripts for all five outcomes
  recorded in this report (§10, §11).
- [x] Full existing suite still passes unmodified: `gofmt -l` clean, `go vet
  ./...` clean, `go test ./... -race -count=1` green (all 9 packages), and
  the golden fingerprint value is byte-for-byte unchanged.
- [x] `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library` are untouched.
- [x] `docs/regression-scenarios.md`, `README.md`, `docs/architecture.md`,
  `docs/product-scope.md`, `docs/threat-model.md`, `docs/privacy-model.md`
  updated to match implemented behavior.
- [x] `scripts/verify-scenario-demo.sh` created and passes standalone and as
  part of `make verify`.
- [x] `Makefile` gained `example-scenario`, wired into `verify` and
  `.PHONY`.
- [x] `.github/workflows/ci.yml` updated to run the scenario demo step,
  preserving every existing Phase 1/2/3 CI step unchanged.
- [x] `git diff --check`, `gofmt -l .`, `go vet ./...`, `go test ./...
  -race -count=1`, `go build`, `scripts/verify-golden-fingerprint.sh`,
  `scripts/verify-evidence-demo.sh`, `scripts/verify-library-demo.sh`,
  `scripts/verify-scenario-demo.sh`, and `make verify` all passed locally
  and in the Docker dev container (exit 0 throughout, §11).
- [x] No `.incidentdna` directory or generated evidence/library-store
  artifacts remain anywhere in the repository.
- [x] `examples/duplicate-payment/`, `examples/evidence-storage-demo/`, and
  `examples/incident-library-demo/` confirmed byte-for-byte unchanged.
- [x] The runner never invokes a shell and never performs implicit `$PATH`
  resolution — proven by `TestValidate_RejectsBareCommand` and
  `TestCLI_ScenarioRun_InvalidReturnsExitOne`, not merely documented.
- [x] `scenario run` writes no file outside its resolved workspace root and
  the user-named `--report` path; the workspace is removed by default and
  only retained when `--keep-workspace` is explicitly given.
- [x] The new example contains no credentials, personal information,
  customer data, or real incident material.
- [x] Diff inspected for security regressions, incorrect documentation
  claims, incomplete CLI wiring, unsafe filesystem behavior, test gaps,
  accidental Phase 5 scope, sensitive/real data, and generated artifacts —
  none found.
- [ ] Hosted GitHub Actions run — **not yet confirmed** (requires pushing
  this branch, not done as part of this task).
- [ ] Commit / push / merge — **not done**, per explicit instruction for
  this task.

No production adoption, deployment, customer usage, or performance
benchmarking is claimed anywhere in this report or in Phase 4 generally.
