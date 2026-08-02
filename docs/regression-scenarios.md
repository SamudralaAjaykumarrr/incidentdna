# Deterministic Executable Regression Scenarios

Phase 4 adds a small, reviewable, versioned document format — a **regression
scenario** (IRS v0.1, "Incident Regression Scenario") — and a local, bounded,
offline runner that executes the one command a scenario declares and
classifies the result. This document describes what that runner is, what it
guarantees, and — just as importantly — what it does not. `docs/phase-4-plan.md`
is the approved plan this implements; this document describes the resulting
design as built, for the same audience `docs/evidence-storage.md` and
`docs/incident-library.md` serve.

Implementation: `internal/scenario/` (`types.go`, `load.go`, `validate.go`,
`fingerprint_link.go`, `run.go`, `report.go`, `limits.go`, `errors.go`) and
`cmd/incidentdna/cmd_scenario.go`.

## What problem this closes

Phases 1–3 built a complete *memory* system: incidents can be represented,
validated, deterministically fingerprinted by failure class,
integrity-checked against stored evidence, and accumulated in a local
library grouped by fingerprint. None of that memory is ever *exercised*.
`incidentdna library check` can tell an engineer "a failure with this
fingerprint has been recorded before" — but there was no way to ask the
sharper question a regression suite exists to answer: "if I run something
locally right now, does it still reproduce (or still avoid) that recorded
failure?" Phase 4 closes that gap narrowly: a scenario document declares one
bounded local command, the fixed inputs it runs against, and the expected
outcome to compare against; `incidentdna scenario run` executes it
deterministically and reports one of five outcomes.

Phase 4 does not change `idir.Document`, the JSON Schema,
`internal/validate`, `internal/canonical`, `internal/fingerprint`,
`internal/compare`, `internal/evidence`, or `internal/library`.
`internal/scenario` imports `internal/idir`, `internal/validate`, and
`internal/fingerprint` only for the optional `--source` cross-check, reusing
`fingerprint.Compute` unchanged; it does not import, and is not imported by,
`internal/evidence` or `internal/library` — scenario execution is a fourth
parallel leaf concern, exactly as `internal/library` was a third, relative to
`internal/evidence`'s precedent as the second.

## A new class of risk: local process execution

Every prior phase's operations were pure data/filesystem transformations —
reading, hashing, comparing, storing files whose *content* the tool never
executes. Phase 4 introduces local process execution for the first time in
this codebase, and its safety model is explicitly weaker in one dimension
than that: **the runner's own code never shells out, never does implicit
`$PATH` resolution, never writes outside the workspace it creates, and never
reads any file a scenario document didn't explicitly declare** — this part
is a real, testable guarantee, covered by `internal/scenario`'s tests. But
**the runner does not sandbox the process it launches.** Once
`execution.command` starts, it runs with the invoking user's full OS-level
permissions — no seccomp, no cgroups, no container, no chroot — and can, in
principle, read/write anywhere that user's account can, or reach the
network, exactly as any locally-executed program can.

**The actual safety mechanism is human review before running, not runtime
containment.** A scenario's `execution.command` is fully visible before it
is ever run — `scenario verify` never executes anything — and nothing about
the format allows a command to be constructed, discovered, or mutated at run
time. A reviewer who approves a scenario file is approving exactly the
command that will run — the same epistemic model this project already
applies to `privacy.redacted` (author-declared, mechanically checked for a
fixed rule, not proven true) and to evidence/library digest integrity
(proves internal consistency, not truthfulness). "Bounded, not sandboxed" is
the operative phrase: timeouts, output caps, and workspace containment bound
how long a *reviewed* command can run and how much it can write to
stdout/stderr and to the one directory it's launched in — they do not
attempt to bound what a malicious command could do with the invoking user's
actual permissions.

## The scenario document format (IRS v0.1)

A scenario is a small, standalone YAML or JSON document, deliberately not an
extension of `idir.Document` and not decoded through `internal/idir` at all
— a scenario describes an *execution*, not an *incident*. New, dedicated
types in `internal/scenario`, loaded through a size-capped loader
(`scenario.LoadFile`, mirroring `idir.LoadFile`'s size-cap-then-typed-decode
discipline, not reusing it — the two document types are unrelated).

```yaml
schema_version: irs/v0.1

scenario:
  id: duplicate-payment-refingerprint
  title: Duplicate payment example still fingerprints and validates cleanly
  description: >
    Re-validates and re-fingerprints a frozen copy of the duplicate-payment
    example's content through the compiled incidentdna binary, and asserts
    the result exactly matches the pinned golden fingerprint.

linked_fingerprint: "sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e"

execution:
  workspace_files:
    - source: fixtures/incident.yaml     # relative to this scenario file's own directory
      destination: incident.yaml         # relative path inside the workspace
  command:
    - "/absolute/path/to/bin/incidentdna"
    - "fingerprint"
    - "incident.yaml"
  env:
    LC_ALL: "C"
  timeout_seconds: 10

expected:
  exit_code: 0
  stdout:
    mode: contains
    value: "sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e"
```

Field notes:

- **`schema_version`** — fixed literal `irs/v0.1`; checked the same way
  `idir.Document.SchemaVersion` is: exact match, reject anything else.
- **`scenario.id`** — required, non-empty, stable, author-chosen identifier;
  used only for human/report readability, never as a filesystem path
  component (there is no store, so nothing to key by it).
- **`linked_fingerprint`** — required, format-checked exactly like
  `evidence[].digest` (`^sha256:[0-9a-f]{64}$`), but **not** required to
  exist in any local library — a scenario must be runnable in a checkout
  that has never run `library add`. It is a declared, reviewable claim
  ("this scenario regression-tests this failure class"), mechanically
  checkable against a real incident file only when `--source` is supplied.
- **`execution.workspace_files`** — a bounded list (`MaxWorkspaceFiles`) of
  `{source, destination}` pairs. `source` is resolved relative to the
  scenario file's own directory; `destination` is resolved relative to the
  workspace root. Both are rejected if absolute, containing `..`, or
  resolving outside their respective root. A `source` that is a symlink is
  rejected, not followed. Two entries mapping to the same `destination` are
  rejected as invalid, not silently last-write-wins.
- **`execution.command`** — a required, non-empty argv list. `command[0]`
  must be either an absolute path or a path containing a `/` (interpreted as
  relative to the workspace root); a bare command name (`"python3"`, `"sh"`,
  `"echo"`) is rejected at verify time, because resolving it would require an
  implicit, host-dependent `$PATH` lookup. The runner never wraps this argv
  in a shell (`exec.Command(path, args...)`, never `sh -c`).
- **`execution.env`** — an optional, fixed map of extra environment
  variables set for the child process. The child's environment is **not**
  inherited from the invoking `incidentdna` process — only a minimal fixed
  base (`PATH` unset, `HOME` pointed at the workspace, `TZ=UTC`) plus
  whatever `env` declares reaches the child.
- **`execution.timeout_seconds`** — optional; defaults to
  `DefaultScenarioTimeoutSeconds` (30) if omitted, must be between
  `MinScenarioTimeoutSeconds` (1) and `MaxScenarioTimeoutSeconds` (300) —
  checked at verify time, not only at run time.
- **`expected.exit_code`** — required integer (a declared `0` is a fully
  valid expectation, distinguished from "not declared" via a pointer field).
- **`expected.stdout` / `expected.stderr`** — optional; if present, `mode` is
  one of `exact`, `contains`, `regex`, and `value` is the string/pattern
  compared against the captured (and possibly truncated) stream. Absent
  means "not checked."

## The two commands

```
incidentdna scenario verify [--source <incident-file>] <scenario-file>
incidentdna scenario run [--workspace <dir>] [--report <file>] [--keep-workspace] <scenario-file>
```

**`scenario verify <scenario-file>`** — loads and structurally/semantically
validates `<scenario-file>` against every IRS v0.1 rule above. **Never
executes anything.** If `--source <incident-file>` is given, additionally
loads, validates, and fingerprints `<incident-file>` through the unchanged
Phase 1 pipeline (`idir.LoadFile` → `validate.Validate` →
`fingerprint.Compute`) and asserts the result equals the scenario's declared
`linked_fingerprint`. This mirrors the existing `validate`/`evidence
verify`/`library check` split between "is this trustworthy to run" and "did
running it produce the expected result."

**`scenario run <scenario-file>`** — runs the same pre-execution checks
`scenario verify` performs (without `--source`). If they pass, creates a
workspace, stages `execution.workspace_files`, executes
`execution.command` under the declared/default timeout with output capped
per stream, compares the actual outcome against `expected`, prints a
human-readable summary, and (if `--report <file>` is given) writes the same
outcome as one deterministic JSON report file.

Sample transcript (see `examples/regression-scenario-demo/` for the full,
runnable version):

```
$ incidentdna scenario verify --source examples/duplicate-payment/incident.yaml \
    examples/regression-scenario-demo/scenario-pass.yaml
Scenario: duplicate-payment-refingerprint (schema irs/v0.1)
Linked fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
Source fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
OK: scenario is structurally valid and its linked_fingerprint matches --source

$ incidentdna scenario run examples/regression-scenario-demo/scenario-pass.yaml
Scenario: duplicate-payment-refingerprint
Result: PASS
Command exited 0 (expected 0)
Duration: 4ms
Workspace: removed (pass --keep-workspace to retain it)
```

### Exit codes

**`scenario verify`** (mirrors `validate`'s existing 0/1/2 split):

| Exit | Meaning |
|---|---|
| `0` | Structurally and semantically valid (and, with `--source`, `linked_fingerprint` matches) |
| `1` | I/O/parse/usage error — can't read the scenario file or `--source` file, bad CLI usage |
| `2` | Decodes but fails a semantic IRS v0.1 rule, or (with `--source`) a fingerprint mismatch |

**`scenario run`** (extends the same philosophy to five named outcomes):

| Outcome | Exit | Meaning |
|---|---|---|
| `PASS` | `0` | Command ran to completion within the timeout and matched every declared `expected` field |
| `FAIL` | `2` | Command ran to completion within the timeout but did not match `expected` |
| `TIMEOUT` | `2` | Command was launched but killed after exceeding `timeout_seconds` — a real, semantic negative finding, not a usage error |
| `INVALID` | `1` | Scenario failed the same pre-execution checks `scenario verify` performs — nothing was ever executed |
| `INTERNAL_ERROR` | `1` | An environmental failure prevented determining PASS/FAIL/TIMEOUT — workspace creation failed, `command[0]` could not be found/executed, a `workspace_files` staging I/O error, etc. |

`FAIL`/`TIMEOUT` sharing exit `2` and `INVALID`/`INTERNAL_ERROR` sharing exit
`1` mirrors the existing project-wide convention that `2` means
"well-formed input, materially negative finding" and `1` means
"environmental/usage failure" — `docs/incident-library.md` already documents
exit code `2` being reused for a *different specific condition* across
commands; this follows that same documented precedent.

## Execution isolation boundaries

| Boundary | Mechanism | Guarantee level |
|---|---|---|
| No shell interpretation | `exec.Command(path, args...)` — never `sh -c`, never a string command line | Structural |
| No implicit `$PATH` lookup | `command[0]` must be absolute or contain `/`; `exec.LookPath` is never called | Structural |
| Workspace containment (runner's own writes) | A fresh directory via `os.MkdirTemp` (default) or an explicitly empty, runner-created `--workspace <dir>` (fails if the given path already exists and is non-empty); every `workspace_files` destination is re-checked to have the workspace root as a prefix before any write | Structural, for the runner's own file operations |
| Child process cwd | Set to the workspace root; the child cannot be launched with any other working directory | Structural |
| Child process environment | Fixed minimal base + `execution.env` only, never `os.Environ()` passed through | Structural |
| Child process own filesystem/network access | **Not bounded** — the child has full OS-level permissions of the invoking user | Documented limitation, not a guarantee |
| Time | `context.WithTimeout` wraps the whole execution; on expiry the process's whole process group (`Setpgid`) is killed, catching children it spawned | Structural for the parent-tracked process tree; a detached grandchild that escapes the process group is a known, documented residual gap |
| Output volume | Both stdout and stderr captured through independent bounded writers; once a cap is hit, further bytes are discarded (not buffered) and the report marks that stream `truncated: true` — the process itself is not killed merely for exceeding the output cap, only for exceeding the timeout | Structural |
| Persistence | The workspace is removed after the run completes (any outcome) unless `--keep-workspace` is passed, in which case its path is printed/reported and left for manual inspection/cleanup | Structural |

## Resource limits

Eight independent, fixed constants (`internal/scenario/limits.go`), each with
its own distinct error and its own passing/failing boundary test pair:

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

`scenario run` is **not** run under `main.go`'s usual 30-second overall
command context — that timeout was sized for parsing/hashing a document, a
categorically different workload from executing an arbitrary bounded child
process. `scenario run` instead uses its own context derived from
`timeout_seconds` (bounded by `MaxScenarioTimeoutSeconds`), so a
legitimately slower regression check is not truncated by an unrelated,
smaller global budget. `scenario verify` (which never executes anything)
continues to run under the existing 30-second command context, unchanged —
`main.go` special-cases the `scenario` command's dispatch to skip its usual
context wrapping, and `runScenario` applies the 30-second budget itself only
for the `verify` subcommand.

## Result and report format

Two outputs, always produced together by `scenario run`:

1. **Human-readable stdout summary** (always printed).
2. **Machine-readable JSON report**, written to `--report <path>` only if
   given; deterministic field order via a fixed Go struct (no map
   iteration), one file per run, **overwritten unconditionally** on each run
   (no append, no versioning):

```json
{
  "schema_version": "irs-report/v0.1",
  "scenario_id": "duplicate-payment-refingerprint",
  "scenario_file": "examples/regression-scenario-demo/scenario-pass.yaml",
  "linked_fingerprint": "sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e",
  "result": "PASS",
  "exit_code_expected": 0,
  "exit_code_actual": 0,
  "duration_ms": 4,
  "stdout": {"excerpt": "sha256:fc3dac...", "truncated": false},
  "stderr": {"excerpt": "", "truncated": false},
  "workspace_path": null,
  "error": null
}
```

`workspace_path` is populated only when `--keep-workspace` was given.
`error` is populated only for `INVALID`/`INTERNAL_ERROR`, with a short,
actionable message — never the full captured command output, which already
has its own bounded `stdout`/`stderr` fields.

Even if the scenario file itself fails to load (a parse error, or exceeding
`MaxScenarioDocumentSize`), `scenario run --report <path>` still writes a
best-effort report classifying the outcome `INTERNAL_ERROR` with
`scenario_id`/`linked_fingerprint` left empty — so a caller scripting against
`--report` output always finds a report at the path it named, never only
sometimes.

## Idempotency behavior

`scenario run` has **no persistent side effects on the repository or any
store** — it does not write into `.incidentdna/evidence` or
`.incidentdna/library`, does not modify the scenario file, and does not
accumulate state between runs. Re-running the same scenario:

- Creates a brand-new, independent workspace each time (never reuses or
  appends to a prior one).
- Removes it afterward (default) — nothing observable persists.
- Overwrites `--report <path>` each run — running twice with the same value
  leaves exactly one, current report on disk.
- Produces the same `result` classification on every run **only if the
  declared command itself is deterministic** — Phase 4 cannot and does not
  guarantee an arbitrary reviewed command is itself deterministic; it
  guarantees only that the *runner's* own behavior around that command
  (workspace creation, env, timeout, output capture) introduces no
  additional nondeterminism.

This is a narrower guarantee than `library add`'s idempotency — there is no
"already present" outcome for `scenario run`, because there is nothing being
stored; idempotency here means "repeatable classification given a repeatable
command," not "second write is a no-op."

## Corruption and malformed-scenario handling

- **Scenario file fails to parse** (bad YAML/JSON, exceeds
  `MaxScenarioDocumentSize`): `scenario verify` exits `1`; `scenario run`
  reports `INTERNAL_ERROR` and exits `1`.
- **Scenario decodes but fails a semantic rule** (missing/wrong
  `schema_version`, empty `scenario.id`, empty `command`, bare `command[0]`,
  out-of-range `timeout_seconds`, a `workspace_files` entry with a
  traversal/absolute/symlinked/missing/duplicate-destination path, malformed
  `linked_fingerprint`, `expected.exit_code` absent, an unknown
  `stdout`/`stderr` `mode`, or an unparseable `regex` value): `scenario
  verify` exits `2`; `scenario run` reports `INVALID` and exits `1` —
  pre-execution rejection is a usage-class outcome for `run`, distinct from
  `verify`'s own "well-formed but semantically wrong" `2`, because for `run`
  nothing was ever attempted.
- **`--source` file fails to validate**: `scenario verify` exits `1` if it
  can't even be loaded, `2` if it loads but fails `validate.Validate` or its
  computed fingerprint doesn't match `linked_fingerprint`.
- **A declared `workspace_files` source file is missing at verify/run
  time**: reported distinctly (`ErrWorkspaceFileMissing`), `INVALID` for
  `run` if caught at the pre-execution check (the common case), or
  `INTERNAL_ERROR` if a TOCTOU race caused it to disappear between the
  check and the copy (documented residual gap).
- **`command[0]` does not exist or is not executable at run time**:
  `INTERNAL_ERROR`, exit `1` — caught as early as possible, immediately
  after workspace staging and before the process is ever started.
- **A pre-existing, non-empty `--workspace <dir>`**: refused immediately,
  `INTERNAL_ERROR`, exit `1` — the runner never writes into a directory it
  did not create empty.
- **Report file cannot be written** (`--report` path's parent doesn't
  exist, permission denied): reported to stderr and the command exits `1`
  even if the scenario itself PASSed — the run's own result is still
  printed to stdout before this failure is surfaced.

## Symlink and path-traversal protections

- The only values ever turned into a workspace filesystem path are a
  `workspace_files` `source` (resolved against, and re-checked to remain
  within, the scenario file's own directory) and `destination` (resolved
  against, and re-checked to remain within, the workspace root). Both are
  `filepath.Clean`ed and rejected outright if absolute or containing `..`.
- A `workspace_files` `source` that is a symlink is rejected, not followed,
  both at verify time and again (TOCTOU re-check) immediately before
  staging.
- `command[0]`, when workspace-relative, is deliberately **not**
  containment-checked against the workspace root — a reviewed scenario may
  legitimately reference a binary outside the workspace (e.g. the project's
  own built `incidentdna`), which is why `workspace_files` and `command[0]`
  have different path rules: one stages untrusted-shaped declared content
  into a sandboxed destination, the other names a program to execute, which
  the reviewer already approved by approving the scenario file.

## Why the demo uses an absolute command path

`examples/regression-scenario-demo/scenario-pass.yaml` and
`scenario-fail.yaml` reference the built `incidentdna` binary via an
absolute path (substituted at demo-run time by
`scripts/verify-scenario-demo.sh`), not a workspace-relative one like
`../../../bin/incidentdna`. A workspace-relative reference back to the
repository's `bin/` directory would need to know exactly how many directory
levels separate the run's workspace from the repository root — but the
default workspace is a fresh `os.MkdirTemp` directory under the OS temporary
directory, whose depth relative to any given checkout is neither fixed nor
predictable across machines and CI runners. This is a property of *this
demo's* portability requirement, not a limitation of the IRS v0.1 format
itself: `execution.command[0]` legitimately supports both absolute and
workspace-relative forms (`internal/scenario/validate_test.go` covers both),
and a scenario author who controls their own fixed `--workspace` location is
free to use a workspace-relative path exactly as `docs/phase-4-plan.md` §5
illustrates.

## Privacy implications

Scenario documents carry no dedicated privacy/redaction block (no
`privacy.redacted` equivalent) — unlike an IDIR document, a scenario is not
a permanent incident record; it is short-lived, reviewable execution
metadata. If a scenario's captured `stdout`/`stderr` or a `workspace_files`
fixture happens to contain sensitive content, that is entirely the author's
responsibility, exactly as it already is for evidence files and library
occurrence content — Phase 4 introduces no new automatic redaction or PII
scanning. Captured stdout/stderr in the report are bounded excerpts, not
full dumps beyond `MaxScenarioOutputBytes`, but nothing scans them for
sensitive content before writing the report file. No network access means
no telemetry and no external transmission of scenario content, ever. The
reviewed command's own privacy behavior — what it reads, what it does with
those bytes — is entirely outside this tool's control.

## No network access, no telemetry

Every `scenario` subcommand's own Go code is pure local file I/O and local
process execution: `grep -rn '"net' internal/scenario cmd/incidentdna/cmd_scenario.go`
finds no network package imported. This is a claim about the *runner's own
code*, not about what a scenario's *executed command* might itself do — see
"A new class of risk" above for that distinction.

## What Phase 4 explicitly does not provide

- **No sandbox.** Process isolation is argv/env/cwd/timeout/output bounding
  only — no seccomp, cgroups, container, or VM isolation. A reviewed
  scenario's command runs with the full permissions of the invoking user.
- **No suite runner.** Exactly one scenario per invocation; discovering,
  aggregating, or parallelizing multiple scenarios is not built.
- **No coupling to the incident library.** A scenario's `linked_fingerprint`
  is never checked against stored library occurrences automatically — only
  against a directly-named `--source` incident file.
- **No release gating.** `scenario run`'s exit code and JSON report are
  *available* to be consumed by something else, but `incidentdna` itself
  does not wire any scenario result into a gate, a policy decision, a merge
  check, or any other blocking mechanism.
- **Detached-process timeout evasion is a known, accepted gap.**
- **No enforcement that a reviewed command's own filesystem/network
  behavior stays within the workspace** — only the runner's own operations
  are bounded.
- **`--report`'s output path has no overwrite protection** — always
  overwrites the named path unconditionally.
- **Fixed, non-configurable resource limits.**
- **No automatic scenario generation using AI.** A scenario document is
  hand-authored; `incidentdna` only validates and runs one, never invents
  one.
- **No signing, no encryption, no multi-tenancy.**

None of these are claimed to be solved, partially solved, or planned for a
specific near-term release — they are simply out of scope for Phase 4, the
same way analogous items were out of scope for Phases 1 through 3.

## Why the regression-scenario runner does not affect fingerprints

`internal/fingerprint`'s identity payload is unchanged by Phase 4: nothing
the scenario runner introduces or reads (a scenario document, a workspace, a
captured output stream, or an execution report) is part of `idir.Document`
at all, let alone the identity payload. `fingerprint.Compute` is called
unchanged, exactly the way `internal/library` already calls it, only for the
optional `--source` cross-check — it never runs as part of `scenario run`
itself. `examples/duplicate-payment/incident.yaml` and
`testdata/golden/duplicate-payment.fingerprint` are untouched by this phase.

## The `regression-scenario-demo` example

`examples/regression-scenario-demo/` is a fourth, self-contained example,
entirely separate from `examples/duplicate-payment/`,
`examples/evidence-storage-demo/`, and `examples/incident-library-demo/`.
`fixtures/incident.yaml` is a frozen copy (not a symlink or reference) of
`examples/duplicate-payment/incident.yaml`'s content. Four scenario files
demonstrate `PASS`, `FAIL`, `TIMEOUT`, and `INVALID` end to end;
`scripts/verify-scenario-demo.sh` additionally demonstrates
`INTERNAL_ERROR`, `--source`, and `--keep-workspace`. See
`examples/regression-scenario-demo/README.md` for the exact commands and
expected output. No real credentials, personal information, or customer data
appear anywhere in the directory.

## Current Phase 4 status and remaining limitations

Implemented, per `internal/scenario/` and `cmd/incidentdna/cmd_scenario.go`:
IRS v0.1 document loading and semantic validation, the `--source`
fingerprint cross-check, the bounded local runner (all five outcomes), and
deterministic JSON report generation. This document describes that
implementation as it exists in the code today.

Known, accepted limitations, restated from the sections above so they are
findable in one place:

- Not a sandbox — bounded, not contained (see "A new class of risk").
- No suite runner; exactly one scenario per invocation.
- No automatic coupling to the incident library.
- Detached-process timeout evasion is a known, accepted gap.
- A reviewed command's own filesystem/network behavior is entirely outside
  this tool's control.
- `--report` always overwrites its target path.
- Eight fixed, non-configurable resource limits.
- No release gating — `scenario run`'s result is available to be consumed
  by something else, but nothing in this codebase consumes it.

This document intentionally does not claim CI results, production usage, or
performance benchmarks for this phase — none of those have been established
here.
