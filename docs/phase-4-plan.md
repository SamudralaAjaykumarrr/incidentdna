# Phase 4 Plan: Deterministic Executable Regression Scenarios (Draft for Approval)

**Status: direction approved, concrete design proposed here for review.**
`Deterministic Executable Regression Scenarios` has been selected as the
Phase 4 increment. This document proposes the concrete scope, format,
safety model, and CLI surface needed to build it. **No implementation has
started.** No line of Go code, no schema, no example, and no test has been
written against this plan; nothing in this document has been merged into
`Makefile`, CI, or any existing package. It follows the same role
`docs/phase-3-plan.md` played before Phase 3 implementation began — a
settled design to implement against, not an implementer's inference — with
the difference that this revision has not yet been through a decisions
round-trip with the user (`phase-3-plan.md` §20's "Approved decisions"
pattern). Everything below is proposed, pending that review.

## 1. Problem statement

Phases 1–3 built a complete *memory* system: incidents can be represented
(IDIR), validated, deterministically fingerprinted by failure class,
integrity-checked against stored evidence, and accumulated in a local
library grouped by fingerprint. None of that memory is ever *exercised*.
`incidentdna library check` can tell an engineer "a failure with this
fingerprint has been recorded before" — but there is no way to ask the
sharper question a regression suite exists to answer: **"if I run something
locally right now, does it still reproduce (or still avoid) that recorded
failure?"** Closing that gap requires introducing, for the first time in
this codebase, local process execution — which is exactly why this phase
carries a much stricter constraint list than Phases 1–3: execution is a new
class of risk (arbitrary code, non-termination, unbounded output, filesystem
writes outside the intended scope) that reading/hashing/storing documents
never was.

Phase 4 closes this gap narrowly: it defines a small, reviewable, versioned
document format — a **regression scenario** — that a local runner can
execute deterministically, compare against a declared expectation, and
report pass/fail/timeout/invalid/internal-error for, without becoming a
general-purpose test framework, a CI product, or a sandboxing platform.

## 2. Goals

1. Define a **deterministic, reviewable-as-text scenario document format**
   (tentatively **IRS v0.1**, "Incident Regression Scenario") that declares:
   one bounded local command to execute, the fixed inputs it runs against,
   and the expected outcome to compare against.
2. Let a scenario **declare which failure class it regression-tests** by
   referencing an existing IDIR fingerprint (`internal/fingerprint.Compute`,
   unchanged) — and let that declaration be **mechanically checked** against
   a real incident file, not just asserted in prose.
3. Provide a **local runner** (`incidentdna scenario run`) that executes
   exactly one declared command, with no shell interpretation, inside a
   freshly created, bounded, non-persistent workspace, under a timeout and
   output cap, and classifies the result into one of five outcomes: PASS,
   FAIL, TIMEOUT, INVALID, INTERNAL_ERROR.
4. Provide a **local verifier** (`incidentdna scenario verify`) that checks
   a scenario document's structural and semantic validity — and, optionally,
   its fingerprint linkage against a real incident file — **without ever
   executing anything**, mirroring the existing `validate`/`evidence
   verify`/`library check` split between "is this trustworthy to run" and
   "did running it produce the expected result."
5. Emit a **machine-readable execution report** (JSON) suitable for a human
   or a future tool to consume, alongside the existing human-readable stdout
   summary every other subcommand already produces.
6. Do all of this **without touching, or depending on the presence of,**
   `internal/evidence` or `internal/library` — scenario execution is a
   fourth parallel leaf concern, exactly as `internal/library` was a third,
   relative to `internal/evidence`'s precedent as the second.
7. Make the whole feature **testable without any external service**: no
   Docker daemon, no network, no long-running process, so `go test
   ./internal/scenario/... -race -count=1` is self-contained the same way
   every existing package's test suite already is.

## 3. Explicit non-goals

Restating and extending the design constraints given for this phase, each
tied to why it is excluded now rather than merely unmentioned:

- **No release gating.** `scenario run`'s exit code is meaningful and could
  in principle be consumed by an external CI job, exactly the way `library
  check`'s exit code already could be — but IncidentDNA itself does not
  wire any scenario result into a gate, a policy decision, a merge check, or
  any other blocking mechanism. See §28.
- **No scenario discovery, aggregation, or "run all" command.** Phase 4
  operates on exactly one scenario file per invocation, mirroring
  `evidence inspect`'s "one digest, no batch mode" discipline. A directory
  walk / suite runner is deliberately deferred (§27).
- **No coupling to the incident library or evidence store.** A scenario's
  `linked_fingerprint` is checked, at most, against a fingerprint computed
  fresh from an incident file the caller names directly
  (`--source <file>`) — never by looking up `internal/library`. This keeps
  `internal/scenario` independent and keeps a scenario runnable in a
  checkout that has never run `library add`.
- **No arbitrary shell execution and no unrestricted command execution.**
  The runner never invokes `/bin/sh -c` or any shell; it never performs
  `$PATH` lookup for a bare command name. A scenario's `execution.command`
  must be a full argv list whose first element is either an absolute path
  or a path relative to the scenario's own workspace — both fully visible
  to a human reviewing the scenario file, with nothing resolved implicitly
  from the invoking machine's environment.
- **No Docker socket access, no container/VM/chroot isolation, no cgroups.**
  Isolation is process-level only (§9) — a deliberate, documented boundary,
  not an oversight.
- **No host filesystem writes outside an explicitly created temporary
  workspace.** The runner itself never writes anywhere else; see §9 for
  what this guarantee does and does not cover.
- **No mutation of incident-library occurrences or evidence-store
  objects.** `internal/scenario` does not import `internal/library` or
  `internal/evidence`, and no scenario command opens either store for
  writing. (An optional, read-only fingerprint cross-check against a named
  incident *file* is in scope — see §2.2 — but that never touches a store.)
- **No change to fingerprint identity semantics.** `internal/fingerprint`
  is called, unchanged, exactly the way `internal/library` already calls
  it. No new field is added to `idir.Document`, no new dimension is added
  to the identity payload.
- **No automatic scenario generation using AI.** A scenario document is
  hand-authored (or generated by some future, separate, human-supervised
  tool) — `incidentdna` itself only validates and runs one, never invents
  one.
- **No signing, no encryption, no multi-tenancy, no network, no
  telemetry** — the same standing invariants every prior phase restates and
  Phase 4 restates unconditionally again (§18, §19).
- **No configurable resource limits.** As with Phases 2 and 3, every bound
  in §10 is a fixed, named Go constant, not a config surface.

## 4. User workflows

**Workflow A — link a scenario to a specific incident, and check the
link is real:**

```
$ incidentdna scenario verify --source examples/duplicate-payment/incident.yaml \
    examples/regression-scenario-demo/scenario.yaml
Scenario: duplicate-payment-refingerprint (schema irs/v0.1)
Linked fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
Source fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
OK: scenario is structurally valid and its linked_fingerprint matches --source
```

This runs the exact same `idir.LoadFile` → `validate.Validate` →
`fingerprint.Compute` pipeline every other command uses against
`--source`, and never executes the scenario's own command.

**Workflow B — run a scenario and see whether the release still avoids
(or still reproduces) the recorded failure:**

```
$ incidentdna scenario run examples/regression-scenario-demo/scenario.yaml
Scenario: duplicate-payment-refingerprint
Result: PASS
Command exited 0 (expected 0)
Duration: 42ms
Workspace: removed (pass --keep-workspace to retain it)
```

Exit code `0`. A `--report <file>` flag additionally writes the same
outcome as one deterministic JSON document (§15).

**Workflow C — a scenario that fails, times out, or is malformed** exercises
the other four outcomes; see §11's examples and §24's demo requirements for
concrete scenario files that deliberately produce each one.

## 5. Scenario document format (IRS v0.1)

A scenario is a small, standalone YAML or JSON document — deliberately not
an extension of `idir.Document` and not decoded through `internal/idir` at
all, since a scenario describes an *execution*, not an *incident*. New,
dedicated types in `internal/scenario`, loaded through a new,
similarly size-capped loader (`internal/scenario.LoadFile`, mirroring
`idir.LoadFile`'s size-cap-then-typed-decode discipline, not reusing it,
since the two types are unrelated).

```yaml
schema_version: irs/v0.1

scenario:
  id: duplicate-payment-refingerprint
  title: Duplicate payment example still fingerprints and validates cleanly
  description: >
    Re-validates and re-fingerprints the frozen duplicate-payment example
    through the compiled incidentdna binary, and asserts the result exactly
    matches the pinned golden fingerprint.

linked_fingerprint: "sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e"

execution:
  workspace_files:
    - source: fixtures/incident.yaml     # relative to this scenario file's own directory
      destination: incident.yaml         # relative path inside the workspace
  command:
    - "../../../bin/incidentdna"         # resolved relative to the workspace root
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

- `schema_version` — fixed literal `irs/v0.1` for this phase, checked the
  same way `idir.Document.SchemaVersion` is checked today (exact match,
  reject anything else, so a future `irs/v0.2` cannot be silently
  misinterpreted).
- `scenario.id` — stable, author-chosen identifier; not used as a
  filesystem path component anywhere (no store, so nothing to key by it);
  present only for human/report readability.
- `linked_fingerprint` — required, format-checked exactly like
  `evidence[].digest` (`^sha256:[0-9a-f]{64}$`), but **not** required to
  exist in any local library — a scenario must be runnable in a checkout
  that has never run `library add`. It is a declared, reviewable claim
  ("this scenario regression-tests this failure class"), mechanically
  checkable against a real incident file only when `--source` is supplied
  (§4 Workflow A).
- `execution.workspace_files` — a bounded list (`MaxWorkspaceFiles`, §10)
  of `{source, destination}` pairs. `source` is resolved relative to the
  scenario file's own directory; `destination` is resolved relative to the
  workspace root. Both are rejected if they are absolute, contain `..`, or
  resolve (after `filepath.Clean`) outside their respective root —
  structurally, the same discipline `internal/evidence`/`internal/library`
  already apply to digest-derived paths, applied here to declared relative
  paths instead. A `source` that is a symlink is rejected, not followed.
- `execution.command` — a required, non-empty argv list. `command[0]` must
  be either an absolute path or a path relative to the workspace root; bare
  command names (`"python3"`, `"sh"`) are rejected at verify time, because
  resolving those would mean an implicit, host-dependent `$PATH` lookup —
  exactly the kind of hidden, non-reviewable behavior this format is
  designed to avoid (§8, §11). The runner never wraps this argv in a shell.
- `execution.env` — an optional, fixed map of extra environment variables
  set for the child process. The child's environment is **not** inherited
  from the invoking `incidentdna` process (§7) — only a minimal fixed base
  (`PATH` unset, `HOME` pointed at the workspace, `TZ=UTC`) plus whatever
  `env` declares.
- `execution.timeout_seconds` — optional; defaults to
  `DefaultScenarioTimeoutSeconds` (30) if omitted, must not exceed
  `MaxScenarioTimeoutSeconds` (300) — checked at verify time, not just at
  run time, so an over-limit scenario is rejected before any process is
  ever started.
- `expected.exit_code` — required integer.
- `expected.stdout` / `expected.stderr` — optional; if present, `mode` is
  one of `exact`, `contains`, `regex`, and `value` is the string/pattern to
  compare against the captured (and possibly truncated, §10) stream. Absent
  means "not checked" — a scenario may assert only the exit code if that is
  all that matters for it.

## 6. Relationship between the five artifacts

```
incident document (idir.Document)
   │  idir.LoadFile + validate.Validate            (unchanged, Phase 1)
   ▼
valid idir.Document
   │  fingerprint.Compute                          (unchanged, Phase 1)
   ▼
failure fingerprint  ──────────────────────┐
   │  library.Add (optional, independent)  │  scenario.linked_fingerprint
   ▼                                        │  (declared in the scenario file,
incident-library occurrence(s)              │   format-checked always, and
   (Phase 3, grouped by fingerprint,        │   cross-checked against a fresh
    zero or more per fingerprint,           │   fingerprint.Compute(--source)
    entirely optional for Phase 4           │   only when --source is given)
    to have ever run)                       │
                                             ▼
                                   regression scenario (IRS v0.1 document)
                                             │  scenario.Run
                                             ▼
                                   execution result / report (§15)
```

The load-bearing relationship is the **dashed line through the
fingerprint**, not through the library: a scenario names a *failure class*
(a fingerprint value), the same identity every occurrence in the library
already shares, but Phase 4 does not require an occurrence to exist, be
looked up, or be modified for a scenario to be valid or runnable. This
keeps `internal/scenario` a true parallel leaf package (§3) and keeps the
demo (§24) runnable without ever invoking `library add`. A future phase
that wants "does this scenario's fingerprint have library occurrences" is a
pure read-only composition of two already-existing pieces
(`library.Check` + a scenario's `linked_fingerprint` field) and needs no
change to either package — named explicitly as a future integration point,
not built now (§28).

## 7. Determinism requirements

1. **Fingerprint linkage is exactly as deterministic as Phase 1's
   fingerprint already is** — `--source` cross-checks call
   `fingerprint.Compute` unchanged; no new hashing logic is introduced.
2. **The child process environment is fixed, not inherited.** The invoking
   user's shell environment (arbitrary `$PATH`, locale, credentials in env
   vars, etc.) is never passed through. Only a minimal fixed base plus the
   scenario's own declared `execution.env` reaches the child. This is the
   single largest source of non-reproducibility an unconstrained "just run
   this command" tool would have, and Phase 4 closes it structurally.
3. **`command[0]` resolution has no implicit `$PATH` search** (§5, §11) —
   what runs is exactly what a reviewer of the scenario file can see, not
   whatever happens to be first on some machine's `$PATH` that day.
4. **`workspace_files` staging is byte-for-byte and order-independent.**
   Each declared file is copied verbatim (no templating, no substitution);
   staging order does not affect the resulting workspace contents, since
   destinations are distinct paths (validated at verify time — a scenario
   with two entries mapping to the same `destination` is rejected as
   invalid, not silently last-write-wins).
5. **Output capture is bounded and stream-separated.** stdout and stderr
   are captured into independent, size-capped buffers (§10) — never
   interleaved — so `expected.stdout`/`expected.stderr` assertions are
   comparing exactly the bytes the child wrote to that specific stream, up
   to the cap.
6. **The runner performs no network access and no telemetry itself.**
   `grep -rn '"net' internal/scenario cmd/` must stay empty, the same
   invariant every prior phase's acceptance criteria checked (§18). This
   is a claim about the *runner's own Go code*, not about what a scenario's
   *executed command* might itself do — see §8 for that distinction, stated
   as a residual, documented limitation rather than a guarantee.
7. **Absolute paths must not leak into deterministic assertions by
   default.** Because the workspace root is a fresh temporary directory
   with a different absolute path on every run, `expected.stdout`/`stderr`
   assertions that need to reference "the workspace" should use paths
   relative to the working directory (which *is* the workspace root) —
   documented as an authoring rule in `docs/regression-scenarios.md` (§15
   of the implementation slice list), not mechanically enforced, since the
   runner cannot know which substrings of a command's output are
   meaningfully path-shaped.
8. **`scenario verify`'s result depends only on the scenario file's own
   bytes** (and, if given, `--source`'s bytes) — never on prior runs, the
   filesystem state of any workspace, or wall-clock time.

## 8. Safety model

Phase 4 introduces local process execution for the first time in this
codebase, so its safety model is explicitly weaker in one dimension than
Phases 1–3's pure-data-and-filesystem operations, and the plan states this
plainly rather than implying a guarantee that isn't real:

- **The runner's own code never shells out, never does implicit `$PATH`
  resolution, never writes outside the workspace it creates, and never
  reads any file the scenario document didn't explicitly declare.** This
  part is a real, testable guarantee (§9, §23).
- **The runner does not sandbox the process it launches.** Once
  `execution.command` starts, it runs with the invoking user's full OS-level
  permissions (no seccomp, no cgroups, no container, no chroot — explicitly
  excluded, §3) and can, in principle, read/write anywhere that user's
  account can, or reach the network, exactly as any locally-executed program
  can. This is the same "trust boundary is the invoking user's OS
  permissions, not a sandbox" stance `docs/threat-model.md` already states
  for every prior phase's filesystem operations, extended here to process
  execution rather than newly invented.
- **The actual safety mechanism is human review before running, not runtime
  containment.** This is why the design constraint requires scenario
  definitions to be reviewable text files: a scenario's `execution.command`
  is fully visible before it is ever run (`scenario verify` never executes
  anything), and nothing about the format allows a command to be
  constructed, discovered, or mutated at run time. A reviewer who approves a
  scenario file is approving exactly the command that will run — the same
  epistemic model this project already applies to `privacy.redacted`
  (author-declared, mechanically checked for a fixed rule, not proven true)
  and to evidence/library digest integrity (proves internal consistency,
  not truthfulness). Phase 4 does not claim to solve "run untrusted code
  safely"; it claims to make *reviewed* local commands runnable
  deterministically and boundedly.
- **Bounded, not sandboxed, is the operative phrase for §9–§11**: timeouts,
  output caps, and workspace containment bound how long a *reviewed* command
  can run and how much it can write to stdout/stderr and to the one
  directory it's launched in — they do not attempt to bound what a
  malicious command could do with the invoking user's actual permissions.

## 9. Execution isolation boundaries

| Boundary | Mechanism | Guarantee level |
|---|---|---|
| No shell interpretation | `exec.CommandContext(ctx, argv[0], argv[1:]...)` — never `sh -c`, never a string command line | Structural — there is no code path that constructs a shell command string |
| No implicit `$PATH` lookup | `command[0]` must be absolute or workspace-relative; resolved and existence/executable-checked at verify time; `exec.LookPath` is never called | Structural |
| Workspace containment (runner's own writes) | A fresh directory created via `os.MkdirTemp` (default) or an explicitly empty, runner-created `--workspace <dir>` (fails if the given path already exists and is non-empty); every `workspace_files` destination is resolved and re-checked to have the workspace root as a prefix before any write, mirroring `library.Store.checkContained` | Structural, for the runner's own file operations |
| Child process cwd | Set to the workspace root; the child cannot be launched with any other working directory | Structural |
| Child process environment | Fixed minimal base + `execution.env` only, never `os.Environ()` passed through | Structural |
| Child process own filesystem/network access | **Not bounded** — the child has full OS-level permissions of the invoking user | Documented limitation, not a guarantee (§8) |
| Time | `context.WithTimeout` wrapping the whole `exec.CommandContext` call; on expiry the process (and its process group, to catch children it spawns) is killed | Structural for the parent-tracked process tree; a detached grandchild that escapes the process group is a known, documented residual gap, consistent with `docs/threat-model.md`'s existing TOCTOU-class disclaimers |
| Output volume | Both stdout and stderr captured through independent `io.LimitReader`-backed bounded buffers (§10); once a cap is hit, further bytes are discarded (not buffered) and the report marks that stream `truncated: true` — the process itself is not killed merely for exceeding the output cap, only for exceeding the timeout | Structural |
| Persistence | The workspace is removed after the run completes (any outcome) unless `--keep-workspace` is passed, in which case its path is printed/reported and left for manual inspection/cleanup | Structural |

## 10. Timeouts and resource limits

New, independent, named constants in `internal/scenario/limits.go`, each
with its own distinct error and its own passing/failing boundary test pair
— the same discipline `internal/evidence/limits.go` and
`internal/library/limits.go` already established:

| Constant | Value | Applies to |
|---|---|---|
| `MaxScenarioDocumentSize` | 1 MiB | The scenario file's own on-disk size, enforced by `scenario.LoadFile` the same way `idir.LoadFile` enforces `MaxDocumentSize` |
| `MinScenarioTimeoutSeconds` | 1 | Lower bound on a declared `timeout_seconds` (rejects `0`/negative as invalid rather than silently treating it as "no timeout") |
| `DefaultScenarioTimeoutSeconds` | 30 | Used when `timeout_seconds` is omitted |
| `MaxScenarioTimeoutSeconds` | 300 | Upper bound a declared `timeout_seconds` must not exceed; checked at verify time, not only at run time |
| `MaxWorkspaceFiles` | 50 | Number of `workspace_files` entries a scenario may declare |
| `MaxWorkspaceFileSize` | 10 MiB | Size of any single declared `workspace_files` source file |
| `MaxWorkspaceTotalBytes` | 50 MiB | Sum of all staged `workspace_files` sizes |
| `MaxScenarioOutputBytes` | 1 MiB | Captured bytes per stream (stdout, stderr independently) before truncation |

`scenario run` is **not** run under `main.go`'s existing 30-second overall
command context — that timeout was designed for parsing/hashing a document,
a categorically different workload from executing an arbitrary bounded
child process. `scenario run` instead uses its own context derived from
`timeout_seconds` (bounded by `MaxScenarioTimeoutSeconds` above), so a
legitimately slower regression check is not truncated by an unrelated,
smaller global budget. `scenario verify` (which never executes anything)
continues to run under the existing 30-second command context, unchanged,
exactly like `validate`/`library check`.

## 11. Allowed and forbidden operations

**Allowed:**

- Reading the scenario file itself, and each declared `workspace_files`
  `source` file, both resolved relative to fixed, validated roots.
- Creating and writing inside exactly one freshly created temporary
  workspace directory.
- Launching exactly one child process via `exec.CommandContext`, argv-only,
  no shell, with a fixed cwd and a fixed/declared environment.
- Reading the child's stdout/stderr up to `MaxScenarioOutputBytes` per
  stream.
- Writing exactly one report file, only if `--report <path>` is given —
  the path is taken directly from the command line (same trust boundary as
  any file path argument elsewhere in this project) and is overwritten
  unconditionally on each run (no append, no versioning — see §16).
- Removing the temporary workspace on completion (default) or leaving it in
  place (`--keep-workspace`).

**Forbidden (by construction, not merely by convention):**

- Shell interpretation of any kind (`sh -c`, backticks, globbing performed
  by a shell rather than the child program itself).
- Implicit `$PATH` resolution of `command[0]`.
- Any write by the runner's own code outside the resolved workspace root
  (belt-and-suspenders path-containment check, mirroring
  `library.Store.checkContained`).
- Any read of `workspace_files` `source` paths that are absolute, contain
  `..`, or resolve outside the scenario file's own directory; any symlink
  encountered at a `source` or `destination` path is rejected, not
  followed.
- Any mutation of `.incidentdna/evidence/objects` or
  `.incidentdna/library/objects` — `internal/scenario` never imports
  `internal/evidence` or `internal/library`.
- Any change to `idir.Document`, the JSON Schema, or the fingerprint
  algorithm.
- Spawning a Docker container, opening a Docker socket, or any other
  container-runtime interaction.
- Network access by the runner's own code (child-process network access is
  not forbidden by the runner — it is simply unbounded and undetected, §8).

## 12. Local filesystem layout

Unlike `internal/evidence` and `internal/library`, Phase 4 introduces **no
new permanent local store**. There is no `.incidentdna/scenario/...`
default root, because a scenario has nothing durable to persist — running
it is the entire point, and its only optional durable output is a
user-named `--report <file>`. The only filesystem footprint at run time is:

```
<workspace root>/                      os.MkdirTemp() by default, or an
                                         explicitly empty --workspace <dir>
  <staged workspace_files, per their
   declared destination paths>
  (the child process's own writes, if any — unbounded, see §8)
```

removed after the run unless `--keep-workspace` is given. `.gitignore`
requires no new entry, since nothing is written under the repository by
default (the temporary workspace lives under the OS temp directory unless
`--workspace` explicitly points elsewhere).

## 13. CLI commands

```
incidentdna scenario verify [--source <incident-file>] <scenario-file>
    Load and structurally/semantically validate <scenario-file> against the
    IRS v0.1 rules (schema_version, non-empty command with a non-bare
    command[0], workspace_files path safety, timeout bounds, linked_fingerprint
    format). If --source is given, additionally load, validate, and
    fingerprint <incident-file> through the unchanged Phase 1 pipeline and
    assert the result equals the scenario's declared linked_fingerprint.
    Never executes the scenario's command.

incidentdna scenario run [--workspace <dir>] [--report <file>]
                          [--keep-workspace] <scenario-file>
    Run the same checks as `scenario verify` (without --source). If they
    pass, create a workspace, stage workspace_files, execute the declared
    command under the declared/default timeout with output capped at
    MaxScenarioOutputBytes per stream, compare the actual outcome against
    `expected`, print a human-readable summary, and (if --report is given)
    write the same outcome as one deterministic JSON report file (§15).
```

`main.go`'s `commands` table gains one entry, `{"scenario", runScenario}`,
following the exact `{"evidence", runEvidence}` / `{"library", runLibrary}`
precedent. `runScenario` self-dispatches `verify`/`run` using the same
`flag.NewFlagSet` idiom every existing command already uses — no CLI
framework. `init`, `validate`, `fingerprint`, `inspect`, `compare`, all four
`evidence` subcommands, and all three `library` subcommands remain
byte-for-byte unchanged in behavior.

## 14. Exit-code contract

**`scenario verify`** (mirrors `validate`'s existing 0/1/2 split):

| Exit | Meaning |
|---|---|
| `0` | Scenario is structurally and semantically valid (and, if `--source` given, its `linked_fingerprint` matches) |
| `1` | I/O/parse/usage error — can't read the scenario file or `--source` file, bad CLI usage |
| `2` | Scenario decodes but fails a semantic IRS v0.1 rule, or (with `--source`) fingerprint mismatch |

**`scenario run`** (extends the same 0/1/2 philosophy to five named
outcomes):

| Outcome | Exit | Meaning |
|---|---|---|
| `PASS` | `0` | Command ran to completion within the timeout and matched every declared `expected` field |
| `FAIL` | `2` | Command ran to completion within the timeout but did not match `expected` (exit code and/or stdout/stderr assertion) |
| `TIMEOUT` | `2` | Command was launched but killed after exceeding `timeout_seconds` — treated as a real, semantic negative finding, not a usage error |
| `INVALID` | `1` | Scenario failed the same pre-execution checks `scenario verify` performs — nothing was ever executed |
| `INTERNAL_ERROR` | `1` | An environmental failure prevented determining PASS/FAIL/TIMEOUT — workspace creation failed, `command[0]` could not be found/executed, a `workspace_files` staging I/O error, etc. |

`FAIL`/`TIMEOUT` sharing exit `2` and `INVALID`/`INTERNAL_ERROR` sharing
exit `1` mirrors the existing project-wide convention that `2` means
"well-formed input, materially negative finding" and `1` means
"environmental/usage failure" — `library check`'s exit-code documentation
(`docs/incident-library.md`) already states explicitly that exit code `2`
is reused for *different specific conditions* across commands; this plan
follows that same documented precedent rather than inventing a new exit
code space.

## 15. Result and report format

Two outputs, always produced together by `scenario run`:

1. **Human-readable stdout summary** (always printed), in the style every
   existing command already uses — see §4's transcripts.
2. **Machine-readable JSON report**, written to `--report <path>` only if
   given; deterministic field order via a fixed Go struct (no map
   iteration), one file per run, overwritten each time:

```json
{
  "schema_version": "irs-report/v0.1",
  "scenario_id": "duplicate-payment-refingerprint",
  "scenario_file": "examples/regression-scenario-demo/scenario.yaml",
  "linked_fingerprint": "sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e",
  "result": "PASS",
  "exit_code_expected": 0,
  "exit_code_actual": 0,
  "duration_ms": 42,
  "stdout": {"excerpt": "sha256:fc3dac...", "truncated": false},
  "stderr": {"excerpt": "", "truncated": false},
  "workspace_path": null,
  "error": null
}
```

`workspace_path` is populated only when `--keep-workspace` was given.
`error` is populated only for `INVALID`/`INTERNAL_ERROR`, with a short,
actionable, content-free-of-secrets message (no captured environment
variables or full command output embedded in the error string itself —
those already have their own `stdout`/`stderr` fields, bounded the same
way).

## 16. Idempotency behavior

`scenario run` has **no persistent side effects on the repository or any
store** — it does not write into `.incidentdna/evidence` or
`.incidentdna/library`, does not modify the scenario file, and does not
accumulate state between runs. Re-running the same scenario against an
unchanged environment:

- Creates a brand-new, independent workspace each time (never reuses or
  appends to a prior one).
- Removes it afterward (default) — nothing observable persists.
- If `--report <path>` is given, **overwrites** that single file each run
  (not appended, not versioned, not timestamped into the filename) — so
  running twice with the same `--report` value leaves exactly one, current
  report on disk, matching the deterministic-output requirement (§7).
- Produces the same `result` classification on every run **only if the
  declared command itself is deterministic** — Phase 4 cannot and does not
  guarantee an arbitrary reviewed command is itself deterministic; it
  guarantees that the *runner's* behavior around that command (workspace
  creation, env, timeout, output capture) introduces no additional
  nondeterminism of its own (§7).

This "idempotent" claim is a different, narrower guarantee than
`library add`'s idempotency (§ of `docs/incident-library.md`) — there is no
"already present" outcome for `scenario run`, because there is nothing
being stored; idempotency here means "repeatable classification given a
repeatable command," not "second write is a no-op."

## 17. Corruption and malformed-scenario handling

- **Scenario file fails to parse** (bad YAML/JSON, exceeds
  `MaxScenarioDocumentSize`): `scenario verify` exits `1`; `scenario run`
  reports `INTERNAL_ERROR` and exits `1` (this is an I/O/parse failure, the
  same class `scenario verify` reports as exit `1` — a document that never
  even decoded cannot be meaningfully called "semantically INVALID" versus
  "couldn't be read at all," so it is classified with `INTERNAL_ERROR`
  rather than `INVALID` to keep that distinction visible in the report's
  `error` field, even though both currently map to exit `1`).
- **Scenario decodes but fails a semantic rule** (missing `schema_version`,
  empty `command`, bare `command[0]`, out-of-range `timeout_seconds`, a
  `workspace_files` entry with a traversal/absolute/symlinked path,
  duplicate `destination` values, malformed `linked_fingerprint`,
  `expected.exit_code` absent, an unknown `stdout`/`stderr` `mode`):
  `scenario verify` exits `2`; `scenario run` reports `INVALID` and exits
  `1` (§14 — pre-execution rejection is a usage-class outcome for `run`,
  distinct from `verify`'s own "well-formed but semantically wrong" `2`,
  because for `run` nothing was ever attempted).
- **`--source` file fails to validate**: `scenario verify` exits `1` if it
  can't even be loaded, `2` if it loads but fails `validate.Validate` or
  its computed fingerprint doesn't match `linked_fingerprint`.
- **A declared `workspace_files` source file is missing at verify/run
  time**: reported distinctly (`ErrWorkspaceFileMissing`), `INVALID` for
  `run` if caught at the pre-execution check, `INTERNAL_ERROR` if a TOCTOU
  race caused it to disappear between check and copy (documented residual
  gap, consistent with §9's process-tree caveat and the project's existing
  TOCTOU disclaimers in `docs/incident-library.md`).
- **`command[0]` does not exist or is not executable at run time**:
  `INTERNAL_ERROR`, exit `1` — this is caught as early as possible (a
  pre-flight `os.Stat` + executable-bit check during the same
  pre-execution phase `scenario verify` runs), so the common case surfaces
  as a fast, clear error rather than an opaque `exec` failure.
- **A pre-existing, non-empty `--workspace <dir>`**: refused immediately,
  `INTERNAL_ERROR`, exit `1` — the runner never writes into a directory it
  did not create empty, mirroring `init`'s "won't silently overwrite"
  discipline (§9).
- **Report file cannot be written** (`--report` path's parent doesn't
  exist, permission denied): reported to stderr and the command exits `1`
  even if the scenario itself PASSed — the run's own result is still
  printed to stdout before this failure is surfaced, but the process exit
  code reflects that the requested report could not be produced.

## 18. Privacy implications

- **Scenario documents carry no dedicated privacy/redaction block** (no
  `privacy.redacted` equivalent) — unlike an IDIR document, a scenario is
  not a permanent incident record; it is short-lived, reviewable execution
  metadata. If a scenario's `stdout`/`stderr` excerpt or a `workspace_files`
  fixture happens to contain sensitive content, that is entirely the
  author's responsibility, exactly as it already is for evidence files
  (`docs/privacy-model.md`, "Evidence storage") and library occurrence
  content — Phase 4 introduces no new automatic redaction or PII scanning,
  consistent with the project-wide stance that redaction is
  mechanically-checked-when-declared, never automatic.
- **Captured stdout/stderr in the report are bounded excerpts, not full
  dumps beyond `MaxScenarioOutputBytes`**, but nothing scans them for
  sensitive content before writing the report file — the same "no PII
  detection over captured content" stance `docs/privacy-model.md` already
  states for stored evidence bytes and library occurrences.
- **No network access means no telemetry, no external transmission of
  scenario content, ever** — restating the unconditional project invariant
  (§3, §11).
- **The child process's own privacy behavior is entirely outside this
  tool's control** — if a reviewed command itself reads sensitive local
  files or reaches the network, that is a property of the reviewed command,
  not of `incidentdna scenario run`, and is explicitly out of scope to
  detect or prevent (§8).

## 19. Threat model

New section proposed for `docs/threat-model.md`, "Executable regression
scenarios: local process-execution risks (Phase 4)," analogous in style to
the existing "Evidence store" and "Incident library" sections:

- **Malicious or careless scenario authorship.** A scenario file is
  reviewable text, but nothing in Phase 4 prevents its author from
  declaring a `command` that is itself destructive, reaches the network, or
  reads unrelated local files — the safety model is *review before
  running*, not runtime sandboxing (§8). This is stated as an accepted,
  fundamental limitation, not a gap to be closed later without a real
  sandboxing mechanism (containers/VMs/seccomp), which is explicitly out of
  scope for this phase (§3).
- **Workspace path-traversal via `workspace_files`.** Mitigated
  structurally: both `source` (resolved against the scenario file's own
  directory) and `destination` (resolved against the workspace root) are
  `filepath.Clean`ed and re-checked to have their respective root as a
  prefix before any read/write; symlinks at either path are rejected, not
  followed — the same defense-in-depth pattern `internal/library`'s
  `checkContained` already established, applied to declared relative paths
  instead of digest-derived ones.
- **Resource exhaustion via a runaway or malicious command.** Mitigated by
  `timeout_seconds` (hard-killing the process tree at the bound) and
  per-stream output caps (§10) — bounds *this tool's* exposure (hang
  forever, consume unbounded memory buffering output), not the reviewed
  command's own resource usage on the host, which remains bounded only by
  whatever OS-level limits (ulimits, cgroups) the invoking environment
  already applies outside `incidentdna`.
- **A scenario's `command[0]` being itself a shell or interpreter.** The
  runner refuses to *invoke* a shell itself, but cannot prevent a reviewed
  scenario's own declared `command[0]` from being `/bin/sh` (or `python3`,
  `perl`, etc., referenced by an explicit path) — stated explicitly as a
  limitation of the format rather than implied to be prevented: the "no
  arbitrary shell execution" design constraint is a property of the
  *runner's own code path*, and the reviewability requirement (§8) is the
  actual control on what an author declares.
- **TOCTOU between `scenario verify`'s pre-execution checks and
  `scenario run`'s actual execution.** A `workspace_files` source file or
  `command[0]` could change or disappear between the check and the use —
  the same class of gap `docs/incident-library.md`'s "Filesystem and TOCTOU
  limitations" already accepts for the library, restated here for process
  execution rather than newly discovered.
- **Detached child processes escaping timeout enforcement.** `context`-based
  cancellation reliably terminates the direct child; a grandchild that
  double-forks or otherwise detaches from the process group may survive
  past the timeout — documented as a known, accepted gap (§9), consistent
  with this project's stance that OS-level guarantees beyond what Go's
  standard library provides are not independently re-implemented.
- **No network access, no telemetry, from the runner's own code** —
  restated as unconditional (§7, §18), verified the same way every prior
  phase verified it: `grep -rn '"net' cmd/ internal/` stays empty for
  everything except the intentional, already-reviewed absence of any such
  import in `internal/scenario` itself.

## 20. Backward compatibility with Phases 1, 2, and 3

- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library` are **untouched** — `internal/scenario` imports only
  `internal/idir`, `internal/validate`, and `internal/fingerprint` (the
  exact three needed for the optional `--source` fingerprint cross-check,
  reusing them unchanged, the same "leaf package importing shared
  primitives, imported by nothing" shape `internal/evidence` and
  `internal/library` already established), and is imported by nothing else
  in `internal/`.
- `examples/duplicate-payment/incident.yaml`,
  `testdata/golden/duplicate-payment.fingerprint`,
  `examples/evidence-storage-demo/`, and `examples/incident-library-demo/`
  are **not modified** — the new example (§24) is a new, separate
  directory that only *references* the duplicate-payment example's
  fingerprint value and reads a copy of its content into its own fixtures,
  never edits the original files.
- All existing CLI subcommands (`init`, `validate`, `fingerprint`,
  `inspect`, `compare`, all four `evidence` subcommands, all three
  `library` subcommands) keep their exact current exit-code and output
  behavior — proven the same way Phase 3 proved it: the full existing test
  suite passes unmodified, and no file under any of those packages appears
  in the implementation diff.
- No `schema_version` bump for IDIR — Phase 4 does not touch the incident
  document format at all. IRS v0.1 is a wholly new, separate format with
  its own version string.
- A document authored under Phase 1, 2, or 3 (with no awareness Phase 4
  exists) is usable with `scenario verify --source` exactly as-is — no
  re-authoring required.

## 21. Implementation slices in exact order

Following the exact "each slice independently mergeable and testable"
discipline `phase-3-plan.md` §18 established:

1. `internal/scenario/types.go` + `load.go` + `load_test.go` — IRS v0.1
   struct definitions, size-capped loader, no validation logic yet.
2. `internal/scenario/validate.go` + `validate_test.go` — every semantic
   rule in §5/§17 (schema version, command shape, workspace_files path
   safety, timeout bounds, linked_fingerprint format, duplicate
   destinations, expected-assertion shape), returning a `Result`/`Issue`
   shape mirroring `internal/validate`'s existing pattern.
3. `internal/scenario/fingerprint_link.go` + tests — the `--source`
   cross-check: `idir.LoadFile` + `validate.Validate` +
   `fingerprint.Compute`, then compare to `linked_fingerprint`.
4. `internal/scenario/limits.go` + `errors.go` + tests — the eight §10
   constants, each with its own passing/failing boundary test pair.
5. `internal/scenario/run.go` + `run_test.go` — workspace creation/staging,
   `exec.CommandContext` invocation, timeout enforcement, bounded
   stdout/stderr capture, outcome classification (PASS/FAIL/TIMEOUT/
   INVALID/INTERNAL_ERROR) — the highest-risk slice, tested with small,
   fast, self-contained fixture commands (`/bin/true`, `/bin/false`,
   `/bin/sleep` with a short timeout, a script that writes past the output
   cap) rather than anything requiring the built `incidentdna` binary.
6. `internal/scenario/report.go` + `report_test.go` — the JSON report type
   (§15), deterministic field order, `--report` file-write behavior.
7. `cmd/incidentdna/cmd_scenario.go` + `cli_scenario_test.go` — CLI wiring
   for `verify`/`run`, `--source`/`--workspace`/`--report`/
   `--keep-workspace` flags, exit codes from §14, black-box tests against
   the real compiled binary.
8. `examples/regression-scenario-demo/` — the new example (§24), built
   against the already-built `bin/incidentdna` and the frozen
   `duplicate-payment` fingerprint, covering PASS, FAIL, TIMEOUT, and
   INVALID outcomes with dedicated scenario files.
9. `docs/regression-scenarios.md` — new dedicated design doc, the Phase 4
   analog of `docs/evidence-storage.md`/`docs/incident-library.md`; updates
   to `README.md`, `docs/architecture.md`, `docs/product-scope.md`,
   `docs/threat-model.md`, `docs/privacy-model.md` — done after the code so
   they describe actual behavior, not the plan.
10. `Makefile` + `.github/workflows/ci.yml` — new `example-scenario`
    target (mirroring `example-evidence`/`example-library`) wired into
    `verify`; a new `scripts/verify-scenario-demo.sh`; a new CI step.
11. `docs/phase-4-report.md` — written last, after real command transcripts
    exist to record, mirroring `docs/phase-3-report.md`'s role.

Each slice through 7 should independently pass `go test ./... -race
-count=1`; slices 8–10 should independently pass `make verify` once slice
10 lands.

## 22. Files expected to be added or modified

**New:**

```
internal/scenario/
  types.go, load.go, validate.go, fingerprint_link.go, run.go, report.go,
  limits.go, errors.go
  load_test.go, validate_test.go, fingerprint_link_test.go, run_test.go,
  report_test.go, limits_test.go

cmd/incidentdna/
  cmd_scenario.go
  cli_scenario_test.go

examples/regression-scenario-demo/
  README.md
  scenario-pass.yaml            — PASS outcome (re-fingerprints duplicate-payment)
  scenario-fail.yaml            — FAIL outcome (wrong expected exit code)
  scenario-timeout.yaml         — TIMEOUT outcome (sleep past a short timeout)
  scenario-invalid.yaml         — INVALID outcome (bare command[0])
  fixtures/incident.yaml        — a copy of the duplicate-payment content
                                   staged into each scenario's workspace,
                                   never a symlink or reference to the
                                   original example file

docs/
  regression-scenarios.md
  phase-4-report.md             (written last, per §21 slice 11)

scripts/
  verify-scenario-demo.sh
```

**Modified:**

```
cmd/incidentdna/main.go          — one new {"scenario", runScenario} entry
README.md                        — CLI commands list, roadmap, known-limitations
docs/architecture.md             — package layout table, dependency direction, new data-flow diagram
docs/product-scope.md            — new "Phase 4 scope" section; out-of-scope list updated
docs/threat-model.md             — new "Executable regression scenarios" section (§19)
docs/privacy-model.md            — new subsection (§18)
Makefile                         — example-scenario target, wired into verify
.github/workflows/ci.yml         — new "Regression scenario demo validation" step
.gitignore                       — no new entry expected (§12); confirmed during implementation
```

**Untouched (explicit, per §20):** `internal/idir/`, `internal/validate/`,
`internal/canonical/`, `internal/fingerprint/`, `internal/compare/`,
`internal/evidence/`, `internal/library/`, `examples/duplicate-payment/`,
`examples/evidence-storage-demo/`, `examples/incident-library-demo/`,
`testdata/golden/`.

## 23. Unit, integration, CLI, race, corruption, and boundary tests

Following the exact table-driven, real-filesystem (`t.TempDir()`),
no-mocking style every prior phase established:

- **Unit — `internal/scenario/validate_test.go`**: one table-driven test
  per §5/§17 semantic rule (missing/wrong `schema_version`, empty
  `command`, bare `command[0]`, absolute vs. workspace-relative
  `command[0]` acceptance, `workspace_files` traversal/absolute/symlink
  rejection, duplicate `destination` rejection, `timeout_seconds` below
  `MinScenarioTimeoutSeconds` and above `MaxScenarioTimeoutSeconds`,
  malformed `linked_fingerprint`, unknown `stdout`/`stderr` `mode`, missing
  `expected.exit_code`).
- **Unit — `internal/scenario/fingerprint_link_test.go`**: `--source`
  cross-check matches for an unmodified `duplicate-payment`-equivalent
  fixture; mismatches for a fixture with a materially different causal
  structure; fails cleanly if `--source` itself fails `validate.Validate`.
- **Unit — `internal/scenario/run_test.go`** (the highest-value new
  coverage in this phase): PASS (matches exit code + stdout `contains`);
  FAIL (exit code mismatch; stdout `exact` mismatch; stderr `regex`
  mismatch); TIMEOUT (a fixture command that sleeps past a short declared
  `timeout_seconds`, asserting the process is actually killed, not just
  that the context expired); INVALID (a scenario that fails
  `validate.go`'s checks is never executed — asserted via a marker file the
  fixture command would have created, confirmed absent); INTERNAL_ERROR
  (nonexistent `command[0]`, a `workspace_files` source that disappears
  between verify and run, a pre-existing non-empty `--workspace`);
  output-cap truncation (a fixture command that writes well past
  `MaxScenarioOutputBytes`, asserting the report's `truncated: true` and
  that memory use stayed bounded — not just that the final buffer size is
  capped); environment isolation (a fixture command that echoes an env var
  set only in the parent's `os.Environ()`, asserting it is absent from
  captured output); cwd isolation (a fixture command that writes a file to
  a relative path, asserting it landed inside the workspace, not the
  repository).
- **Unit — `internal/scenario/report_test.go`**: deterministic JSON field
  order across repeated marshals of the same `Report` value; `--report`
  overwrite behavior (write twice, assert only the second content
  remains).
- **Unit — `internal/scenario/limits_test.go`**: passing/failing boundary
  pair for each of the eight §10 constants.
- **CLI — `cmd/incidentdna/cli_scenario_test.go`** (black-box against the
  real compiled binary, the same style `cli_library_test.go` already
  uses): `scenario verify` exit 0/1/2 paths including `--source` match and
  mismatch; `scenario run` exit 0/1/2 across all five outcomes; `--report`
  file contents match the printed summary; `--keep-workspace` leaves a
  real, inspectable directory whose path is both printed and reported, and
  its absence when the flag is omitted; a scenario whose `command[0]`
  points at the real `bin/incidentdna` binary, demonstrating the
  self-referential regression check the demo (§24) uses.
- **Race** — `go test ./... -race -count=1` must stay green with
  `internal/scenario` included; specific attention to the run-time output
  capture goroutines (stdout/stderr are typically read concurrently via
  two goroutines feeding the bounded buffers) being properly
  synchronized/joined before the `Run` function returns.
- **Corruption/boundary** — covered inline above (malformed scenario
  documents, oversized documents, oversized workspace files, oversized
  captured output, out-of-range timeouts, path-traversal attempts) rather
  than as a separate test category, consistent with how Phases 2 and 3
  organized their own "corruption and limits" coverage.
- **Regression discipline**: the full existing suite
  (`go test ./... -race -count=1`) must pass with zero modification to any
  existing test file's expectations; the golden fingerprint test/script
  must produce the unchanged Phase 1 value.

## 24. Runnable demo requirements

`examples/regression-scenario-demo/` — a new, self-contained directory
demonstrating all five outcomes end-to-end via `scripts/verify-scenario-demo.sh`
(mirroring `scripts/verify-library-demo.sh`'s structure: build the binary
once, then drive it against a temporary `--workspace`/`--report` location,
never leaving generated output in the repository):

1. **PASS** — `scenario-pass.yaml` stages a copy of the duplicate-payment
   incident content into its workspace and runs
   `<repo-root>/bin/incidentdna fingerprint incident.yaml` (via a
   workspace-relative path back to the built binary), asserting exit `0`
   and stdout containing the exact pinned golden fingerprint from
   `testdata/golden/duplicate-payment.fingerprint` — the concrete
   demonstration of "a scenario linked to a failure fingerprint" and
   "deterministic local execution" together.
2. **FAIL** — `scenario-fail.yaml` runs the same command but declares an
   `expected.exit_code` that doesn't match (or an `expected.stdout` that
   doesn't match the real fingerprint), demonstrating a genuine "expected
   versus actual" mismatch.
3. **TIMEOUT** — `scenario-timeout.yaml` runs a short `sleep`-based command
   against a `timeout_seconds` shorter than the sleep duration,asserting
   `scenario run` exits `2` in bounded wall-clock time (not hanging).
4. **INVALID** — `scenario-invalid.yaml` declares a bare `command[0]`
   (`"echo"`, no path), demonstrating rejection before any execution is
   attempted.
5. **`scenario verify --source`** — run against `scenario-pass.yaml` with
   `--source examples/duplicate-payment/incident.yaml`, demonstrating the
   fingerprint-linkage cross-check (§4 Workflow A) using the existing,
   unmodified Phase 1 example — the "runnable example based on an existing
   IncidentDNA example" requirement, satisfied without ever editing that
   example.

`examples/regression-scenario-demo/README.md` documents the exact commands
and expected output for all five, the same documentation discipline
`examples/incident-library-demo/README.md` already follows. No real
credentials, personal information, or customer data anywhere in the
directory — same synthetic-only discipline as every prior example.

## 25. Makefile and CI requirements

- `make example-scenario`: builds the binary if needed, then runs
  `scripts/verify-scenario-demo.sh` — mirroring `example-library`'s exact
  shape in the current `Makefile`.
- `make verify`'s dependency chain gains `example-scenario`, positioned
  after `example-library` and before the golden-fingerprint script (the
  golden-fingerprint check itself remains untouched — Phase 4 does not
  change the fingerprint algorithm or the duplicate-payment example, so
  there is nothing new for that script to regress against).
- `.github/workflows/ci.yml` gains one new step, "Regression scenario demo
  validation" (`make example-scenario`), inserted after "Incident library
  demo validation" and before "Golden fingerprint verification" — mirroring
  exactly how the Phase 3 CI step was added after the Phase 2 one.
- No change to `Dockerfile.dev`/`compose.yaml` is anticipated: `exec`,
  `os/exec`, `context`, and the standard library are sufficient; the demo's
  `sleep`/`true`/`false` fixture commands are ordinary coreutils already
  present in the `golang:*` base image used today.

## 26. Acceptance criteria

1. `internal/scenario` implements load, validate, the `--source`
   fingerprint cross-check, run (with all five outcomes), and report
   generation, each covered by table-driven tests (§21, §23).
2. `scenario verify` and `scenario run` work end-to-end against real
   scenario files, with real command transcripts for all five outcomes
   recorded in a future `docs/phase-4-report.md`.
3. Full existing suite still passes unmodified: `gofmt -l` clean, `go vet
   ./...` clean, `go test ./... -race -count=1` green, and the golden
   fingerprint value is byte-for-byte unchanged.
4. `internal/idir`, `internal/validate`, `internal/canonical`,
   `internal/fingerprint`, `internal/compare`, `internal/evidence`,
   `internal/library` are untouched (or any unavoidable change is called
   out explicitly, not silently).
5. No `workspace_files` path, and no report-file path, can escape its
   resolved root — proven the same way Phases 2 and 3 proved their own
   path-containment guarantees.
6. Each of the eight §10 resource limits is enforced by its own dedicated
   constant, produces a distinct clear error, and is covered by an
   independent passing/failing test pair.
7. All five outcomes (PASS, FAIL, TIMEOUT, INVALID, INTERNAL_ERROR) are
   independently, deterministically reproducible by a dedicated test and a
   dedicated demo scenario file.
8. The runner never invokes a shell and never performs implicit `$PATH`
   resolution — proven by a test asserting a bare `command[0]` is rejected
   at verify time, not merely discouraged in documentation.
9. `examples/duplicate-payment/`, `examples/evidence-storage-demo/`, and
   `examples/incident-library-demo/` are confirmed byte-for-byte unchanged
   by `git diff --stat`.
10. No network access, no telemetry introduced by the runner's own code
    (`grep -rn '"net' cmd/ internal/` stays empty for `internal/scenario`
    and `cmd_scenario.go`).
11. The new example contains no credentials, personal information,
    customer data, or real incident material.
12. `scenario run` writes no file outside its resolved workspace root and
    the user-named `--report` path; the workspace is removed by default
    and only retained when `--keep-workspace` is explicitly given.
13. Documentation updates listed in §22 are made, and no doc makes a
    now-false claim about what is/isn't implemented.
14. Nothing is committed, staged, or pushed during implementation, unless
    the user explicitly instructs otherwise at that time.

## 27. Known limitations

Anticipated and stated up front, so a future `docs/phase-4-report.md` is
not the first place they appear:

- **Not a sandbox.** Process isolation is argv/env/cwd/timeout/output
  bounding only — no seccomp, cgroups, container, or VM isolation (§8, §9).
  A reviewed scenario's command runs with the full permissions of the
  invoking user.
- **No suite runner.** Exactly one scenario per invocation; discovering,
  aggregating, or parallelizing multiple scenarios is not built (§3).
- **No coupling to the incident library.** A scenario's `linked_fingerprint`
  is never checked against stored library occurrences automatically — only
  against a directly-named `--source` incident file (§6, §28).
- **Detached-process timeout evasion is a known, accepted gap** (§9, §19),
  consistent with this project's existing TOCTOU-class disclaimers.
- **No enforcement that a reviewed command's own filesystem/network
  behavior stays within the workspace** — only the runner's own operations
  are bounded (§8).
- **`--report`'s output path has no overwrite protection** — unlike
  `init`'s `--force` gate, `scenario run --report` always overwrites the
  named path; a caller who wants to preserve prior reports must use a
  distinct path or an external file-management step themselves.
- **Fixed, non-configurable resource limits** (§10), consistent with the
  fixed-constants discipline every prior phase already applied.
- **No release gating** — restated as unconditional (§3, §28).

## 28. Future release-gate integration boundary

Named explicitly, as `docs/phase-3-report.md` §17 named the Phase 4
boundary before this phase existed: Phase 4 makes `scenario run`'s exit
code and JSON report *available* to be consumed by something else, but
IncidentDNA itself does not consume it. A plausible, deliberately
**unbuilt** future increment would:

- Add a suite/aggregation layer (run many scenarios, summarize pass/fail
  counts) — not built in Phase 4 (§3, §27).
- Cross-reference a scenario's `linked_fingerprint` against
  `internal/library`'s stored occurrences automatically (e.g., "warn if
  this scenario's fingerprint has no library record") — a pure read-only
  composition of two already-existing, unmodified pieces, explicitly named
  here as the natural next integration point and explicitly not built now
  (§6).
- Wire a scenario suite's aggregate result into an actual CI/CD gate or
  merge check — explicitly excluded from this phase and from every phase
  before it (§3, product-scope.md's standing "no release gating" exclusion,
  restated rather than lifted).

None of this is claimed to be started, scoped in code, or implicitly
implied by anything in this plan. This document makes no claim about what
a future Phase 5 would contain beyond naming this one boundary explicitly,
the same discipline `docs/product-scope.md` and every prior phase plan
already followed.
