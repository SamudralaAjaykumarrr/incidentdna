# Phase 6 Plan: Library Cross-Reference (Draft for Approval)

**Status: proposed for review. No implementation has started.** No line of
Go code, no schema, no example, and no test has been written against this
plan; nothing in this document has been merged into `Makefile`, CI, or any
existing package. It follows the same role `docs/phase-4-plan.md` and
`docs/phase-5-plan.md` played before their respective implementations began
— a settled design to implement against, not an implementer's inference —
and has not yet been through a decisions round-trip with the user. Everything
below is proposed, pending that review.

## 0. Why this candidate, and not another

This plan was written after a full inventory of every future-work reference
in the repository (`docs/phase-4-plan.md` §6/§28, `docs/phase-5-plan.md`
§3/§25/§26, `docs/phase-5-report.md` §16, `docs/product-scope.md`
"Explicitly out of scope for Phase 1 through Phase 5," and `README.md`'s
"Future work" list). Six candidates are named across those documents:

1. **Read-only cross-reference between a scenario's/suite's
   `linked_fingerprint` and the incident library's stored occurrences.**
2. Release-gate / CI-CD integration for `scenario run`/`suite run`.
3. Sandboxed (seccomp/cgroups/container/VM) scenario execution.
4. Remote/cloud storage for the evidence store, incident library, scenarios,
   or suites.
5. Evidence/library signing or authenticity proof.
6. Ingestion from observability/event systems.
7. Parallel suite execution; directory/glob-based scenario discovery.

Only candidate 1 is described, consistently and repeatedly, as a **narrow,
already-decomposed, read-only composition of pieces that already exist and
would need no change to reuse**:

- `docs/phase-4-plan.md` §6: *"A future phase that wants 'does this
  scenario's fingerprint have library occurrences' is a pure read-only
  composition of two already-existing pieces (`library.Check` + a scenario's
  `linked_fingerprint` field) and needs no change to either package —
  named explicitly as a future integration point, not built now (§28)."*
- `docs/phase-4-plan.md` §28: names it a second time as *"the natural next
  integration point."*
- `docs/phase-5-plan.md` §25/§26: carries the same item forward, unbuilt,
  explicitly distinguishing it from parallel execution and release-gate
  integration, which are named as separate, larger, still-unbuilt items.
- `docs/phase-5-report.md` §16: restates it as the first item in Phase 6's
  boundary list.
- `docs/product-scope.md`, "Explicitly out of scope for Phase 1 through
  Phase 5": states plainly *"neither `internal/scenario` nor `internal/suite`
  imports or queries `internal/library`"* — phrased as a fact about today,
  not a permanent prohibition.

Every other candidate (2, 3, 4, 5, 6, 7) is named as a **larger, separate,
higher-risk increment** — a new trust boundary (signing), a new execution
model (sandboxing, parallelism), a new transport (remote storage,
observability ingestion), or a policy decision with consequences outside
this codebase (release gating). None of them is described anywhere as ready
to build with "no change to either package." Building any of them now would
mean *inventing* scope rather than *closing a boundary this repository's own
documents have already drawn and repeatedly pointed at*.

**Conclusion: candidate 1, "read-only library cross-reference," is the one
coherent Phase 6 increment the repository's own history supports.** The rest
of this document is its concrete design.

## 1. Problem statement

Phases 1–3 built a complete *memory* system: incidents can be represented,
validated, fingerprinted, and accumulated in a local library grouped by
failure-class fingerprint. Phases 4–5 built a complete, bounded, offline
*execution* system on top: a single scenario can be run and classified
(Phase 4), and a whole manifest of scenarios can be run and aggregated
(Phase 5). These two systems have been kept deliberately, explicitly
disconnected: a scenario declares a `linked_fingerprint` — the failure class
it regression-tests — but that declaration has never been checked against
what the library actually holds. Today, an operator who wants to know "does
this scenario (or this whole suite) correspond to a failure class this team
has actually recorded as an incident, or is `linked_fingerprint` a typo/stale
value?" has no way to ask `incidentdna` that question. They would have to run
`incidentdna library check` by hand against some incident file and compare
the printed fingerprint to the scenario's `linked_fingerprint` themselves.

Phase 6 closes exactly this gap, and nothing else: it lets `scenario verify`
and `suite verify` optionally report, for each linked fingerprint they
already know about, whether the incident library holds one or more
occurrences under it — a pure, read-only lookup, with no effect on whether a
scenario or suite is considered valid, and no effect on `scenario run` or
`suite run` at all.

## 2. Goals

1. Add an optional `--library <dir>` flag to `incidentdna scenario verify`,
   mirroring the existing `--source` flag's shape: when given, after the
   existing structural/semantic checks pass, additionally look up the
   scenario's own `linked_fingerprint` against the named (or default)
   incident library and print whether it has recorded occurrences.
2. Add an optional `--library <dir>` flag to `incidentdna suite verify`: when
   given, after the existing per-listed-scenario checks pass, look up every
   *distinct* `linked_fingerprint` among the listed scenarios (in
   first-occurrence order) and annotate each scenario's own summary line
   with its lookup result.
3. Implement this as **a pure, read-only composition of already-existing
   pieces**, exactly as `docs/phase-4-plan.md` §6 described it: one small,
   additive, exported function in `internal/library` (§5), reused unchanged
   by both `scenario verify` and `suite verify` at the `cmd/incidentdna`
   layer. `internal/scenario` and `internal/suite` are not modified at all —
   neither package gains an import of `internal/library`, and neither
   package's `Document`, `Validate`, or `Run` changes in any way.
4. Keep this **strictly informational**: a "no occurrences found" result
   never changes `scenario verify`'s or `suite verify`'s existing 0/2
   exit-code meaning (valid/invalid), and `scenario run`/`suite run` are not
   touched at all — no new exit code, no new JSON report field, no new
   behavior when `--library` is omitted.
5. Introduce **no new class of risk**: the lookup is read-only local
   filesystem I/O against the exact same library store `library check`
   already reads, under the exact same trust boundary. No new write path is
   introduced anywhere by this phase.
6. Make the whole feature **testable without any external service**: no
   Docker daemon, no network — `go test ./internal/library/... -race
   -count=1` and the two CLI test files remain fully self-contained, the
   same discipline every prior phase's test suite already established.

## 3. Explicit non-goals

- **No release gating.** A "no occurrences found" result is printed, never
  enforced. Neither `scenario verify --library` nor `suite verify --library`
  fails (exit 2, or any exit code) because a fingerprint has no library
  occurrences — that would turn an informational lookup into a policy
  decision this codebase does not make (`docs/product-scope.md`, restated
  unconditionally again here).
- **No change to `scenario run` or `suite run`.** Cross-referencing is
  available only through the two `verify` subcommands, which already
  established the "inspect without executing" role every prior phase relies
  on (`validate`/`evidence verify`/`library check`/`scenario verify`/`suite
  verify`). `scenario run`'s and `suite run`'s exit codes, stdout format, and
  JSON report schemas (`irs-report/v0.1`, `suite-report/v0.1`) are
  byte-for-byte unchanged.
- **No new document field, anywhere.** `idir.Document`, `scenario.Document`,
  and `suite.Document` are unchanged Go types — Phase 6 adds no
  `library_fingerprint` field, no `checked_against_library` field, nothing.
  The lookup key is the already-existing `scenario.Document.LinkedFingerprint`
  field, read as-is.
- **No mutation of the library.** The only library operation Phase 6 ever
  performs is a lookup; `library.Add` is never called by any code path this
  phase introduces. A scenario or suite author who wants a fingerprint to
  show up as "found" must still run `incidentdna library add` themselves,
  exactly as today.
- **No coupling built into `internal/scenario` or `internal/suite`
  themselves.** Both packages remain true parallel leaf packages, unchanged
  — `docs/architecture.md`'s dependency-direction statement
  ("`internal/scenario` ... is not imported by ... `library`; `internal/suite`
  ... is not imported by ... `library`") continues to hold exactly as
  written. The composition lives entirely in `cmd/incidentdna`, which already
  imports all three packages today.
- **No new resource limits beyond what already exists.** A suite's listed-
  scenario count is already bounded by `suite.MaxScenariosPerSuite` (100);
  that bound already caps how many distinct fingerprints one `suite verify
  --library` call can look up. No new named constant is introduced.
- **No JSON output for the cross-reference result.** The lookup result is
  printed to stdout as part of the existing human-readable `verify` summary
  only — `scenario verify`/`suite verify` do not gain a `--report` flag or
  any machine-readable output mode in this phase (neither has one today).
- **No batch/multi-fingerprint library query command.** `internal/library`
  gains exactly one new function (§5), reusing `Check`'s existing internal
  lookup logic; it does not grow a new "check many fingerprints in one call"
  API surface — `cmd/incidentdna` loops over distinct fingerprints itself,
  the same way `suite.Validate` already loops over listed scenarios calling
  `scenario.Validate` once each.
- **No signing, no encryption, no multi-tenancy, no network, no
  telemetry** — the same standing invariants every prior phase restates.
- **No sandboxing, no parallel execution, no directory/glob discovery, no
  remote storage** — all remain exactly as out of scope as
  `docs/product-scope.md` already states; Phase 6 does not touch any of
  them.

## 4. User workflows

**Workflow A — check whether a scenario's linked fingerprint is a recorded
failure class:**

```
$ incidentdna scenario verify --library .incidentdna/library/objects \
    examples/regression-scenario-demo/scenario-pass.yaml
Scenario: duplicate-payment-refingerprint (schema irs/v0.1)
Linked fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
Library: 1 occurrence(s) found for this fingerprint
OK: scenario is structurally valid
```

or, against a library that has never recorded this fingerprint:

```
$ incidentdna scenario verify --library .incidentdna/library/objects \
    examples/regression-scenario-demo/scenario-timeout.yaml
Scenario: duplicate-payment-timeout (schema irs/v0.1)
Linked fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
Library: no occurrences found for this fingerprint (not yet recorded in the library)
OK: scenario is structurally valid
```

Exit code `0` in both cases — the scenario is structurally valid either way;
the `Library:` line is informational only. `--library` can be combined with
the existing `--source` flag; both cross-checks run independently and both
are printed.

**Workflow B — see, per listed scenario, which of a suite's failure classes
are actually recorded incidents:**

```
$ incidentdna suite verify --library .incidentdna/library/objects \
    examples/regression-suite-demo/suite-mixed.yaml
Suite: suite-demo-mixed (schema suite/v0.1)
Scenarios: 3 listed
  [OK] suite-demo-pass (scenarios/scenario-pass.yaml) — library: 1 occurrence(s)
  [OK] suite-demo-fail (scenarios/scenario-fail.yaml) — library: 1 occurrence(s)
  [OK] suite-demo-timeout (scenarios/scenario-timeout.yaml) — library: no occurrences
Library cross-reference: 2 of 3 distinct linked fingerprint(s) have library occurrences
OK: suite manifest and all 3 listed scenarios are structurally valid
```

Two listed scenarios sharing the same `linked_fingerprint` are looked up
once, not twice — the summary counts **distinct** fingerprints, and each
scenario's own line still shows its (identical, cached) result.

**Workflow C — `--library` omitted (default, unchanged behavior):**

```
$ incidentdna scenario verify examples/regression-scenario-demo/scenario-pass.yaml
Scenario: duplicate-payment-refingerprint (schema irs/v0.1)
Linked fingerprint: sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
OK: scenario is structurally valid
```

Byte-for-byte identical to today's output — no `Library:` line appears
unless `--library` is explicitly passed. This is true for `suite verify`
too.

**Workflow D — the library at the given/default path has never been
created:** treated as "no occurrences found," never an error, mirroring
`library.Open`'s existing "not-yet-created is not an error" stance
(`docs/incident-library.md`) — a scenario/suite author working in a fresh
checkout that has never run `library add` gets a normal, non-error "no
occurrences found" line, not a crash.

**Workflow E — the library exists but is malformed** (corrupted
`index.json`, an unexpected shard/fingerprint directory shape): `scenario
verify --library`/`suite verify --library` exit `1` — an environmental
failure, the same class `library check` already reports for a malformed
library, distinct from and never conflated with "the scenario itself is
invalid" (exit `2`).

## 5. The one library-package change: `library.CheckFingerprint`

`internal/library.Check` (existing, unchanged in its own public behavior)
takes a `*idir.Document`: it validates the document, computes its
fingerprint via `fingerprint.Compute`, then looks up that fingerprint in the
store. `scenario.Document` and `suite.ScenarioEntry` never hold a full
`idir.Document` — a scenario only ever holds `LinkedFingerprint`, a plain
`sha256:`-prefixed string, declared by the scenario's author and format-
checked at `scenario.Validate` time, never re-derived from an incident file
unless `--source` is also given. Phase 6 therefore needs a lookup entry
point that takes a **fingerprint string directly**, not a document.

The implementation is a small refactor of `internal/library/check.go`,
extracting the part of `Check` that runs *after* fingerprint computation
into an unexported helper, and adding one new exported function that calls
that same helper after validating the string is fingerprint-shaped:

```go
// CheckFingerprint reports whether the library holds one or more intact
// occurrences under fp directly, without requiring a full idir.Document to
// compute it from. fp must already be a well-formed "sha256:"-prefixed,
// 64-lowercase-hex fingerprint string — the same format
// internal/fingerprint.Compute produces and Check's own internal validation
// already enforces once it has computed one from a document.
//
// This is the read-only lookup entry point Phase 6's scenario/suite
// cross-reference uses: a scenario or suite manifest declares a
// linked_fingerprint directly, with no idir.Document available to recompute
// it from, so Check (which requires one) cannot be reused as-is.
// CheckFingerprint and Check share every line of lookup logic after
// fingerprint computation — only how the fingerprint is obtained differs.
func CheckFingerprint(ctx context.Context, s *Store, fp string) (CheckResult, error) {
	if err := ctx.Err(); err != nil {
		return CheckResult{}, fmt.Errorf("library: check canceled: %w", err)
	}
	if _, err := parseFingerprint(fp); err != nil {
		return CheckResult{}, fmt.Errorf("library: %w: %v", ErrInvalidFingerprint, err)
	}
	return checkFingerprint(ctx, s, fp)
}
```

`Check` itself becomes a two-line wrapper: validate the document, compute
`fp := fingerprint.Compute(doc)`, then `return checkFingerprint(ctx, s, fp)`
— identical behavior, identical signature, identical error types, proven by
the existing `check_test.go` suite passing completely unmodified. One new
sentinel, `ErrInvalidFingerprint`, is added to `errors.go` alongside the
existing five (§ `internal/library/errors.go`) for the one new failure mode
`Check` could never previously reach (a syntactically malformed fingerprint
string) — `Check` can never hit this path itself, since
`fingerprint.Compute` always produces a well-formed string, so this is a
new, additive error case, not a change to any existing one.

**This is the only change to `internal/library`'s existing files.** No
existing exported name, signature, or documented behavior changes.
`add.go`, `list.go`, `privacy.go`, `store.go`, `index.go`, `limits.go` are
untouched.

## 6. Relationship between the artifacts (updated)

```
incident document (idir.Document)
   │  idir.LoadFile + validate.Validate            (unchanged, Phase 1)
   ▼
valid idir.Document
   │  fingerprint.Compute                          (unchanged, Phase 1)
   ▼
failure fingerprint  ──────────────────────────────────────┐
   │  library.Add (optional, independent)                  │  scenario.LinkedFingerprint /
   ▼                                                        │  suite scenario entries' own
incident-library occurrence(s)                              │  LinkedFingerprint (declared,
   (Phase 3, grouped by fingerprint,                        │  format-checked always)
    zero or more per fingerprint)                           │
        ▲                                                   │
        │  library.CheckFingerprint (Phase 6, read-only,    │
        │  NEW — reuses Check's existing lookup logic)      │
        └──────────────────────────────────────────────────┘
                    read only — never writes, never blocks
```

The dashed line `docs/phase-4-plan.md` §6 explicitly deferred — "a future
phase that wants 'does this scenario's fingerprint have library
occurrences' ... needs no change to either package" — is exactly the line
Phase 6 draws. `internal/scenario` and `internal/suite` are unaware this
line exists; the composition happens one layer up, in `cmd/incidentdna`,
which already imports all three leaf packages today (`cmd_scenario.go`
already imports `internal/scenario`; `cmd_suite.go` already imports
`internal/suite`; `cmd_library.go` already imports `internal/library`).
Phase 6 adds `internal/library` as a new import to `cmd_scenario.go` and
`cmd_suite.go` — the first time either file imports it — but neither
underlying package gains a new dependency on the other.

## 7. Determinism requirements

1. **A cross-reference result depends on the library's on-disk state at the
   moment of the call**, not just on the scenario/suite's own bytes — this
   is a genuinely different determinism statement from everything Phase 6
   builds on top of. `scenario verify`'s and `suite verify`'s own
   structural/semantic validity continues to depend only on the scenario/
   suite's own bytes (and, for `--source`, the named incident file's bytes),
   exactly as before; the *added* `Library:` line can legitimately differ
   between two otherwise-identical invocations if an intervening
   `library add` changed the library's contents. This mirrors
   `library check`'s own existing, already-documented non-claim — a
   library's contents are mutable local state, and no command in this
   codebase claims otherwise.
2. **Given a fixed library state, the result is deterministic and
   repeatable** — `library.CheckFingerprint` performs the identical
   integrity verification `Check` already performs (re-hash every indexed
   occurrence before counting it), so a corrupted occurrence is never
   silently reported as a match, exactly as today.
3. **Suite-level lookup order is exactly the declared scenario order**,
   deduplicated to first occurrence: the first listed scenario naming a
   given `linked_fingerprint` triggers the lookup; every later scenario
   naming the same fingerprint reuses that result rather than querying the
   store again. This bounds `suite verify --library`'s library I/O to at
   most one lookup per *distinct* fingerprint, never per scenario entry.
4. **`--library` omitted is byte-for-byte identical to today's output** —
   no code path this phase adds runs unless the flag is explicitly given,
   for both `scenario verify` and `suite verify`.

## 8. Safety model

Phase 6 introduces **no new class of risk**. It is read-only local
filesystem I/O, reusing `library.Open`'s and `library.Check`'s already-
reviewed path-resolution, symlink-rejection, and occurrence-integrity-
verification logic exactly as `library check` already exercises them today
— `library.CheckFingerprint` performs the same reads `Check` does, just
keyed by a caller-supplied fingerprint string instead of one computed from a
document. No new write path exists anywhere in this phase: `scenario verify
--library` and `suite verify --library` never call `library.Add`, never
create the library root if it doesn't exist, and never modify any file
`library add`/`library check`/`library list` could also touch.

The one new consideration, stated plainly rather than implied away: running
`scenario verify --library`/`suite verify --library` reveals, to whoever
runs the command, whether *any* occurrence exists under a given fingerprint
— but this is exactly the same information `library check` already reveals
today for a fingerprint computed from a document the caller already has
locally; Phase 6 adds no new information-disclosure surface, only a second,
more convenient way to ask the same already-answerable question when the
caller already has the fingerprint value (from a scenario or suite file)
rather than a full incident document.

## 9. CLI commands

```
incidentdna scenario verify [--source <incident-file>] [--library <dir>] <scenario-file>
    Unchanged existing behavior (structural/semantic validation, optional
    --source fingerprint-linkage cross-check), plus: if --library is given,
    after the above checks pass, look up the scenario's own
    linked_fingerprint against the named (or default) incident library and
    print whether it has one or more recorded occurrences. Never executes
    anything. The Library: line never changes the command's own exit code.

incidentdna suite verify [--library <dir>] <suite-file>
    Unchanged existing behavior (suite manifest + every listed scenario's
    structural/semantic validation), plus: if --library is given, after the
    above checks pass, look up every distinct linked_fingerprint among the
    listed scenarios (first-occurrence order) against the named (or
    default) incident library, annotate each scenario's own summary line,
    and print a one-line aggregate count. Never executes anything. The
    Library: annotations never change the command's own exit code.
```

`--library <path>` mirrors the exact flag name, default-value handling
(empty string → `library.DefaultLibraryRoot`), and help text style the three
`incidentdna library` subcommands already use (`cmd/incidentdna/cmd_library.go`'s
`libraryFlag` helper is reused unchanged, not reimplemented, by
`cmd_scenario.go` and `cmd_suite.go`). `scenario run`, `suite run`, `init`,
`validate`, `fingerprint`, `inspect`, `compare`, all four `evidence`
subcommands, and `library add`/`check`/`list` remain byte-for-byte unchanged
in behavior.

## 10. Exit-code contract

**`scenario verify`** (extends, does not replace, the existing 0/1/2 split
from `docs/regression-scenarios.md`):

| Exit | Meaning |
|---|---|
| `0` | Scenario is structurally/semantically valid (and, if `--source` given, matches; the `--library` lookup result, if requested, is printed but never affects this) |
| `1` | I/O/parse/usage error — unchanged causes, **plus**: `--library` given and the library exists but is malformed, or cannot be read for a permission/environmental reason |
| `2` | Scenario decodes but fails a semantic IRS v0.1 rule, or (with `--source`) fingerprint mismatch — unchanged; a `--library` "no occurrences found" result never produces this |

**`suite verify`** (extends, does not replace, the existing 0/1/2 split from
`docs/scenario-suites.md`), identically: `0`/`2` unchanged in every existing
cause; `1` gains the same new cause (a malformed/unreadable library named by
`--library`).

**`scenario run` and `suite run`**: **entirely unchanged** — no new flag, no
new exit-code cause, no new report field. This is a deliberate, explicit
restatement, not an oversight: cross-referencing is a `verify`-only
capability in Phase 6.

## 11. Corruption and malformed-input handling

- **`--library` omitted**: no new behavior of any kind; every existing
  corruption/malformed-input case for `scenario verify`/`suite verify`
  (`docs/regression-scenarios.md`/`docs/scenario-suites.md`, "Corruption and
  malformed-...-handling") is completely unchanged.
- **`--library` given, library root does not exist yet**: `library.Open`'s
  existing "not yet created is not an error" behavior applies; the lookup
  proceeds and reports "no occurrences found," exit code unaffected by this
  alone.
- **`--library` given, library root exists but its layout is malformed**
  (unexpected shard/fingerprint directory shape, corrupted `index.json` for
  the specific fingerprint being looked up): `library.CheckFingerprint`
  returns the same `ErrMalformedLibrary`/`ErrMalformedIndex`/
  `ErrCorruptedOccurrence` family `Check` already returns today; `scenario
  verify`/`suite verify` map this to exit `1`, printed to stderr, exactly
  the same "environmental/usage failure" class every other exit-`1` cause
  in this codebase already belongs to.
- **`--library` given, `--source` also given (scenario verify only)**: both
  checks run independently; either can independently fail (exit `2` for a
  `--source` mismatch, exit `1` for a `--library` I/O failure) — the more
  severe applicable exit code wins using the existing project-wide
  precedence (structural/semantic failures and `--source` mismatches, exit
  `2`, are checked and would already have returned before the `--library`
  lookup ever runs, since `--library`'s lookup only runs "after the above
  checks pass," per §9).
- **A suite's listed scenarios include one whose `LinkedFingerprint` is
  identical to another's**: not an error (suites already permit this — only
  duplicate `path` values are rejected); the fingerprint is looked up once
  and the cached result is shown on every scenario line that shares it (§7
  point 3).

## 12. Privacy implications

Identical in kind to every prior phase's own statement, restated rather than
newly invented:

- **No new document field, no new persisted data.** Phase 6 introduces no
  privacy/redaction-relevant field anywhere; it reads an already-declared,
  already-format-checked `linked_fingerprint` value and an already-existing
  library's already-stored occurrence metadata (fingerprint + occurrence
  count only — never a stored occurrence's full content, exactly as
  `library check` already limits itself to today).
- **No new content is ever printed.** The `Library:` line prints only a
  fingerprint (already visible in the scenario/suite's own `Linked
  fingerprint:` line) and an occurrence count — never an occurrence's
  incident id, title, service, or any other stored field. `library list`
  remains the only command in this codebase that prints per-occurrence
  metadata.
- **No network access means no telemetry, no external transmission** —
  restating the unconditional project invariant, extended to the one new
  `cmd/incidentdna` → `internal/library` call path this phase adds.

## 13. Threat model additions

New subsection proposed for `docs/threat-model.md`, "Library cross-reference:
read-only lookup risk (Phase 6)," stating plainly that Phase 6 introduces no
new category of risk beyond what the existing "Incident library: local
filesystem risks (Phase 3)" section already documents for `library check`:
every filesystem read `library.CheckFingerprint` performs is the identical
read `library.Check` already performs (symlink rejection, occurrence
re-hash verification, `checkContained` path safety) — only the fingerprint's
origin differs (caller-supplied string vs. computed from a document). The
one genuinely new consideration, restated from §8: `scenario verify
--library`/`suite verify --library` let a caller ask "does the library
record this fingerprint" using only a fingerprint string they already have
(from a scenario or suite file) rather than a full incident document — a
strictly more convenient path to information `library check` already made
obtainable, not a new disclosure.

## 14. Backward compatibility with Phases 1–5

- `internal/idir`, `internal/validate`, `internal/canonical`,
  `internal/fingerprint`, `internal/compare`, `internal/evidence`,
  `internal/scenario`, `internal/suite` are **fully untouched** — no file
  under any of these packages appears in the implementation diff.
- `internal/library`: the **only** package with a code change, and the only
  change is additive (§5) — one new exported function
  (`CheckFingerprint`), one new sentinel error (`ErrInvalidFingerprint`),
  and an internal (unexported) refactor of `Check`'s own body that changes
  none of `Check`'s existing exported behavior. Proven by the existing
  `internal/library` test suite passing with zero modification to any
  existing test's expectations.
- `examples/duplicate-payment/`, `examples/evidence-storage-demo/`,
  `examples/incident-library-demo/`, `examples/regression-scenario-demo/`,
  and `examples/regression-suite-demo/` are **not modified** — the new demo
  (§18) composes copies/temporary invocations against existing, unmodified
  fixtures from `examples/duplicate-payment/` and
  `examples/regression-scenario-demo/`/`examples/regression-suite-demo/`,
  the same "reference, never edit" discipline every prior phase's demo
  followed.
- Every existing CLI subcommand's exit-code and output behavior is
  unchanged when Phase 6's new flags are omitted — proven the same way every
  prior phase proved it: the full existing test suite passes unmodified.
- No `schema_version` bump anywhere — Phase 6 defines no new document
  format and touches no existing one.
- A scenario or suite file authored before Phase 6 existed works with
  `--library` exactly as-is — no re-authoring required, since the lookup key
  (`linked_fingerprint`) already existed in both formats since Phase 4/5.

## 15. Implementation slices in exact order

Following the exact "each slice independently mergeable and testable"
discipline `docs/phase-4-plan.md` §21 and `docs/phase-5-plan.md` §21
established:

1. `internal/library/check.go` + `check_test.go` — extract the shared
   `checkFingerprint` helper, add `CheckFingerprint`, add
   `ErrInvalidFingerprint` to `errors.go`. New tests: malformed-fingerprint
   rejection; match/no-match via `CheckFingerprint` directly against a
   `t.TempDir()` library seeded by a direct `Add` call; a parity test
   asserting `Check(doc)` and `CheckFingerprint(fingerprint.Compute(doc))`
   return identical `CheckResult` values for the same document and library
   state; existing `Check`-focused tests in `check_test.go` must continue to
   pass unmodified.
2. `cmd/incidentdna/cmd_scenario.go` + tests — add `--library` flag to
   `runScenarioVerify`, wire `openLibraryStore`/`library.CheckFingerprint`
   (reusing `cmd_library.go`'s existing `libraryFlag`/`openLibraryStore`
   helpers unchanged), print the `Library:` line, map library errors to
   exit `1`. No change to `runScenarioRun`.
3. `cmd/incidentdna/cmd_suite.go` + tests — add `--library` flag to
   `runSuiteVerify`, dedupe listed scenarios' `LinkedFingerprint` values in
   first-occurrence order, look each up once, annotate per-scenario lines,
   print the aggregate count line, map library errors to exit `1`. No
   change to `runSuiteRun`.
4. `cmd/incidentdna/cli_scenario_test.go` + `cli_suite_test.go` — black-box
   tests against the real compiled binary: `--library` match, no-match,
   not-yet-created library, malformed library (exit `1`), `--library`
   combined with `--source`, `--library` omitted producing byte-identical
   output to the pre-Phase-6 baseline, suite-level dedup (two scenarios
   sharing one fingerprint trigger exactly one lookup — assertable via a
   library fixture that would error on a second lookup, e.g. a
   `t.TempDir()`-based library whose index is deliberately made to fail on
   a second read attempt, or more simply by asserting on printed output
   only, per implementer's judgment during this slice).
5. `docs/library-crossref.md` — new dedicated design doc, the Phase 6 analog
   of `docs/regression-scenarios.md`/`docs/scenario-suites.md`; updates to
   `README.md`, `docs/architecture.md`, `docs/product-scope.md`,
   `docs/threat-model.md`, `docs/privacy-model.md`,
   `docs/regression-scenarios.md` (new "Library cross-reference" subsection
   plus updating its "What Phase 4 explicitly does not provide" list),
   `docs/scenario-suites.md` (same treatment) — done after the code so they
   describe actual behavior, not the plan.
6. `scripts/verify-library-crossref-demo.sh` + `Makefile` (`example-library-crossref`
   target, wired into `verify`) + `.github/workflows/ci.yml` (new step) —
   see §18 for the demo's exact composition.
7. `docs/phase-6-report.md` — written last, after real command transcripts
   exist to record, mirroring `docs/phase-5-report.md`'s role.

Each slice through 4 should independently pass `go test ./... -race
-count=1`; slices 5–6 should independently pass `make verify` once slice 6
lands.

## 16. Files expected to be added or modified

**New:**

```
docs/
  library-crossref.md
  phase-6-report.md             (written last, per §15 slice 7)

scripts/
  verify-library-crossref-demo.sh
```

**Modified:**

```
internal/library/
  check.go                       — extract checkFingerprint helper, add CheckFingerprint
  errors.go                      — add ErrInvalidFingerprint
  check_test.go                  — new tests per §15 slice 1

cmd/incidentdna/
  cmd_scenario.go                — add --library flag/wiring to runScenarioVerify only
  cmd_suite.go                   — add --library flag/wiring to runSuiteVerify only
  cli_scenario_test.go           — new --library test cases
  cli_suite_test.go              — new --library test cases

README.md                        — CLI commands list, roadmap, known-limitations
docs/architecture.md             — dependency-direction paragraph, new "Library cross-reference" data-flow note
docs/product-scope.md            — new "Phase 6 scope" section; out-of-scope list updated
docs/threat-model.md             — new "Library cross-reference" section (§13)
docs/privacy-model.md            — new subsection (§12)
docs/regression-scenarios.md     — new subsection documenting scenario verify --library; "does not provide" list updated
docs/scenario-suites.md          — new subsection documenting suite verify --library; "does not provide" list updated
Makefile                         — example-library-crossref target, wired into verify
.github/workflows/ci.yml         — new "Library cross-reference demo validation" step
.gitignore                       — no new entry expected; confirmed during implementation
```

**Untouched (explicit):** `internal/idir/`, `internal/validate/`,
`internal/canonical/`, `internal/fingerprint/`, `internal/compare/`,
`internal/evidence/`, `internal/scenario/`, `internal/suite/`,
`internal/library/add.go`, `internal/library/list.go`,
`internal/library/privacy.go`, `internal/library/store.go`,
`internal/library/index.go`, `internal/library/limits.go`,
`cmd/incidentdna/cmd_library.go`, `cmd/incidentdna/main.go` (no new
top-level command is added — `--library` is a flag on two existing
subcommands, not a new command; `main.go`'s `commands` table and its
`"scenario"`/`"suite"` context-timeout special-case are unaffected, since
`scenario verify`/`suite verify` already run under the standard
30-second command context, unchanged), all five `examples/` directories,
`testdata/golden/`.

## 17. Unit, integration, CLI, race, and boundary tests

Following the exact table-driven, real-filesystem (`t.TempDir()`),
no-mocking style every prior phase established:

- **Unit — `internal/library/check_test.go`**: malformed-fingerprint-string
  rejection via `CheckFingerprint` (empty string, wrong prefix, wrong
  length, uppercase hex); match against a library seeded with one and with
  multiple occurrences under the same fingerprint; no-match against an
  empty (but existing) library and against a not-yet-created library root;
  corrupted-occurrence propagation (`ErrCorruptedOccurrence`) identical to
  `Check`'s own existing corrupted-occurrence test; malformed-library
  propagation (`ErrMalformedLibrary`/`ErrMalformedIndex`) identical to
  `Check`'s/`List`'s own existing tests; a direct parity test asserting
  `Check(ctx, s, doc)` and `CheckFingerprint(ctx, s,
  fingerprint.Compute(doc))` return identical `CheckResult` values, for
  both a match and a no-match library state; context cancellation
  (`ctx.Err()` checked before any I/O, matching `Check`'s existing
  behavior).
- **CLI — `cmd/incidentdna/cli_scenario_test.go`** (black-box against the
  real compiled binary): `scenario verify --library` match (exit `0`, line
  present); no-match (exit `0`, "no occurrences found" line, **not** exit
  `2`); library not yet created (exit `0`, same "no occurrences found"
  treatment); malformed library (exit `1`, distinct error message); combined
  with `--source` (both lines present, independent pass/fail); `--library`
  omitted produces stdout byte-identical to the pre-Phase-6 baseline
  (regression assertion against the exact string every existing
  `TestCLI_ScenarioVerify*` test already asserts).
- **CLI — `cmd/incidentdna/cli_suite_test.go`** (same style): `suite verify
  --library` with all-match, all-no-match, and mixed listed scenarios;
  two listed scenarios sharing one `linked_fingerprint` (assert the
  aggregate "N of M distinct" count reflects deduplication, not the raw
  listed-scenario count); malformed library (exit `1`, nothing about the
  existing per-scenario structural validation output changes); `--library`
  omitted produces stdout byte-identical to the pre-Phase-6 baseline.
- **Race** — `go test ./... -race -count=1` must stay green with the
  modified `internal/library` and the two modified `cmd/incidentdna` files
  included.
- **Regression discipline**: the full existing suite (every currently
  passing top-level test across `cmd/incidentdna` and all 8 `internal/*`
  packages) must pass with zero modification to any existing test file's
  expectations; the golden fingerprint test/script must produce the
  unchanged Phase 1 value; `git diff --stat -- internal/idir
  internal/validate internal/canonical internal/fingerprint internal/compare
  internal/evidence internal/scenario internal/suite` must be empty.

## 18. Runnable demo requirements

`scripts/verify-library-crossref-demo.sh` — a new script, not a new example
directory, since Phase 6 introduces no new document format and no new
fixture content: it demonstrates the feature by **composing already-
existing, unmodified fixtures** from `examples/duplicate-payment/` and
`examples/regression-scenario-demo/`/`examples/regression-suite-demo/`, the
same "pure composition of already-existing pieces" the feature itself
embodies (§0, §6). Concretely, in a temporary directory (never touching a
real `.incidentdna/` or any checked-in file):

1. **Match** — `library add examples/duplicate-payment/incident.yaml` into a
   fresh temporary `--library` directory, then `scenario verify --library
   <temp> examples/regression-scenario-demo/scenario-pass.yaml` — this
   scenario's checked-in `linked_fingerprint` is already the duplicate-
   payment example's own golden fingerprint (`docs/regression-scenarios.md`,
   §4 Workflow A), so no new fixture value needs to be invented; asserts the
   `Library: 1 occurrence(s) found` line and exit `0`.
2. **No match** — the same lookup against a **freshly created, empty**
   temporary library (no `library add` run against it yet); asserts the
   `Library: no occurrences found` line and exit `0` — never exit `2`.
3. **Not-yet-created library** — the same lookup against a `--library` path
   that does not exist on disk at all; asserts identical "no occurrences
   found" treatment and exit `0`, distinguishing this from a genuinely
   malformed library.
4. **Suite-level, mixed** — seed the same temporary library with
   `examples/duplicate-payment/incident.yaml`, then `suite verify --library
   <temp> examples/regression-suite-demo/suite-mixed.yaml` (whose listed
   `scenarios/scenario-pass.yaml` shares the same `linked_fingerprint`);
   asserts the per-scenario annotation and the aggregate "N of M distinct"
   summary line.
5. **Malformed library** — corrupt a fingerprint directory's `index.json`
   inside a temporary library the script controls, then assert `scenario
   verify --library <temp>` exits `1` with a clear, actionable error.

No real credentials, personal information, or customer data anywhere —
every fixture this demo touches is a pre-existing, already-reviewed
synthetic example. `make example-library-crossref` builds the binary if
needed, then runs this script; `make verify`'s dependency chain gains it,
positioned after `example-suite` and before the golden-fingerprint check,
mirroring exactly how every prior phase's demo step was inserted.

## 19. Known limitations (anticipated up front)

- **Point-in-time only.** A cross-reference result reflects the library's
  contents at the moment `scenario verify --library`/`suite verify
  --library` runs; it is not re-checked by `scenario run`/`suite run`, and
  nothing in this codebase re-validates it later (§7).
- **No release gating**, restated as unconditional (§3).
- **No coupling inside `internal/scenario`/`internal/suite`** — both remain
  unaware the incident library exists; the composition is entirely a
  `cmd/incidentdna`-layer concern (§6).
- **Occurrence count only, not occurrence content.** The `Library:` line
  never reveals which specific incident(s) are recorded under a fingerprint
  — an operator who wants that detail still runs `incidentdna library list`
  or `library check` against the actual incident file themselves (§12).
- **No new resource limits** — bounded entirely by the existing
  `suite.MaxScenariosPerSuite` (100) and the existing `library` package's
  own limits, reused unchanged (§3).
- **Not a substitute for `library check`.** `library check` remains the
  only command that validates and fingerprints a *fresh* incident document
  against the library; Phase 6's lookup only ever consults a fingerprint a
  scenario or suite has already declared.

## 20. Future release-gate integration boundary

Named explicitly, the same way `docs/phase-4-plan.md` §28 and
`docs/phase-5-plan.md` §26 named their own phase's boundary before it
existed: Phase 6 makes "does this scenario's/suite's failure class have a
recorded incident" a question `incidentdna` can answer directly, but it
remains purely informational — nothing in this codebase turns a "no
occurrences found" result into a blocking condition. Plausible, deliberately
**unbuilt** future increments, named here rather than started:

- Wiring a "no occurrences found" (or, inversely, "found") result into an
  actual CI/CD gate or merge check — explicitly excluded from this phase and
  from every phase before it.
- Sandboxed scenario/suite execution, remote/shared library or evidence
  storage, evidence/library signing or authenticity proof, observability/
  event-system ingestion, parallel suite execution, and directory/glob-based
  scenario discovery — all still named, still unbuilt, still out of scope,
  carried forward unchanged from `docs/product-scope.md`.

None of this is claimed to be started, scoped in code, or implicitly implied
by anything in this plan. This document makes no claim about what a future
Phase 7 would contain beyond naming these boundaries explicitly, the same
discipline every prior phase plan already followed.
