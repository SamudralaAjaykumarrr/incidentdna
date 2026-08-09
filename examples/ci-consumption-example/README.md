# Vendor-neutral CI consumption example

This directory demonstrates the pattern `docs/policy-evaluation.md` already
describes as IncidentDNA's intended use from an external CI system: run a
regression suite, evaluate the result against a declared release-gate
policy, and act on the process exit code. It is documentation and a runnable
example only — it is not part of the `incidentdna` binary, imports nothing
from this module, and is exercised (both the PASS and FAIL exit-code paths)
by `scripts/verify-ci-consumption-example.sh`, wired into `make verify`
exactly like every other checked-in demo.

**Core IncidentDNA remains vendor-neutral.** Nothing in `cmd/incidentdna` or
`internal/` calls a GitHub API, a GitLab API, or contains Jenkins-specific
logic. The only thing an external CI system needs is the ability to run a
shell command and inspect its exit code — the same integration point every
other CLI-based check already uses.

## The pattern

1. Run the regression suite (or a single scenario) that names the release
   gate's declared incidents, writing a deterministic JSON report:

   ```
   incidentdna suite run --report suite-report.json <suite-file>
   ```

2. Evaluate that report against a declared policy:

   ```
   incidentdna policy evaluate --policy <policy-file> --suite-report suite-report.json
   ```

3. Check the process exit code of step 2:

   | Exit | Meaning |
   |---|---|
   | `0` | Release allowed — verdict `PASS` |
   | `2` | Release blocked — verdict `FAIL` |
   | `1` | Environmental failure needing human attention (I/O/usage error, `INVALID`, `INTERNAL_ERROR`) |

This is the identical exit-code contract `docs/policy-evaluation.md`
("Exit-code contract") already documents for `policy evaluate` — this
directory restates it as a consumption recipe for someone who has not read
this repository's Go source, not a new contract.

## `check-release-gate.sh`

[`check-release-gate.sh`](check-release-gate.sh) is a small, portable POSIX
`sh` script (no bashisms, `set -eu`, the same discipline every
`scripts/verify-*.sh` in this repository already follows) that runs the two
commands above against caller-supplied paths and exits with `policy
evaluate`'s own code, unmodified:

```
check-release-gate.sh <incidentdna-binary> <suite-file> <policy-file> [library-dir]
```

`<library-dir>` is optional. When given, it is passed through as `policy
evaluate --library`, enabling `require_library_occurrence` rules. When
omitted, such rules are reported `SKIP` (and therefore count as not
satisfied) — the same behavior `incidentdna policy evaluate` always has
without `--library`.

Example, run against this repository's own checked-in fixtures:

```
$ ./examples/ci-consumption-example/check-release-gate.sh \
    ./bin/incidentdna \
    examples/regression-suite-demo/suite-all-pass.yaml \
    policies/release-gate-example.yaml \
    /tmp/incidentdna-library
check-release-gate: ./bin/incidentdna suite run --report /tmp/.../suite-report.json examples/regression-suite-demo/suite-all-pass.yaml
check-release-gate: ./bin/incidentdna policy evaluate --policy policies/release-gate-example.yaml --suite-report /tmp/.../suite-report.json --library /tmp/incidentdna-library
Policy: default-release-gate (schema policy/v0.1)
Input: suite report (result: PASS), ...
Verdict: PASS
$ echo $?
0
```

## Illustration only: wiring this into GitHub Actions

The following is a fenced code block a reader adapts to their own CI
system — it is **not** a file this repository executes, is not referenced
by `.github/workflows/ci.yml`, and is not this project's own CI. It exists
solely to make the pattern above concrete for one popular CI vendor,
without making that vendor's YAML schema, API, or marketplace a dependency
of `incidentdna` itself.

```yaml
# .github/workflows/release-gate.yml (illustrative only — not present in
# this repository's own .github/workflows/)
name: Release gate
on: [pull_request]
jobs:
  release-gate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Download incidentdna
        run: |
          curl -LO https://github.com/SamudralaAjaykumarrr/incidentdna/releases/download/v0.1.0/incidentdna-v0.1.0-linux-amd64.tar.gz
          tar xzf incidentdna-v0.1.0-linux-amd64.tar.gz
      - name: Check release gate
        run: |
          ./examples/ci-consumption-example/check-release-gate.sh \
            ./incidentdna \
            path/to/your-suite.yaml \
            path/to/your-policy.yaml
          # A non-zero exit here fails this GitHub Actions step, which is
          # this vendor's own, ordinary way of blocking a PR — nothing
          # IncidentDNA-specific about that mechanism.
```

## What this is not

- Not a published GitHub Action, GitLab CI template, or any other
  CI-marketplace artifact — a reader copies and adapts
  `check-release-gate.sh` by hand.
- Not a hosted service. Everything here runs locally, offline, exactly like
  every other IncidentDNA command (`docs/threat-model.md`).
- Not a change to this repository's own `.github/workflows/ci.yml` gating —
  that workflow is unmodified by this example (`docs/phase-8-plan.md` §4).
