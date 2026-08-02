# regression-scenario-demo

A fourth, self-contained example — unrelated to
[`examples/duplicate-payment/`](../duplicate-payment/),
[`examples/evidence-storage-demo/`](../evidence-storage-demo/), and
[`examples/incident-library-demo/`](../incident-library-demo/) — that exists
to demonstrate the Phase 4 `incidentdna scenario` commands (`verify`, `run`)
end-to-end, covering all five documented outcomes
(`PASS`/`FAIL`/`TIMEOUT`/`INVALID`/`INTERNAL_ERROR`) plus the
`--source` fingerprint-linkage cross-check, as described in
`docs/regression-scenarios.md`.

All content in this directory is fictional. There are no credentials, API
keys, personal information, customer data, or real production/incident
evidence anywhere in this directory.

## Contents

```
fixtures/incident.yaml   — a frozen COPY (not a symlink or reference) of
                            examples/duplicate-payment/incident.yaml's
                            content, staged into each scenario's workspace
                            via execution.workspace_files
scenario-pass.yaml        — PASS: re-fingerprints the staged copy through
                             the built incidentdna binary and asserts the
                             pinned golden fingerprint appears in stdout
scenario-fail.yaml        — FAIL: the same command, but declares an
                             expected.exit_code that does not match
scenario-timeout.yaml     — TIMEOUT: sleeps past a deliberately short
                             timeout_seconds
scenario-invalid.yaml     — INVALID: declares a bare command[0] ("echo"),
                             rejected before anything executes
```

`fixtures/incident.yaml` is never edited by anything in this directory or by
`scripts/verify-scenario-demo.sh` — `examples/duplicate-payment/incident.yaml`
and `testdata/golden/duplicate-payment.fingerprint` remain byte-for-byte
untouched by Phase 4, exactly as `docs/phase-4-plan.md` §20 requires.

## Why this demo uses an absolute command path

`scenario-pass.yaml` and `scenario-fail.yaml`'s `execution.command[0]` is the
literal string `/INCIDENTDNA_BIN_PLACEHOLDER` — a syntactically absolute path
(so `incidentdna scenario verify` accepts its shape) that does not exist on
any real filesystem. `scripts/verify-scenario-demo.sh` substitutes it for the
real, checkout-specific absolute path to the built `./bin/incidentdna` binary
before running those two scenarios. A genuinely portable *relative*
`command[0]` (e.g. `../../../bin/incidentdna`, resolved against the
workspace root) would need to know how many directory levels separate the
run's workspace from the repository's `bin/` directory — but that workspace
is, by default, a fresh `os.MkdirTemp` directory under the OS temporary
directory, whose depth relative to the repository checkout is neither fixed
nor predictable across machines and CI runners. An absolute path, filled in
by the script that actually knows where the repository and its built binary
live, is the portable choice; see `docs/regression-scenarios.md`, "Why the
demo uses an absolute command path", for the same reasoning in the main
design document. `scenario-timeout.yaml` and `scenario-invalid.yaml` need no
substitution: they reference `/bin/sleep` (an ordinary coreutils binary) and
a deliberately bare `echo`, neither of which depends on this checkout's
layout.

## Running the demo

Everything below is offline and local-filesystem-only. Run these from the
repository root, after `make build` (or
`go build -o bin/incidentdna ./cmd/incidentdna`); `scripts/verify-scenario-demo.sh`
automates exactly this sequence, using a temporary `--workspace`/`--report`
location for every `scenario run` call (never the current directory, and
never anything left behind in the repository):

```sh
DEMO=examples/regression-scenario-demo
BIN="$PWD/bin/incidentdna"

# 1. scenario verify — structural validation only, against the checked-in,
#    unmodified files. Never executes anything. scenario-invalid.yaml is
#    deliberately semantically invalid (a bare command[0]), so its verify
#    call is expected to exit 2, not 0 — verify's job is exactly to catch
#    that before scenario run ever attempts to execute it.
./bin/incidentdna scenario verify "$DEMO/scenario-pass.yaml"
./bin/incidentdna scenario verify "$DEMO/scenario-fail.yaml"
./bin/incidentdna scenario verify "$DEMO/scenario-timeout.yaml"
./bin/incidentdna scenario verify "$DEMO/scenario-invalid.yaml"   # exits 2 here, by design

# 2. scenario verify --source — cross-checks linked_fingerprint against a
#    fingerprint freshly computed from the existing, unmodified Phase 1
#    example, satisfying "a runnable example based on an existing
#    IncidentDNA example" without ever editing that example.
./bin/incidentdna scenario verify \
  --source examples/duplicate-payment/incident.yaml \
  "$DEMO/scenario-pass.yaml"

# 3. scenario run — PASS. Requires the placeholder substitution described
#    above; scripts/verify-scenario-demo.sh does this into a temporary copy.
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$DEMO/scenario-pass.yaml" > /tmp/scenario-pass.yaml
./bin/incidentdna scenario run --report /tmp/pass-report.json /tmp/scenario-pass.yaml

# 4. scenario run — FAIL.
sed "s#/INCIDENTDNA_BIN_PLACEHOLDER#$BIN#" "$DEMO/scenario-fail.yaml" > /tmp/scenario-fail.yaml
./bin/incidentdna scenario run --report /tmp/fail-report.json /tmp/scenario-fail.yaml

# 5. scenario run — TIMEOUT. No substitution needed.
./bin/incidentdna scenario run --report /tmp/timeout-report.json "$DEMO/scenario-timeout.yaml"

# 6. scenario run — INVALID. No substitution needed.
./bin/incidentdna scenario run --report /tmp/invalid-report.json "$DEMO/scenario-invalid.yaml"
```

Expected outcome: step 1's first three `scenario verify` calls exit `0` and
print `OK: scenario is structurally valid`; the fourth
(`scenario-invalid.yaml`) exits `2` and reports the bare-`command[0]` issue —
`scenario verify`'s job is exactly to catch this before `scenario run` ever
attempts to execute it. Step 2 additionally prints `OK: scenario is
structurally valid and its linked_fingerprint matches --source` and exits
`0`. Step 3 prints `Result: PASS` and exits `0`. Step 4 prints
`Result: FAIL` (actual exit code `0` vs. expected `1`) and exits `2`. Step 5
prints `Result: TIMEOUT` and exits `2`, completing in well under 5 seconds.
Step 6 prints `Result: INVALID` (bare `command[0]`) and exits `1`, having
never started a process. Each `--report` path receives a corresponding
deterministic JSON execution report (`docs/regression-scenarios.md`,
"Result and report format").

`scripts/verify-scenario-demo.sh` additionally demonstrates `INTERNAL_ERROR`
(a scenario whose `command[0]` names a nonexistent absolute path) and
`--keep-workspace`, neither of which needed its own checked-in example file.

## Notes

- Every command above is fully offline: no network access, no telemetry,
  consistent with every prior phase.
- `incidentdna scenario` never invokes a shell and never performs an
  implicit `$PATH` lookup — `scenario-invalid.yaml`'s bare `"echo"` is
  rejected for exactly that reason, not executed with a fallback shell.
- This demo does not sandbox `scenario-pass.yaml`/`scenario-fail.yaml`'s
  command: it runs the compiled `incidentdna` binary itself, which is
  already trusted in this environment. See
  `docs/regression-scenarios.md`, "Safety model", for what "bounded, not
  sandboxed" means for an arbitrary reviewed command.
