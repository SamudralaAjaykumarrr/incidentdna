# Phase 7 Report: Local Policy Evaluation

This report documents the state of Phase 7 as implemented in the working
tree of branch `phase-7-local-policy-evaluation`, after the final
verification pass described below. It follows the same role
`docs/phase-6-report.md` served for Phase 6: a record of what was built, how
it was verified, and what is explicitly not yet true. `docs/phase-7-plan.md`
is the approved plan this implements.

## 1. Phase 7 goals

Phases 4-6 built a complete, bounded, offline *execution and cross-reference*
system: a scenario or suite can be run and classified deterministically
(Phases 4-5), and `scenario verify --library`/`suite verify --library` can
report whether a declared `linked_fingerprint` is a recorded failure class
(Phase 6). None of this was *interpreted*. Phase 7's entire scope is closing
exactly that gap, and nothing else:

1. A new, versioned policy document format — **IGP v0.1** ("Incident Gate
   Policy") — declaring an ordered list of rules to evaluate against exactly
   one already-produced report.
2. A local verifier, `incidentdna policy verify`, mirroring the existing
   `validate`/`scenario verify`/`suite verify` split.
3. A local evaluator, `incidentdna policy evaluate`, that loads a named
   policy file and a named, already-produced report file (a
   `scenario run --report`/`suite run --report` JSON document, unchanged
   shape), evaluates every declared rule, and reports a verdict
   (`PASS`/`FAIL`) with a meaningful exit code and an optional deterministic
   JSON verdict report.
4. Exactly two rule types: `require_result` and `require_library_occurrence`.
5. No new process-execution primitive: `internal/policy` never calls
   `exec.Command`, `scenario.Run`, or `suite.Run`.
6. `internal/policy` unaware the incident library exists: the
   `require_library_occurrence` rule is evaluated against a
   caller-supplied lookup result; the composition lives entirely in
   `cmd/incidentdna`.
7. Fully testable without any external service.

Explicitly not part of Phase 7 (see `docs/phase-7-plan.md` §3/§21/§22 for the
full list): CI/CD platform integration of any kind, fresh execution inside
the evaluator, more than two rule types, batch evaluation, mutation of any
existing store, and any change to `idir.Document`, `internal/validate`,
`internal/canonical`, `internal/fingerprint`, `internal/compare`,
`internal/evidence`, `internal/library`, `internal/scenario`, or
`internal/suite`.

## 2. Implementation summary

`internal/policy` is a new, sixth parallel leaf package: `types.go`,
`load.go`, `validate.go`, `report_load.go`, `evaluate.go`, `report.go`,
`limits.go`, `errors.go`, plus a matching `_test.go` for each. It imports
`internal/scenario` and `internal/suite` **only** for their existing,
unchanged `Report` struct definitions — it never calls `LoadFile`,
`Validate`, or `Run` from either package, and neither package gained a new
export or a new import. `internal/policy` does not import `internal/library`
at all; `require_library_occurrence` is evaluated against a caller-supplied
`policy.LibraryLookups` map, built by a new `cmd/incidentdna/cmd_policy.go`
composition that opens the library and calls the unchanged
`library.CheckFingerprint` once per distinct fingerprint — the identical
discipline `cmd_scenario.go`'s/`cmd_suite.go`'s Phase 6 composition already
established.

```
scenario.Report / suite.Report JSON file (Phase 4/5, unchanged)
   |  policy.LoadScenarioReport / LoadSuiteReport
   v
policy.LoadedReport{Kind, Result, DistinctFingerprints}
   |                                    policy document (IGP v0.1)
   |                                       |  policy.LoadFile + Validate
   |                                       v
   |                                (valid) policy.Document
   |          distinct fingerprints             |
   |             |  library.CheckFingerprint     |
   |             |  (Phase 6, unchanged --        |
   |             v  composed in cmd/incidentdna)  |
   |          policy.LibraryLookups ---------------+
   v             v
policy.Evaluate(policyDoc, report, libraryLookups)
   v
per-rule OK/FAIL/SKIP + overall verdict PASS/FAIL
   |  policy.BuildReport + policy.WriteReport (--report only)
   v
deterministic JSON verdict report (policy-report/v0.1)
```

Full design writeup: `docs/policy-evaluation.md`.

## 3. Architecture

New package `internal/policy` (8 non-test files, 8 test files). New CLI
files `cmd/incidentdna/cmd_policy.go` and
`cmd/incidentdna/cli_policy_test.go`. `cmd/incidentdna/main.go` gained one
new `{"policy", runPolicy}` command-table entry and one new usage-text
paragraph — nothing else in `main.go` changed; `policy` is **not**
special-cased in the dispatch loop the way `scenario`/`suite` are, since
`policy evaluate` performs no process execution and needs no larger timeout
budget than every other subcommand's existing fixed 30-second
`commandTimeout`.

`docs/architecture.md`'s dependency-direction statement continues to hold
exactly as written for Phase 6, extended with one new leaf: `internal/policy`
imports `internal/scenario`/`internal/suite` for their `Report` types only,
is not imported by anything in `internal/`, and does not import
`internal/library` — `cmd/incidentdna` is the only place all three of
`policy`, `scenario`/`suite`, and `library` meet.

## 4. The two commands and exit-code contract

```
incidentdna policy verify <policy-file>
incidentdna policy evaluate --policy <policy-file>
                             (--scenario-report <file> | --suite-report <file>)
                             [--library <dir>] [--report <file>]
```

**`policy verify`** (mirrors `scenario verify`'s/`suite verify`'s existing
0/1/2 split): `0` valid; `1` I/O/parse/usage error; `2` semantic IGP v0.1
rule failure.

**`policy evaluate`**: verdict `PASS` -> exit `0`; verdict `FAIL` (including
a `SKIP`ped rule) -> exit `2`; `INVALID` (policy or report file failed
pre-evaluation checks) -> exit `1`; `INTERNAL_ERROR` (e.g. a malformed
`--library`) -> exit `1`.

Both subcommands run under the existing fixed 30-second command context
every other subcommand already uses.

## 5. Determinism

1. `policy verify`'s result depends only on the policy file's own bytes.
2. `require_result` is a pure `==` string comparison against an
   already-deterministic report field.
3. `require_library_occurrence`'s result depends on the library's on-disk
   state at the moment `policy evaluate` runs, the identical non-claim
   `docs/library-crossref.md` already states.
4. Distinct-fingerprint extraction and lookup order is deterministic:
   first-occurrence declared order for a suite report, deduplicated; a
   `SKIPPED` scenario entry (`report: null`) contributes no fingerprint.
5. Given a fixed report file and a fixed library state, the overall verdict
   is deterministic and repeatable.

## 6. Resource limits and safety model

Four new, independent, fixed constants (`internal/policy/limits.go`):
`MaxPolicyDocumentSize` (64 KiB), `MaxRulesPerPolicy` (20),
`MaxReportDocumentSize` (256 MiB), `MaxDistinctFingerprintsPerEvaluation`
(100). No existing limit in any other package was redefined or widened.
Phase 7 introduces no new class of risk: `internal/policy` performs local
file reads only (a policy file, a report file, and, via `cmd/incidentdna`'s
composition, the identical read `library.CheckFingerprint` already
performs); no new write path exists anywhere in this phase beyond an
optional, caller-named `--report <path>` file, the same class every prior
`run`/`verify` command already has. `internal/policy` never calls
`exec.Command`, never calls `scenario.Run`/`suite.Run`, never calls
`library.Add`, and never creates the library root.

## 7. Corruption and malformed-input handling

Verified directly by both the unit test suite and the CLI black-box suite,
and by `scripts/verify-policy-demo.sh`'s malformed-report-file check (§9):
a malformed/oversized policy file, a semantically invalid policy, a
malformed/oversized/wrong-schema-kind report file, and a
too-many-distinct-fingerprints report all map to `INVALID`/exit `1` (with a
best-effort `--report` still written, `verdict: "INVALID"`, `error`
populated); a malformed `--library` maps to `INTERNAL_ERROR`/exit `1`;
`--library` omitted with a `require_library_occurrence` rule present maps
to a well-formed `FAIL`/exit `2` (`SKIP`, never a silent `PASS`); a
report-write failure is reported to stderr and exits `1` even after a
`PASS` verdict was already printed to stdout.

## 8. Demo coverage

`scripts/verify-policy-demo.sh` — a new script, not a new example directory
(Phase 7 introduces no new fixture content beyond one small, reviewable
policy file): composes already-existing, unmodified fixtures from
`examples/duplicate-payment/incident.yaml`,
`examples/regression-scenario-demo/scenario-pass.yaml`, and
`examples/regression-suite-demo/{suite-all-pass,suite-mixed}.yaml`, plus the
new checked-in `policies/release-gate-example.yaml`. Seven checks, all
against temporary report/`--library`/`--report` locations (never the
default `.incidentdna/library/objects`, never leaving generated output in
the repository): (1) generate real reports via `scenario run --report`/
`suite run --report`; (2) `policy verify` the checked-in example policy;
(3) evaluate a library-independent, single-rule policy against a PASS suite
report, verdict `PASS`; (4) the same policy against a FAIL suite report,
verdict `FAIL` via `require_result`; (5) seed a temporary library with
`duplicate-payment/incident.yaml` and evaluate the checked-in
`release-gate-example.yaml` (both rules) with `--library`, verdict `PASS`,
`[OK] require_library_occurrence`; (6) the identical evaluation with
`--library` omitted, verdict `FAIL`, `[SKIP] require_library_occurrence`;
(7) a deliberately corrupted report JSON file, `INVALID`, exit `1`, with the
best-effort `--report` content confirmed on disk.

Real transcript from this verification pass
(`scripts/verify-policy-demo.sh`, run standalone):

```
verify-policy-demo: generating real scenario/suite reports
verify-policy-demo: policy verify policies/release-gate-example.yaml (expect: valid, exit 0)
Policy: default-release-gate (schema policy/v0.1)
Rules: 2 declared
  [OK] require_result: PASS
  [OK] require_library_occurrence
OK: policy is structurally valid
verify-policy-demo: policy evaluate (require_result only) against a PASS suite report (expect: verdict PASS, exit 0)
Policy: require-result-only (schema policy/v0.1)
Input: suite report (result: PASS), 1 distinct linked fingerprint(s)
  [OK] require_result: PASS (report result was PASS)
Verdict: PASS
verify-policy-demo: policy evaluate (require_result only) against a FAIL suite report (expect: verdict FAIL, exit 2)
Policy: require-result-only (schema policy/v0.1)
Input: suite report (result: FAIL), 1 distinct linked fingerprint(s)
  [FAIL] require_result: PASS (report result was FAIL, expected PASS)
Verdict: FAIL
verify-policy-demo: library add duplicate-payment/incident.yaml into a fresh temporary library
warning: --allow-unredacted override was used: this document does not declare privacy.redacted == true; it was stored anyway
verify-policy-demo: policy evaluate (both rules) against the scenario PASS report with --library (expect: verdict PASS, exit 0)
Policy: default-release-gate (schema policy/v0.1)
Input: scenario report (result: PASS), 1 distinct linked fingerprint(s)
  [OK] require_result: PASS (report result was PASS)
  [OK] require_library_occurrence: 1 of 1 distinct fingerprint(s) have library occurrences
Verdict: PASS
verify-policy-demo: policy evaluate (both rules) against the scenario PASS report without --library (expect: SKIP, verdict FAIL, exit 2)
Policy: default-release-gate (schema policy/v0.1)
Input: scenario report (result: PASS), 1 distinct linked fingerprint(s)
  [OK] require_result: PASS (report result was PASS)
  [SKIP] require_library_occurrence: not evaluated (--library not given)
Verdict: FAIL
verify-policy-demo: policy evaluate against a malformed report file (expect: INVALID, exit 1)
incidentdna: policy: parse scenario report /tmp/tmp.CP45bovC1T/malformed-report.json: invalid character 'n' looking for beginning of object key string
verify-policy-demo: OK
```

## 9. Test coverage

- **`internal/policy`**: 51 top-level `Test*` functions across
  `load_test.go`, `validate_test.go`, `report_load_test.go`,
  `evaluate_test.go`, `report_test.go`, `limits_test.go` — covering both
  format sniffing paths, every documented semantic validation rule and its
  boundary, oversized/malformed/wrong-schema report rejection,
  distinct-fingerprint dedup and `MaxDistinctFingerprintsPerEvaluation`
  enforcement, every `require_result`/`require_library_occurrence`
  combination (match, mismatch, all-match, partial-match, no-match, `SKIP`
  via nil lookups, vacuous zero-fingerprint case, an unknown-rule-type
  defensive error path, and `nil` lookups proven safe when no
  library-dependent rule exists), deterministic JSON field order across
  repeated marshals, and `--report` overwrite behavior.
- **`cmd/incidentdna/cli_policy_test.go`**: 23 top-level `Test*` functions,
  black-box against the real compiled binary — `policy verify` exit 0/1/2
  paths (including a per-rule `[FAIL]` line assertion); `policy evaluate`
  exit 0/1/2 across `PASS`/`FAIL`/`INVALID`/`INTERNAL_ERROR`, using report
  files produced by real, preceding `scenario run --report`/
  `suite run --report` invocations (not only hand-authored report JSON);
  `--library` given/omitted with a `require_library_occurrence` rule
  present, both match and no-match; a malformed library (exit `1`, stderr
  mentions "malformed"); `--scenario-report`/`--suite-report` mutual
  exclusivity and "exactly one required" enforcement; `--policy` required;
  an incompatible report schema (a suite report loaded as
  `--scenario-report`); `--report` content matching the printed summary,
  including on the `INVALID` best-effort path; and `policy help`/unknown
  subcommand handling.
- **Race**: `go test ./... -race -count=1` is green across all 11 packages.
- **Regression discipline**: the full existing suite (453 top-level `Test*`
  functions across `cmd/incidentdna` and all `internal/*` packages,
  including this phase's 74 new ones, per `go test ./... -list '.*'`) passed
  unmodified where pre-existing; the golden fingerprint test/script produced
  the unchanged Phase 1 value; `git diff --stat -- internal/idir
  internal/validate internal/canonical internal/fingerprint internal/compare
  internal/evidence internal/library` is empty; `git diff --stat --
  internal/scenario internal/suite` shows no `.go` file changed.

## 10. Files added and modified

**New**: `internal/policy/{types,load,validate,report_load,evaluate,report,
limits,errors}.go` and matching `_test.go` files;
`cmd/incidentdna/cmd_policy.go`, `cmd/incidentdna/cli_policy_test.go`;
`policies/release-gate-example.yaml`; `docs/policy-evaluation.md`, this
report (`docs/phase-7-report.md`); `scripts/verify-policy-demo.sh`.

**Modified**: `cmd/incidentdna/main.go` (one new `{"policy", runPolicy}`
entry + usage text); `README.md`, `docs/architecture.md`,
`docs/product-scope.md`, `docs/threat-model.md`, `docs/privacy-model.md`;
`Makefile` (`example-policy` target, wired into `verify` and `.PHONY`);
`.github/workflows/ci.yml` (new "Policy evaluation demo validation" step).
No change to `.gitignore` was needed.

No file under `internal/idir/`, `internal/validate/`, `internal/canonical/`,
`internal/fingerprint/`, `internal/compare/`, `internal/evidence/`,
`internal/library/`, any `.go` file under `internal/scenario/` or
`internal/suite/`, `cmd/incidentdna/cmd_scenario.go`,
`cmd/incidentdna/cmd_suite.go`, `cmd/incidentdna/cmd_library.go`, any of the
five pre-existing `examples/` directories, or `testdata/golden/` was touched
at any point in Phase 7.

## 11. Commands run and real results

All commands below were run in this verification pass, from the repository
root, using a locally installed Go toolchain directly (per `CLAUDE.md`'s
stated fallback).

| # | Command | Result |
|---|---|---|
| 1 | `git diff --check` | Exit 0 — no whitespace errors, no conflict markers |
| 2 | `gofmt -l .` | Exit 0 — empty output, no unformatted files |
| 3 | `go vet ./...` | Exit 0 — no findings |
| 4 | `go test ./... -race -count=1` | Exit 0 — all 11 packages `ok` |
| 5 | `go build -buildvcs=false -o bin/incidentdna ./cmd/incidentdna` | Exit 0 — binary built |
| 6 | `scripts/verify-golden-fingerprint.sh` | Exit 0 — `verify-golden-fingerprint: OK (sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e)` |
| 7 | `scripts/verify-evidence-demo.sh` | Exit 0 — `verify-evidence-demo: OK` |
| 8 | `scripts/verify-library-demo.sh` | Exit 0 — `verify-library-demo: OK` |
| 9 | `scripts/verify-scenario-demo.sh` | Exit 0 — `verify-scenario-demo: OK` |
| 10 | `scripts/verify-suite-demo.sh` | Exit 0 — `verify-suite-demo: OK` |
| 11 | `scripts/verify-library-crossref-demo.sh` | Exit 0 — `verify-library-crossref-demo: OK` |
| 12 | `scripts/verify-policy-demo.sh` | Exit 0 — `verify-policy-demo: OK` (full transcript in §8) |
| 13 | `git diff --stat -- testdata/golden` | Empty — no output |
| 14 | `git diff --stat -- examples/duplicate-payment examples/evidence-storage-demo examples/incident-library-demo examples/regression-scenario-demo examples/regression-suite-demo` | Empty — no output |
| 15 | `git diff --stat -- internal/idir internal/validate internal/canonical internal/fingerprint internal/compare internal/evidence internal/library` | Empty — no output |
| 16 | `git diff --stat -- internal/scenario internal/suite \| grep '\.go'` | Empty — no `.go` file changed |
| 17 | `grep -rn '"net' cmd/ internal/` | No hits — no network package imported (verified during review) |
| 18 | `find . -maxdepth 3 -name .incidentdna` | Empty — no generated store directory left in the repository |
| 19 | `git status` | Working tree: 8 modified files, 6 new untracked files/directories, nothing staged, nothing else untracked |

Note on the Makefile/Docker path: the containerized `make verify` target was
not separately re-run inside `docker compose run --rm dev` in this session;
the equivalent `go`/`gofmt` commands were run directly per the documented
fallback, which produces identical results since the Makefile is a thin
wrapper. `Makefile` and `.github/workflows/ci.yml` were updated so a future
containerized run exercises the identical `example-policy` target this
report's §8 transcript already demonstrates.

## 12. Phase 1 through Phase 6 regression status

- `examples/duplicate-payment/incident.yaml` and
  `testdata/golden/duplicate-payment.fingerprint`: confirmed byte-for-byte
  unchanged; golden fingerprint reproduced identically at
  `sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e`.
- `examples/evidence-storage-demo/`, `examples/incident-library-demo/`,
  `examples/regression-scenario-demo/`, and `examples/regression-suite-demo/`:
  confirmed unchanged; all five prior demo scripts (including the Phase 6
  library cross-reference demo) passed in full, unmodified.
- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library`, `internal/scenario`, `internal/suite`: no `.go` file
  modified — none appear in `git diff --stat`.
- All pre-existing CLI subcommands and their tests (`cli_test.go`,
  `cli_evidence_test.go`, `cli_library_test.go`, `cli_scenario_test.go`,
  `cli_suite_test.go`) are unmodified and pass under
  `go test ./... -race -count=1`.
- `main.go`'s pre-existing `"scenario"`/`"suite"` context-timeout
  special-case is unaffected — `"policy"` was deliberately **not** added to
  that special-case list, since `policy evaluate` needs no larger timeout
  budget than the existing fixed 30-second `commandTimeout`.

## 13. Known limitations

Restated from `docs/policy-evaluation.md` for completeness:

- **No CI/CD platform integration.** `policy evaluate`'s exit code and JSON
  verdict are available to be consumed by something else; IncidentDNA
  itself does not consume them into any external system.
- **No fresh execution.** `policy evaluate` reads an already-produced
  report file; it does not itself run `scenario.Run`/`suite.Run`.
- **Only two rule types in v0.1** — no boolean combinators, no numeric
  thresholds, no per-scenario (as opposed to per-report) rules.
- **Point-in-time library check**, restated from Phase 6's own identical
  limitation.
- **No batch evaluation** — one policy file and one report file per
  invocation.
- **Fixed, non-configurable resource limits.**
- **A `require_library_occurrence` rule over zero distinct fingerprints is
  vacuously satisfied** (a documented judgment call — the plan does not
  state this case explicitly; a report naming no fingerprints has nothing
  to require an occurrence for).

## 14. Acceptance criteria status

Every criterion named in `docs/phase-7-plan.md`'s goals (§2), non-goals
(§3), implementation slices (§17), file list (§18), test requirements
(§19), and demo requirements (§20) is satisfied:

1. IGP v0.1 policy document format defined, loaded through a dedicated,
   size-capped loader, structurally/semantically validated. ✅
2. `incidentdna policy verify` implemented, mirroring the existing
   `validate`/`scenario verify`/`suite verify` split. ✅
3. `incidentdna policy evaluate` implemented: loads a named policy and a
   named report file, evaluates every declared rule, reports a
   `PASS`/`FAIL` verdict with a meaningful exit code, and optionally writes
   a deterministic JSON verdict report. ✅
4. Exactly two rule types (`require_result`, `require_library_occurrence`),
   each a pure, read-only composition of an already-existing, unchanged
   piece. ✅
5. No new process-execution primitive: `internal/policy` never calls
   `exec.Command`, `scenario.Run`, or `suite.Run`, and does not parse a
   scenario or suite document. ✅
6. `internal/policy` is a library-store-unaware leaf package: it does not
   import `internal/library`; the `require_library_occurrence` composition
   lives entirely in `cmd/incidentdna`. ✅
7. Fully testable without any external service: `go test
   ./internal/policy/... -race -count=1` is self-contained. ✅
8. Deterministic rule ordering and output ordering: every rule evaluated,
   never short-circuited, in declared order. ✅
9. Human-readable output and machine-readable JSON decision reports, both
   implemented and verified against real command transcripts. ✅
10. Exact documented exit-code contract (§11) implemented and verified by
    both unit and CLI black-box tests. ✅
11. Malformed-policy, malformed-report, incompatible-report-kind, and
    missing-required-field handling all implemented and tested. ✅
12. Bounded file-size and resource behavior: four new fixed limits, each
    with its own error and boundary test pair. ✅
13. Local, offline, read-only operation: no network access, no telemetry,
    no mutation of any existing store. ✅
14. Focused unit, integration, CLI, race, boundary, malformed-input,
    corruption, and regression tests: 74 new top-level test functions. ✅
15. Runnable Phase 7 example (`policies/release-gate-example.yaml` +
    `scripts/verify-policy-demo.sh`) and Phase 7 verification script,
    Makefile target, and GitHub Actions integration, all wired and
    passing. ✅
16. Documentation updates made (`docs/policy-evaluation.md`, `README.md`,
    `docs/architecture.md`, `docs/product-scope.md`,
    `docs/threat-model.md`, `docs/privacy-model.md`) and this report. ✅
17. Preserved: Phase 1 canonicalization/fingerprint semantics and golden
    fingerprint; Phase 2 evidence-storage behavior; Phase 3 incident-library
    behavior; Phase 4 scenario verify/run behavior; Phase 5 suite
    verify/run behavior; Phase 6 read-only library correlation behavior;
    existing scenario/suite JSON report formats; existing CLI contracts
    except the approved additive `policy` commands; existing privacy and
    resource-limit protections; deterministic, local, offline operation —
    all confirmed by inspection of the diff and by the full existing test
    suite passing unmodified. ✅
18. No unrelated features, no execution of scenarios/suites inside the
    policy evaluator, no CI-vendor-specific APIs, no network access, no
    cloud storage, no telemetry, no signing/key management, no sandboxing
    technology, no deployment/release-execution functionality, no mutation
    of incidents/evidence/library occurrences/scenario or suite files, no
    change to existing report schemas, no change to fingerprint identity
    semantics, no silent golden-file regeneration. ✅ (confirmed by
    inspection of the diff and by the full existing test suite passing
    unmodified)
19. Nothing was committed, staged, or pushed during implementation. ✅

## 15. Phase 8 boundary (explicit)

Everything listed as explicitly out of scope in `docs/product-scope.md` and
`docs/phase-7-plan.md` §22 remains deferred, unstarted, and unscoped in this
codebase: CI/CD platform integration of `policy evaluate`'s exit code,
additional policy rule types (boolean combinators, numeric thresholds,
per-scenario rules), fresh-execution convenience inside `policy evaluate`,
sandboxed scenario/suite execution, remote/shared library or evidence
storage, evidence/library signing or authenticity proof,
observability/event-system ingestion, parallel suite execution,
directory/glob-based scenario discovery, retention/garbage collection, and
any change to `idir.Document`, the JSON Schema, or the fingerprint
algorithm. **Phase 8 has not been started or scoped as part of this work.**
This report makes no claim about what a future Phase 8 would contain beyond
what `docs/product-scope.md`'s existing "future work" list already names.

## 16. GitHub Actions status

`.github/workflows/ci.yml` has been updated to add a "Policy evaluation
demo validation" step (`make example-policy`) to the existing job, so the
workflow-as-configured now covers the complete Phase 7 verification path in
addition to every prior phase's step.

**This configuration has not yet been exercised by the GitHub-hosted
runner.** Every command in §11 was run locally with a direct Go toolchain
install, with results recorded above — but the actual hosted GitHub Actions
workflow run has not been confirmed, because these changes have not been
pushed. This report makes no claim that the hosted CI workflow has passed.

## 17. Final completion checklist

- [x] `internal/policy` implemented: IGP v0.1 document model, size-capped
  loader, semantic validator, report-artifact loader with
  distinct-fingerprint extraction, deterministic rule evaluator, JSON
  verdict report — 51 new table-driven tests.
- [x] `incidentdna policy verify`/`evaluate` work end-to-end against real
  fixtures, with real command transcripts recorded in this report (§8) — 23
  new CLI black-box tests.
- [x] Full existing suite still passes unmodified: `gofmt -l` clean, `go vet
  ./...` clean, `go test ./... -race -count=1` green (all 11 packages), and
  the golden fingerprint value is byte-for-byte unchanged.
- [x] `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/library`, `internal/scenario`, `internal/suite` are untouched.
- [x] `docs/policy-evaluation.md`, `README.md`, `docs/architecture.md`,
  `docs/product-scope.md`, `docs/threat-model.md`, `docs/privacy-model.md`
  updated to match implemented behavior.
- [x] `scripts/verify-policy-demo.sh` created and passes standalone.
- [x] `Makefile` gained `example-policy`, wired into `verify` and
  `.PHONY`.
- [x] `.github/workflows/ci.yml` updated to run the new demo step,
  preserving every existing Phase 1-6 CI step unchanged.
- [x] `git diff --check`, `gofmt -l .`, `go vet ./...`, `go test ./...
  -race -count=1`, `go build`, and every one of
  `scripts/verify-golden-fingerprint.sh`, `scripts/verify-evidence-demo.sh`,
  `scripts/verify-library-demo.sh`, `scripts/verify-scenario-demo.sh`,
  `scripts/verify-suite-demo.sh`, `scripts/verify-library-crossref-demo.sh`,
  `scripts/verify-policy-demo.sh` passed locally (exit 0 throughout, §11).
- [x] No `.incidentdna` directory or generated evidence/library-store
  artifacts remain anywhere in the repository.
- [x] `examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
  `examples/incident-library-demo/`, `examples/regression-scenario-demo/`,
  and `examples/regression-suite-demo/` confirmed byte-for-byte unchanged.
- [x] `internal/policy` never writes anywhere beyond an optional,
  caller-named `--report` file; never calls `library.Add`; never creates
  the library root; is unreachable from `scenario run`/`suite run` —
  proven by inspection and by every test passing.
- [x] The new demo script and checked-in example policy contain no
  credentials, personal information, customer data, or real incident
  material — composed entirely from already-reviewed, pre-existing
  synthetic fixtures plus one new small, reviewable policy file.
- [x] Diff inspected for security regressions, incorrect documentation
  claims, incomplete CLI wiring, unsafe filesystem behavior, test gaps,
  accidental Phase 8 scope, sensitive/real data, and generated artifacts —
  none found.
- [x] One deliberate judgment call, not explicitly resolved by the approved
  plan, is documented explicitly rather than silently resolved: a
  `require_library_occurrence` rule over a report naming zero distinct
  fingerprints is treated as vacuously satisfied (§13).
- [ ] Hosted GitHub Actions run — **not yet confirmed** (requires pushing
  this branch, not done as part of this task).
- [ ] Commit / push / merge — **not done**, per explicit instruction for
  this task.

No production adoption, deployment, customer usage, or performance
benchmarking is claimed anywhere in this report or in Phase 7 generally.
