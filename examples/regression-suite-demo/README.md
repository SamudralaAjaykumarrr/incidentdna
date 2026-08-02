# regression-suite-demo

A fifth, self-contained example — unrelated to
[`examples/duplicate-payment/`](../duplicate-payment/),
[`examples/evidence-storage-demo/`](../evidence-storage-demo/),
[`examples/incident-library-demo/`](../incident-library-demo/), and
[`examples/regression-scenario-demo/`](../regression-scenario-demo/) — that
exists to demonstrate the Phase 5 `incidentdna suite` commands (`verify`,
`run`) end-to-end, covering both the aggregate `PASS` and aggregate `FAIL`
outcomes plus `--fail-fast`, as described in `docs/scenario-suites.md`.

All content in this directory is fictional. There are no credentials, API
keys, personal information, customer data, or real production/incident
evidence anywhere in this directory.

## Contents

```
suite-all-pass.yaml           — aggregate PASS: every listed scenario PASSes
suite-mixed.yaml              — aggregate FAIL: a mix of PASS/FAIL/TIMEOUT
scenarios/
  scenario-pass.yaml            PASS  — re-fingerprints a staged copy of the
                                        duplicate-payment example through the
                                        built incidentdna binary
  scenario-trivial-pass.yaml    PASS  — /bin/true, no workspace_files at all
  scenario-fail.yaml            FAIL  — the same command as scenario-pass,
                                        but with a deliberately wrong
                                        expected.exit_code
  scenario-timeout.yaml         TIMEOUT — sleeps past a deliberately short
                                        timeout_seconds
  scenario-invalid.yaml         (structurally invalid, never listed in
                                        either suite manifest above — see
                                        "Demonstrating aggregate INVALID")
  fixtures/incident.yaml        — a frozen COPY (not a symlink or reference)
                                        of examples/duplicate-payment/
                                        incident.yaml's content
```

`scenarios/fixtures/incident.yaml` is never edited by anything in this
directory or by `scripts/verify-suite-demo.sh` —
`examples/duplicate-payment/incident.yaml` and
`testdata/golden/duplicate-payment.fingerprint` remain byte-for-byte
untouched by Phase 5, exactly as `docs/phase-5-plan.md` §20 requires.

## Why `fixtures/` is nested under `scenarios/`, not a sibling of it

`docs/phase-5-plan.md` §22 illustrates this example's expected layout with
`fixtures/incident.yaml` as a sibling of `scenarios/`. In practice, an IRS
v0.1 scenario's `execution.workspace_files[].source` is resolved relative to
*that scenario file's own directory* and is rejected outright if the
declared path contains `".."` (`internal/scenario`'s existing, unchanged
path-safety rule — see `docs/regression-scenarios.md`, "Symlink and
path-traversal protections"). A scenario file living in `scenarios/` cannot
therefore reference a fixture one level back up at `../fixtures/...` without
tripping that rule. Nesting the fixture at `scenarios/fixtures/incident.yaml`
(the same shape `examples/regression-scenario-demo/fixtures/incident.yaml`
already uses relative to its own scenario files) lets every scenario here
declare a plain `source: fixtures/incident.yaml`, with no traversal and no
change to `internal/scenario`'s existing, unmodified rule. Nothing about the
suite manifests themselves is affected: `scenarios[].path` values like
`scenarios/scenario-pass.yaml` are ordinary subdirectory references, not
traversal, and resolve exactly as declared.

## Demonstrating aggregate INVALID

`docs/phase-5-plan.md` §17 states plainly that a suite listing even one
structurally invalid scenario is rejected **as a whole**, at `suite verify`/
`suite run` pre-execution time — never partially run, skipping only the
invalid entry. `scenarios/scenario-invalid.yaml` (a bare `command[0]`, the
same rejected shape `examples/regression-scenario-demo/scenario-invalid.yaml`
already demonstrates for a standalone scenario) exists specifically to
exercise this: it is deliberately never listed in `suite-all-pass.yaml` or
`suite-mixed.yaml`, since listing it there would make *those* manifests
aggregate `INVALID` instead of demonstrating `PASS`/`FAIL`.
`scripts/verify-suite-demo.sh` instead generates a small, throwaway suite
manifest at verification time that lists only this one file, and confirms
`suite verify` exits `2` and `suite run` reports aggregate `INVALID` and
exits `1`, having executed nothing.

## Why this demo uses an absolute command path

`scenario-pass.yaml` and `scenario-fail.yaml`'s `execution.command[0]` is the
literal string `/INCIDENTDNA_BIN_PLACEHOLDER` — a syntactically absolute path
(so `incidentdna suite verify`/`scenario verify` accept its shape) that does
not exist on any real filesystem. `scripts/verify-suite-demo.sh` substitutes
it for the real, checkout-specific absolute path to the built
`./bin/incidentdna` binary before running those scenarios via `suite run`.
See `examples/regression-scenario-demo/README.md`, "Why this demo uses an
absolute command path", for the full reasoning (a portable relative path
back to the repository's `bin/` directory would need to know the run's
workspace depth below the repository root, which is neither fixed nor
predictable across machines and CI runners). `scenario-timeout.yaml` and
`scenario-invalid.yaml` need no substitution: they reference `/bin/sleep`
and a deliberately bare `echo`, neither of which depends on this checkout's
layout.

## Running the demo

Everything below is offline and local-filesystem-only. Run these from the
repository root, after `make build` (or
`go build -o bin/incidentdna ./cmd/incidentdna`); `scripts/verify-suite-demo.sh`
automates exactly this sequence, using a temporary `--workspace-root`/
`--report` location for every `suite run` call (never the current directory,
and never anything left behind in the repository):

```sh
DEMO=examples/regression-suite-demo
BIN="$PWD/bin/incidentdna"

# 1. suite verify — structural validation of the manifest and every
#    scenario it lists, against the checked-in, unmodified files. Never
#    executes anything.
./bin/incidentdna suite verify "$DEMO/suite-all-pass.yaml"
./bin/incidentdna suite verify "$DEMO/suite-mixed.yaml"

# 2. suite run — aggregate PASS. Requires the placeholder substitution
#    described above; scripts/verify-suite-demo.sh does this into a
#    temporary copy of the whole demo directory.
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$DEMO/scenarios/scenario-pass.yaml" > /tmp/scenario-pass.yaml
# (scripts/verify-suite-demo.sh stages a full temporary copy of the demo
#  directory tree with this substitution applied, then runs:)
./bin/incidentdna suite run --report /tmp/all-pass-report.json /tmp/suite-demo-copy/suite-all-pass.yaml

# 3. suite run — aggregate FAIL.
./bin/incidentdna suite run --report /tmp/mixed-report.json /tmp/suite-demo-copy/suite-mixed.yaml

# 4. suite run --fail-fast — early termination, SKIPPED entries.
./bin/incidentdna suite run --fail-fast --report /tmp/fail-fast-report.json /tmp/suite-demo-copy/suite-mixed.yaml
```

Expected outcome: step 1's two `suite verify` calls both exit `0` and print
`OK: suite manifest and all N listed scenarios are structurally valid`,
each preceded by one `[OK] <scenario-id> (<path>)` line per listed scenario.
Step 2 prints `Result: PASS` and exits `0`. Step 3 prints `Result: FAIL`
(with counts `1 PASS, 1 FAIL, 1 TIMEOUT, 0 INVALID, 0 INTERNAL_ERROR`) and
exits `2`. Step 4 additionally reports one scenario `SKIPPED` (the timeout
scenario, never reached because the fail scenario ahead of it already
produced a non-`PASS` outcome) and exits `2`. Each `--report` path receives
a corresponding deterministic JSON aggregate report
(`docs/scenario-suites.md`, "Result and report format"), embedding every
executed scenario's own unmodified `scenario.Report`.

`scripts/verify-suite-demo.sh` additionally demonstrates aggregate
`INVALID` (see "Demonstrating aggregate INVALID" above) and
`--keep-workspaces`/`--workspace-root`, neither of which needed its own
checked-in top-level suite-manifest file.

## Notes

- Every command above is fully offline: no network access, no telemetry,
  consistent with every prior phase.
- `incidentdna suite` never discovers scenarios via directory walk or glob
  — both manifests above list every scenario file explicitly, by path, in
  the exact order they run.
- This demo does not sandbox `scenario-pass.yaml`/`scenario-fail.yaml`'s
  command: it runs the compiled `incidentdna` binary itself, which is
  already trusted in this environment, exactly as
  `examples/regression-scenario-demo/` already does for the underlying
  Phase 4 scenario runner. See `docs/scenario-suites.md`, "Safety model",
  for what "bounded, not sandboxed" means here.
