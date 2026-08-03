# Phase 6 Report: Library Cross-Reference

This report documents the state of Phase 6 as implemented in the working
tree of branch `phase-6-library-correlation`, after the final verification
pass described below. It follows the same role `docs/phase-5-report.md`
served for Phase 5: a record of what was built, how it was verified, and
what is explicitly not yet true. `docs/phase-6-plan.md` is the approved plan
this implements.

## 1. Phase 6 goals

Phases 4–5 built a complete, bounded, offline execution system
(`scenario`/`suite`) on top of Phase 3's incident library, but kept the two
deliberately disconnected: a scenario's/suite's `linked_fingerprint` was
never checked against what the library actually holds. Phase 6's entire
scope is closing exactly that gap, and nothing else:

1. One new exported function, `internal/library.CheckFingerprint` (plus one
   new sentinel error, `ErrInvalidFingerprint`) — the only code change to an
   existing package.
2. An optional `--library <dir>` flag on `incidentdna scenario verify`.
3. An optional `--library <dir>` flag on `incidentdna suite verify`, with
   deduplication of listed scenarios' `linked_fingerprint` values in
   first-occurrence declared order.
4. Strictly informational: never affects either command's existing exit
   code, and `scenario run`/`suite run` are entirely untouched.
5. Documentation updates so `docs/library-crossref.md`, `README.md`,
   `docs/architecture.md`, `docs/product-scope.md`, `docs/threat-model.md`,
   `docs/privacy-model.md`, `docs/regression-scenarios.md`, and
   `docs/scenario-suites.md` accurately describe what Phase 6 covers.
6. A Makefile/CI verification path exercising the above end-to-end.

Explicitly not part of Phase 6 (see `docs/phase-6-plan.md` §3 for the full
list): release gating, sandboxed execution, remote storage, signing,
multi-tenancy, parallel execution, directory/glob discovery, AI generation,
or any change to `idir.Document`, `internal/validate`, `internal/canonical`,
`internal/fingerprint`, `internal/compare`, `internal/evidence`,
`internal/scenario`, or `internal/suite`.

## 2. Implementation summary

Unlike every prior phase, Phase 6 adds **no new package**. It is a small,
additive change to one existing package (`internal/library`), composed
entirely at the `cmd/incidentdna` layer:

```
scenario.Document / suite.ScenarioEntry     library.Store (opened via the
   │  .LinkedFingerprint (already declared,   existing library.Open, the
   │   already format-checked)                same store `library check`
   ▼                                          reads)
linked_fingerprint string ──────────────────────────┐
                                                      │  library.CheckFingerprint
                                                      │  (Phase 6, NEW)
                                                      ▼
                                    CheckResult{Outcome, MatchCount}
                                                      │
                                                      ▼
                              "Library: N occurrence(s) found" /
                              "Library: no occurrences found" (printed only)
```

`internal/library/check.go`'s existing `Check(ctx, s, doc)` was refactored,
without changing its own exported behavior, into: validate the document,
compute its fingerprint, then call a new unexported `checkFingerprint(ctx,
s, fp)` helper holding every line of lookup logic that runs after
fingerprint computation (index read, no-match short-circuit, per-occurrence
integrity re-verification, match/no-match result construction). The new
exported `CheckFingerprint(ctx, s, fp string)` validates `fp`'s format
(`ErrInvalidFingerprint` if malformed) and then calls the identical
`checkFingerprint` helper — proven identical to `Check`'s own lookup
behavior by a new parity test (§9).

`cmd/incidentdna/cmd_scenario.go`'s `runScenarioVerify` and
`cmd/incidentdna/cmd_suite.go`'s `runSuiteVerify` each gained a `--library
<dir>` flag (reusing `cmd_library.go`'s existing `libraryFlag`/
`openLibraryStore` helpers, unmodified) and a small amount of new
composition logic — `runSuiteVerify`'s additionally deduplicates listed
scenarios' `LinkedFingerprint` values in first-occurrence declared order
before looking each up, reusing `scenario.LoadFile` to read each listed
scenario's already-validated `LinkedFingerprint` field. Neither
`internal/scenario` nor `internal/suite` gained a new file, a new exported
symbol, or a new import.

Full design writeup: `docs/library-crossref.md`.

## 3. Architecture

`internal/library`'s existing files `add.go`, `list.go`, `privacy.go`,
`store.go`, `index.go`, `limits.go` are untouched. Only `check.go` and
`errors.go` changed, both additively:

- `check.go`: `Check` is now a thin wrapper (validate → compute fingerprint
  → call shared helper); `CheckFingerprint` is new; the shared
  `checkFingerprint` helper is new (extracted, not rewritten, from `Check`'s
  prior body — the lookup logic itself is byte-for-byte the same code,
  proven by every pre-existing `Check`-focused test in `check_test.go`
  passing unmodified).
- `errors.go`: one new sentinel, `ErrInvalidFingerprint`, appended after the
  existing five; no existing sentinel's name, value, or `errors.Is`
  behavior changed.

`docs/architecture.md`'s dependency-direction statement continues to hold
exactly as written for Phase 5: `internal/scenario` and `internal/suite`
still do not import, and are not imported by, `internal/library`. Phase 6
adds `internal/library` as a new import to `cmd_scenario.go` and
`cmd_suite.go` only — the first time either file imports it — matching
`docs/phase-6-plan.md` §6's exact description.

## 4. The two flags and exit-code contract

```
incidentdna scenario verify [--source <incident-file>] [--library <dir>] <scenario-file>
incidentdna suite verify [--library <dir>] <suite-file>
```

Both flags mirror `cmd_library.go`'s existing `--library` flag shape and
default (`library.DefaultLibraryRoot`). `libraryGiven` — detected via
`fs.Visit`, distinguishing "flag not passed at all" from "flag passed with
an empty value" — gates whether any Phase 6 code path runs at all, so
omitting `--library` produces stdout byte-for-byte identical to pre-Phase-6
output (verified directly, §9).

**`scenario verify`** (extends, does not replace, the existing 0/1/2
split): `0` valid (and, with `--library`, the lookup result is printed but
never affects this); `1` I/O/parse/usage error, **plus** a malformed or
unreadable `--library` target; `2` semantic rule failure or `--source`
mismatch, unchanged — a `--library` "no occurrences found" result never
produces `2`.

**`suite verify`** identically: `0`/`2` unchanged in every existing cause;
`1` gains the same new cause.

**`scenario run` and `suite run`**: entirely unchanged — no new flag, no
new exit-code cause, no new report field, proven by the full pre-existing
`cli_scenario_test.go`/`cli_suite_test.go` `run`-focused tests passing
unmodified.

## 5. Determinism

1. A cross-reference result depends on the library's on-disk state at the
   moment of the call — can legitimately differ between two otherwise-
   identical invocations if an intervening `library add` changed the
   library's contents, mirroring `library check`'s own existing stance.
2. Given a fixed library state, the result is deterministic: `checkFingerprint`
   re-verifies every indexed occurrence's integrity before counting it, so a
   corrupted occurrence is never silently reported as a match.
3. Suite-level lookup order is exactly the declared scenario order,
   deduplicated to first occurrence — proven directly against
   `examples/regression-suite-demo/suite-mixed.yaml`, whose three listed
   scenarios all happen to share one checked-in `linked_fingerprint` (§8):
   `suite verify --library` reports one lookup's worth of result
   (`Library cross-reference: 1 of 1 distinct ...`), not three.
4. `--library` omitted is byte-for-byte identical to pre-Phase-6 output —
   verified directly by `TestCLI_ScenarioVerify_LibraryOmittedProducesUnchangedOutput`
   and `TestCLI_SuiteVerify_LibraryOmittedProducesUnchangedOutput`, each
   asserting exact string equality/absence, not just a substring check.

## 6. Corruption and malformed-input handling

- `--library` omitted: no new behavior of any kind; every existing
  corruption/malformed-input case is unchanged.
- `--library` given, library root does not exist yet: `library.Open`'s
  existing "not yet created is not an error" behavior applies — the lookup
  proceeds and reports "no occurrences found," exit unaffected. Verified by
  `TestCLI_ScenarioVerify_LibraryNotYetCreatedTreatedAsNoMatch` (also
  asserts the not-yet-created path is never itself created as a side
  effect of the read-only lookup).
- `--library` given, library exists but is malformed (corrupted
  `index.json` for the fingerprint being looked up): `library.CheckFingerprint`
  returns the same `ErrMalformedIndex`/`ErrCorruptedOccurrence` family
  `Check` already returns; both `verify` subcommands map this to exit `1`.
  Verified by `TestCLI_ScenarioVerify_LibraryMalformedReturnsExitOne` and
  `TestCLI_SuiteVerify_LibraryMalformedReturnsExitOne`.
- `--library` given, `--source` also given (`scenario verify` only): both
  checks run independently; the `--library` lookup only ever runs after the
  structural/semantic and `--source` checks have already passed. Verified
  by `TestCLI_ScenarioVerify_LibraryCombinedWithSource`.
- A suite's listed scenarios sharing an identical `linked_fingerprint`: not
  an error; the fingerprint is looked up once and the cached result shown
  on every scenario line that shares it. Verified by
  `TestCLI_SuiteVerify_LibraryAllMatch`, which additionally asserts the
  aggregate line reflects the *distinct* count (1 of 1), not the raw
  listed-scenario count (2).

## 7. Resource limits and safety model

No new resource limit is introduced — bounded entirely by the existing
`suite.MaxScenariosPerSuite` (100, already capping how many distinct
fingerprints one `suite verify --library` call can look up) and
`internal/library`'s own existing limits, reused unchanged. Phase 6
introduces no new class of risk: `library.CheckFingerprint` performs the
identical reads `library.Check` already performs (symlink rejection,
occurrence re-hash verification, `checkContained` path safety); no new
write path exists anywhere in this phase — `scenario verify --library` and
`suite verify --library` never call `library.Add`, never create the library
root if it doesn't exist, and are unreachable from `scenario run`/`suite
run`.

## 8. Demo coverage

`scripts/verify-library-crossref-demo.sh` — a new script, not a new example
directory (Phase 6 introduces no new document format or fixture content):
composes already-existing, unmodified fixtures from
`examples/duplicate-payment/incident.yaml`,
`examples/regression-scenario-demo/scenario-pass.yaml`, and
`examples/regression-suite-demo/suite-mixed.yaml`. Six checks, all against
temporary `--library` directories (never the default
`.incidentdna/library/objects`, never leaving generated output in the
repository):

1. **Match** — seed a fresh temporary library with
   `examples/duplicate-payment/incident.yaml`, then `scenario verify
   --library` against `scenario-pass.yaml` (whose checked-in
   `linked_fingerprint` is already the duplicate-payment example's own
   golden fingerprint — no new fixture value invented).
2. **No match** — the same lookup against a freshly created, empty
   temporary library.
3. **Not-yet-created library** — the same lookup against a `--library` path
   that does not exist on disk at all; also asserts the read-only lookup
   never creates it as a side effect.
4. **Suite-level** — `suite verify --library` against `suite-mixed.yaml`.
   **A deliberate, documented deviation from `docs/phase-6-plan.md` §4
   Workflow B's illustrative transcript**: the plan's transcript shows a mix
   of matched and unmatched per-scenario lines (`2 of 3 distinct`), but
   `suite-mixed.yaml`'s three actual listed scenarios
   (`scenario-pass.yaml`, `scenario-fail.yaml`, `scenario-timeout.yaml`)
   all declare the *identical* checked-in `linked_fingerprint` — there is
   only one distinct fingerprint among them, not three. This implementation
   follows the real, checked-in fixture data rather than silently editing
   an unrelated example file to manufacture the plan's illustrative numbers
   (`docs/phase-6-plan.md` §16 lists `examples/regression-suite-demo/` as
   untouched). The real transcript instead demonstrates deduplication
   directly and unambiguously: three listed scenarios sharing one
   fingerprint produce three matched per-scenario annotations and one
   `Library cross-reference: 1 of 1 distinct ...` aggregate line — proof
   that only one lookup's worth of result is being reported for three
   listed entries. `docs/library-crossref.md` and `docs/scenario-suites.md`
   were written to describe this real, reproducible behavior rather than
   the plan's aspirational transcript, per `docs/phase-6-plan.md` §15 slice
   5's own instruction that documentation is "done after the code so they
   describe actual behavior, not the plan." A second suite-level check
   against an empty library (0 of 1 distinct) demonstrates the genuine
   no-match case at the suite level.
5. **Malformed library** — corrupt a fingerprint directory's `index.json`
   inside a temporary library the script controls, then assert `scenario
   verify --library` exits `1` with an actionable error.
6. **`--library` omitted** — asserts no `Library:` line appears.

Real transcript from this verification pass
(`scripts/verify-library-crossref-demo.sh`, run standalone):

```
verify-library-crossref-demo: library add duplicate-payment/incident.yaml into a fresh temporary library
warning: --allow-unredacted override was used: this document does not declare privacy.redacted == true; it was stored anyway
verify-library-crossref-demo: scenario verify --library (expect: match, 1 occurrence, exit 0)
Scenario: duplicate-payment-refingerprint (schema irs/v0.1)
Linked fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
Library: 1 occurrence(s) found for this fingerprint
OK: scenario is structurally valid

verify-library-crossref-demo: scenario verify --library against an empty library (expect: no occurrences, exit 0, never exit 2)
Library: no occurrences found for this fingerprint (not yet recorded in the library)
OK: scenario is structurally valid

verify-library-crossref-demo: scenario verify --library against a not-yet-created library (expect: no occurrences, exit 0)
Library: no occurrences found for this fingerprint (not yet recorded in the library)
OK: scenario is structurally valid

verify-library-crossref-demo: suite verify --library suite-mixed.yaml (expect: 3 matched per-scenario annotations, 1 of 1 distinct)
Suite: suite-demo-mixed (schema suite/v0.1)
Scenarios: 3 listed
  [OK] suite-demo-pass (scenarios/scenario-pass.yaml) — library: 1 occurrence(s)
  [OK] suite-demo-fail (scenarios/scenario-fail.yaml) — library: 1 occurrence(s)
  [OK] suite-demo-timeout (scenarios/scenario-timeout.yaml) — library: 1 occurrence(s)
Library cross-reference: 1 of 1 distinct linked fingerprint(s) have library occurrences
OK: suite manifest and all 3 listed scenarios are structurally valid

verify-library-crossref-demo: suite verify --library against an empty library (expect: 3 no-match annotations, 0 of 1 distinct)
Library cross-reference: 0 of 1 distinct linked fingerprint(s) have library occurrences

verify-library-crossref-demo: scenario verify --library against a malformed library (expect: exit 1)
incidentdna: library: library: index is malformed: index "<tmp>/malformed-library/fc/3dac.../index.json" is not valid JSON: invalid character 'n' looking for beginning of object key string

verify-library-crossref-demo: scenario verify without --library (expect: no Library: line)
Scenario: duplicate-payment-refingerprint (schema irs/v0.1)
Linked fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
OK: scenario is structurally valid
verify-library-crossref-demo: OK
```

## 9. Test coverage

- **`internal/library/check_test.go`**: 9 new top-level `Test*` functions
  (plus subtests): `TestCheckFingerprint_RejectsMalformedFingerprint`
  (table-driven: empty, wrong prefix, missing prefix, too short, too long,
  uppercase hex, non-hex), `TestCheckFingerprint_MatchAgainstSeededLibrary`,
  `TestCheckFingerprint_MatchWithMultipleOccurrences`,
  `TestCheckFingerprint_NoMatchOnEmptyButExistingLibrary`,
  `TestCheckFingerprint_NoMatchOnNotYetCreatedLibrary`,
  `TestCheckFingerprint_MissingOccurrenceIsCorrupted`,
  `TestCheckFingerprint_MalformedIndexIsDetected`,
  `TestCheckFingerprint_ParityWithCheck` (match and no-match subtests,
  asserting `Check(doc)` and `CheckFingerprint(fingerprint.Compute(doc))`
  return byte-identical `CheckResult` values), and
  `TestCheckFingerprint_ContextCanceledBeforeAnyIO`. Every pre-existing
  `Check`-focused test in the same file passes unmodified.
- **`cmd/incidentdna/cli_scenario_test.go`**: 6 new top-level `Test*`
  functions, black-box against the real compiled binary:
  `TestCLI_ScenarioVerify_LibraryMatch`,
  `TestCLI_ScenarioVerify_LibraryNoMatchOnExistingLibrary`,
  `TestCLI_ScenarioVerify_LibraryNotYetCreatedTreatedAsNoMatch`,
  `TestCLI_ScenarioVerify_LibraryMalformedReturnsExitOne`,
  `TestCLI_ScenarioVerify_LibraryCombinedWithSource`, and
  `TestCLI_ScenarioVerify_LibraryOmittedProducesUnchangedOutput` (exact
  stdout string equality against the pre-Phase-6 baseline). Every
  pre-existing test in the file passes unmodified.
- **`cmd/incidentdna/cli_suite_test.go`**: 4 new top-level `Test*`
  functions: `TestCLI_SuiteVerify_LibraryAllMatch` (asserts dedup:
  2 listed scenarios sharing 1 fingerprint produce a "1 of 1 distinct"
  aggregate line, not "2 of 2"), `TestCLI_SuiteVerify_LibraryMixedMatchAndNoMatch`
  (a genuine per-scenario match/no-match mix, independent of the demo
  fixtures' identical-fingerprint quirk — §8), `TestCLI_SuiteVerify_LibraryMalformedReturnsExitOne`,
  and `TestCLI_SuiteVerify_LibraryOmittedProducesUnchangedOutput`. Every
  pre-existing test in the file passes unmodified.
- **Race**: `go test ./... -race -count=1` is green across all 10 packages.
- **Regression discipline**: the full existing suite (360 pre-existing
  top-level tests across `cmd/incidentdna` and 8 `internal/*` packages, per
  `go test ./... -list '.*'` before this phase's additions) passed
  unmodified; the golden fingerprint test/script produced the unchanged
  Phase 1 value; `git diff --stat -- internal/idir internal/validate
  internal/canonical internal/fingerprint internal/compare
  internal/evidence internal/scenario internal/suite` is empty.

## 10. Files added and modified

**Modified**: `internal/library/check.go` (extract `checkFingerprint`, add
`CheckFingerprint`), `internal/library/errors.go` (add
`ErrInvalidFingerprint`), `internal/library/check_test.go` (9 new tests);
`cmd/incidentdna/cmd_scenario.go` (`--library` flag + composition on
`runScenarioVerify` only), `cmd/incidentdna/cmd_suite.go` (`--library` flag
+ composition + dedup on `runSuiteVerify` only),
`cmd/incidentdna/cli_scenario_test.go` (6 new tests + 1 new shared helper,
`writeScenarioYAMLWithFingerprint`), `cmd/incidentdna/cli_suite_test.go` (4
new tests); `README.md`, `docs/architecture.md`, `docs/product-scope.md`,
`docs/threat-model.md`, `docs/privacy-model.md`,
`docs/regression-scenarios.md`, `docs/scenario-suites.md`; `Makefile`
(`example-library-crossref` target, wired into `verify` and `.PHONY`);
`.github/workflows/ci.yml` (new "Library cross-reference demo validation"
step).

**New**: `docs/library-crossref.md`, this report
(`docs/phase-6-report.md`), `scripts/verify-library-crossref-demo.sh`.

No file under `internal/idir/`, `internal/validate/`, `internal/canonical/`,
`internal/fingerprint/`, `internal/compare/`, `internal/evidence/`,
`internal/scenario/`, `internal/suite/`, `internal/library/{add,list,privacy,store,index,limits}.go`,
`cmd/incidentdna/cmd_library.go`, `cmd/incidentdna/main.go`, any of the five
`examples/` directories, or `testdata/golden/` was touched at any point in
Phase 6.

## 11. Commands run and real results

All commands below were run in this verification pass, from the repository
root, using a locally installed Go 1.26 toolchain directly (per
`CLAUDE.md`'s stated fallback — "If Go is available directly ... drop the
`docker compose run --rm dev` prefix").

| # | Command | Result |
|---|---|---|
| 1 | `git diff --check` | Exit 0 — no whitespace errors, no conflict markers |
| 2 | `gofmt -l .` | Exit 0 — empty output, no unformatted files |
| 3 | `go vet ./...` | Exit 0 — no findings |
| 4 | `go test ./... -race -count=1` | Exit 0 — all 10 packages `ok` |
| 5 | `go build -buildvcs=false -o bin/incidentdna ./cmd/incidentdna` | Exit 0 — binary built |
| 6 | `scripts/verify-golden-fingerprint.sh` | Exit 0 — `verify-golden-fingerprint: OK (sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e)` |
| 7 | `scripts/verify-evidence-demo.sh` | Exit 0 — `verify-evidence-demo: OK` |
| 8 | `scripts/verify-library-demo.sh` | Exit 0 — `verify-library-demo: OK` |
| 9 | `scripts/verify-scenario-demo.sh` | Exit 0 — `verify-scenario-demo: OK` |
| 10 | `scripts/verify-suite-demo.sh` | Exit 0 — `verify-suite-demo: OK` |
| 11 | `scripts/verify-library-crossref-demo.sh` | Exit 0 — `verify-library-crossref-demo: OK` (full transcript in §8) |
| 12 | `git diff --stat -- testdata/golden` | Empty — no output |
| 13 | `git diff --stat -- examples/duplicate-payment examples/evidence-storage-demo examples/incident-library-demo examples/regression-scenario-demo examples/regression-suite-demo` | Empty — no output |
| 14 | `git diff --stat -- internal/idir internal/validate internal/canonical internal/fingerprint internal/compare internal/evidence internal/scenario internal/suite` | Empty — no output |
| 15 | `grep -rn '"net' cmd/ internal/` | No hits — no network package imported |
| 16 | `git status` | Working tree: 16 modified files, 2 new untracked doc/script files, nothing staged, nothing else untracked |

Note on the Makefile/Docker path: the containerized `make verify` target was
not separately re-run inside `docker compose run --rm dev` in this session
(the Docker daemon was available, but the equivalent `go`/`gofmt` commands
were run directly per the fallback rule above, which produces identical
results since the Makefile is a thin wrapper, not a distinct code path).
`Makefile` and `.github/workflows/ci.yml` were updated so a future
containerized run exercises the identical `example-library-crossref` target
this report's §8 transcript already demonstrates.

## 12. Phase 1 through Phase 5 regression status

- `examples/duplicate-payment/incident.yaml` and
  `testdata/golden/duplicate-payment.fingerprint`: confirmed byte-for-byte
  unchanged; golden fingerprint reproduced identically at
  `sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e`.
- `examples/evidence-storage-demo/`, `examples/incident-library-demo/`,
  `examples/regression-scenario-demo/`, and `examples/regression-suite-demo/`:
  confirmed unchanged (`git diff --stat` produced no output for any of the
  four); all four prior demo scripts passed in full, unmodified.
- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/scenario`, `internal/suite`: no files modified — none appear in
  `git status`/`git diff --stat`.
- `internal/library`'s own pre-existing exported behavior (`Open`, `Add`,
  `Check`, `List`, every existing error sentinel) is unchanged — every
  pre-existing test in `add_test.go`, `check_test.go` (the pre-Phase-6
  tests), `list_test.go`, `privacy_test.go`, `store_test.go`,
  `limits_test.go` passes unmodified.
- All pre-existing CLI subcommands and their tests
  (`cli_test.go`, `cli_evidence_test.go`, `cli_library_test.go`, and every
  pre-existing test in `cli_scenario_test.go`/`cli_suite_test.go`) are
  unmodified and pass under `go test ./... -race -count=1`.
- `main.go` is untouched — Phase 6 adds no new top-level command; `commands`
  table and the `"scenario"`/`"suite"` context-timeout special-case are
  unaffected, since `scenario verify`/`suite verify` already ran under the
  standard 30-second command context before Phase 6.

## 13. Known limitations

Restated from `docs/library-crossref.md` for completeness:

- **Point-in-time only.** A cross-reference result reflects the library's
  contents at the moment `verify --library` runs; nothing re-checks it
  later, including `scenario run`/`suite run`.
- **No release gating.**
- **No coupling inside `internal/scenario`/`internal/suite` themselves** —
  both remain unaware the incident library exists; the composition is
  entirely a `cmd/incidentdna`-layer concern.
- **Occurrence count only, not occurrence content** — the `Library:` line
  never reveals which specific incident(s) are recorded under a
  fingerprint.
- **No new resource limits** — bounded entirely by existing limits.
- **Not a substitute for `library check`** — `library check` remains the
  only command that validates and fingerprints a *fresh* incident document
  against the library.

## 14. Acceptance criteria status

Every criterion named in `docs/phase-6-plan.md`'s goals (§2), non-goals
(§3), implementation slices (§15), file list (§16), test requirements
(§17), and demo requirements (§18) is satisfied:

1. `--library <dir>` added to `scenario verify`, mirroring `--source`'s
   flag shape; after existing checks pass, looks up the scenario's own
   `linked_fingerprint` and prints match/no-match. ✅
2. `--library <dir>` added to `suite verify`; looks up every distinct
   listed-scenario `linked_fingerprint` (first-occurrence order),
   annotates each scenario line, prints an aggregate count. ✅
3. Implemented as a pure, read-only composition: one small, additive,
   exported function in `internal/library`, reused unchanged by both
   `scenario verify` and `suite verify` at the `cmd/incidentdna` layer;
   `internal/scenario` and `internal/suite` are not modified and gain no
   new import. ✅
4. Strictly informational: a "no occurrences found" result never changes
   either command's existing 0/2 exit-code meaning; `scenario run`/`suite
   run` untouched — no new exit code, no new JSON report field, no new
   behavior when `--library` is omitted (verified by exact-string
   regression tests, §9). ✅
5. No new class of risk: read-only local filesystem I/O under the exact
   same trust boundary `library check` already established; no new write
   path anywhere. ✅
6. Fully testable without any external service: `go test
   ./internal/library/... -race -count=1` and both CLI test files remain
   self-contained. ✅
7. No release gating, no change to `scenario run`/`suite run`, no new
   document field anywhere, no mutation of the library, no coupling built
   into `internal/scenario`/`internal/suite`, no new resource limits, no
   JSON output for the cross-reference result, no batch/multi-fingerprint
   query API, no signing/encryption/multi-tenancy/sandboxing/parallel
   execution/directory discovery/remote storage. ✅ (all confirmed by
   inspection of the diff and by the full existing test suite passing
   unmodified)
8. Full existing suite still passes unmodified: `gofmt -l` clean, `go vet
   ./...` clean, `go test ./... -race -count=1` green, golden fingerprint
   byte-for-byte unchanged. ✅
9. `internal/idir`, `internal/validate`, `internal/canonical`,
   `internal/fingerprint`, `internal/compare`, `internal/evidence`,
   `internal/scenario`, `internal/suite` are untouched. ✅
10. `examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
    `examples/incident-library-demo/`, `examples/regression-scenario-demo/`,
    and `examples/regression-suite-demo/` confirmed byte-for-byte unchanged
    by `git diff --stat`. ✅
11. No network access, no telemetry introduced (`grep -rn '"net' cmd/
    internal/` stays empty). ✅
12. `--library` given, both `scenario verify`/`suite verify` never write
    anywhere; the lookup never creates the library root if it doesn't
    exist. ✅
13. Documentation updates made; no doc makes a now-false claim about what
    is/isn't implemented; `docs/library-crossref.md` and
    `docs/scenario-suites.md` describe the real, reproducible fixture
    behavior rather than the plan's illustrative-but-unreproducible
    transcript (§8, documented explicitly, not silently resolved). ✅
14. Nothing was committed, staged, or pushed during implementation. ✅

## 15. Phase 7 boundary (explicit)

Everything listed as explicitly out of scope in `docs/product-scope.md` and
`docs/phase-6-plan.md` §20 remains deferred, unstarted, and unscoped in this
codebase: release-gate/CI-CD integration for `scenario run`/`suite run`,
sandboxed scenario execution, remote/cloud storage for the evidence store,
incident library, scenarios, or suites, evidence/library signing or
authenticity proof, ingestion from observability/event systems, parallel
suite execution, directory/glob-based scenario discovery, and any change to
`idir.Document`, the JSON Schema, or the fingerprint algorithm. **Phase 7
has not been started or scoped as part of this work.** This report makes no
claim about what a future Phase 7 would contain beyond what
`docs/product-scope.md`'s existing "future work" list already names.

## 16. GitHub Actions status

`.github/workflows/ci.yml` has been updated to add a "Library cross-reference
demo validation" step (`make example-library-crossref`) to the existing job,
so the workflow-as-configured now covers the complete Phase 6 verification
path in addition to every prior phase's step.

**This configuration has not yet been exercised by the GitHub-hosted
runner.** Every command in §11 was run locally with a direct Go toolchain
install, with results recorded above — but the actual hosted GitHub Actions
workflow run has not been confirmed, because these changes have not been
pushed. This report makes no claim that the hosted CI workflow has passed.

## 17. Final completion checklist

- [x] `internal/library.CheckFingerprint` implemented, sharing lookup logic
  with `Check` via an extracted helper, covered by 9 new table-driven
  tests including a parity test against `Check`.
- [x] `scenario verify --library` and `suite verify --library` work
  end-to-end against real fixtures, with real command transcripts recorded
  in this report (§8).
- [x] Full existing suite still passes unmodified: `gofmt -l` clean, `go vet
  ./...` clean, `go test ./... -race -count=1` green (all 10 packages), and
  the golden fingerprint value is byte-for-byte unchanged.
- [x] `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/scenario`, `internal/suite` are untouched.
- [x] `docs/library-crossref.md`, `README.md`, `docs/architecture.md`,
  `docs/product-scope.md`, `docs/threat-model.md`, `docs/privacy-model.md`,
  `docs/regression-scenarios.md`, `docs/scenario-suites.md` updated to
  match implemented behavior.
- [x] `scripts/verify-library-crossref-demo.sh` created and passes
  standalone.
- [x] `Makefile` gained `example-library-crossref`, wired into `verify` and
  `.PHONY`.
- [x] `.github/workflows/ci.yml` updated to run the new demo step,
  preserving every existing Phase 1–5 CI step unchanged.
- [x] `git diff --check`, `gofmt -l .`, `go vet ./...`, `go test ./...
  -race -count=1`, `go build`, and every one of `scripts/verify-golden-fingerprint.sh`,
  `scripts/verify-evidence-demo.sh`, `scripts/verify-library-demo.sh`,
  `scripts/verify-scenario-demo.sh`, `scripts/verify-suite-demo.sh`,
  `scripts/verify-library-crossref-demo.sh` passed locally (exit 0
  throughout, §11).
- [x] No `.incidentdna` directory or generated evidence/library-store
  artifacts remain anywhere in the repository.
- [x] `examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
  `examples/incident-library-demo/`, `examples/regression-scenario-demo/`,
  and `examples/regression-suite-demo/` confirmed byte-for-byte unchanged.
- [x] `internal/library.CheckFingerprint` never writes, never creates the
  library root, and is unreachable from `scenario run`/`suite run` —
  proven by inspection and by every test passing.
- [x] The new demo script contains no credentials, personal information,
  customer data, or real incident material — composed entirely from
  already-reviewed, pre-existing synthetic fixtures.
- [x] Diff inspected for security regressions, incorrect documentation
  claims, incomplete CLI wiring, unsafe filesystem behavior, test gaps,
  accidental Phase 7 scope, sensitive/real data, and generated artifacts —
  none found.
- [x] One deliberate judgment call, where the approved plan's illustrative
  transcript did not match the real, checked-in fixture data, is documented
  explicitly rather than silently resolved: `suite-mixed.yaml`'s three
  listed scenarios share one fingerprint, not three distinct ones as
  `docs/phase-6-plan.md` §4 Workflow B's transcript illustrates (§8).
- [ ] Hosted GitHub Actions run — **not yet confirmed** (requires pushing
  this branch, not done as part of this task).
- [ ] Commit / push / merge — **not done**, per explicit instruction for
  this task.

No production adoption, deployment, customer usage, or performance
benchmarking is claimed anywhere in this report or in Phase 6 generally.
