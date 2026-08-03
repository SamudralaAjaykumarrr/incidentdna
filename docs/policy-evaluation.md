# Local Policy Evaluation

Phase 7 adds a small, reviewable, versioned policy document format —
**IGP v0.1** ("Incident Gate Policy") — and a local, deterministic
**evaluator** (`incidentdna policy verify|evaluate`) that checks an
already-produced `scenario run --report`/`suite run --report` JSON artifact
(Phase 4/5, unchanged) against a declared policy and reports a `PASS`/`FAIL`
verdict with a meaningful exit code. This document describes what that
evaluator is, what it guarantees, and — just as importantly — what it does
not. `docs/phase-7-plan.md` is the approved plan this implements; this
document describes the resulting design as built, for the same audience
`docs/regression-scenarios.md`, `docs/scenario-suites.md`, and
`docs/library-crossref.md` serve for Phases 4 through 6.

Implementation: `internal/policy/` (`types.go`, `load.go`, `validate.go`,
`report_load.go`, `evaluate.go`, `report.go`, `limits.go`, `errors.go`) and
`cmd/incidentdna/cmd_policy.go`.

## What problem this closes

Phases 4-6 built a complete, bounded, offline *execution and cross-reference*
system: a scenario or suite can be run and classified deterministically
(Phases 4-5), and `scenario verify --library`/`suite verify --library` can
report whether a declared `linked_fingerprint` is a recorded failure class
(Phase 6). None of this is *interpreted*. An operator (or an external CI job)
who wanted to answer "should this specific suite result cause a release to be
blocked" had to read `suite run`'s exit code or JSON report by hand and apply
their own ad-hoc logic — there was no declared, reviewable, versioned
statement of *what counts as acceptable*, and no single command that
evaluates a result against one.

Phase 7 closes exactly this gap, and nothing else: a new, small, reviewable
policy document format that declares an ordered list of rules to evaluate
against exactly one already-produced report (and, optionally, the incident
library), and a new local command that evaluates a report against a named
policy file and reports whether it is satisfied.

Phase 7 does not change `idir.Document`, the JSON Schema, `internal/validate`,
`internal/canonical`, `internal/fingerprint`, `internal/compare`,
`internal/evidence`, `internal/library`, `internal/scenario`, or
`internal/suite`. `internal/policy` imports `internal/scenario` and
`internal/suite` only for their existing, unchanged `Report` struct
definitions (to decode a report file into a typed value instead of
`map[string]interface{}`) — it never calls `LoadFile`, `Validate`, or `Run`
from either package, and neither package gains a new export or a new import.
`internal/policy` does not import `internal/library` at all and does not know
the incident library exists — the `require_library_occurrence` rule is
evaluated against a caller-supplied lookup result; the composition that opens
the library and calls `library.CheckFingerprint` lives entirely in
`cmd/incidentdna`, exactly the discipline Phase 6 established for
`scenario verify --library`/`suite verify --library`.

## No new class of risk

`internal/policy` never calls `exec.Command`, never calls `scenario.Run` or
`suite.Run`, and does not even parse a scenario or suite *document* — it only
parses an already-produced report *artifact*, the same kind of "consume a
deterministic JSON shape another package already produces" operation Phase 6
established for `library.CheckFingerprint`'s relationship to a scenario's
`linked_fingerprint` string. It performs local file reads only: a policy
file, a report file, and (via `cmd/incidentdna`'s composition) the identical
read-only `library.CheckFingerprint` Phase 6 already reviewed. No new write
path exists anywhere in this phase beyond an optional, caller-named
`--report <path>` file, the same class every prior `run`/`verify` command
already has.

The one new consideration, stated plainly: `policy evaluate`'s verdict could
be consumed by a human or an external script to make a real, consequential
release decision — but that consumption happens entirely outside this
codebase. `incidentdna` itself never contacts, authenticates against, or
blocks anything in an external system — its own behavior is fully described
by "read two local files, print a verdict, set an exit code," identical in
kind to what `scenario run`'s and `suite run`'s own exit codes already
provide.

## The policy document format (IGP v0.1)

A policy is a small, standalone YAML or JSON document, loaded through a
dedicated, size-capped loader (`policy.LoadFile`, mirroring
`scenario.LoadFile`'s/`suite.LoadFile`'s discipline, not reusing either — a
policy describes *acceptance criteria*, not an *execution* or a *manifest*).

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

- **`schema_version`** — fixed literal `policy/v0.1`; checked the same exact
  way every prior phase's format is: exact match, reject anything else.
- **`policy.id`** — stable, author-chosen identifier; present only for
  human/report readability, never used as a filesystem path component (no
  store, so nothing to key by it).
- **`rules`** — a required, non-empty, ordered list, bounded by
  `MaxRulesPerPolicy` (20). Every rule is evaluated, never short-circuited,
  so a `policy evaluate` run always reports the full rule-by-rule picture.
- **`rules[].type`** — one of exactly `require_result`,
  `require_library_occurrence`; any other value is rejected at verify time.
- **`rules[].value`** — required for `require_result` (a non-empty string;
  not cross-checked against either report schema's actual possible values at
  verify time, since a policy file does not yet know which report kind it
  will be evaluated against — a mismatched value is not a validation error,
  it simply means that rule is never satisfiable against that report kind,
  surfaced plainly in the evaluate-time output); forbidden (rejected as
  invalid) for `require_library_occurrence`, which takes no parameters in
  v0.1.
- **No policy-level `library` path field.** The library to check against is
  supplied via `policy evaluate --library <dir>` at evaluation time, not
  declared inside the policy document — mirroring `scenario verify --library`'s/
  `suite verify --library`'s existing flag-not-document precedent (Phase 6).

## The two rule types

- **`require_result`** — the report's own top-level `result` field must
  equal the declared `value`. Works identically for a scenario report's
  `PASS`/`FAIL`/`TIMEOUT`/`INVALID`/`INTERNAL_ERROR` and a suite report's
  aggregate `PASS`/`FAIL`/`INVALID`/`INTERNAL_ERROR`, since both schemas
  already use the same field name for this purpose. A pure string
  comparison (`==`) — no new hashing, canonicalization, or comparison logic.
- **`require_library_occurrence`** — every distinct `linked_fingerprint`
  named anywhere in the report must have at least one recorded incident
  library occurrence, checked via a fresh `library.CheckFingerprint` call
  (Phase 6, unchanged) against a named or default library, composed entirely
  in `cmd/incidentdna`. Zero distinct fingerprints (a report naming none) is
  vacuously satisfied.

No more than these two rule types exist in v0.1 — the same "small fixed set,
extend later if needed" discipline IRS v0.1's `stdout`/`stderr` `mode`
(`exact`/`contains`/`regex`) already established.

## Relationship between the artifacts

```
scenario/suite document (Phase 4/5, unchanged)
   |  scenario.Run / suite.Run                    (unchanged, exec'd
   |                                                separately, by the
   v                                                operator, before
scenario.Report / suite.Report                     policy evaluate runs)
   |  --report <file>  (already-existing flag)
   v
report JSON file on disk
   |  internal/policy.LoadScenarioReport /                    incident library
   |  LoadSuiteReport — parses into the unchanged                (Phase 3,
   |  scenario.Report/suite.Report struct shape,                 unchanged)
   |  extracts result + distinct linked_fingerprint                    ^
   |  values                                                            |  library.CheckFingerprint
   v                                                                    |  (Phase 6, unchanged) --
policy document (IGP v0.1, Phase 7)                                     |  composed in
   |  internal/policy.LoadFile + Validate                               |  cmd/incidentdna, never
   v                                                                     |  inside internal/policy
(valid) policy.Document ------------------------------------------------+
   |  internal/policy.Evaluate(policyDoc, report, libraryLookups)
   v
per-rule OK/FAIL/SKIP + overall verdict PASS/FAIL
   |  policy.BuildReport + policy.WriteReport (--report only)
   v
deterministic JSON verdict report
```

The load-bearing relationship is identical in shape to Phase 6's: a pure
read-only composition of two already-existing, unmodified pieces (a report
file's own bytes, and an optional fresh library lookup), performed one layer
up from any leaf package, in `cmd/incidentdna`.

## The two commands

```
incidentdna policy verify <policy-file>
incidentdna policy evaluate --policy <policy-file>
                             (--scenario-report <file> | --suite-report <file>)
                             [--library <dir>] [--report <file>]
```

**`policy verify <policy-file>`** — loads and structurally/semantically
validates `<policy-file>` against every IGP v0.1 rule above. **Never
evaluates anything.**

```
$ incidentdna policy verify policies/release-gate-example.yaml
Policy: default-release-gate (schema policy/v0.1)
Rules: 2 declared
  [OK] require_result: PASS
  [OK] require_library_occurrence
OK: policy is structurally valid
```

**`policy evaluate`** — runs the same checks `policy verify` performs. If
they pass, loads the named report file (scenario or suite, unchanged JSON
shapes from Phase 4/5), evaluates every declared rule against it — for
`require_library_occurrence`, only if `--library` is given; otherwise that
rule is reported `SKIP` and counted as not satisfied — prints a per-rule and
overall verdict line, and (if `--report` is given) writes the same outcome as
one deterministic JSON verdict report.

Real transcript (`scripts/verify-policy-demo.sh`, run standalone; suite
manifests substitute the real, checkout-specific absolute binary path for
`/INCIDENTDNA_BIN_PLACEHOLDER` exactly as `scripts/verify-scenario-demo.sh`/
`scripts/verify-suite-demo.sh` already do):

```
$ incidentdna suite run --report /tmp/suite-pass-report.json examples/regression-suite-demo/suite-all-pass.yaml
$ incidentdna policy evaluate --policy /tmp/require-result-only.yaml \
    --suite-report /tmp/suite-pass-report.json
Policy: require-result-only (schema policy/v0.1)
Input: suite report (result: PASS), 1 distinct linked fingerprint(s)
  [OK] require_result: PASS (report result was PASS)
Verdict: PASS
```

Exit code `0`. Against a suite report whose aggregate result was `FAIL`:

```
$ incidentdna policy evaluate --policy /tmp/require-result-only.yaml \
    --suite-report /tmp/suite-mixed-report.json
Policy: require-result-only (schema policy/v0.1)
Input: suite report (result: FAIL), 1 distinct linked fingerprint(s)
  [FAIL] require_result: PASS (report result was FAIL, expected PASS)
Verdict: FAIL
```

Exit code `2`. With `--library` and both rules declared, against a scenario
report whose `linked_fingerprint` is a recorded library occurrence:

```
$ incidentdna library add --library /tmp/lib --allow-unredacted \
    examples/duplicate-payment/incident.yaml
$ incidentdna scenario run --report /tmp/scenario-pass-report.json \
    examples/regression-scenario-demo/scenario-pass.yaml
$ incidentdna policy evaluate --policy policies/release-gate-example.yaml \
    --scenario-report /tmp/scenario-pass-report.json --library /tmp/lib
Policy: default-release-gate (schema policy/v0.1)
Input: scenario report (result: PASS), 1 distinct linked fingerprint(s)
  [OK] require_result: PASS (report result was PASS)
  [OK] require_library_occurrence: 1 of 1 distinct fingerprint(s) have library occurrences
Verdict: PASS
```

Exit code `0`. The identical evaluation with `--library` omitted:

```
$ incidentdna policy evaluate --policy policies/release-gate-example.yaml \
    --scenario-report /tmp/scenario-pass-report.json
Policy: default-release-gate (schema policy/v0.1)
Input: scenario report (result: PASS), 1 distinct linked fingerprint(s)
  [OK] require_result: PASS (report result was PASS)
  [SKIP] require_library_occurrence: not evaluated (--library not given)
Verdict: FAIL
```

Exit code `2`. A rule that cannot be evaluated because a needed flag was
omitted is reported `SKIP`, not silently ignored and not treated as
satisfied — a `SKIP`ped rule counts as *not satisfied* toward the overall
verdict, so a policy author cannot accidentally get a `PASS` verdict by
forgetting a flag.

`--scenario-report` and `--suite-report` are mutually exclusive; exactly one
is required.

## Exit-code contract

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
| Verdict `FAIL` | `2` | At least one declared rule was evaluated and not satisfied, or was `SKIP`ped |
| `INVALID` | `1` | The policy file, or the named report file, failed pre-evaluation checks — nothing was evaluated |
| `INTERNAL_ERROR` | `1` | An environmental failure prevented determining the verdict (e.g. `--library` given but malformed, per the Phase 6 error family) |

This reuses, rather than reinvents, the exact exit-code space every prior
phase already established: `2` means "well-formed input, materially negative
finding," `1` means "environmental/usage failure."

## Result and report format

```json
{
  "schema_version": "policy-report/v0.1",
  "policy_id": "default-release-gate",
  "policy_file": "policies/release-gate-example.yaml",
  "input_report_kind": "suite",
  "input_report_file": "/tmp/suite-report.json",
  "verdict": "FAIL",
  "rules": [
    {"type": "require_result", "value": "PASS", "status": "FAIL", "detail": "report result was FAIL, expected PASS"},
    {"type": "require_library_occurrence", "status": "SKIP", "detail": "not evaluated (--library not given)"}
  ],
  "error": null
}
```

Each rule's `status` is one of `OK`, `FAIL`, `SKIP` — deterministic field
order via a fixed Go struct (no map iteration). `verdict` also takes the
values `INVALID`/`INTERNAL_ERROR` for the two pre-evaluation-failure cases,
in which case `rules` is empty and `error` holds a short, actionable
message. `--report` overwrites unconditionally on each run, matching
`scenario run --report`'s/`suite run --report`'s existing behavior — even a
policy or report load failure still produces a best-effort report at the
named path (mirroring `scenario run --report`'s "always finds a report at
the path it named, never only sometimes" discipline), so a caller scripting
against `--report` output always finds one.

## Determinism

1. **`policy verify`'s result depends only on the policy file's own bytes**
   — never on any report file, library state, or wall-clock time.
2. **`require_result` is a pure string comparison** against an
   already-deterministic field a prior, already-verified
   `scenario run --report`/`suite run --report` invocation produced —
   `internal/policy` introduces no new hashing, canonicalization, or
   comparison logic of its own beyond `==`.
3. **`require_library_occurrence`'s result depends on the library's on-disk
   state at the moment `policy evaluate` runs**, not on the report file's
   contents alone — the identical, already-documented non-claim
   `docs/library-crossref.md` states for `scenario verify --library`/
   `suite verify --library`: an intervening `library add` between two
   otherwise-identical `policy evaluate` invocations can legitimately change
   this rule's outcome.
4. **Distinct-fingerprint extraction and lookup order is deterministic**:
   for a suite report, fingerprints are collected in the order their owning
   scenario entries appear in `scenarios[]`, deduplicated to first
   occurrence — the identical discipline `suite verify --library` already
   established. A `scenarios[]` entry whose `report` is `null` (a `SKIPPED`
   scenario) contributes no fingerprint.
5. **Given a fixed report file and a fixed library state, the overall
   verdict is deterministic and repeatable** — every rule is evaluated by a
   pure function of `(policy.Document, report, libraryLookups)`, with no
   internal ordering dependency beyond "declared order, all rules
   evaluated."

## Resource limits

Four independent, fixed constants (`internal/policy/limits.go`), each with
its own distinct error and its own passing/failing boundary test pair:

| Constant | Value | Applies to |
|---|---|---|
| `MaxPolicyDocumentSize` | 64 KiB | The policy file's own on-disk size |
| `MaxRulesPerPolicy` | 20 | Number of `rules[]` entries a policy may declare |
| `MaxReportDocumentSize` | 256 MiB | The input report file's own on-disk size — sized above the worst-case `suite-report/v0.1` document |
| `MaxDistinctFingerprintsPerEvaluation` | 100 | Number of distinct `linked_fingerprint` values one `policy evaluate --library` call will look up — bounded by, and equal to, `suite.MaxScenariosPerSuite` |

Every prior phase's limit (`MaxScenarioDocumentSize`, `MaxSuiteDocumentSize`,
`MaxWorkspaceFiles`, `MaxScenarioOutputBytes`, `MaxScenariosPerSuite`,
`internal/library`'s four limits, and so on) is inherited unchanged — Phase 7
does not redefine or widen any of them. Both `policy` subcommands run under
the existing fixed 30-second command context every other subcommand already
uses (`main.go`'s usual `context.WithTimeout` wrapping is not special-cased
for `policy`, unlike `scenario`/`suite`) — `policy evaluate` performs no
process execution and no unbounded-duration operation of its own.

## Corruption and malformed-input handling

- **Policy file fails to parse** (bad YAML/JSON, exceeds
  `MaxPolicyDocumentSize`): `policy verify` exits `1`; `policy evaluate`
  reports `INVALID` and exits `1`.
- **Policy decodes but fails a semantic rule** (missing/wrong
  `schema_version`, empty `rules`, unknown `rules[].type`, missing `value`
  on `require_result`, a `value` present on `require_library_occurrence`,
  declared rule count exceeding `MaxRulesPerPolicy`): `policy verify` exits
  `2`; `policy evaluate` reports `INVALID` and exits `1` — nothing is
  evaluated.
- **Report file fails to parse, exceeds `MaxReportDocumentSize`, names more
  than `MaxDistinctFingerprintsPerEvaluation` distinct fingerprints, or does
  not match the expected report kind's own `schema_version`** (an
  `irs-report/v0.1` file loaded as `--suite-report`, or vice versa):
  `policy evaluate` reports `INVALID` and exits `1` — distinguished from a
  policy-file failure in the printed error message and the JSON report's
  `error` field.
- **`--library` given, library malformed** (the same
  `ErrMalformedLibrary`/`ErrMalformedIndex`/`ErrCorruptedOccurrence` family
  Phase 6 already returns): `policy evaluate` reports `INTERNAL_ERROR` and
  exits `1` — distinct from a `require_library_occurrence` rule simply not
  being satisfied (which is a well-formed `FAIL`, exit `2`).
- **`--library` omitted, policy declares `require_library_occurrence`**:
  not an error at `policy verify` time (the policy document alone cannot
  know what flags a future `evaluate` invocation will pass) — at `evaluate`
  time, that rule is reported `SKIP`, counted as not satisfied, producing
  verdict `FAIL`, exit `2` — a well-formed, informative negative finding,
  never a crash or a silent `PASS`.
- **`--report` cannot be written**: reported to stderr, exit `1`, even if
  the verdict itself was `PASS` — the per-rule and verdict lines are still
  printed to stdout first, mirroring `scenario run`'s/`suite run`'s existing
  behavior exactly.
- **`--scenario-report` and `--suite-report` both given, or neither given**:
  rejected as a usage error, exit `1`, before either file is ever read.
- **`--policy` omitted**: rejected as a usage error, exit `1`.

## Privacy implications

Identical in kind to every prior phase's own statement:

- **No new document field, no new persisted data on any existing type.** A
  policy document is new, but carries no privacy/redaction block, for the
  identical reason a scenario or suite document carries none: it is
  short-lived, reviewable acceptance-criteria metadata, not a permanent
  incident record.
- **No new content is ever printed beyond what the input report and the
  Phase 6 library lookup already expose.** The verdict output prints a
  report's own `result` field (already visible in that report's own
  stdout/JSON) and a fingerprint occurrence count (already the exact
  content `scenario verify --library`/`suite verify --library` print) —
  never a stored library occurrence's content, never raw captured
  stdout/stderr excerpts from the underlying report beyond what a
  `require_result` mismatch's `detail` string states (the declared vs.
  actual `result` value only).
- **No network access means no telemetry, no external transmission** —
  restating the unconditional project invariant, extended to the one new
  `internal/policy` package and `cmd/incidentdna` -> `internal/policy` call
  path this phase adds.

## What Phase 7 explicitly does not provide

- **No CI/CD platform integration of any kind.** `policy evaluate` never
  calls a webhook, a status-check API, a merge-queue API, or any other
  external system — it reads two local files and prints a verdict plus exit
  code. What an external CI job does with that exit code is entirely
  outside this codebase.
- **No fresh execution.** `policy evaluate` reads an already-produced report
  file; it does not itself run `scenario.Run`/`suite.Run` — an operator must
  run `scenario run --report`/`suite run --report` themselves first, as an
  explicit prior step.
- **Only two rule types in v0.1** (`require_result`,
  `require_library_occurrence`) — no boolean combinators, no numeric
  thresholds, no per-scenario (as opposed to per-report) rules.
- **No batch evaluation.** `policy evaluate` takes exactly one policy file
  and exactly one report file per invocation.
- **No mutation of any existing store.** `policy evaluate` never calls
  `library.Add`; the only library operation it triggers is the same
  read-only `library.CheckFingerprint` Phase 6 already introduced.
- **No coupling built into `internal/scenario`, `internal/suite`, or
  `internal/library` themselves.** None of the three gains a new export, a
  new file, or a new import.
- **Point-in-time library check**, restated from Phase 6's own identical
  limitation: a `require_library_occurrence` result reflects the library's
  contents at the moment `policy evaluate` runs, not at the moment the
  underlying report was produced.
- **No signing, no encryption, no multi-tenancy, no network, no
  telemetry** — the same standing invariants every prior phase restates.
- **Fixed, non-configurable resource limits.**

None of this is claimed to be started, scoped in code, or implicitly implied
beyond what is stated here — a future Phase 8 is not scoped by this
document.

## The `verify-policy-demo.sh` script

Unlike Phases 2-5, Phase 7 introduces no new example directory — it
demonstrates the feature by composing already-existing, unmodified fixtures
from `examples/duplicate-payment/`, `examples/regression-scenario-demo/`, and
`examples/regression-suite-demo/`, plus the one new checked-in
`policies/release-gate-example.yaml`. See `scripts/verify-policy-demo.sh` for
the exact seven checks it performs (real report generation, `policy verify`,
verdict `PASS`/`FAIL` via `require_result`, `require_library_occurrence`
match and `SKIP`, and a malformed report file), and `make example-policy` to
run it.

## Current Phase 7 status and remaining limitations

Implemented, per `internal/policy/` and `cmd/incidentdna/cmd_policy.go`: IGP
v0.1 document loading and semantic validation, loading an already-produced
scenario/suite report into a typed value with distinct-fingerprint
extraction, the deterministic rule evaluator, deterministic JSON verdict
report generation, and the two `incidentdna policy` subcommands. This
document describes that implementation as it exists in the code today.

Known, accepted limitations, restated from the sections above so they are
findable in one place:

- No CI/CD platform integration.
- No fresh execution — reads an already-produced report file only.
- Only two rule types; no boolean combinators, no numeric thresholds, no
  per-scenario rules.
- Point-in-time library check.
- No batch evaluation — one policy file and one report file per invocation.
- Fixed, non-configurable resource limits.

This document intentionally does not claim CI results, production usage, or
performance benchmarks beyond what is recorded in `docs/phase-7-report.md`.
