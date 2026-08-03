# Library Cross-Reference

Phase 6 adds a read-only, purely informational cross-reference between a
scenario's (or a suite's) declared `linked_fingerprint` and the incident
library's stored occurrences, exposed as an optional `--library <dir>` flag
on the two existing `verify` subcommands. This document describes what that
cross-reference is, what it guarantees, and — just as importantly — what it
does not. `docs/phase-6-plan.md` is the approved plan this implements; this
document describes the resulting design as built, for the same audience
`docs/regression-scenarios.md` and `docs/scenario-suites.md` serve for
Phases 4 and 5.

Implementation: one additive change to `internal/library/check.go` (plus
`errors.go`), and additive flag handling in `cmd/incidentdna/cmd_scenario.go`
and `cmd/incidentdna/cmd_suite.go`. No other package changes.

## What problem this closes

Phases 1–3 built a complete *memory* system: incidents can be represented,
validated, fingerprinted, and accumulated in a local library grouped by
failure-class fingerprint. Phases 4–5 built a complete, bounded, offline
*execution* system on top: a scenario declares which failure class it
regression-tests via `linked_fingerprint`, and a suite lists many scenarios.
These two systems were kept deliberately disconnected through Phase 5 — a
scenario's `linked_fingerprint` was never checked against what the library
actually holds. An operator who wanted to know "does this scenario (or this
whole suite) correspond to a failure class my team has actually recorded as
an incident, or is `linked_fingerprint` a typo/stale value?" had no way to
ask `incidentdna` that question directly; they had to run
`incidentdna library check` by hand against some incident file and compare
the printed fingerprint themselves.

Phase 6 closes exactly this gap, and nothing else: `scenario verify` and
`suite verify` can now optionally report, for each linked fingerprint they
already know about, whether the incident library holds one or more
occurrences under it — with no effect on whether a scenario or suite is
considered valid, and no effect on `scenario run` or `suite run` at all.

Phase 6 does not change `idir.Document`, the JSON Schema,
`internal/validate`, `internal/canonical`, `internal/fingerprint`,
`internal/compare`, `internal/evidence`, `internal/scenario`, or
`internal/suite`. `internal/library` gains exactly one new exported function
(`CheckFingerprint`) and one new sentinel error (`ErrInvalidFingerprint`);
`internal/scenario` and `internal/suite` are not modified at all and gain no
new import — the composition lives entirely in `cmd/incidentdna`, which
already imported all three packages before Phase 6.

## The one library-package change: `library.CheckFingerprint`

`internal/library.Check` takes a `*idir.Document`: it validates the
document, computes its fingerprint via `fingerprint.Compute`, then looks up
that fingerprint in the store. A scenario or suite entry never holds a full
`idir.Document` — only a plain, already format-checked `linked_fingerprint`
string. `CheckFingerprint` is a new entry point that performs the identical
lookup, keyed by a caller-supplied fingerprint string instead of one
computed from a document:

```go
func CheckFingerprint(ctx context.Context, s *Store, fp string) (CheckResult, error)
```

`fp` must already be a well-formed `"sha256:"`-prefixed, 64-lowercase-hex
string; a malformed value returns a wrapped `ErrInvalidFingerprint`. `Check`
and `CheckFingerprint` share every line of lookup logic after fingerprint
computation, via a shared unexported `checkFingerprint` helper — `Check`
itself is now a two-line wrapper: validate the document, compute the
fingerprint, then call the shared helper. This is proven by the existing
`check_test.go` test suite passing completely unmodified, plus a new parity
test asserting `Check(doc)` and `CheckFingerprint(fingerprint.Compute(doc))`
return identical `CheckResult` values for the same library state.

## The composition: `cmd/incidentdna` only

`internal/library` gains no "check many fingerprints" API — `cmd_scenario.go`
and `cmd_suite.go` each loop and call `CheckFingerprint` once per distinct
fingerprint themselves, the same way `suite.Validate` already loops calling
`scenario.Validate` once per listed scenario:

```
incident document (idir.Document)          scenario.LinkedFingerprint /
   │  idir.LoadFile + validate.Validate      suite scenario entries' own
   ▼                                         LinkedFingerprint (declared,
valid idir.Document                          format-checked always)
   │  fingerprint.Compute                        │
   ▼                                              │
failure fingerprint ─────────────────────────────┤
   │  library.Add (optional, independent)         │
   ▼                                              │
incident-library occurrence(s)                    │
   (Phase 3, grouped by fingerprint,               │
    zero or more per fingerprint)                  │
        ▲                                          │
        │  library.CheckFingerprint (Phase 6,      │
        │  read-only, NEW)                         │
        └──────────────────────────────────────────┘
              cmd/incidentdna composes this — never
              internal/scenario or internal/suite
```

## The two flags

```
incidentdna scenario verify [--source <incident-file>] [--library <dir>] <scenario-file>
incidentdna suite verify [--library <dir>] <suite-file>
```

`--library <path>` mirrors the exact flag name, default-value handling
(empty string → `library.DefaultLibraryRoot`), and help-text style the three
`incidentdna library` subcommands already use — `cmd_library.go`'s
`libraryFlag`/`openLibraryStore` helpers are reused unchanged, not
reimplemented. `scenario run`, `suite run`, and every other subcommand are
byte-for-byte unchanged in behavior.

**Workflow A — scenario verify:**

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
Library: no occurrences found for this fingerprint (not yet recorded in the library)
```

Exit code `0` in both cases — the `Library:` line is informational only,
never affecting `scenario verify`'s existing pass/fail determination.
`--library` can be combined with the existing `--source` flag; both
cross-checks run independently and both are printed.

**Workflow B — suite verify:**

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
checked-in `linked_fingerprint` (the duplicate-payment example's own golden
fingerprint), so this transcript also demonstrates deduplication directly:
three listed scenarios sharing one fingerprint trigger exactly **one**
library lookup, not three — the aggregate line counts **distinct**
fingerprints, and each scenario's own line still shows the same cached
result. A suite whose listed scenarios declare genuinely different
fingerprints would instead show a mix of "N occurrence(s)" and "no
occurrences" per-scenario lines, with the aggregate line's denominator equal
to however many distinct fingerprints were actually declared.

**Workflow C — `--library` omitted (default, unchanged behavior):** output
is byte-for-byte identical to pre-Phase-6 output — no `Library:` line
appears, for either command, unless `--library` is explicitly passed
(verified by regression tests asserting exact stdout equality).

**Workflow D — the library has never been created:** treated as "no
occurrences found," never an error, mirroring `library.Open`'s existing
"not-yet-created is not an error" stance.

**Workflow E — the library exists but is malformed** (corrupted
`index.json`, an unexpected shard/fingerprint directory shape): `scenario
verify --library`/`suite verify --library` exit `1` — an environmental
failure, the same class `library check` already reports for a malformed
library, distinct from and never conflated with "the scenario itself is
invalid" (exit `2`).

## Exit-code contract

**`scenario verify`** (extends, does not replace, the existing 0/1/2 split):

| Exit | Meaning |
|---|---|
| `0` | Structurally/semantically valid (and, if `--source` given, matches; the `--library` lookup result, if requested, is printed but never affects this) |
| `1` | I/O/parse/usage error — unchanged causes, **plus**: `--library` given and the library exists but is malformed, or cannot be read for a permission/environmental reason |
| `2` | Decodes but fails a semantic IRS v0.1 rule, or (with `--source`) fingerprint mismatch — unchanged; a `--library` "no occurrences found" result never produces this |

**`suite verify`** identically: `0`/`2` unchanged in every existing cause;
`1` gains the same new cause (a malformed/unreadable library named by
`--library`).

**`scenario run` and `suite run`**: entirely unchanged — no new flag, no
new exit-code cause, no new report field. Cross-referencing is a
`verify`-only capability.

## Determinism

1. A cross-reference result depends on the library's on-disk state at the
   moment of the call, not just on the scenario/suite's own bytes — this can
   legitimately differ between two otherwise-identical invocations if an
   intervening `library add` changed the library's contents, mirroring
   `library check`'s own existing, already-documented non-claim.
2. Given a fixed library state, the result is deterministic and repeatable —
   `library.CheckFingerprint` performs the identical integrity verification
   `Check` already performs (re-hash every indexed occurrence before
   counting it), so a corrupted occurrence is never silently reported as a
   match.
3. Suite-level lookup order is exactly the declared scenario order,
   deduplicated to first occurrence: the first listed scenario naming a
   given `linked_fingerprint` triggers the lookup; every later scenario
   naming the same fingerprint reuses that cached result. This bounds
   `suite verify --library`'s library I/O to at most one lookup per
   *distinct* fingerprint, never per scenario entry.
4. `--library` omitted is byte-for-byte identical to pre-Phase-6 output —
   for both `scenario verify` and `suite verify`.

## Safety model

Phase 6 introduces no new class of risk. It is read-only local filesystem
I/O, reusing `library.Open`'s and `library.Check`'s already-reviewed
path-resolution, symlink-rejection, and occurrence-integrity-verification
logic exactly as `library check` already exercises them — only the
fingerprint's origin differs (caller-supplied string vs. computed from a
document). No new write path exists anywhere in this phase: `scenario verify
--library` and `suite verify --library` never call `library.Add`, never
create the library root if it doesn't exist, and never modify any file
`library add`/`library check`/`library list` could also touch.

The one new consideration, stated plainly: running `scenario verify
--library`/`suite verify --library` reveals whether *any* occurrence exists
under a given fingerprint — but this is exactly the same information
`library check` already reveals today for a fingerprint computed from a
document the caller already has locally; Phase 6 adds no new
information-disclosure surface, only a second, more convenient way to ask
the same already-answerable question when the caller already has the
fingerprint value (from a scenario or suite file) rather than a full
incident document.

## Privacy implications

Identical in kind to every prior phase's own statement, restated rather than
newly invented:

- **No new document field, no new persisted data.** Phase 6 reads an
  already-declared, already-format-checked `linked_fingerprint` value and an
  already-existing library's already-stored occurrence metadata (fingerprint
  + occurrence count only — never a stored occurrence's full content).
- **No new content is ever printed.** The `Library:` line prints only a
  fingerprint (already visible in the scenario/suite's own `Linked
  fingerprint:` line) and an occurrence count — never an occurrence's
  incident id, title, service, or any other stored field. `library list`
  remains the only command in this codebase that prints per-occurrence
  metadata.
- **No network access means no telemetry, no external transmission** —
  restating the unconditional project invariant, extended to the one new
  `cmd/incidentdna` → `internal/library` call path this phase adds.

## Corruption and malformed-input handling

- **`--library` omitted**: no new behavior of any kind; every existing
  corruption/malformed-input case for `scenario verify`/`suite verify` is
  completely unchanged.
- **`--library` given, library root does not exist yet**: `library.Open`'s
  existing "not yet created is not an error" behavior applies; the lookup
  proceeds and reports "no occurrences found," exit code unaffected.
- **`--library` given, library root exists but its layout is malformed**
  (unexpected shard/fingerprint directory shape, corrupted `index.json` for
  the fingerprint being looked up): `library.CheckFingerprint` returns the
  same `ErrMalformedLibrary`/`ErrMalformedIndex`/`ErrCorruptedOccurrence`
  family `Check` already returns; `scenario verify`/`suite verify` map this
  to exit `1`.
- **`--library` given, `--source` also given (scenario verify only)**: both
  checks run independently; either can independently fail — the `--library`
  lookup only ever runs after the structural/semantic and `--source` checks
  have already passed, so a `--source` mismatch (exit `2`) is returned
  before `--library` is ever consulted.
- **A suite's listed scenarios include one whose `LinkedFingerprint` is
  identical to another's**: not an error (suites already permit this); the
  fingerprint is looked up once and the cached result is shown on every
  scenario line that shares it.

## What Phase 6 explicitly does not provide

- **No release gating.** A "no occurrences found" result is printed, never
  enforced — neither `verify` subcommand fails because a fingerprint has no
  library occurrences.
- **No change to `scenario run` or `suite run`.** Cross-referencing is
  available only through the two `verify` subcommands.
- **No new document field anywhere.** `idir.Document`, `scenario.Document`,
  and `suite.Document` are unchanged Go types.
- **No mutation of the library.** The only library operation this phase ever
  performs is a lookup; `library.Add` is never called by any code path this
  phase introduces.
- **No coupling built into `internal/scenario` or `internal/suite`
  themselves.** Both remain true parallel leaf packages, unchanged — the
  composition lives entirely in `cmd/incidentdna`.
- **No new resource limits beyond what already exists.** A suite's
  listed-scenario count is already bounded by `suite.MaxScenariosPerSuite`
  (100); that bound already caps how many distinct fingerprints one `suite
  verify --library` call can look up.
- **No JSON output for the cross-reference result.** The lookup result is
  printed to stdout as part of the existing human-readable `verify` summary
  only.
- **No batch/multi-fingerprint library query command.** `internal/library`
  gains exactly one new function, reusing `Check`'s existing internal lookup
  logic.
- **No signing, no encryption, no multi-tenancy, no network, no
  telemetry, no sandboxing, no parallel execution, no directory/glob
  discovery, no remote storage** — all remain exactly as out of scope as
  `docs/product-scope.md` already states.

None of this is claimed to be started, scoped in code, or implicitly implied
beyond what is stated here — a future Phase 7 is not scoped by this
document.

## The `verify-library-crossref-demo.sh` script

Unlike Phases 2–5, Phase 6 introduces no new example directory — it
demonstrates the feature by composing already-existing, unmodified fixtures
from `examples/duplicate-payment/`,
`examples/regression-scenario-demo/scenario-pass.yaml`, and
`examples/regression-suite-demo/suite-mixed.yaml`, the same "pure
composition of already-existing pieces" the feature itself embodies. See
`scripts/verify-library-crossref-demo.sh` for the exact five checks it
performs (match, no-match, not-yet-created library, suite-level mixed
result with dedup, malformed library), and `make example-library-crossref`
to run it.

## Current Phase 6 status and remaining limitations

Implemented, per `internal/library/check.go` and
`cmd/incidentdna/{cmd_scenario,cmd_suite}.go`: `library.CheckFingerprint`,
the `--library` flag on both `verify` subcommands, per-scenario and
aggregate suite annotations, and the exit-code contract described above.
This document describes that implementation as it exists in the code today.

Known, accepted limitations, restated from the sections above so they are
findable in one place:

- Point-in-time only — a cross-reference result reflects the library's
  contents at the moment `verify --library` runs; nothing re-checks it
  later, including `scenario run`/`suite run`.
- No release gating.
- No coupling inside `internal/scenario`/`internal/suite` themselves — both
  remain unaware the incident library exists.
- Occurrence count only, not occurrence content.
- No new resource limits — bounded entirely by existing limits, reused
  unchanged.
- Not a substitute for `library check` — `library check` remains the only
  command that validates and fingerprints a *fresh* incident document
  against the library.

This document intentionally does not claim CI results, production usage, or
performance benchmarks beyond what is recorded in `docs/phase-6-report.md`.
