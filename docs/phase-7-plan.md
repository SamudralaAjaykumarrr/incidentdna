# Phase 7 Plan: Local Policy Evaluation (Draft for Approval)

**Status: proposed for review. No implementation has started.** No line of
Go code, no schema, no example, and no test has been written against this
plan; nothing in this document has been merged into `Makefile`, CI, or any
existing package. It follows the same role `docs/phase-4-plan.md`,
`docs/phase-5-plan.md`, and `docs/phase-6-plan.md` played before their
respective implementations began — a settled design to implement against,
not an implementer's inference.

## 0. Why this candidate, and the explicit scope decision behind it

This plan follows a Phase 7 scoping session that inventoried every
documented future-work candidate in the repository (`README.md` "Future
work"; `docs/product-scope.md` "Explicitly out of scope for Phase 1 through
Phase 6"; `docs/phase-4-plan.md` §3/§28; `docs/phase-5-plan.md` §3/§25/§26;
`docs/phase-6-plan.md` §0/§20; `docs/phase-6-report.md` §15). Eight
candidates are named, consistently, across those documents: release-gate/
CI-CD integration, sandboxed execution, remote/cloud storage,
evidence/library signing, observability ingestion, parallel suite
execution, directory/glob scenario discovery, and retention/garbage
collection.

**Unlike Phase 6's candidate (the read-only library cross-reference), none
of these eight is described anywhere in the repository as a narrow,
already-decomposed, "no change to either package" increment.**
`docs/phase-6-plan.md` §0 says this explicitly about this exact list:
every one of them is "a new trust boundary (signing), a new execution model
(sandboxing, parallelism), a new transport (remote storage, observability
ingestion), or a policy decision with consequences outside this codebase
(release gating)." A scoping pass at the start of this Phase 7 planning
session confirmed the same conclusion and surfaced it back to the user
rather than silently picking one, per the project's own "an implementer's
inference is not the same as an approved plan" discipline.

**The specific product decision made in that session**, analogous to
`docs/phase-3-plan.md` §20's "Approved decisions" round-trip: split
"release gating" into two distinct things the prior documents had always
named as one:

1. **Release-gate / CI-CD *integration*** — IncidentDNA itself calling out
   to, authenticating against, or blocking merges within some specific
   external CI/CD platform (GitHub Actions status checks, a webhook, a
   merge-queue API, etc.). **This remains fully out of scope** — it is a
   policy decision with consequences (which platforms, what
   authentication, what blocking semantics) this codebase's own documents
   have never resolved and this plan does not resolve either.
2. **Local, deterministic *policy evaluation* over already-produced
   results** — a new, small, local, offline computation that reads
   artifacts this codebase already produces deterministically (a
   `scenario run --report` or `suite run --report` JSON file, unchanged
   shape) plus an optional fresh Phase 6-style library cross-reference,
   evaluates them against a small, reviewable, declared policy document,
   and emits a meaningful exit code and JSON verdict — **which some
   external system may then use to implement its own gate**, exactly the
   same "makes results available to be consumed by something else, but
   IncidentDNA itself does not consume it into an external system" boundary
   `docs/phase-4-plan.md` §28 and `docs/phase-5-plan.md` §26 already
   accepted for `scenario run`'s and `suite run`'s own exit codes.

Item 2 is the Phase 7 increment this document designs. It is chosen because
it is the only one of the eight (once decomposed this way) that can be
built as a **pure, read-only, additive composition of pieces that already
exist**: `scenario.Report`/`suite.Report`'s already-deterministic JSON
shapes (Phases 4–5, unchanged) and `library.CheckFingerprint` (Phase 6,
unchanged) — the identical "no new process-execution primitive, no new
trust boundary, no network" shape that made Phase 6 buildable. It does not
resolve, and does not attempt to resolve, item 1 — that remains an
unstarted, unscoped future decision, restated in §22.

## 1. Problem statement

Phases 4–6 built a complete, bounded, offline *execution and
cross-reference* system: a scenario or suite can be run and classified
deterministically (Phases 4–5), and `scenario verify --library`/`suite
verify --library` can report whether a declared `linked_fingerprint` is a
recorded failure class (Phase 6). None of this is *interpreted*. Today, an
operator (or an external CI job) who wants to answer "should this specific
suite result cause a release to be blocked" must read `suite run`'s exit
code or JSON report by hand and apply their own ad-hoc logic — there is no
declared, reviewable, versioned statement of *what counts as acceptable*,
and no single command that evaluates a result against one.

Phase 7 closes exactly this gap, and nothing else: a new, small, reviewable
policy document format that declares a short list of rules over an
already-produced scenario/suite report (and, optionally, the incident
library), and a new local command that evaluates a report against a named
policy file and reports whether it is satisfied.

## 2. Goals

1. Define a **deterministic, reviewable-as-text policy document format**
   (tentatively **IGP v0.1**, "Incident Gate Policy") declaring an ordered
   list of rules to evaluate against exactly one already-produced report.
2. Provide a **local verifier** (`incidentdna policy verify`) that checks a
   policy document's structural/semantic validity, mirroring the existing
   `validate`/`scenario verify`/`suite verify` split between "is this
   trustworthy to evaluate with" and "did evaluation succeed."
3. Provide a **local evaluator** (`incidentdna policy evaluate`) that loads
   a named policy file and a named, already-produced report file (a
   `scenario run --report` or `suite run --report` JSON document, unchanged
   shape), evaluates every declared rule deterministically, and reports a
   verdict (`PASS`/`FAIL`) with a meaningful exit code and an optional
   deterministic JSON verdict report.
4. Support exactly two rule types in v0.1, each a pure, read-only
   composition of an already-existing, unchanged piece:
   - `require_result` — the report's own top-level `result` field must
     equal a declared value (works identically for a scenario report's
     `PASS`/`FAIL`/`TIMEOUT`/`INVALID`/`INTERNAL_ERROR` and a suite
     report's aggregate `PASS`/`FAIL`/`INVALID`/`INTERNAL_ERROR`, since both
     schemas already use the same field name for this purpose).
   - `require_library_occurrence` — every distinct `linked_fingerprint`
     named anywhere in the report must have at least one recorded incident
     library occurrence, checked via a fresh `library.CheckFingerprint`
     call (Phase 6, unchanged) against a named or default library.
5. Introduce **no new process-execution primitive**: `internal/policy`
   never calls `exec.Command`, never calls `scenario.Run`/`suite.Run`, and
   does not even parse a scenario or suite *document* — it only parses an
   already-produced report *artifact*, the same kind of "consume a
   deterministic JSON shape another package already produces" operation
   `docs/phase-6-plan.md` established for `library.CheckFingerprint`'s
   relationship to a scenario's `linked_fingerprint` string.
6. Keep `internal/policy` a **library-store-unaware leaf package**, exactly
   the discipline Phase 6 established for `internal/scenario`/
   `internal/suite`: `internal/policy` does not import `internal/library`
   and does not know the incident library exists. The
   `require_library_occurrence` rule is evaluated with a caller-supplied
   lookup result, not by `internal/policy` opening a store itself — the
   composition (open the library, call `library.CheckFingerprint` once per
   distinct fingerprint, hand the results to `internal/policy.Evaluate`)
   lives entirely in `cmd/incidentdna`, mirroring `cmd_scenario.go`'s/
   `cmd_suite.go`'s Phase 6 composition exactly.
7. Make the whole feature **testable without any external service**: no
   Docker daemon, no network, no child process — `go test
   ./internal/policy/... -race -count=1` is self-contained using only
   fixture JSON report files and fixture library stores under `t.TempDir()`.

## 3. Explicit non-goals

- **No CI/CD platform integration of any kind.** `incidentdna policy
  evaluate` never calls a webhook, a status-check API, a merge-queue API,
  or any other external system — it reads two local files and prints a
  verdict plus exit code. What an external CI job does with that exit code
  is entirely outside this codebase, exactly as `scenario run`'s and `suite
  run`'s exit codes already are (`docs/phase-4-plan.md` §28,
  `docs/phase-5-plan.md` §26).
- **No new process-execution primitive.** `internal/policy` does not call
  `exec.Command`, `scenario.Run`, or `suite.Run`. It evaluates an
  *already-produced* report file; an operator who wants a fresh result
  still runs `scenario run --report <file>`/`suite run --report <file>`
  themselves, then runs `policy evaluate` against that file, as two
  explicit, reviewable steps rather than one command silently executing
  something on a caller's behalf.
- **No mutation of any existing store.** `policy evaluate` never calls
  `library.Add`; the only library operation it triggers (via
  `cmd/incidentdna`, when `require_library_occurrence` is present) is the
  same read-only `library.CheckFingerprint` Phase 6 already introduced.
- **No coupling built into `internal/scenario`, `internal/suite`, or
  `internal/library` themselves.** None of the three gains a new export, a
  new file, or a new import. `internal/policy` imports `internal/scenario`
  and `internal/suite` for their existing, unchanged `Report` struct
  definitions only (to unmarshal an already-produced report file into a
  typed value instead of `map[string]interface{}`) — it does not import
  `LoadFile`, `Validate`, or `Run` from either package, and neither package
  is modified to accommodate this.
- **No new report field on `scenario.Report` or `suite.Report`.** Both
  remain byte-for-byte unchanged; `docs/regression-scenarios.md`'s and
  `docs/scenario-suites.md`'s report schemas (`irs-report/v0.1`,
  `suite-report/v0.1`) are read, never written to, by this phase.
- **No more than two rule types in v0.1.** `require_result` and
  `require_library_occurrence` are the only rules a policy document may
  declare — the same "small fixed set, extend later if needed" discipline
  IRS v0.1's `stdout`/`stderr` `mode` (`exact`/`contains`/`regex`) already
  established. A policy needing anything more expressive (arbitrary
  boolean combinators, numeric thresholds on counts, per-scenario rather
  than per-report rules) is out of scope for this phase.
- **No batch evaluation.** `policy evaluate` takes exactly one policy file
  and exactly one report file per invocation, mirroring `evidence
  inspect`'s "one digest, no batch mode" precedent and `scenario run`'s
  "exactly one scenario per invocation" precedent.
- **No signing, no encryption, no multi-tenancy, no network, no
  telemetry** — the same standing invariants every prior phase restates.
- **No configurable resource limits** — every bound in §9 is a fixed,
  named Go constant, matching Phases 2–6.

## 4. User workflows

**Workflow A — validate a policy document without evaluating anything:**

```
$ incidentdna policy verify policies/release-gate.yaml
Policy: default-release-gate (schema policy/v0.1)
Rules: 2 declared
  [OK] require_result: PASS
  [OK] require_library_occurrence
OK: policy is structurally valid
```

**Workflow B — evaluate a suite report against a policy, both rules
satisfied:**

```
$ incidentdna suite run --report /tmp/suite-report.json examples/regression-suite-demo/suite-all-pass.yaml
...
$ incidentdna policy evaluate --policy policies/release-gate.yaml \
    --suite-report /tmp/suite-report.json --library .incidentdna/library/objects
Policy: default-release-gate (schema policy/v0.1)
Input: suite report (result: PASS), 2 distinct linked fingerprint(s)
  [OK] require_result: PASS (report result was PASS)
  [OK] require_library_occurrence: 2 of 2 distinct fingerprint(s) have library occurrences
Verdict: PASS
```

Exit code `0`.

**Workflow C — evaluate a report that violates the policy:**

```
$ incidentdna policy evaluate --policy policies/release-gate.yaml \
    --suite-report /tmp/suite-mixed-report.json
Policy: default-release-gate (schema policy/v0.1)
Input: suite report (result: FAIL), 1 distinct linked fingerprint(s)
  [FAIL] require_result: PASS (report result was FAIL)
  [SKIP] require_library_occurrence: not evaluated (--library not given)
Verdict: FAIL
```

Exit code `2`. A rule that cannot be evaluated because a needed flag was
omitted (`require_library_occurrence` without `--library`) is reported as
`SKIP`, not silently ignored and not treated as satisfied — a `SKIP`ped
rule counts as *not satisfied* toward the overall verdict (§11), so a
policy author cannot accidentally get a `PASS` verdict by forgetting a
flag.

**Workflow D — evaluate a scenario (not suite) report:**

```
$ incidentdna scenario run --report /tmp/scenario-report.json examples/regression-scenario-demo/scenario-pass.yaml
$ incidentdna policy evaluate --policy policies/scenario-gate.yaml \
    --scenario-report /tmp/scenario-report.json
Policy: default-scenario-gate (schema policy/v0.1)
Input: scenario report (result: PASS), 1 distinct linked fingerprint(s)
  [OK] require_result: PASS (report result was PASS)
Verdict: PASS
```

`--scenario-report` and `--suite-report` are mutually exclusive; exactly
one is required.

## 5. Policy document format (IGP v0.1)

A policy is a small, standalone YAML or JSON document, decoded through a
new, dedicated, size-capped loader (`internal/policy.LoadFile`, mirroring
`scenario.LoadFile`'s/`suite.LoadFile`'s discipline, not reusing either —
deliberately not an extension of either report format, since a policy
describes *acceptance criteria*, not an *execution* or a *manifest*).

```yaml
schema_version: policy/v0.1

policy:
  id: default-release-gate
  title: Require every listed scenario to pass and be a recorded failure class
  description: >
    Blocks a release if the referenced suite result was not an aggregate
    PASS, or if any distinct linked fingerprint among its scenarios has no
    recorded incident-library occurrence.

rules:
  - type: require_result
    value: PASS
  - type: require_library_occurrence
```

Field notes:

- `schema_version` — fixed literal `policy/v0.1`, checked the same exact
  way as every prior phase's format (exact match, reject anything else).
- `policy.id` — stable, author-chosen identifier; present only for
  human/report readability, never used as a filesystem path component (no
  store, so nothing to key by it — the same rationale `scenario.id`/
  `suite.id` already have).
- `rules` — a required, non-empty, ordered list, bounded by
  `MaxRulesPerPolicy` (§9). Every rule is evaluated (never short-circuited)
  so a `policy evaluate` run always reports the full rule-by-rule picture,
  mirroring `suite run`'s "run every listed scenario, don't stop at the
  first failure unless `--fail-fast`" default — `policy evaluate` has no
  `--fail-fast` equivalent in v0.1, since evaluation has no meaningful
  "cost" the way running a scenario's command does.
- `rules[].type` — one of exactly `require_result`, `require_library_occurrence`
  (§2 goal 4); any other value is rejected at verify time.
- `rules[].value` — required for `require_result` (a non-empty string; not
  cross-checked against either report schema's actual possible values at
  verify time, since a policy file does not yet know which report kind it
  will be evaluated against — a mismatched value is not a validation error,
  it simply means that rule is never satisfiable against that report kind,
  surfaced plainly in the evaluate-time output); forbidden (rejected as
  invalid) for `require_library_occurrence`, which takes no parameters in
  v0.1.
- No policy-level `library` path field — the library to check against is
  supplied via `policy evaluate --library <dir>` at evaluation time, not
  declared inside the policy document, mirroring `scenario verify --library`'s/
  `suite verify --library`'s existing flag-not-document precedent
  (Phase 6) rather than introducing a new place to declare a filesystem
  path inside a document.

## 6. Relationship between the artifacts

```
scenario/suite document (Phase 4/5, unchanged)
   │  scenario.Run / suite.Run                    (unchanged, exec'd
   │                                                separately, by the
   ▼                                                operator, before
scenario.Report / suite.Report                     policy evaluate runs)
   │  --report <file>  (already-existing flag)
   ▼
report JSON file on disk
   │  internal/policy.LoadReport — parses into                incident library
   │  the unchanged scenario.Report/suite.Report               (Phase 3, unchanged)
   │  struct shape, extracts result + distinct                       ▲
   │  linked_fingerprint values                                       │  library.CheckFingerprint
   ▼                                                                   │  (Phase 6, unchanged) —
policy document (IGP v0.1, Phase 7)                                    │  composed in cmd/incidentdna,
   │  internal/policy.LoadFile + Validate                              │  never inside internal/policy
   ▼                                                                   │
(valid) policy.Document ──────────────────────────────────────────────┘
   │  internal/policy.Evaluate(policyDoc, report, libraryLookups)
   ▼
per-rule OK/FAIL/SKIP + overall verdict PASS/FAIL
   │  policy.BuildReport + policy.WriteReport (--report only)
   ▼
deterministic JSON verdict report
```

The load-bearing relationship is identical in shape to Phase 6's: a pure
read-only composition of two already-existing, unmodified pieces (a report
file's own bytes, and an optional fresh library lookup), performed one
layer up from any leaf package, in `cmd/incidentdna`. `internal/policy`
imports `internal/scenario` and `internal/suite` **only** for their
existing `Report` type definitions (to decode a report file into a typed
struct rather than `map[string]interface{}`, the same "documents decode
into a fixed typed struct" discipline `docs/threat-model.md` states as a
project-wide invariant) — it does not call `LoadFile`, `Validate`, or `Run`
from either package, and neither package gains an import of
`internal/policy` or any other change.

## 7. Determinism requirements

1. **`policy verify`'s result depends only on the policy file's own
   bytes** — never on any report file, library state, or wall-clock time.
2. **`policy evaluate`'s `require_result` rule is a pure string comparison**
   against an already-deterministic field a prior, already-verified
   `scenario run --report`/`suite run --report` invocation produced —
   `internal/policy` introduces no new hashing, canonicalization, or
   comparison logic of its own beyond `==`.
3. **`require_library_occurrence`'s result depends on the library's
   on-disk state at the moment `policy evaluate` runs**, not on the report
   file's contents alone — this is the identical, already-documented
   non-claim `docs/library-crossref.md` states for `scenario verify
   --library`/`suite verify --library` (§ "Determinism," point 1), restated
   here rather than newly invented: an intervening `library add` between
   two otherwise-identical `policy evaluate` invocations can legitimately
   change this rule's outcome.
4. **Distinct-fingerprint extraction and lookup order is deterministic**:
   for a suite report, fingerprints are collected in the order their
   owning scenario entries appear in `scenarios[]`, deduplicated to first
   occurrence — the identical discipline `suite verify --library` already
   established (`docs/library-crossref.md` §"Determinism," point 3),
   reused rather than reinvented. A `scenarios[]` entry whose `report` is
   `null` (a `SKIPPED` scenario, per `docs/scenario-suites.md`) contributes
   no fingerprint, since it has none to extract.
5. **Given a fixed report file and a fixed library state, the overall
   verdict is deterministic and repeatable** — every rule is evaluated by
   a pure function of `(policy.Document, report, libraryLookups)`, with no
   internal ordering dependency beyond §5's "declared order, all rules
   evaluated" rule.

## 8. Safety model

Phase 7 introduces **no new class of risk**. It is local file I/O only:
reading a policy file, reading an already-produced report file, and
(optionally) the identical read-only library lookup Phase 6 already
reviewed. `internal/policy` never calls `exec.Command`, never writes to any
existing store, and never creates the library root if it doesn't exist
(the same "never a side effect of a read-only lookup" guarantee
`library.CheckFingerprint` already provides, unchanged). No new write path
exists anywhere in this phase beyond the same kind of optional,
caller-named `--report <path>` file every prior phase's `run`/`verify`
command already supports.

The one new consideration, stated plainly: a policy document, and its
evaluation result, could in principle be used by an operator to make a
real release decision — but IncidentDNA's own role stops at printing a
verdict and an exit code to local stdout/a local file; it never contacts,
authenticates against, or blocks anything in an external system itself.
This is the exact boundary named in §0 and restated unconditionally in
§22.

## 9. Resource limits

New, independent, named constants in `internal/policy/limits.go`, each
with its own distinct error and its own passing/failing boundary test pair
— the same discipline every prior phase's `limits.go` established:

| Constant | Value (proposed) | Applies to |
|---|---|---|
| `MaxPolicyDocumentSize` | 64 KiB | The policy file's own on-disk size |
| `MaxRulesPerPolicy` | 20 | Number of `rules[]` entries a policy may declare |
| `MaxReportDocumentSize` | 256 MiB | The input report file's own on-disk size — sized above the worst-case `suite-report/v0.1` document (`suite.MaxScenariosPerSuite` (100) × 2 output streams × `scenario.MaxScenarioOutputBytes` (1 MiB) plus structural overhead), so a legitimate maximal suite report is never rejected |
| `MaxDistinctFingerprintsPerEvaluation` | 100 | Number of distinct `linked_fingerprint` values one `policy evaluate --library` call will look up — bounded by, and equal to, `suite.MaxScenariosPerSuite`, the same reasoning `docs/library-crossref.md` already applied to `suite verify --library` |

`MaxScenarioDocumentSize`, `MaxSuiteDocumentSize`, `MaxWorkspaceFiles`,
`MaxScenarioOutputBytes`, `MaxScenariosPerSuite`, and every other Phase
4/5/6 limit are inherited unchanged — Phase 7 does not redefine or widen
any of them; `MaxReportDocumentSize` is a genuinely new limit governing a
kind of file (a report artifact) no prior phase ever read back in.

## 10. CLI commands

```
incidentdna policy verify <policy-file>
    Load and structurally/semantically validate <policy-file> against the
    IGP v0.1 rules (schema_version, non-empty rules, known rule type,
    required/forbidden value per rule type, MaxRulesPerPolicy). Never
    evaluates anything.

incidentdna policy evaluate --policy <policy-file>
                             (--scenario-report <file> | --suite-report <file>)
                             [--library <dir>] [--report <file>]
    Run the same checks as `policy verify`. If they pass, load the named
    report file (scenario or suite, unchanged JSON shapes from Phase 4/5),
    evaluate every declared rule against it — for require_library_occurrence,
    only if --library is given; otherwise that rule is reported SKIP and
    counted as not satisfied — print a per-rule and overall verdict line,
    and (if --report is given) write the same outcome as one deterministic
    JSON verdict report (§12).
```

`main.go`'s `commands` table gains one entry, `{"policy", runPolicy}`,
following the exact `{"suite", runSuite}` precedent. `runPolicy`
self-dispatches `verify`/`evaluate` using the same `flag.NewFlagSet` idiom
every existing command already uses. Both `policy` subcommands run under
the existing fixed 30-second command context — `policy evaluate` performs
no process execution and no unbounded-duration operation of its own, so it
needs no special-case exemption the way `scenario run`/`suite run` do.
`--library <path>` reuses `cmd_library.go`'s existing `libraryFlag`/
`openLibraryStore` helpers, unchanged, exactly as `cmd_scenario.go`'s/
`cmd_suite.go`'s Phase 6 `--library` flags already do. `init`, `validate`,
`fingerprint`, `inspect`, `compare`, all four `evidence` subcommands, all
three `library` subcommands, both `scenario` subcommands, and both `suite`
subcommands remain byte-for-byte unchanged in behavior.

## 11. Exit-code contract

**`policy verify`** (mirrors `scenario verify`'s/`suite verify`'s existing
0/1/2 split):

| Exit | Meaning |
|---|---|
| `0` | Policy document is structurally/semantically valid |
| `1` | I/O/parse/usage error — can't read the policy file, bad CLI usage |
| `2` | Policy document decodes but fails a semantic IGP v0.1 rule |

**`policy evaluate`**:

| Outcome | Exit | Meaning |
|---|---|---|
| Verdict `PASS` | `0` | Every declared rule was evaluated and satisfied |
| Verdict `FAIL` | `2` | At least one declared rule was evaluated and not satisfied, or was `SKIP`ped (§4 Workflow C) — a well-formed policy and report, materially negative finding |
| `INVALID` | `1` | The policy file, or the named report file, failed pre-evaluation checks — nothing was evaluated |
| `INTERNAL_ERROR` | `1` | An environmental failure prevented determining the verdict (e.g. `--report` path unwritable, `--library` given but malformed per the Phase 6 error family) |

This reuses, rather than reinvents, the exact exit-code space
`docs/regression-scenarios.md` and `docs/scenario-suites.md` already
established: `2` means "well-formed input, materially negative finding,"
`1` means "environmental/usage failure."

## 12. Result and report format

```json
{
  "schema_version": "policy-report/v0.1",
  "policy_id": "default-release-gate",
  "policy_file": "policies/release-gate.yaml",
  "input_report_kind": "suite",
  "input_report_file": "/tmp/suite-report.json",
  "verdict": "FAIL",
  "rules": [
    {"type": "require_result", "value": "PASS", "status": "FAIL", "detail": "report result was FAIL, expected PASS"},
    {"type": "require_library_occurrence", "status": "SKIP", "detail": "not evaluated: --library not given"}
  ],
  "error": null
}
```

Each rule's `status` is one of `OK`, `FAIL`, `SKIP` — deterministic field
order via a fixed Go struct (no map iteration), matching every prior
phase's report-determinism discipline. `--report` overwrites
unconditionally on each run, matching `scenario run --report`'s/`suite run
--report`'s existing behavior.

## 13. Corruption and malformed-input handling

- **Policy file fails to parse** (bad YAML/JSON, exceeds
  `MaxPolicyDocumentSize`): `policy verify` exits `1`; `policy evaluate`
  reports `INVALID` and exits `1`.
- **Policy decodes but fails a semantic rule** (missing/wrong
  `schema_version`, empty `rules`, unknown `rules[].type`, missing
  `value` on `require_result`, a `value` present on
  `require_library_occurrence`, declared rule count exceeding
  `MaxRulesPerPolicy`): `policy verify` exits `2`; `policy evaluate`
  reports `INVALID` and exits `1` — nothing is evaluated, the same
  "pre-evaluation rejection is a usage-class outcome" reasoning
  `docs/regression-scenarios.md`/`docs/scenario-suites.md` already apply.
- **Report file fails to parse, exceeds `MaxReportDocumentSize`, or does
  not match either `irs-report/v0.1`'s or `suite-report/v0.1`'s
  `schema_version`**: `policy evaluate` reports `INVALID` and exits `1` —
  distinguished from a policy-file failure in the printed error message
  and the JSON report's `error` field.
- **`--library` given, library malformed** (the same
  `ErrMalformedLibrary`/`ErrMalformedIndex`/`ErrCorruptedOccurrence` family
  Phase 6 already returns): `policy evaluate` reports `INTERNAL_ERROR` and
  exits `1` — distinct from a `require_library_occurrence` rule simply not
  being satisfied (which is a well-formed `FAIL`, exit `2`).
- **`--library` omitted, policy declares `require_library_occurrence`**:
  not an error at `policy verify` time (the policy document alone cannot
  know what flags a future `evaluate` invocation will pass) — at
  `evaluate` time, that rule is reported `SKIP` (§4 Workflow C), counted as
  not satisfied, producing verdict `FAIL`, exit `2` — a well-formed,
  informative negative finding, never a crash or a silent `PASS`.
- **`--report` cannot be written**: reported to stderr, exit `1`, even if
  the verdict itself was `PASS` — the per-rule and verdict lines are still
  printed to stdout first, mirroring `scenario run`'s/`suite run`'s
  existing behavior exactly.

## 14. Privacy implications

Identical in kind to every prior phase's own statement:

- **No new document field, no new persisted data on any existing type.**
  A policy document is new, but carries no privacy/redaction block, for
  the identical reason a scenario or suite document carries none
  (`docs/privacy-model.md`, "Regression scenario privacy implications" /
  "Scenario suite privacy implications"): it is short-lived, reviewable
  acceptance-criteria metadata, not a permanent incident record.
- **No new content is ever printed beyond what the input report and the
  Phase 6 library lookup already expose.** The verdict output prints a
  report's own `result` field (already visible in that report's own
  stdout/JSON) and a fingerprint occurrence count (already the exact
  content `scenario verify --library`/`suite verify --library` print) —
  never a stored library occurrence's content, never raw captured
  stdout/stderr excerpts from the underlying report beyond what a
  `require_result` mismatch's `detail` string states (the declared vs.
  actual `result` value only, never full output).
- **No network access means no telemetry, no external transmission** —
  restating the unconditional project invariant, extended to the one new
  `internal/policy` package and `cmd/incidentdna` → `internal/policy` call
  path this phase adds.

## 15. Threat model additions

New subsection proposed for `docs/threat-model.md`, "Policy evaluation:
read-only local computation risk (Phase 7)," stating plainly that Phase 7
introduces no new category of risk: `internal/policy` performs local file
reads only (a policy file, a report file, and — via `cmd/incidentdna`'s
composition — the identical read `library.CheckFingerprint` already
performs). No new write path exists beyond an optional, caller-named
`--report` file, the same class every prior `run`/`verify` command already
has. The one new consideration, stated plainly rather than implied away:
`policy evaluate`'s verdict could be consumed by a human or an external
script to make a real, consequential release decision — but that
consumption happens entirely outside this codebase; IncidentDNA's own
behavior is fully described by "read two local files, print a verdict, set
an exit code," identical in kind (not merely similar) to what `scenario
run`'s and `suite run`'s own exit codes already provide today.

## 16. Backward compatibility with Phases 1–6

- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library` are **fully untouched** — no file under any of these
  packages appears in the implementation diff.
- `internal/scenario`, `internal/suite`: **untouched except as a read-only
  type dependency.** `internal/policy` imports each for its existing
  `Report` struct definition only; neither package's own `.go` files
  change, neither gains a new exported symbol, and neither gains an import
  of `internal/policy`.
- `examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
  `examples/incident-library-demo/`, `examples/regression-scenario-demo/`,
  and `examples/regression-suite-demo/` are **not modified** — the new
  demo (§20) generates report files at verification time from these
  existing, unmodified fixtures rather than editing any of them, the same
  "reference, never edit" discipline every prior phase's demo followed.
- Every existing CLI subcommand's exit-code and output behavior is
  unchanged — proven the same way every prior phase proved it: the full
  existing test suite passes unmodified.
- No `schema_version` bump anywhere — Phase 7 defines two wholly new,
  separate formats (`policy/v0.1` for the policy document,
  `policy-report/v0.1` for its verdict report) and touches no existing
  one.
- `scenario.Report`/`suite.Report` (the `irs-report/v0.1`/
  `suite-report/v0.1` JSON shapes) are read, never written, by this phase
  — a report file produced by a pre-Phase-7 build of `incidentdna` is
  usable with `policy evaluate` exactly as-is, no re-running required.

## 17. Implementation slices in exact order

Following the exact "each slice independently mergeable and testable"
discipline every prior phase plan established:

1. `internal/policy/types.go` + `load.go` + `load_test.go` — IGP v0.1
   struct definitions, size-capped loader, no validation logic yet.
2. `internal/policy/validate.go` + `validate_test.go` — every semantic
   rule in §5/§13 (schema version, non-empty rules, known rule type,
   required/forbidden `value` per type, `MaxRulesPerPolicy`).
3. `internal/policy/report_load.go` + `report_load_test.go` — loading a
   `scenario-report/v0.1`/`suite-report/v0.1` JSON file into typed
   `scenario.Report`/`suite.Report` values (reusing those unchanged
   struct definitions), `MaxReportDocumentSize` enforcement, distinct
   linked-fingerprint extraction (§7 point 4) for both report kinds.
4. `internal/policy/limits.go` + `errors.go` + tests — the four §9
   constants, each with its own passing/failing boundary test pair.
5. `internal/policy/evaluate.go` + `evaluate_test.go` — the core rule
   evaluator: `Evaluate(policyDoc, report, libraryLookups) (Verdict,
   error)`, taking library lookup results as caller-supplied input (§2
   goal 6) rather than opening a store itself; every combination of
   `require_result` match/mismatch and `require_library_occurrence`
   present/absent/all-match/partial-match/`SKIP`, tested with fixture
   report values constructed directly in Go (no need for a real
   `scenario run`/`suite run` invocation at this layer).
6. `internal/policy/report.go` + `report_test.go` — the JSON verdict
   report type (§12), deterministic field order, `--report` file-write
   behavior.
7. `cmd/incidentdna/cmd_policy.go` + `cli_policy_test.go` — CLI wiring for
   `verify`/`evaluate`, `--policy`/`--scenario-report`/`--suite-report`/
   `--library`/`--report` flags, the `cmd/incidentdna`-layer composition
   that opens the library and calls `library.CheckFingerprint` once per
   distinct fingerprint (reusing `cmd_library.go`'s existing
   `libraryFlag`/`openLibraryStore` helpers, unmodified), exit codes from
   §11, black-box tests against the real compiled binary, including tests
   that first run real `scenario run --report`/`suite run --report`
   invocations to produce genuine report fixtures rather than only
   hand-authored ones.
8. `docs/policy-evaluation.md` — new dedicated design doc, the Phase 7
   analog of `docs/library-crossref.md`; updates to `README.md`,
   `docs/architecture.md`, `docs/product-scope.md`, `docs/threat-model.md`,
   `docs/privacy-model.md` — done after the code so they describe actual
   behavior, not the plan.
9. `scripts/verify-policy-demo.sh` + `Makefile` (`example-policy` target,
   wired into `verify`) + `.github/workflows/ci.yml` (new step) — see §20
   for the demo's exact composition.
10. `docs/phase-7-report.md` — written last, after real command
    transcripts exist to record, mirroring `docs/phase-6-report.md`'s
    role.

Each slice through 7 should independently pass `go test ./... -race
-count=1`; slices 8–9 should independently pass `make verify` once slice 9
lands.

## 18. Files expected to be added or modified

**New:**

```
internal/policy/
  types.go, load.go, validate.go, report_load.go, evaluate.go, report.go,
  limits.go, errors.go
  load_test.go, validate_test.go, report_load_test.go, evaluate_test.go,
  report_test.go, limits_test.go

cmd/incidentdna/
  cmd_policy.go
  cli_policy_test.go

policies/
  release-gate-example.yaml       — a checked-in, reviewable example policy
                                     used by the demo script, not a new
                                     "examples/" directory (this is a
                                     single small config-shaped file, not a
                                     multi-file demo bundle)

docs/
  policy-evaluation.md
  phase-7-report.md               (written last, per §17 slice 10)

scripts/
  verify-policy-demo.sh
```

**Modified:**

```
cmd/incidentdna/main.go          — one new {"policy", runPolicy} entry
README.md                        — CLI commands list, roadmap, known-limitations
docs/architecture.md             — package layout table, dependency direction, new data-flow diagram
docs/product-scope.md            — new "Phase 7 scope" section; out-of-scope list updated to state the release-gate/policy-evaluation split from §0 explicitly
docs/threat-model.md             — new "Policy evaluation" section (§15)
docs/privacy-model.md            — new subsection (§14)
Makefile                         — example-policy target, wired into verify
.github/workflows/ci.yml         — new "Policy evaluation demo validation" step
.gitignore                       — no new entry expected; confirmed during implementation
```

**Untouched (explicit):** `internal/idir/`, `internal/validate/`,
`internal/canonical/`, `internal/fingerprint/`, `internal/compare/`,
`internal/evidence/`, `internal/library/`, every `.go` file under
`internal/scenario/` and `internal/suite/`, `cmd/incidentdna/cmd_scenario.go`,
`cmd/incidentdna/cmd_suite.go`, `cmd/incidentdna/cmd_library.go`, all five
existing `examples/` directories, `testdata/golden/`.

## 19. Unit, integration, CLI, race, and boundary tests

Following the exact table-driven, real-filesystem (`t.TempDir()`),
no-mocking style every prior phase established:

- **Unit — `internal/policy/validate_test.go`**: one table-driven test per
  §5/§13 semantic rule (missing/wrong `schema_version`, empty `rules`,
  unknown `rules[].type`, missing `value` on `require_result`, a `value`
  present on `require_library_occurrence`, over-limit rule count).
- **Unit — `internal/policy/report_load_test.go`**: successful load of a
  genuine `scenario-report/v0.1` and `suite-report/v0.1` fixture (hand
  constructed to match the documented schema exactly); rejection of a
  document whose `schema_version` matches neither; oversized-report
  rejection at `MaxReportDocumentSize`; distinct-fingerprint extraction for
  a suite report with duplicate fingerprints (dedup, first-occurrence
  order) and with a `SKIPPED` (null-report) entry (contributes nothing).
- **Unit — `internal/policy/evaluate_test.go`** (the highest-value new
  coverage in this phase): `require_result` match and mismatch for both
  report kinds; `require_library_occurrence` all-match, partial-match,
  no-match, and `SKIP` (nil `libraryLookups` argument, simulating
  `--library` omitted); overall verdict `PASS` only when every rule is
  `OK`; a policy with zero library-dependent rules never requires a
  `libraryLookups` argument at all (proven by a test passing `nil`
  successfully when the policy has no `require_library_occurrence` rule).
- **Unit — `internal/policy/report_test.go`**: deterministic JSON field
  order across repeated marshals; `--report` overwrite behavior.
- **Unit — `internal/policy/limits_test.go`**: passing/failing boundary
  pair for each of the four §9 constants.
- **CLI — `cmd/incidentdna/cli_policy_test.go`** (black-box against the
  real compiled binary): `policy verify` exit 0/1/2 paths; `policy
  evaluate` exit 0/1/2 across `PASS`/`FAIL`/`INVALID`/`INTERNAL_ERROR`,
  using report files produced by real, preceding `scenario run --report`/
  `suite run --report` invocations in the test (not only hand-authored
  report JSON) so the integration with Phase 4/5's actual output is
  exercised, not merely the documented schema; `--library` given/omitted
  with a `require_library_occurrence` rule present; malformed library
  (exit `1`); `--report` file contents match the printed summary;
  `--scenario-report`/`--suite-report` mutual exclusivity and
  "exactly one required" enforcement.
- **Race** — `go test ./... -race -count=1` must stay green with
  `internal/policy` and the two modified `cmd/incidentdna` files included.
- **Regression discipline**: the full existing suite (every currently
  passing top-level test across `cmd/incidentdna` and all packages under
  `internal/`) must pass with zero modification to any existing test
  file's expectations; the golden fingerprint test/script must produce the
  unchanged Phase 1 value; `git diff --stat -- internal/idir
  internal/validate internal/canonical internal/fingerprint internal/compare
  internal/evidence internal/library` must be empty, and `git diff --stat
  -- internal/scenario internal/suite` must show no `.go` file changed
  (only, if anything, doc-adjacent or none at all — §16).

## 20. Runnable demo requirements

`scripts/verify-policy-demo.sh` — a new script, not a new example
directory, following Phase 6's precedent of composing already-existing
fixtures rather than inventing new ones wherever possible. Concretely, in
a temporary directory (never touching a real `.incidentdna/` or any
checked-in file, except reading the new checked-in
`policies/release-gate-example.yaml`):

1. **Generate real reports** — run `suite run --report <tmp>/suite-pass.json`
   against `examples/regression-suite-demo/suite-all-pass.yaml` and
   `suite run --report <tmp>/suite-mixed.json` against
   `examples/regression-suite-demo/suite-mixed.yaml` (both already-existing,
   unmodified fixtures); also run `scenario run --report
   <tmp>/scenario-pass.json` against
   `examples/regression-scenario-demo/scenario-pass.yaml`.
2. **Policy verify** — `policy verify policies/release-gate-example.yaml`,
   asserting exit `0`.
3. **Evaluate: verdict PASS** — `policy evaluate --policy
   policies/release-gate-example.yaml --suite-report <tmp>/suite-pass.json`
   (no `--library`, using a policy variant/second policy file with only
   `require_result` to keep this check library-independent), asserting
   exit `0`.
4. **Evaluate: verdict FAIL via `require_result`** — the same policy
   against `<tmp>/suite-mixed.json`, asserting exit `2` and a `[FAIL]
   require_result` line.
5. **Evaluate: `require_library_occurrence` match** — seed a fresh
   temporary library with `examples/duplicate-payment/incident.yaml` (the
   same fixture Phase 6's demo already uses, whose fingerprint the
   regression-scenario-demo/regression-suite-demo fixtures already
   reference), then evaluate `policies/release-gate-example.yaml` (which
   declares both rules) against `<tmp>/scenario-pass.json` with
   `--library <tmp-library>`, asserting exit `0` and an `[OK]
   require_library_occurrence` line.
6. **Evaluate: `require_library_occurrence` `SKIP`** — the same evaluation
   with `--library` omitted, asserting exit `2` and a `[SKIP]
   require_library_occurrence` line (§4 Workflow C, §13).
7. **Malformed report file** — a deliberately corrupted/truncated report
   JSON, asserting `policy evaluate` reports `INVALID` and exits `1`.

No real credentials, personal information, or customer data anywhere —
every fixture this demo touches is a pre-existing, already-reviewed
synthetic example, plus one new small, reviewable policy file.
`make example-policy` builds the binary if needed, then runs this script;
`make verify`'s dependency chain gains it, positioned after
`example-library-crossref` and before the golden-fingerprint check,
mirroring exactly how every prior phase's demo step was inserted.

## 21. Known limitations (anticipated up front)

- **No CI/CD platform integration.** `policy evaluate`'s exit code and
  JSON verdict are available to be consumed by something else; IncidentDNA
  itself does not consume them into any external system (§0, §3, §22).
- **No fresh execution.** `policy evaluate` reads an already-produced
  report file; it does not itself run `scenario.Run`/`suite.Run` — an
  operator must run `scenario run --report`/`suite run --report`
  themselves first, as an explicit prior step.
- **Only two rule types in v0.1** (`require_result`,
  `require_library_occurrence`) — no boolean combinators, no numeric
  thresholds, no per-scenario (as opposed to per-report) rules.
- **Point-in-time library check**, restated from Phase 6's own identical
  limitation (`docs/library-crossref.md`): a `require_library_occurrence`
  result reflects the library's contents at the moment `policy evaluate`
  runs, not at the moment the underlying report was produced.
- **No batch evaluation** — one policy file and one report file per
  invocation.
- **Fixed, non-configurable resource limits** (§9), consistent with every
  prior phase's design.

## 22. Future release-gate integration boundary (restated, narrower)

Named explicitly, the same way `docs/phase-4-plan.md` §28,
`docs/phase-5-plan.md` §26, and `docs/phase-6-plan.md` §20 named their own
phase's boundary before it existed: Phase 7 makes "does this result satisfy
a declared policy" a question `incidentdna` can answer directly and
deterministically, entirely locally — but it remains purely a local
computation. Nothing in this codebase, after Phase 7, calls out to, blocks,
or authenticates against any external CI/CD system. Plausible, deliberately
**unbuilt** future increments, named here rather than started:

- Wiring `policy evaluate`'s exit code into an actual external CI/CD
  status check, webhook, or merge-queue API — this is precisely "item 1"
  from §0, explicitly excluded from this phase and from every phase before
  it.
- Additional rule types (numeric thresholds, boolean combinators,
  per-scenario rules within a suite report) beyond the two named in §2
  goal 4.
- Fresh-execution convenience (`policy evaluate` itself invoking
  `scenario.Run`/`suite.Run` rather than requiring a pre-produced report
  file) — deliberately deferred (§3) to keep this phase's own package a
  pure, read-only artifact consumer with no process-execution surface of
  its own.
- Sandboxed scenario/suite execution, remote/shared library or evidence
  storage, evidence/library signing or authenticity proof, observability/
  event-system ingestion, parallel suite execution, directory/glob-based
  scenario discovery, and retention/garbage collection — all still named,
  still unbuilt, still out of scope, carried forward unchanged from
  `docs/product-scope.md`.

None of this is claimed to be started, scoped in code, or implicitly
implied by anything in this plan. This document makes no claim about what
a future Phase 8 would contain beyond naming these boundaries explicitly,
the same discipline every prior phase plan already followed.
