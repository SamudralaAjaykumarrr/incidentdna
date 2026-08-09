# Release process

This document is the Phase 8 analog of `docs/policy-evaluation.md`/
`docs/library-crossref.md`: the full design for turning the seven
already-built product layers (Phases 1-7) into a versioned, checksummed,
installable release artifact. It covers the build strategy, the platform
matrix, artifact naming, checksum and SBOM generation, benchmarks, the
vendor-neutral CI-consumption example, the v0.1.0 compatibility contract,
and the release checklist — everything a future maintainer cutting `v0.1.1`
or later needs in one place.

Phase 8 builds no new product capability. Every `internal/` package Phases
1-7 built (`idir`, `validate`, `canonical`, `fingerprint`, `compare`,
`evidence`, `library`, `scenario`, `suite`, `policy`) is byte-for-byte
unchanged except for four new, additive benchmark test files. See
`docs/product-scope.md`, "Phase 8 scope," and `docs/architecture.md`,
"Release artifacts and reproducible builds (Phase 8)."

## The version contract

```
incidentdna version
incidentdna --version
incidentdna -v
```

All three print `incidentdna <version>` to stdout and exit `0` — never
fail. `<version>` is a package-level `var version = "dev"` in
`cmd/incidentdna/version.go`, overridden only via `-ldflags "-X
main.version=$VERSION"` at build time. `make build` (the ordinary dev
target) is unchanged and always produces a binary reporting `dev`. Only
`scripts/build-release.sh` injects a real version string, supplied by the
caller (e.g. `VERSION=v0.1.0`) — never derived from `git describe`, `git
rev-parse`, or any other VCS-sourced signal, preserving the same
determinism discipline the existing `-buildvcs=false` flag already
established for `make build`.

## Reproducible release build strategy

`scripts/build-release.sh` builds one binary per supported platform (see
"Supported platform matrix" below), inside the identical
`Dockerfile.dev`/`golang:1.26.5` container every other `make` target
already uses:

```
CGO_ENABLED=0 GOOS=<os> GOARCH=<arch> go build \
  -trimpath -buildvcs=false \
  -ldflags "-s -w -X main.version=$VERSION" \
  -o <binary> ./cmd/incidentdna
```

- **`CGO_ENABLED=0`** — the module has zero cgo dependencies
  (`gopkg.in/yaml.v3` is pure Go), so this changes nothing about what
  compiles; it guarantees a statically linked, fully portable binary per
  platform.
- **`-trimpath`** (new) — strips local filesystem build paths, the
  standard companion to `-buildvcs=false`: two checkouts at different
  absolute paths produce byte-identical output.
- **`-buildvcs=false`** (existing, unchanged) — no VCS stamping.
- **`-ldflags "-s -w -X main.version=$VERSION"`** — strips debug
  symbols/DWARF (`-s -w`) and injects the version string. `$VERSION` is a
  caller-supplied string, never derived from the working tree.

**Reproducibility is a checked property, not an assumption.** Every
platform target is built **twice**, in immediate succession, into two
separate temporary directories; the two binaries' SHA-256 digests are
compared before anything is archived. A mismatch aborts the whole release
build with a non-zero exit and a clear error — the same "don't assume,
verify" discipline `scripts/verify-golden-fingerprint.sh` already applies
to the fingerprint algorithm, applied here to the build process itself.

## Supported platform matrix

| GOOS | GOARCH | Artifact identifier | Built by default? |
|---|---|---|---|
| `linux` | `amd64` | `incidentdna-linux-amd64` | Yes |
| `linux` | `arm64` | `incidentdna-linux-arm64` | Yes |
| `darwin` | `amd64` | `incidentdna-darwin-amd64` | Yes |
| `darwin` | `arm64` | `incidentdna-darwin-arm64` | Yes |
| `windows` | `amd64` | `incidentdna-windows-amd64` | Yes |

All builds use `CGO_ENABLED=0` from the single `golang:1.26.5` container,
with no per-platform build environment. No `386`, no 32-bit `arm`, no
`freebsd`, no `riscv64` — these are plausible future additions, not built
now, because nothing in this project's test matrix (`.github/workflows/
ci.yml` runs `ubuntu-latest` only) exercises them.

### Windows portability fix

An earlier iteration of this release build shipped only four of the five
platforms above: `internal/scenario/run.go` called
`syscall.SysProcAttr{Setpgid: true}` and `syscall.Kill(-pid, ...)` directly
to enforce scenario timeouts across a whole process group, and neither
symbol exists on `GOOS=windows`, so `cmd/incidentdna` (which imports
`internal/scenario`) failed to compile for that target. This was a real
defect against the plan's five-platform matrix, not an accepted scope
reduction, and has been fixed: the process-group logic was extracted out
of `run.go` into two `//go:build`-tagged files, so the OS-specific pieces
live next to each other without either needing an unbuildable import on
the other platform:

- **`internal/scenario/run_unix.go`** (`//go:build !windows`) — the exact
  pre-existing behavior, relocated unchanged: `Setpgid: true` on the child
  process's `SysProcAttr`, and `syscall.Kill(-pid, syscall.SIGKILL)` on
  timeout to kill the whole process group.
- **`internal/scenario/run_windows.go`** (`//go:build windows`) — the
  closest Windows equivalent to a POSIX process group, a Job Object: the
  child is assigned to a `CreateJobObjectW`-created job after `Start`, and
  a timeout calls `TerminateJobObject`, killing every process in the job
  (the child and its descendants), not just the direct child. These Win32
  calls are made directly against `kernel32.dll` via `syscall.NewLazyDLL`
  — part of the standard library's `syscall` package — so this adds no
  module dependency beyond what `go.mod` already declares.

`run.go` itself now calls a small OS-agnostic `processGroup` type
(`newProcessGroup`/`started`/`kill`) instead of `syscall` directly, and no
longer imports `syscall` at all. Every other line of `internal/scenario`'s
behavior — execution semantics, timeout duration, resource bounds, output
capture, outcome classification (`PASS`/`FAIL`/`TIMEOUT`/`INVALID`/
`INTERNAL_ERROR`), report schema — is unchanged; only the OS-specific
mechanism for grouping and killing the child process tree was relocated
and, for Windows, newly implemented. `scripts/build-release.sh` now builds
all five platforms above by default.

## Deterministic artifact naming

```
dist/
  incidentdna-<version>-linux-amd64.tar.gz
  incidentdna-<version>-linux-arm64.tar.gz
  incidentdna-<version>-darwin-amd64.tar.gz
  incidentdna-<version>-darwin-arm64.tar.gz
  incidentdna-<version>-windows-amd64.zip
  SHA256SUMS
  incidentdna-<version>-sbom.json
  incidentdna-<version>-bench.txt
```

- `<version>` is exactly the `$VERSION` string used at build time (e.g.
  `v0.1.0`), never a git commit hash, timestamp, or build number.
- `.tar.gz` for every POSIX platform; `windows/amd64` uses `.zip` instead —
  the conventional archive format per platform.
- Each archive contains exactly two files at its root: `incidentdna` (or
  `incidentdna.exe` on windows) and a copy of `LICENSE` — no `README.md`,
  no `docs/`, keeping the artifact minimal.
- `SHA256SUMS` is one flat file at the top of `dist/`, not per-archive: a
  list of `<digest>  <filename>` lines, one per artifact, sorted by
  filename, so `sha256sum -c SHA256SUMS` works unmodified.
- `incidentdna-<version>-sbom.json` and `-bench.txt` are per-release, not
  per-platform — neither the dependency graph nor the benchmark results
  vary by target platform.

## SHA-256 checksum generation and verification

**Generation** (`scripts/generate-checksums.sh`): after all archives exist
under `dist/`, runs `sha256sum` (falling back to `shasum -a 256` on macOS,
or a temporary `go run`-based `crypto/sha256` helper if neither system tool
is present — no new checked-in dependency either way) over every
`dist/*.tar.gz` and `dist/*.zip`, sorted by filename, writing
`dist/SHA256SUMS`.

**Verification** — two independent paths:

1. **Standard tooling**: `sha256sum -c SHA256SUMS` (Linux) or `shasum -a
   256 -c SHA256SUMS` (macOS) — no IncidentDNA tooling required.
2. **Self-verification via the built binary**: `incidentdna version`
   printing the correct `$VERSION` string is a secondary, informational
   integrity signal only — a checksum-tampered binary that still ran would
   still report its embedded build version, so this is *not* a substitute
   for the checksum check (see `docs/threat-model.md`, "Release artifact
   supply-chain considerations (Phase 8)").

**Explicitly not built**: GPG/cosign signing of `SHA256SUMS` itself — the
checksum file's own integrity relies on whatever transport delivers it (an
HTTPS-served GitHub Release asset), the same trust boundary every other
unsigned open-source CLI release already has.

## SBOM / dependency inventory

**Constraint**: no large new dependency, no network access at generation
time. This project's entire non-stdlib dependency surface is already
minimal — `go.mod` names exactly one direct dependency
(`gopkg.in/yaml.v3 v3.0.1`), and `go.sum` names exactly one additional
transitive/test-only module (`gopkg.in/check.v1`, a test dependency of
`yaml.v3` itself, never imported by this module's own code).

`scripts/generate-sbom.sh` uses only `go list -m -json all` (a stdlib `go`
command, reads the local `go.sum`-pinned module cache, no network request
when the cache is already populated) and reshapes that output into a
minimal, hand-specified JSON document:

```json
{
  "format": "incidentdna-sbom/v1",
  "generated_for": "v0.1.0",
  "go_version": "go1.26.5",
  "module": "github.com/SamudralaAjaykumarrr/incidentdna",
  "dependencies": [
    {"path": "gopkg.in/check.v1", "version": "v0.0.0-20161208181325-20d25e280405", "direct": false},
    {"path": "gopkg.in/yaml.v3", "version": "v3.0.1", "direct": true}
  ]
}
```

Deterministic field order (a fixed Go struct, never map iteration),
dependencies sorted by `path`. If a future dependency is ever added to
`go.mod`, this file's shape does not change — only its `dependencies` array
grows. Named `incidentdna-sbom/v1` (not `policy/v0.1`-style) since it is
release metadata, not an IDIR-family document format — it does not
participate in the compatibility contract below.

**Explicitly not built**: a full CycloneDX or SPDX document, or any
general-purpose SBOM tool (`syft`, `cyclonedx-gomod`, etc.) — both would be
strictly disproportionate to a two-line dependency graph and would
themselves become the largest new dependency this phase adds. The SBOM
covers only this module's own Go dependency graph; it says nothing about
base OS packages inside `golang:1.26.5` itself (not applicable to a
`CGO_ENABLED=0` static Go binary, which does not link against or ship
anything from that base image).

## Informational benchmark strategy

**Constraint**: benchmarks are informational only and never gate CI or a
release.

**What is benchmarked** — deterministic, CPU-bound hot paths, using Go's
standard `testing.B` support:

- `internal/canonical`: `BenchmarkMarshal` over the duplicate-payment
  fixture — the operation every `fingerprint`, `compare`, `scenario verify
  --source`, and `policy evaluate` invocation performs at least once.
- `internal/fingerprint`: `BenchmarkCompute` over the same fixture —
  canonicalization plus SHA-256, end to end.
- `internal/evidence`: `BenchmarkStore`/`BenchmarkVerify` over a
  representative 1 MiB object — the store's own content-hashing path.
- `internal/validate`: `BenchmarkValidate` over a minimal valid document —
  the full semantic rule engine.

**What is deliberately not benchmarked**: `internal/scenario.Run` and
`internal/suite.Run` — their wall-clock cost is dominated by the
caller-declared child process being executed, not this codebase's own
logic, so a benchmark of them would measure the fixture's `sleep`/`echo`
call, not this project's code. `internal/policy.Evaluate` is a handful of
string comparisons over an already-small report — its cost is implicitly
bounded by the `canonical`/`fingerprint` benchmarks it composes with.

**Execution and reporting**: `scripts/run-benchmarks.sh` reads `go test
-bench` output from stdin and writes it verbatim to
`dist/incidentdna-<version>-bench.txt` — no threshold, no pass/fail
comparison against a prior run, no regression gate. `make bench` is not
part of `make verify`'s dependency chain and not part of the CI `verify`
job's blocking steps — it runs as its own, separate,
`continue-on-error: true` CI step, uploaded as a workflow artifact.

## Vendor-neutral CI consumption example

`examples/ci-consumption-example/` demonstrates the pattern
`docs/policy-evaluation.md` already describes as IncidentDNA's intended use
from an external CI system, without making any CI vendor a dependency of
`cmd/incidentdna` or `internal/`:

1. `incidentdna suite run --report suite-report.json <suite-file>`
2. `incidentdna policy evaluate --policy <policy-file> --suite-report suite-report.json [--library <dir>]`
3. Check the exit code: `0` = release allowed, `2` = release blocked, `1` =
   environmental failure needing human attention.

`examples/ci-consumption-example/check-release-gate.sh` is a small,
portable POSIX `sh` script running exactly those two commands against
caller-supplied paths and exiting with `policy evaluate`'s own code,
unmodified. `examples/ci-consumption-example/README.md` documents the
pattern in prose and includes one illustrative (not executed by this
repository) GitHub Actions snippet. `scripts/verify-ci-consumption-example.sh`
exercises `check-release-gate.sh` against real, already-existing fixtures
(`examples/regression-suite-demo/`, `policies/release-gate-example.yaml`),
asserting the PASS path (exit 0), the FAIL path (exit 2), and the
environmental-failure path (exit 1).

This repository's own `.github/workflows/ci.yml` is not modified to depend
on this example — it runs `make example-ci-consumption` as its own,
separate, additive step, the same discipline every other checked-in demo
already follows.

## v0.1.0 compatibility contract

The frozen contract a `v0.1.0` tag promises going forward — a future
breaking change to any of these requires a new phase and a new version, not
silent redefinition.

**Frozen document `schema_version` literals:**

| Format | Literal | Introduced |
|---|---|---|
| IDIR | `0.1` (`idir.SupportedSchemaVersion`) | Phase 1 |
| Regression scenario (IRS) | `irs/v0.1` | Phase 4 |
| Scenario suite (ISM) | `suite/v0.1` | Phase 5 |
| Policy (IGP) | `policy/v0.1` | Phase 7 |

**Frozen report `schema_version` literals:**

| Report | Literal | Written by |
|---|---|---|
| Scenario execution report | `irs-report/v0.1` | `scenario run --report` |
| Suite execution report | `suite-report/v0.1` | `suite run --report` |
| Policy verdict report | `policy-report/v0.1` | `policy evaluate --report` |

**Frozen exit-code contract:**

| Command | `0` | `1` | `2` |
|---|---|---|---|
| `validate` | valid | I/O/parse/usage error | semantic validation failure |
| `fingerprint` / `inspect` | success | I/O/parse/usage error | semantic validation failure |
| `compare` | fingerprints match | I/O/parse/usage error | fingerprints differ / validation failure |
| `evidence store` | success | I/O/usage error | — |
| `evidence verify` / `inspect` | intact | I/O/usage error | MISSING/CORRUPTED finding |
| `library add` / `list` | success | I/O/usage/privacy-gate error | — |
| `library check` | occurrence found | I/O/usage error | no matching fingerprint |
| `scenario verify` / `suite verify` | valid | I/O/usage error | semantic validation failure |
| `scenario run` | `PASS` | I/O/usage/`INTERNAL_ERROR` | `FAIL`/`TIMEOUT` |
| `suite run` | aggregate `PASS` | I/O/usage/`INTERNAL_ERROR` | aggregate `FAIL` |
| `policy verify` | valid | I/O/usage error | semantic validation failure |
| `policy evaluate` | verdict `PASS` | I/O/usage/`INTERNAL_ERROR`/`INVALID` | verdict `FAIL` |
| `version` / `--version` / `-v` | always | (never fails) | — |

**What v0.1.0 does *not* freeze**: exact stdout wording (human-readable
text may still improve without a version bump — only `schema_version`
literals, JSON field names/order, and exit codes are load-bearing), the
internal `dist/`/build-artifact naming scheme (a release-process detail,
not a product format), and anything already listed as out of scope in
`docs/product-scope.md` or a known limitation below.

## Flagship end-to-end workflow

The workflow `docs/product-scope.md`, `docs/architecture.md`, and this
document all describe — proven, once, end to end, by
`scripts/verify-release-readiness.sh`, using only already-shipped commands
and already-existing, unmodified fixtures:

```
 1. incident               examples/duplicate-payment/incident.yaml
 2. canonicalization/      incidentdna validate incident.yaml
    fingerprint            incidentdna fingerprint incident.yaml
                           -> checked against testdata/golden/duplicate-payment.fingerprint
 3. evidence integrity     incidentdna evidence store / verify
                           (examples/evidence-storage-demo/)
 4. incident library       incidentdna library add / check
 5. regression scenario    incidentdna scenario verify --source / run
                           (examples/regression-scenario-demo/)
 6. scenario suite         incidentdna suite verify --library / run
                           (examples/regression-suite-demo/)
 7. library correlation    (exercised inside steps 5-6 via --library)
 8. policy evaluation      incidentdna policy verify / evaluate
                           (policies/release-gate-example.yaml)
 9. release decision       exit code of policy evaluate: 0 = PASS, 2 = FAIL
10. release-readiness      incidentdna version
    proof                  scripts/verify-release-readiness.sh
```

## Final release-readiness acceptance script

`scripts/verify-release-readiness.sh` is the single entry point that proves
every claim in this document is actually true of a given build:

1. Builds the platform matrix (`scripts/build-release.sh`), which itself
   enforces the build-twice-and-diff reproducibility check; confirms every
   expected archive exists in `dist/`.
2. Runs `scripts/generate-checksums.sh` and independently re-verifies with
   `sha256sum -c`.
3. Runs `scripts/generate-sbom.sh` and asserts the output is valid JSON
   matching the `incidentdna-sbom/v1` shape with exactly the dependencies
   `go.sum` names.
4. Runs the benchmark suite through `scripts/run-benchmarks.sh` and asserts
   non-empty output — never asserts anything about the numbers themselves.
5. Extracts the `linux-amd64` archive into a clean temporary directory and
   executes the literal quick-start sequence, asserting `incidentdna
   version` prints the expected `$VERSION`, then runs the full flagship
   workflow (stages 1-9 above) end to end, asserting the deterministic
   `PASS`/exit-0 release decision.
6. Runs `scripts/verify-ci-consumption-example.sh`, asserting both the PASS
   and FAIL exit-code paths.
7. Confirms `LICENSE` exists, is non-empty, and is present at the root of
   every generated archive.

Any single stage failure aborts with a non-zero exit and a clearly labeled
stage name. Not wired into `make verify`'s existing chain (unchanged by
Phase 8) — wired into the new, separate `make release-verify` target, since
it is meaningfully slower (it builds five platform targets twice each).

## Installation and five-minute quick start

See `README.md`, "Installing a release," for the end-user-facing version of
this section. In outline:

```
# 1. Download (pick your platform) and verify integrity
curl -LO https://github.com/SamudralaAjaykumarrr/incidentdna/releases/download/v0.1.0/incidentdna-v0.1.0-linux-amd64.tar.gz
curl -LO https://github.com/SamudralaAjaykumarrr/incidentdna/releases/download/v0.1.0/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing

# 2. Extract and run
tar xzf incidentdna-v0.1.0-linux-amd64.tar.gz
./incidentdna version

# 3. Try it against this repository's checked-in examples (clone or
#    download examples/ and policies/ alongside the binary — deliberately
#    not bundled inside the release archive)
./incidentdna validate examples/duplicate-payment/incident.yaml
./incidentdna fingerprint examples/duplicate-payment/incident.yaml
```

This is not aspirational prose — it is the literal sequence
`scripts/verify-release-readiness.sh` executes against a freshly extracted
archive.

## Makefile targets

```
make dist            # scripts/build-release.sh -> dist/*.tar.gz|.zip
make checksums       # scripts/generate-checksums.sh -> dist/SHA256SUMS
make sbom            # scripts/generate-sbom.sh -> dist/*-sbom.json
make bench           # informational benchmarks -> dist/*-bench.txt
make release-verify  # dist + checksums + sbom + bench, then
                      # scripts/verify-release-readiness.sh
```

All five are new, additive targets — `make verify`'s existing dependency
chain and pass/fail semantics are byte-for-byte unchanged. `VERSION` (env,
default `dev`) selects the injected version string, e.g. `VERSION=v0.1.0
make release-verify`. `make clean` now also removes `dist/`.

## CI changes

Every existing step in `.github/workflows/ci.yml` (`make lint`, `make
test`, `make build`, all seven `make example*` targets, the
golden-fingerprint script) is unconditionally unchanged. Additive-only new
steps/jobs:

- `make example-ci-consumption` — a new, ordinary blocking step in the
  existing `verify` job.
- `make bench` — a new, `continue-on-error: true` step in the `verify`
  job, uploaded as a workflow artifact; cannot fail the workflow.
- A new, separate `release-verify` job, scoped to a `v*` tag push or a
  manual `workflow_dispatch` — never on every ordinary push/PR, since it is
  meaningfully slower than `verify`. Uploads `dist/` as a workflow
  artifact.

## v0.1.0 release checklist

The one-time, human-executed checklist for actually cutting the `v0.1.0`
tag, followed once implementation is complete and merged:

1. Confirm every acceptance criterion in `docs/phase-8-report.md` passes on
   the exact commit being tagged.
2. Confirm the root `LICENSE` file contains the standard Apache License 2.0
   text.
3. Confirm `docs/phase-8-report.md` is written and merged.
4. Run `VERSION=v0.1.0 make release-verify` locally and confirm every stage
   passes for the exact commit being tagged.
5. Create the git tag `v0.1.0` on that commit (`git tag -a v0.1.0 -m
   "IncidentDNA v0.1.0"`) — a manual, human-confirmed action.
6. Push the tag (`git push origin v0.1.0`) — requires explicit human
   confirmation.
7. Create a GitHub Release from the tag, uploading all archives, the
   `SHA256SUMS` file, the SBOM, and the benchmark output from `dist/` as
   release assets — a manual, human-confirmed action.
8. Confirm the published release assets are independently downloadable and
   pass the quick-start flow above, performed against the actual published
   URLs (not just the local `dist/` copy).
9. Announce/close out per whatever process the maintainer already uses.

Steps 5-7 are the only irreversible, externally-visible actions in this
checklist; every step before them is local, inspectable, and repeatable
without consequence. **None of these nine steps were performed by the
Phase 8 implementation session** — no tag was created, no tag was pushed,
no GitHub Release was published. See `docs/phase-8-report.md` for the
implementation's actual, checked status against this checklist.

## Known limitations

- **No cryptographic signing.** Checksums are integrity-only; a compromised
  distribution channel could in principle serve a tampered binary with a
  matching, also-tampered `SHA256SUMS` file.
- **The Windows Job Object timeout-kill path has not been exercised on a
  real Windows host.** `windows/amd64` compiles, builds reproducibly, and
  the resulting archive/binary/checksum have been verified structurally
  (see "Windows portability fix" above and `docs/phase-8-report.md`), but
  no CI runner in this project's test matrix (`ubuntu-latest` only) runs a
  built `incidentdna.exe`, so the Job-Object-based process-tree kill in
  `internal/scenario/run_windows.go` is unverified by execution, only by
  successful compilation against the documented Win32 API contract.
- **Benchmarks are a point-in-time snapshot** with no historical
  comparison, no `benchstat`-style regression tracking, and no CI gate on
  the numbers, by design.
- **The SBOM covers only this module's own Go dependency graph** —
  `gopkg.in/yaml.v3` and its test-only transitive dependency
  `gopkg.in/check.v1`. It says nothing about base OS packages inside
  `golang:1.26.5` itself.
- **The CI consumption example is illustrative, not a hosted service.** It
  is not a published GitHub Action, a registered Bitbucket Pipe, or any
  other CI-marketplace artifact.
- **No package-manager distribution.** No Homebrew formula, no
  `apt`/`yum`/`scoop`/`winget` package.
- Every Phase 1-7 known limitation already documented in
  `docs/evidence-storage.md`, `docs/incident-library.md`,
  `docs/regression-scenarios.md`, `docs/scenario-suites.md`,
  `docs/library-crossref.md`, and `docs/policy-evaluation.md` remains true,
  unconditionally, after Phase 8.
