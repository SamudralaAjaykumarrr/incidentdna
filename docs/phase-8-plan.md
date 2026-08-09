# Phase 8 Plan: Release Readiness, Productization, and v0.1.0 (Draft for Approval)

**Status: proposed for review. No implementation has started.** No line of
Go code, no schema, no example, and no test has been written against this
plan; nothing in this document has been merged into `Makefile`, CI, or any
existing package. It follows the same role `docs/phase-4-plan.md` through
`docs/phase-7-plan.md` played before their respective implementations
began — a settled design to implement against, not an implementer's
inference. This plan document was itself produced across two sessions (an
interrupted first pass and this completion pass); §31 records that
provenance for the record.

**Phase 8 is the final planned phase of IncidentDNA.** Phases 1 through 7
built the product: document representation and fingerprinting (Phase 1),
evidence integrity (Phase 2), the incident library (Phase 3), executable
regression scenarios (Phase 4), scenario suites (Phase 5), library
cross-reference (Phase 6), and local policy evaluation (Phase 7). Phase 8
builds no new product capability — it makes the seven already-built layers
**installable, verifiable, and releasable as a versioned artifact**, and
proves, once, end-to-end, that the whole stack composes into the workflow
`docs/product-scope.md` has described since Phase 1. After Phase 8, the
codebase is tagged `v0.1.0` and this repository's planned roadmap is
complete; anything further is an unscoped, unstarted future phase (§4, §30).

## 0. Why this candidate, and the explicit scope decision behind it

Every prior phase plan (`docs/phase-4-plan.md` §0/§3, `docs/phase-5-plan.md`
§0/§3, `docs/phase-6-plan.md` §0, `docs/phase-7-plan.md` §0) inventoried the
same standing "future work" list and deliberately chose the one candidate
buildable as a pure, read-only, additive composition of already-existing
pieces, restating the rest as still out of scope. Phase 8 is different in
kind: there is no eighth product capability left on that list that meets
Phase 1 through 7's own "no new trust boundary, no new execution model, no
new transport" bar without also being one of the explicitly-named,
deliberately-deferred items (CI/CD platform integration, sandboxing, remote
storage, signing, observability ingestion, parallel execution, directory
discovery, retention/GC — `docs/product-scope.md` "Explicitly out of scope
for Phase 1 through Phase 7").

**The scope decision made for Phase 8, surfaced explicitly rather than
silently assumed:** since no new product capability is warranted, Phase 8
does not add one. Instead it closes the gap between "the product works" and
"the product can be handed to someone outside this repository as a
versioned, checksummed, installable binary with a documented five-minute
path to first use" — release engineering and productization, not new
functionality. This is the same category of decision `docs/phase-3-plan.md`
§20's "Approved decisions" round-trip and `docs/phase-7-plan.md` §0's
release-gate/policy-evaluation split both modeled: a scoping question
resolved once, explicitly, and stated in the plan rather than inferred
mid-implementation.

Concretely, a repository-wide inspection for this plan found these actual,
verifiable gaps (not hypothetical ones — each was checked against the
current tree on branch `phase-8-planning`, `main.go`, `go.mod`, `Makefile`,
`.github/workflows/ci.yml`, and `.gitignore`):

- `cmd/incidentdna/main.go`'s `commands` table has no `version` entry, and
  `run()` has no `--version`/`-v` handling — there is no way to ask a built
  binary what version it is.
- No `LICENSE` file exists at the repository root — nothing else in this
  repository can be redistributed as an OSS release without one. The
  license itself is a decided, closed question (Apache License 2.0, per
  the maintainer's decision recorded in §3 and §28); only the mechanical
  gap of adding the file remains outstanding.
- No release artifact generation exists — `make build` produces exactly one
  binary, for the host `docker compose run --rm dev` container's own
  platform, into `bin/incidentdna`, and nothing writes to `dist/`.
- No SHA-256 checksum generation exists for any build output.
- No SBOM or dependency inventory is generated anywhere.
- No performance benchmark exists anywhere in the module (`grep -rn "func
  Benchmark" .` returns nothing).
- No documented, tested "download/build a release binary and run it"
  installation flow exists — the only documented path today is `make
  build`/`make example` from a full repository clone (README.md "Quick
  start (Docker)").
- `dist/` is already listed in `.gitignore` — the logical release-artifact
  output directory already exists as a convention, unused.
- `go.mod` declares `go 1.26` and exactly one non-stdlib dependency,
  `gopkg.in/yaml.v3 v3.0.1`; `Dockerfile.dev` pins `golang:1.26.5`.
- `make build` (`Makefile:24`) runs `go build -buildvcs=false -o
  bin/incidentdna ./cmd/incidentdna` — the existing `-buildvcs=false` flag
  is a deliberate determinism decision (it suppresses embedded VCS stamping
  that would otherwise make two builds of the identical source differ) and
  every Phase 8 build path **must preserve it**, never replace it with `-X
  main.commit=...` or any other VCS-derived stamping.
- `.github/workflows/ci.yml` builds and tests the repository (`make lint`,
  `test`, `build`, all seven `example*` targets, the golden-fingerprint
  script) but never demonstrates an external caller consuming `policy
  evaluate`'s exit code the way `docs/policy-evaluation.md`'s own stated
  purpose (§22 there) describes — there is no runnable proof of the
  "vendor-neutral CI consumption" story the product's own documentation
  already promises.

These eight gaps, and only these, are what Phase 8 closes. Nothing else in
this document is new product scope.

## 1. Problem statement, objective, and current baseline

**Objective:** Release Readiness + Productization + End-to-End Acceptance +
v0.1.0 Readiness. Concretely: someone outside this repository must be able
to (a) download a checksummed, platform-appropriate binary; (b) verify its
integrity; (c) run it against a real incident in under five minutes,
exercising the full pipeline this product exists to provide; and (d) wire
its exit codes into their own CI system, without reading this codebase's
Go source. None of that is true today.

**Current baseline** (verified against branch `phase-8-planning`, same
commit as `main` at `b3f3d18`, no uncommitted changes):

- Seven complete, tested, documented phases: `internal/idir`,
  `internal/validate`, `internal/canonical`, `internal/fingerprint`,
  `internal/compare` (Phase 1); `internal/evidence` (Phase 2);
  `internal/library` (Phase 3); `internal/scenario` (Phase 4);
  `internal/suite` (Phase 5); the Phase 6 `library.CheckFingerprint`
  cross-reference; `internal/policy` (Phase 7).
- Ten CLI subcommands (`init`, `validate`, `fingerprint`, `inspect`,
  `compare`, `evidence`, `library`, `scenario`, `suite`, `policy`), all
  dispatched from the same fixed `commands` table in
  `cmd/incidentdna/main.go`.
- A stable, documented exit-code contract (`0`/`1`/`2`, README.md's "Exit
  codes are meaningful and relied on by CI" paragraph) and six frozen
  `schema_version` literals already shipping in the format:
  `idir.SupportedSchemaVersion = "0.1"`, `irs/v0.1` + `irs-report/v0.1`,
  `suite/v0.1` + `suite-report/v0.1`, `policy/v0.1` + `policy-report/v0.1`.
- A containerized dev workflow (`Dockerfile.dev`, `compose.yaml`,
  `Makefile`) that never assumes a host Go toolchain, and a CI workflow
  that runs the identical `make` targets a contributor runs locally.
- Zero release engineering: no version identifier, no license, no build
  matrix, no checksums, no SBOM, no benchmarks, no installation
  documentation outside "clone and `make build`."

Phase 8 is scoped to close exactly that second list, on top of the first,
without modifying any of it.

## 2. Actual release-readiness gaps (restated as a checklist)

Restating §0's inventory as the working checklist this plan resolves,
one-to-one, against the section that resolves it:

| # | Gap | Resolved by |
|---|---|---|
| 1 | No `version`/`--version` command | §7 |
| 2 | No `LICENSE` file | §16, §28 |
| 3 | No release artifact generation | §8, §9 |
| 4 | No SHA-256 checksum generation | §11 |
| 5 | No SBOM generation | §12 |
| 6 | No established performance benchmarks | §13 |
| 7 | No release-binary installation flow | §17 |
| 8 | No demonstrated external CI consumption of policy exit codes | §14 |

Two more items belong in the same checklist even though they were not
named as open gaps in the interrupted first planning pass, because §21's
consistency review surfaces them as necessary for a v0.1.0 tag to be
meaningful:

| # | Gap | Resolved by |
|---|---|---|
| 9 | No frozen, written compatibility contract for what "v0.1.0" promises not to break | §15 |
| 10 | No single acceptance script that proves the whole flagship workflow (§9's ten stages) end-to-end in one run | §19 |

## 3. Explicit in-scope work

1. A minimal CLI version contract: `incidentdna version` and `incidentdna
   --version`/`-v`, backed by a build-time-injected version string, default
   `dev` for every non-release build (§7).
2. A `LICENSE` file at the repository root, containing the standard
   Apache License 2.0 text. The license choice is a settled maintainer
   decision, not made by this plan: **IncidentDNA v0.1.0 uses the Apache
   License 2.0.** This closes what was previously an open decision (§28);
   only adding the file, and documenting the license where appropriate
   (§18), remains as implementation work.
3. A reproducible, deterministic, multi-platform release build strategy
   built on the existing `-buildvcs=false` discipline, producing
   deterministically named artifacts into `dist/` (§8, §9, §10).
4. SHA-256 checksum generation and verification for every release artifact
   (§11).
5. A minimal, dependency-free SBOM / dependency inventory generated from
   `go list -m -json all` (the module graph is exactly two modules —
   `gopkg.in/yaml.v3` and its own test-only dependency `gopkg.in/check.v1`,
   per `go.sum` — so no third-party SBOM tool is warranted) (§12).
6. Informational, non-gating Go benchmarks for the hot deterministic paths
   (`internal/canonical`, `internal/fingerprint`, `internal/evidence`) (§13).
7. A documented, runnable, vendor-neutral example showing an external CI
   job consuming `incidentdna policy evaluate`'s exit code — documentation
   and a local example script only, never a change to this repository's own
   `.github/workflows/ci.yml` gating and never CI-vendor-specific logic
   inside `cmd/incidentdna` (§14).
8. A written v0.1.0 compatibility contract enumerating every Phase 1–7
   public format, exit code, and CLI flag that is now frozen (§15).
9. A single, new, flagship end-to-end acceptance script exercising all ten
   stages of the workflow named in the Phase 8 objective (incident →
   fingerprint → evidence → library → scenario → suite → cross-reference →
   policy → release decision), composed entirely from already-existing,
   unmodified fixtures and commands (§9, §19).
10. A five-minute quick-start installation flow, documented and tested
    against a real built artifact, not just `make build` from a clone
    (§17).
11. Release-facing documentation: `docs/release-process.md` (new, the
    Phase 8 analog of `docs/policy-evaluation.md`), updates to `README.md`,
    `docs/architecture.md`, `docs/product-scope.md`, `docs/threat-model.md`,
    and `docs/privacy-model.md` (§18).
12. A `v0.1.0` release checklist and an explicit, checkable definition of
    DONE for this phase and for the tag itself (§29, §30).

## 4. Explicit out-of-scope work

Restated from the task's own boundary, made concrete against this
codebase's actual packages and files:

- **No Phase 9, no hosted SaaS, no remote/cloud storage, no network access
  from the CLI, no telemetry, no AI/LLM features.** Every existing "no
  network access, no telemetry" statement in `docs/threat-model.md` and
  README.md's "Security and privacy principles" remains true, unconditionally,
  after Phase 8. `internal/*` and `cmd/incidentdna/*` gain zero new imports
  of `net`, `net/http`, or any HTTP client library. The SBOM (§12) is
  generated from the already-downloaded, already-verified local module
  cache (`go.sum`-pinned) — it does not fetch anything at generation time.
- **No signing or key management.** Checksums (§11) are integrity-only,
  exactly the same "proves bytes match a declared digest, not who produced
  them or that they're trustworthy" property `docs/evidence-storage.md` and
  `docs/incident-library.md` already state for evidence and library
  digests. No GPG, no cosign, no Sigstore, no private key anywhere in this
  repository or its CI.
- **No new sandboxing technology.** `internal/scenario.Run`'s bounded,
  non-sandboxed execution model (`docs/regression-scenarios.md`) is
  unchanged; Phase 8 adds no seccomp, cgroup, container, or VM isolation
  anywhere, including in the release build process itself (still the same
  `Dockerfile.dev` container used for every prior phase's dev/test/build
  workflow).
- **No multi-tenancy.** Nothing about a release artifact, a checksum file,
  or an SBOM introduces a concept of "user" or "tenant."
- **No deployment execution.** Phase 8 produces artifacts and documents how
  to install one by hand; it does not add a deploy script, a package-manager
  publish step (no Homebrew formula, no `apt`/`yum` package, no container
  image publish), or any capability that pushes a built artifact anywhere.
  Publishing a GitHub Release from a tag (§29) is the one exception, and
  even that is a manual, human-triggered action performed once at tag time,
  not an automated deployment pipeline.
- **No CI-vendor APIs in the binary.** `internal/policy` and
  `cmd/incidentdna/cmd_policy.go` remain exactly as Phase 7 left them —
  zero bytes changed. The "vendor-neutral CI consumption example" (§14) is
  a **new example directory plus documentation only**; it calls the
  already-built `incidentdna` binary as a subprocess from a shell script,
  the same way any external CI system would, and never becomes a dependency
  of, or an import into, this module.
- **No parallel suite execution, no retention/GC.** `internal/suite.Run`'s
  strictly-sequential execution (`docs/scenario-suites.md`) and the
  incident library's "permanent record, no delete/expire capability"
  design (`docs/incident-library.md`) are both unchanged; Phase 8 touches
  neither package's `.go` files at all (§21's regression list makes this
  an explicit, checked claim, not an assumption).
- **No unrelated new product features.** No new IDIR field, no new rule
  type in `internal/validate` or `internal/policy`, no new `evidence`/
  `library`/`scenario`/`suite` subcommand or flag. Every behavioral change
  in this phase is additive at the `cmd/incidentdna` top level (`version`)
  or lives entirely outside the Go module (build scripts, CI, docs).

## 5. Architecture boundaries

Phase 8 introduces exactly **one** new Go source concept
(`cmd/incidentdna/version.go`, a package-level `var version = "dev"` string
and the `version` subcommand/flag reading it) and otherwise adds **no new
`internal/` package**. This is a deliberate departure from Phases 2–7,
each of which added or touched exactly one `internal/` package — Phase 8
is not a product-layer phase, so it does not extend the
`internal/idir → internal/validate → internal/canonical →
internal/fingerprint → internal/compare` dependency chain, nor any of the
five leaf packages built on top of it (`evidence`, `library`, `scenario`,
`suite`, `policy`). Every other Phase 8 deliverable is one of:

1. **A `cmd/incidentdna`-level addition** (`version.go`) — following the
   exact precedent every other subcommand file already sets: a `.go` file
   next to `cmd_validate.go`/`cmd_policy.go`, registered in `main.go`'s
   `commands` table, using the same `flag.NewFlagSet` idiom, returning the
   same three exit codes.
2. **Build-time metadata**, injected via `-ldflags "-X
   main.version=$VERSION"` at the point `go build` is invoked — never a
   runtime file read, never a network call, never a `git describe`/`git
   rev-parse` invocation baked into the binary (that would silently
   reintroduce the exact VCS-stamping variability `-buildvcs=false` was
   chosen to suppress). `$VERSION` is a caller-supplied string (from the
   release process, §8), not derived from the working tree at build time.
3. **Shell scripts under `scripts/`**, following the exact
   `verify-golden-fingerprint.sh`/`verify-policy-demo.sh` precedent: `set
   -eu`, POSIX `sh`, no bashisms, operate on the already-built binary in
   `bin/` or produce output under `dist/`, never touch a checked-in file.
4. **Documentation**, following the exact `docs/policy-evaluation.md`
   precedent for a new dedicated design doc, plus the same "update the
   cross-cutting docs after the code, so they describe actual behavior"
   discipline every prior phase's §17/§18 slice-ordering already
   established.
5. **`Makefile`/CI wiring**, additive only: new targets, and new CI steps,
   never a change to what an existing target or step does today.

The one architectural invariant worth stating plainly: **`internal/` gains
no new package, and none of the nine existing `internal/` packages gains a
new exported symbol.** Every one of `internal/idir`, `internal/validate`,
`internal/canonical`, `internal/fingerprint`, `internal/compare`,
`internal/evidence`, `internal/library`, `internal/scenario`,
`internal/suite`, `internal/policy` is byte-for-byte unchanged by this
phase. `docs/architecture.md`'s package-layout table gains a released-ness
note, not a new row.

## 6. End-to-end flagship workflow

The workflow the Phase 8 objective names, and the one the new acceptance
script (§19) proves runs correctly using only already-shipped commands —
no new command in this list, only a new script that chains them:

```
 1. incident               examples/duplicate-payment/incident.yaml
                            (Phase 1 IDIR v0.1 document, unmodified fixture)

 2. canonicalization/       incidentdna validate incident.yaml
    fingerprint             incidentdna fingerprint incident.yaml
                            → sha256:... (internal/canonical + internal/fingerprint,
                              unchanged; must equal the golden value in
                              testdata/golden/duplicate-payment.fingerprint)

 3. evidence integrity      incidentdna evidence store --store <tmp> <file>
                            incidentdna evidence verify --store <tmp> incident.yaml
                            (Phase 2, unchanged; examples/evidence-storage-demo/
                              fixtures)

 4. incident library        incidentdna library add --library <tmp> incident.yaml
                            incidentdna library check --library <tmp> incident.yaml
                            (Phase 3, unchanged)

 5. regression scenario     incidentdna scenario verify --source incident.yaml \
                                --library <tmp> scenario.yaml
                            incidentdna scenario run --report <tmp>/scenario.json \
                                scenario.yaml
                            (Phase 4, unchanged; examples/regression-scenario-demo/)

 6. scenario suite          incidentdna suite verify --library <tmp> suite.yaml
                            incidentdna suite run --report <tmp>/suite.json suite.yaml
                            (Phase 5, unchanged; examples/regression-suite-demo/)

 7. library correlation     (already exercised inside steps 5–6 via --library;
                            Phase 6's read-only, informational cross-reference,
                            unchanged)

 8. policy evaluation       incidentdna policy verify policies/release-gate-example.yaml
                            incidentdna policy evaluate --policy policies/release-gate-example.yaml \
                                --suite-report <tmp>/suite.json --library <tmp> \
                                --report <tmp>/verdict.json
                            (Phase 7, unchanged)

 9. deterministic release   exit code of the policy evaluate invocation above:
    decision                0 → PASS, 2 → FAIL — the same exit-code contract
                            §14's external CI example consumes, unmodified

10. (Phase 8 only) release  incidentdna version   (new, §7)
    readiness proof         scripts/verify-release-readiness.sh (new, §19)
                            — confirms the binary that just produced every
                              result above is itself a correctly identified,
                              checksummed release candidate
```

Stages 1–9 are pure composition of Phase 1–7 commands and fixtures — no
new fixture, no new document format, no modified example. Stage 10 is the
only wholly new stage, and it wraps the other nine rather than replacing
any of them. This mirrors §17 of `docs/phase-6-plan.md` and §20 of
`docs/phase-7-plan.md`'s demo-script precedent exactly: compose
already-existing, unmodified fixtures; never invent new incident content.

## 7. Minimal CLI version contract

**New behavior:**

```
incidentdna version
    Print the version string this binary was built with, one line, to
    stdout, then exit 0. Never fails.

incidentdna --version
incidentdna -v
    Equivalent to `incidentdna version`, handled before subcommand
    dispatch in run(), so it works even if no other argument is given
    (unlike every other subcommand, which requires args[0] to name it).
```

**Implementation shape** (for the eventual implementation phase, not built
now): a new package-level `var version = "dev"` in
`cmd/incidentdna/version.go`, overridden only via `-ldflags "-X
main.version=$VERSION"` at build time (§8). `make build` (the existing dev
target) is **not** changed to inject a real version — it continues to
produce a binary whose `incidentdna version` output is the literal string
`dev`, exactly like every other Go project's unversioned local build.
Only the new release build path (§8) supplies a real `$VERSION`. Output
format:

```
$ incidentdna version
incidentdna dev
$ VERSION=v0.1.0 <release build> && ./dist/.../incidentdna version
incidentdna v0.1.0
```

No JSON output, no `--build-info` flag, no embedded commit hash or build
timestamp — a single human-readable line, matching every other
informational subcommand's (`inspect`, `library list`) plain-text-first
convention, and deliberately avoiding re-introducing VCS-derived data the
existing `-buildvcs=false` decision (§0) already ruled out.

## 8. Reproducible release build strategy

**Goal:** given a fixed source tree, a fixed `$VERSION` string, and a fixed
`GOOS`/`GOARCH`/`GOARM` (where applicable), the same bytes come out every
time, on any machine with the pinned toolchain — the multi-platform
generalization of the exact property `-buildvcs=false` already gives the
single-platform `make build` today.

**Mechanism**, additive to (never replacing) the existing `make build`:

- Same toolchain: the release build runs inside the identical
  `Dockerfile.dev` image (`golang:1.26.5`) every other `make` target
  already uses — no second Docker image, no host-Go dependency introduced.
- `CGO_ENABLED=0` for every artifact (the module already has zero cgo
  dependencies — `gopkg.in/yaml.v3` is pure Go — so this changes nothing
  about what compiles, only guarantees a statically linked, fully portable
  binary per platform).
- `-trimpath` (new) — strips local filesystem build paths from the binary,
  the standard companion to `-buildvcs=false` for path-independent
  reproducibility; two checkouts at different absolute paths must produce
  byte-identical output.
- `-buildvcs=false` (existing, preserved unchanged) — no VCS stamping.
- `-ldflags "-s -w -X main.version=$VERSION"` — strips debug symbols/DWARF
  (`-s -w`, smaller deterministic output, standard for release builds) and
  injects the version string (§7). `$VERSION` is supplied by the caller
  (the person cutting the release, §29) as an explicit input, e.g.
  `VERSION=v0.1.0`, never derived from `git describe` inside the build
  itself — preserving the "no VCS-derived data in the binary" invariant
  `-buildvcs=false` already established, generalized to the version string
  too.
- One `GOOS`/`GOARCH` pair per artifact (§9), each a separate, independent
  `go build` invocation — Go's cross-compilation is native (no per-target
  toolchain or emulation needed) for `CGO_ENABLED=0` builds, so all
  platform artifacts are produced from the single `golang:1.26.5`
  container.
- Output goes to `dist/<artifact-name>/incidentdna[.exe]` (§10), never to
  `bin/` (which remains the existing single-platform dev-build output,
  unchanged).

**Reproducibility is a checked property, not an assumption:** the release
build script builds each platform target **twice** in immediate succession
into two separate temp directories and diffs the SHA-256 of each pair
before publishing anything — a failed match aborts the release build with
a non-zero exit rather than silently shipping non-reproducible output.
This is the same "don't assume, verify" discipline
`scripts/verify-golden-fingerprint.sh` already applies to the fingerprint
algorithm, applied here to the build process itself.

## 9. Supported platform artifact matrix

Chosen to cover the realistic install targets for a CLI tool consumed both
by individual engineers and by CI runners, without speculative platforms
this project has no way to test:

| GOOS | GOARCH | Artifact identifier |
|---|---|---|
| `linux` | `amd64` | `incidentdna-linux-amd64` |
| `linux` | `arm64` | `incidentdna-linux-arm64` |
| `darwin` | `amd64` | `incidentdna-darwin-amd64` |
| `darwin` | `arm64` | `incidentdna-darwin-arm64` |
| `windows` | `amd64` | `incidentdna-windows-amd64` |

Five artifacts, all `CGO_ENABLED=0`, all buildable from the single
`golang:1.26.5` container (§8) with no per-platform build environment.
`windows/amd64` gets a `.exe` suffix on the binary inside its archive;
every other platform does not. No `386`, no `arm` (32-bit), no `freebsd`,
no `riscv64` — these are plausible future additions (§27), not built now,
because nothing in this project's existing test matrix
(`.github/workflows/ci.yml` runs `ubuntu-latest` only) exercises them and
shipping an untested platform artifact would be worse than not shipping
one.

## 10. Deterministic artifact naming

Fixed, generated by the release script, never hand-typed per release:

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

- `<version>` is exactly the `$VERSION` string used in §7/§8 (e.g.
  `v0.1.0`), never a git commit hash, timestamp, or build number.
- `.tar.gz` for the four POSIX platforms, `.zip` for Windows — the
  conventional archive format per platform, chosen for compatibility with
  each OS's built-in extraction tooling (no third-party unarchive step
  required to install).
- Each archive contains exactly two files at its root: `incidentdna` (or
  `incidentdna.exe`) and a copy of `LICENSE` — no `README.md`, no `docs/`,
  keeping the artifact minimal and matching the "download, extract, run"
  quick-start flow (§17).
- `SHA256SUMS` (§11) is one file at the top of `dist/`, not per-archive,
  following the widely recognized `sha256sum`-compatible convention (a
  flat list of `<digest>␠␠<filename>` lines, one per artifact, sorted by
  filename) so `sha256sum -c SHA256SUMS` works unmodified for anyone who
  downloads it alongside the archives.
- `incidentdna-<version>-sbom.json` and `-bench.txt` are per-release, not
  per-platform (the dependency graph and the benchmark results do not vary
  by target platform in any way this project's build produces).

## 11. SHA-256 checksum generation and verification

**Generation** (`scripts/generate-checksums.sh`, new): after all five
archives exist under `dist/`, run `sha256sum` (or, on a host without it, a
`go run`-based fallback using stdlib `crypto/sha256` — no new dependency
either way) over every `dist/*.tar.gz` and `dist/*.zip`, sorted by
filename for determinism, writing `dist/SHA256SUMS`.

**Verification** — two independent paths, both documented in
`docs/release-process.md` and `README.md`'s quick start (§17):

1. **Standard tooling**: `sha256sum -c SHA256SUMS` (Linux/most POSIX
   systems) or `shasum -a 256 -c SHA256SUMS` (macOS) — no IncidentDNA
   tooling required, since this is a plain, widely-supported convention.
2. **Self-verification via the built binary itself**: `incidentdna
   version` printing the correct `$VERSION` string is a secondary,
   informational integrity signal (a checksum-tampered binary that still
   ran would still report its embedded build version, so this is *not* a
   substitute for the checksum check — stated explicitly in
   `docs/release-process.md` and `docs/threat-model.md`, §16 below).

**Explicitly not built:** GPG/cosign signing of `SHA256SUMS` itself (§4)
— the checksum file's own integrity relies on whatever transport delivers
it (an HTTPS-served GitHub Release asset), the same trust boundary every
other unsigned open-source CLI release already has, named plainly rather
than hidden.

## 12. SBOM / dependency inventory strategy

**Constraint restated from the approved decision:** no large new
dependency, no network access at generation time. This project's entire
non-stdlib dependency surface is already minimal — `go.mod` names exactly
one direct dependency (`gopkg.in/yaml.v3 v3.0.1`), and `go.sum` names
exactly one additional transitive/test-only module
(`gopkg.in/check.v1`, a test dependency of `yaml.v3` itself, never
imported by this module's own code). A general-purpose SBOM generator
(`syft`, `cyclonedx-gomod`, etc.) would be strictly disproportionate to a
two-line dependency graph and would itself become the largest new
dependency this phase adds — exactly what the decision rules out.

**Chosen mechanism**: a new `scripts/generate-sbom.sh`, using only `go
list -m -json all` (a stdlib `go` command, already available in
`Dockerfile.dev`, reads the local `go.sum`-pinned module cache, makes no
network request when the cache is already populated — which it always is
in this project's containerized dev image after the first `docker compose
build`). The script reshapes that JSON into a minimal, hand-specified SBOM
document — not a full CycloneDX or SPDX document (both are schemas this
project has no consumer for and no obligation to emit), but a small,
reviewable, deterministic JSON shape recording exactly what a two-dependency
Go module needs to disclose:

```json
{
  "format": "incidentdna-sbom/v1",
  "generated_for": "v0.1.0",
  "go_version": "go1.26.5",
  "module": "github.com/SamudralaAjaykumarrr/incidentdna",
  "dependencies": [
    {"path": "gopkg.in/yaml.v3", "version": "v3.0.1", "direct": true},
    {"path": "gopkg.in/check.v1", "version": "v0.0.0-20161208181325-20d25e280405", "direct": false}
  ]
}
```

Deterministic field order (fixed struct, not map iteration — the same
discipline every prior phase's report format already follows), sorted by
`path`. If a future dependency is ever added to `go.mod`, this file's
shape does not change — only its `dependencies` array grows, keeping the
inventory accurate without requiring a new format version. Named
`incidentdna-sbom/v1` (not `policy/v0.1`-style, since it is release
metadata, not an IDIR-family document format) to avoid implying it
participates in the IDIR schema-version compatibility contract (§15).

## 13. Meaningful informational benchmark strategy

**Constraint restated from the approved decision:** benchmarks are
informational only and must never gate CI or a release.

**What is benchmarked** — the deterministic, CPU-bound hot paths that are
actually meaningful to know the cost of, using Go's standard `testing.B`
benchmark support (already available, zero new dependency):

- `internal/canonical`: `BenchmarkMarshal` over the `duplicate-payment`
  fixture's identity payload — the operation every `fingerprint`,
  `compare`, `scenario verify --source`, and `policy evaluate` invocation
  performs at least once.
- `internal/fingerprint`: `BenchmarkCompute` over the same fixture —
  canonicalization plus SHA-256, end to end.
- `internal/evidence`: `BenchmarkStore`/`BenchmarkVerify` over a
  representative object size (e.g. 1 MiB), the store's own content-hashing
  path.
- `internal/validate`: `BenchmarkValidate` over the same fixture — the
  full semantic rule engine.

**What is deliberately not benchmarked**: `internal/scenario.Run` and
`internal/suite.Run` — their wall-clock cost is dominated by the
caller-declared child process being executed, not by this codebase's own
logic, so a benchmark of them would measure the fixture's `sleep`/`echo`
call, not this project's code, and would be actively misleading.
`internal/policy.Evaluate` is a handful of string comparisons over an
already-small report — not worth a dedicated benchmark file, though its
cost is implicitly bounded by the `internal/canonical`/`internal/
fingerprint` benchmarks it composes with.

**Execution and reporting**: `scripts/run-benchmarks.sh` (new) runs `go
test -bench=. -benchmem -run=^$ ./...` and writes raw `go test -bench`
output to `dist/incidentdna-<version>-bench.txt` (§10) verbatim — no
threshold, no pass/fail comparison against a prior run, no regression
gate. `make bench` (new target, §23) is **not** added to `verify`'s
dependency chain and **not** added to `.github/workflows/ci.yml`'s
blocking steps (§24) — consistent with the explicit decision that
benchmarks never gate CI. If CI runs benchmarks at all (optional, §24), it
is as a separate, `continue-on-error: true` informational step whose
output is uploaded as a build log artifact, never as a check that can fail
the workflow.

## 14. Vendor-neutral CI consumption example

**Constraint restated from scope**: this remains an external example/
wrapper; no CI-vendor logic goes into the IncidentDNA binary, and this
repository's own `.github/workflows/ci.yml` is not modified to add this
(that would make the example *this project's own* CI, not a demonstration
of *someone else's*).

**New directory**: `examples/ci-consumption-example/`, containing:

- `README.md` — explains the pattern in prose: run `incidentdna suite run
  --report suite-report.json <suite-file>`, then `incidentdna policy
  evaluate --policy <policy-file> --suite-report suite-report.json`, then
  check the process exit code (`0` = release allowed, `2` = release
  blocked, `1` = environmental failure needing human attention) — the
  identical contract `docs/policy-evaluation.md` §11 already documents,
  restated here as a consumption recipe rather than a command reference.
- `check-release-gate.sh` — a small, portable POSIX `sh` script (not a
  GitHub Actions YAML file, not a GitLab CI YAML file, not a Jenkinsfile —
  deliberately the lowest common denominator every CI vendor can invoke as
  a single step) that runs the two commands above against
  caller-supplied paths and `exit`s with `incidentdna policy evaluate`'s
  own code, unmodified. This is the "vendor-neutral" property: it contains
  zero references to GitHub Actions, GitLab, CircleCI, or any other
  specific platform's API or YAML schema.
- One short, illustrative **snippet** (not a live workflow file) inside
  `README.md`, fenced as a code block, showing what wiring
  `check-release-gate.sh` into a GitHub Actions step *would* look like —
  clearly labeled as an illustration a reader adapts to their own CI, never
  a file this repository itself executes.

This directory is exercised by a new script,
`scripts/verify-ci-consumption-example.sh` (§19, §22), added to `make
verify`'s chain exactly like every prior phase's `example-*` demo, proving
the example script actually produces the documented exit codes against
real fixtures — the same "a demo that isn't tested isn't trustworthy"
discipline every prior phase's `scripts/verify-*-demo.sh` already applies.

## 15. v0.1.0 compatibility contract for Phase 1–7 public formats and exit codes

Written once, here, as the frozen contract a `v0.1.0` tag promises going
forward (future phases would need to bump a version, not silently change
these). This is new — Phase 8 is the first phase to state it as an
explicit, standalone contract rather than only as "unchanged from the
prior phase" scattered across each phase plan's own §16/§17-equivalent
section.

**Frozen document `schema_version` literals** (any future breaking change
to what these parse or validate as requires a new literal, never
redefinition of an existing one):

| Format | Literal | Introduced |
|---|---|---|
| IDIR | `0.1` (`idir.SupportedSchemaVersion`) | Phase 1 |
| Regression scenario (IRS) | `irs/v0.1` | Phase 4 |
| Scenario suite (ISM) | `suite/v0.1` | Phase 5 |
| Policy (IGP) | `policy/v0.1` | Phase 7 |

**Frozen report `schema_version` literals** (JSON artifacts these commands
write, consumed by external tooling — most load-bearing for §14's CI
consumption story):

| Report | Literal | Written by |
|---|---|---|
| Scenario execution report | `irs-report/v0.1` | `scenario run --report` |
| Suite execution report | `suite-report/v0.1` | `suite run --report` |
| Policy verdict report | `policy-report/v0.1` | `policy evaluate --report` |

**Frozen exit-code contract** (restated from README.md's existing summary
paragraph, made explicit per-command as the thing v0.1.0 promises not to
renumber):

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
| `version` (new, §7) | always | (never fails) | — |

**What v0.1.0 explicitly does *not* freeze**: exact stdout wording (human-
readable text may still improve without a version bump — only the
`schema_version` literals, JSON field names/order, and exit codes are
load-bearing), the internal `dist/`/build-artifact naming scheme (§10, a
release-process detail, not a product format), and anything already listed
as out of scope (§4) or a known limitation (§27) — those remain free to be
built as genuinely new, additively-versioned capability in an unstarted
future phase without breaking this contract.

## 16. Security/privacy/threat-model consistency review

A review pass over `docs/threat-model.md` and `docs/privacy-model.md`
against every Phase 8 deliverable, conducted the same way this plan's §0
inventoried actual gaps rather than assuming none exist:

- **No new document format, no new persisted user data.** `version.go`
  reads no file and persists nothing; `dist/`'s SBOM and benchmark output
  describe this codebase's own dependency graph and performance, never a
  user's incident data — so `docs/privacy-model.md` needs no new
  redaction-relevant subsection, only a short closing note (§18) stating
  that conclusion explicitly rather than leaving Phase 8 unmentioned.
- **Checksums are integrity-only, not authenticity — stated explicitly,
  not implied.** `docs/threat-model.md` gains a new subsection ("Release
  artifact supply-chain considerations (Phase 8)") stating plainly: a
  `SHA256SUMS`-verified download proves the bytes match what the checksum
  file names, not that the checksum file itself, or the GitHub Release
  page serving it, has not been tampered with — the same class of
  residual trust every unsigned OSS release already carries, named rather
  than hidden, mirroring exactly how `docs/evidence-storage.md` and
  `docs/incident-library.md` already state "integrity, not authenticity"
  for their own digest verification.
- **The release build process introduces no new attack surface** beyond
  what `make build` already has: it runs inside the same
  `Dockerfile.dev`/`golang:1.26.5` container, over the same source tree,
  with no new third-party build tool and no network fetch beyond the
  already-existing `go.sum`-pinned module download (identical to what
  every `docker compose build dev` already does today).
- **The vendor-neutral CI example (§14) does not weaken the "no network
  access" invariant.** `check-release-gate.sh` invokes only the already-
  built local `incidentdna` binary; it makes no HTTP request, contacts no
  CI vendor API, and is explicitly documented as something a *user's own*
  CI system runs, not something this repository's CI or the `incidentdna`
  binary itself does.
- **No change to the "documents decode into a fixed typed struct" or
  "size-capped input" invariants** — Phase 8 adds no new document loader,
  so neither invariant is exercised by anything new; both remain true of
  every Phase 1–7 loader, unchanged.

Conclusion: Phase 8 introduces no new category of risk. The one genuinely
new consideration — release-artifact supply-chain trust — is scoped
identically to every prior phase's own "integrity, not authenticity"
disclosure and is written down explicitly (§18) rather than left implicit.

## 17. Installation and five-minute quick start

New `README.md` subsection ("Installing a release," inserted after the
existing "Quick start (Docker)" section, which remains the documented path
for contributors building from source):

```
# 1. Download (pick your platform)
curl -LO https://github.com/SamudralaAjaykumarrr/incidentdna/releases/download/v0.1.0/incidentdna-v0.1.0-linux-amd64.tar.gz
curl -LO https://github.com/SamudralaAjaykumarrr/incidentdna/releases/download/v0.1.0/SHA256SUMS

# 2. Verify integrity (§11)
sha256sum -c SHA256SUMS --ignore-missing

# 3. Extract and run
tar xzf incidentdna-v0.1.0-linux-amd64.tar.gz
./incidentdna version
# incidentdna v0.1.0

# 4. Try it against the checked-in example (five-minute path)
./incidentdna validate examples/duplicate-payment/incident.yaml
./incidentdna fingerprint examples/duplicate-payment/incident.yaml
./incidentdna scenario run examples/regression-scenario-demo/scenario-pass.yaml
./incidentdna policy evaluate --policy policies/release-gate-example.yaml \
    --scenario-report <report-from-previous-step>
```

Step 4 requires cloning (or downloading) this repository's `examples/` and
`policies/` directories alongside the binary — deliberately not bundled
inside the release archive (§10 keeps archives to exactly the binary plus
`LICENSE`), since these are demonstration fixtures, not part of the
product. This is stated explicitly in the quick-start text, not left
implicit. **This flow is not aspirational prose** — it is the literal
sequence `scripts/verify-release-readiness.sh` (§19) executes against a
freshly extracted archive as part of the automated acceptance check, so
the documented quick start and the tested quick start are the same steps,
never allowed to drift apart.

## 18. Release-facing documentation

**New**: `docs/release-process.md` — the Phase 8 analog of
`docs/policy-evaluation.md`/`docs/library-crossref.md`: the full design
this plan describes, written after implementation exists (§20 slice
ordering), covering the build strategy (§8), platform matrix (§9), naming
(§10), checksums (§11), SBOM (§12), benchmarks (§13), the CI consumption
example (§14), and the release checklist (§29) in one place, so a future
maintainer cutting `v0.1.1` or later has a single document to follow.

**Modified** (each gets a small, additive Phase 8 section/subsection,
following the exact "one new `## <Phase N> ...` heading per phase" pattern
already visible in every cross-cutting doc's table of contents, §0's
architecture.md/threat-model.md/privacy-model.md header inventories):

- `README.md` — new "Installing a release" subsection (§17); `version` added
  to the "CLI commands" list; Roadmap gains an "Implemented (Phase 8, this
  repository)" entry, written last, after real command transcripts exist
  (mirroring every prior phase's own discipline, `docs/phase-7-plan.md`
  §17 slice 10); "Status and known limitations" gains a short Phase 8
  paragraph; "Repository structure" mentions `dist/` and
  `examples/ci-consumption-example/`; a short "License" statement (Apache
  License 2.0, linking to the root `LICENSE` file) is added, following
  whatever section README.md already uses for that kind of front-matter.
- `docs/architecture.md` — new "Release artifacts and reproducible builds
  (Phase 8)" section, describing the build/artifact/checksum/SBOM
  pipeline as a data-flow diagram in the same style as its existing "Data
  flow" section, explicit that it sits entirely outside the
  `idir → ... → policy` package chain (§5).
- `docs/product-scope.md` — new "Phase 8 scope" section following the
  exact template every prior phase section already uses; "Explicitly out
  of scope" heading retitled "for Phase 1 through Phase 8"; the out-of-scope
  list gains the release-facing items from §4 that are genuinely new
  namings (signing/key management, deployment execution) rather than
  restated pre-existing ones.
- `docs/threat-model.md` — new "Release artifact supply-chain
  considerations (Phase 8)" subsection (§16).
- `docs/privacy-model.md` — new short "Phase 8" subsection stating
  explicitly that release engineering introduces no new document field, no
  new persisted user data, and no change to redaction/sensitivity handling
  (§16) — a deliberate, written "no-op" entry rather than a silent gap in
  the document's own per-phase inventory.
- `docs/phase-8-report.md` — **not written now.** Per every prior phase's
  own §17/§20-equivalent slice ordering, the report is written last, after
  real command transcripts and a real tagged build exist to describe. This
  plan names it as a required future deliverable (§20 slice 10, §29) but
  does not draft it.

## 19. Final release-readiness acceptance script

**New**: `scripts/verify-release-readiness.sh` — the single entry point
that proves every Phase 8 claim in this document is actually true of a
given build, mirroring `scripts/verify-golden-fingerprint.sh`'s role as
the final, non-negotiable check in `make verify`'s chain, but scoped one
level up: where `verify-golden-fingerprint.sh` proves one algorithm's
output is unchanged, this script proves the *release itself* is
trustworthy. It:

1. Builds the full five-platform matrix (§8, §9) into a temporary `dist/`
   copy, verifying the build-twice-and-diff reproducibility check (§8)
   passes for every platform.
2. Runs `scripts/generate-checksums.sh` and independently re-verifies
   every checksum with `sha256sum -c` (or platform equivalent).
3. Runs `scripts/generate-sbom.sh` and asserts the output is valid JSON
   matching the `incidentdna-sbom/v1` shape (§12) with exactly the
   dependencies `go.sum` names.
4. Runs `scripts/run-benchmarks.sh` and asserts it completes and produces
   non-empty output — **never** asserts anything about the numbers
   themselves (§13's "informational only, never gating" constraint,
   enforced structurally, not just by convention).
5. Extracts the `linux-amd64` archive (the platform CI itself runs on)
   into a clean temporary directory and executes the literal quick-start
   sequence from §17, asserting `incidentdna version` prints the expected
   `$VERSION`, and that the flagship workflow (§6, stages 1–9) completes
   with the expected exit codes end to end.
6. Runs `scripts/verify-ci-consumption-example.sh` (§14) and asserts both
   the PASS and FAIL exit-code paths.
7. Confirms `LICENSE` exists, is non-empty, and is included in every
   generated archive (§10).

Any single failure aborts with a non-zero exit and a clear stage label —
the same "fail loudly, name the stage" discipline
`scripts/verify-golden-fingerprint.sh` already applies to a single check,
generalized to this script's seven. This script is **not** wired into
`make verify`'s existing chain (that chain stays exactly as it is today,
per §21's regression requirement) — it is wired into a new, separate `make
release-verify` target (§23), run explicitly when preparing a release, not
on every ordinary `make verify` invocation, since it is meaningfully
slower (it builds five platform targets twice each).

## 20. Implementation slices in exact order

Following the exact "each slice independently mergeable and testable"
discipline every prior phase plan established (`docs/phase-7-plan.md`
§17):

1. `cmd/incidentdna/version.go` + `cli_version_test.go` — the `version`
   subcommand and `--version`/`-v` flag (§7), `main.go`'s `commands` table
   and `run()` updated, `printUsage()` updated. Independently testable via
   `go test ./cmd/incidentdna/... -run TestVersion -v`.
2. `LICENSE` — the standard Apache License 2.0 text, per the maintainer's
   already-settled license decision (§3, §28); blocks nothing else in this
   list, since every other slice is license-independent, but blocks the
   release checklist (§29) until added.
3. `scripts/build-release.sh` — the reproducible multi-platform build
   (§8, §9), including the build-twice-and-diff check, writing to `dist/`
   with the naming from §10. Testable standalone by running it and
   inspecting `dist/`.
4. `scripts/generate-checksums.sh` (§11) — depends on slice 3's output
   existing; independently testable against any pre-populated `dist/`.
5. `scripts/generate-sbom.sh` (§12) — independent of slices 3–4, testable
   standalone against the repository's own `go.sum`.
6. Benchmark functions: `internal/canonical/marshal_bench_test.go`,
   `internal/fingerprint/compute_bench_test.go`,
   `internal/evidence/store_bench_test.go`,
   `internal/validate/validate_bench_test.go` (§13) +
   `scripts/run-benchmarks.sh` — independent of every other slice, each
   `_bench_test.go` file addable and runnable in isolation
   (`go test -bench=. -run=^$ ./internal/canonical/...`).
7. `examples/ci-consumption-example/` (README.md, `check-release-gate.sh`)
   + `scripts/verify-ci-consumption-example.sh` (§14) — depends only on
   already-existing Phase 4/5/7 fixtures and commands, independent of
   slices 1–6.
8. `scripts/verify-release-readiness.sh` (§19) — depends on slices 1–7 all
   existing (it exercises every one of them); written after they land.
9. `Makefile` (§23) and `.github/workflows/ci.yml` (§24) wiring — new
   targets/steps only, added once slices 1–8 exist and pass standalone.
10. `docs/release-process.md` (new) + updates to `README.md`,
    `docs/architecture.md`, `docs/product-scope.md`, `docs/threat-model.md`,
    `docs/privacy-model.md` (§18) — done after the code, so they describe
    actual behavior, matching every prior phase's own documentation-last
    discipline.
11. `docs/phase-8-report.md` — written last, after real command
    transcripts and (if the checklist in §29 is executed) a real tagged
    build exist to record, mirroring `docs/phase-7-report.md`'s role.

Slices 1, 6, and 7 are ordinary Go changes verified by `go test ./...
-race -count=1` alongside the full existing suite. Slices 3–5 and 8 are
shell scripts verified by running them directly. Slice 9 should be checked
by confirming `make verify`'s existing behavior is byte-for-byte unchanged
(§21) before wiring the new, separate targets alongside it.

## 21. Exact files expected to be added

```
cmd/incidentdna/
  version.go
  cli_version_test.go

internal/canonical/
  marshal_bench_test.go

internal/fingerprint/
  compute_bench_test.go

internal/evidence/
  store_bench_test.go

internal/validate/
  validate_bench_test.go

scripts/
  build-release.sh
  generate-checksums.sh
  generate-sbom.sh
  run-benchmarks.sh
  verify-ci-consumption-example.sh
  verify-release-readiness.sh

examples/ci-consumption-example/
  README.md
  check-release-gate.sh

docs/
  release-process.md
  phase-8-report.md        (written last, per §20 slice 11)

LICENSE
```

## 22. Exact files expected to be modified

```
cmd/incidentdna/main.go   — one new {"version", runVersion} entry, --version/-v
                             handling in run(), printUsage() updated

README.md                 — CLI commands list, new "Installing a release"
                             section, Roadmap Phase 8 entry, "Status and
                             known limitations" Phase 8 paragraph,
                             "Repository structure" mentions dist/ and the
                             new example directory

docs/architecture.md      — new "Release artifacts and reproducible builds
                             (Phase 8)" section

docs/product-scope.md     — new "Phase 8 scope" section; out-of-scope
                             heading and list updated to "Phase 1 through
                             Phase 8"

docs/threat-model.md      — new "Release artifact supply-chain
                             considerations (Phase 8)" section

docs/privacy-model.md     — new short "Phase 8" subsection (explicit no-op
                             statement)

Makefile                  — new targets: dist, checksums, sbom, bench,
                             release-verify (none added to verify's
                             existing dependency chain)

.github/workflows/ci.yml  — no change to existing blocking steps; at most
                             one new, separate, continue-on-error
                             informational benchmark step (§24) — genuinely
                             optional, decided during implementation, not
                             mandated by this plan

.gitignore                — no new entry expected (dist/ is already
                             present); confirmed, not assumed, during
                             implementation
```

**Untouched (explicit)**: every file under `internal/idir/`,
`internal/validate/` (except the one new `_bench_test.go` file, which adds
a benchmark and changes no existing test or non-test file),
`internal/canonical/` (same caveat), `internal/fingerprint/` (same
caveat), `internal/compare/`, `internal/evidence/` (same caveat),
`internal/library/`, `internal/scenario/`, `internal/suite/`,
`internal/policy/`; every existing `cmd/incidentdna/cmd_*.go` and
`cli_*_test.go` file; every file under `examples/duplicate-payment/`,
`examples/evidence-storage-demo/`, `examples/incident-library-demo/`,
`examples/regression-scenario-demo/`, `examples/regression-suite-demo/`;
`policies/release-gate-example.yaml`; every existing `scripts/verify-*.sh`
file; `testdata/golden/`; `go.mod`, `go.sum` (no new dependency, §4);
`Dockerfile.dev`, `compose.yaml`.

## 23. Makefile changes

New targets, each additive, none inserted into `verify`'s existing
dependency chain (`lint test build example example-evidence
example-library example-scenario example-suite example-library-crossref
example-policy`, followed by the golden-fingerprint script) — that chain
remains **exactly** what it is today:

```makefile
dist: build
	$(RUN) sh scripts/build-release.sh

checksums: dist
	$(RUN) sh scripts/generate-checksums.sh

sbom:
	$(RUN) sh scripts/generate-sbom.sh

bench:
	$(RUN) go test -bench=. -benchmem -run=^$$ ./... | tee /dev/stderr | sh scripts/run-benchmarks.sh

release-verify: dist checksums sbom bench
	$(RUN) sh scripts/verify-release-readiness.sh
```

`clean` (existing) gains `dist` to its `rm -rf` target list
(`rm -rf bin dist`), the one genuinely necessary edit to an existing
target — everything `dist`-producing this phase adds must also be
cleanable the same way `bin/` already is.

## 24. CI changes

**Unconditionally unchanged**: every existing step in
`.github/workflows/ci.yml` — `make lint`, `make test`, `make build`, all
seven `make example*` targets, and the golden-fingerprint script — keeps
running on every push to `main`/`phase-*` and every PR, exactly as today,
with the exact same pass/fail semantics. This is the single most important
regression requirement in this whole plan (§25).

**New, additive, and explicitly optional** (a decision left open for
implementation time, named here so it is not silently skipped):

- A new, separate step (or separate job) running `make release-verify`
  — **not on every push**, but scoped to either a manually-triggered
  workflow_dispatch or tag-push (`v*`) trigger, since it is meaningfully
  slower than the existing suite (five platform builds, doubled for the
  reproducibility check) and is only actually needed at release-cut time,
  not on every commit.
- Optionally, a lightweight `make bench` step on ordinary pushes, marked
  `continue-on-error: true` so it can never fail the workflow, with its
  output uploaded as a workflow artifact for a maintainer to read — purely
  informational, consistent with §13's constraint.

Neither of these two additions is required for Phase 8 to be DONE (§30);
they are named as the natural CI-side home for §19/§13's scripts, decided
concretely during implementation, not mandated as a hard requirement of
this plan the way §25's regression requirement is.

## 25. Phase 1–7 regression requirements

Every one of these must hold, checked mechanically, not asserted by
prose, before Phase 8 is considered done (§30):

- `git diff --stat -- internal/idir internal/validate internal/canonical
  internal/fingerprint internal/compare internal/evidence internal/library
  internal/scenario internal/suite internal/policy` must show **zero**
  non-test-file changes; the four new `_bench_test.go` files (§21) are the
  only permitted additions under any of these directories, and each must
  be a wholly new file, never an edit to an existing one.
- `git diff --stat -- examples policies testdata` must be **empty** — no
  existing fixture, example, or golden file is modified.
- `go test ./... -race -count=1` passes, including every existing test
  unmodified, plus the new `cli_version_test.go` and the four new
  benchmark files (benchmarks run under `-run=^$`, so `go test` without
  `-bench` does not execute them at all, and their compile-only presence
  must not slow or break the existing race-tested suite).
- `scripts/verify-golden-fingerprint.sh` continues to pass, producing the
  unchanged golden value in `testdata/golden/duplicate-payment.fingerprint`.
- `make verify`'s existing seven `example-*` targets and the golden check
  all pass unmodified, with `make verify`'s own target list and dependency
  order **unchanged** — `dist`, `checksums`, `sbom`, `bench`, and
  `release-verify` are new, parallel targets, never inserted into
  `verify`'s chain.
- Every existing CLI subcommand's documented exit-code and output behavior
  (§15's frozen contract) is unchanged — proven by the unmodified existing
  `cli_*_test.go` suite passing as-is.
- `main.go`'s existing ten `commands` table entries keep their existing
  `name`/`run` pairing; `version` is appended, never inserted before or
  interleaved with them, preserving the existing dispatch order for any
  caller that (unusually) depends on it.

## 26. Known limitations (anticipated up front)

- **No cryptographic signing.** Checksums are integrity-only; a
  compromised distribution channel (a compromised GitHub account, a
  mirrored download site) could serve a tampered binary with a matching,
  also-tampered `SHA256SUMS` file. Explicitly out of scope (§4); named
  here and in `docs/threat-model.md` (§16, §18) rather than implied away.
- **Five platforms only.** `386`, 32-bit `arm`, `freebsd`, and other GOOS/
  GOARCH combinations Go itself supports are not built or tested, because
  this project's own CI and dev workflow have never exercised them (§9).
- **Benchmarks are a point-in-time snapshot with no historical
  comparison.** `run-benchmarks.sh` records one run's numbers; there is no
  regression-tracking database, no `benchstat`-style comparison against a
  previous release, and no CI gate on the numbers (§13, by design).
- **The SBOM covers only this module's own Go dependency graph** —
  `gopkg.in/yaml.v3` and its test-only transitive dependency
  `gopkg.in/check.v1`. It says nothing about the base OS packages inside
  `golang:1.26.5` itself; a from-scratch OS-level SBOM of the build
  container is not built (disproportionate to a `CGO_ENABLED=0` static Go
  binary, which does not link against, or ship, anything from that base
  image).
- **The CI consumption example is illustrative, not a hosted service.**
  `examples/ci-consumption-example/` proves the pattern works against this
  project's own fixtures; it is not a published GitHub Action, a
  registered Bitbucket Pipe, or any other CI-marketplace artifact — a
  reader adapts the shown script into their own pipeline by hand.
- **No package-manager distribution.** No Homebrew formula, no `apt`/
  `yum`/`scoop`/`winget` package — installation is "download an archive,
  verify its checksum, extract it" (§17), not a package-manager command.
- **License choice is a settled maintainer decision, not an open one.**
  IncidentDNA v0.1.0 uses the Apache License 2.0 (§3, §28). What remains is
  purely mechanical implementation work — adding the root `LICENSE` file
  with the standard Apache-2.0 text and documenting the license where
  appropriate (§18) — not a decision Phase 8 is waiting on.
- Every Phase 1–7 known limitation already documented in
  `docs/evidence-storage.md`, `docs/incident-library.md`,
  `docs/regression-scenarios.md`, `docs/scenario-suites.md`,
  `docs/library-crossref.md`, and `docs/policy-evaluation.md` remains true,
  unconditionally, after Phase 8 — release engineering does not close any
  of them, and this document does not restate them individually beyond
  this pointer, to avoid the list drifting out of sync with the six
  documents that are each format's own source of truth.

## 27. Complete acceptance criteria

Phase 8's implementation is acceptable when, and only when, every one of
these is independently verifiable, not merely asserted:

1. `incidentdna version` and `incidentdna --version`/`-v` all print the
   build's injected version string (or `dev` for `make build`) and exit
   `0`, with a passing black-box test proving it.
2. A `LICENSE` file exists at the repository root, contains the standard,
   unmodified Apache License 2.0 text (the maintainer-decided license,
   §3), and is included in every generated release archive.
3. `scripts/build-release.sh` produces all five platform artifacts named
   in §9/§10, and the build-twice-and-diff reproducibility check (§8)
   passes for each.
4. `scripts/generate-checksums.sh` produces a `SHA256SUMS` file that
   `sha256sum -c` independently verifies against the five archives.
5. `scripts/generate-sbom.sh` produces a valid `incidentdna-sbom/v1` JSON
   document listing exactly the dependencies in `go.sum`.
6. `scripts/run-benchmarks.sh` runs to completion and produces non-empty,
   readable benchmark output for all four new benchmark files, and is
   proven, structurally, not to be part of any gating path.
7. `examples/ci-consumption-example/check-release-gate.sh` exits `0` for a
   passing policy evaluation and `2` for a failing one, against real
   fixtures, proven by `scripts/verify-ci-consumption-example.sh`.
8. `docs/release-process.md` exists and fully documents §8–§14 and §29.
9. `scripts/verify-release-readiness.sh` passes end to end, exercising the
   full flagship workflow (§6) against a freshly extracted `linux-amd64`
   archive.
10. `make release-verify` runs the above from a clean checkout with no
    manual steps beyond having Docker available (the same "no host Go
    required" invariant every other `make` target already has).
11. §25's regression requirements all hold, checked mechanically.
12. `docs/phase-8-report.md` exists, written after the above, recording
    real command transcripts (mirroring every prior phase's `*-report.md`).

## 28. Exact v0.1.0 release checklist

Distinct from §27 (which accepts the *implementation*): this is the
one-time, human-executed checklist for actually cutting the `v0.1.0` tag,
to be followed once implementation (§20's eleven slices) is complete and
merged:

1. Confirm §27's twelve acceptance criteria all pass on the exact commit
   being tagged.
2. Confirm the root `LICENSE` file contains the standard Apache License 2.0
   text — the license itself is already decided (Apache License 2.0, §3,
   §26); this step is a mechanical presence/correctness check, not an open
   decision to make at tag time.
3. Confirm `docs/phase-8-report.md` (§18, §20 slice 11) is written and
   merged.
4. Run `VERSION=v0.1.0 make release-verify` locally (or via the
   tag-triggered CI workflow, if built per §24) and confirm every stage in
   §19 passes for the exact commit being tagged.
5. Create the git tag `v0.1.0` on that commit (`git tag -a v0.1.0 -m
   "IncidentDNA v0.1.0"`) — a manual, human-confirmed action, not scripted
   or automatic.
6. Push the tag (`git push origin v0.1.0`) — requires explicit human
   confirmation per this project's standing "confirm before push" practice;
   not performed by this plan or by any script.
7. Create a GitHub Release from the tag, uploading all five archives, the
   `SHA256SUMS` file, the SBOM, and the benchmark output from `dist/`
   (§10) as release assets — a manual, human-confirmed action.
8. Confirm the published release assets are independently downloadable and
   pass §17's documented quick-start flow, performed against the actual
   published URLs (not just the local `dist/` copy) as a final live check.
9. Announce/close out per whatever process the maintainer already uses for
   this repository (outside this plan's scope to prescribe).

Steps 5–7 are the only irreversible, externally-visible actions in this
entire checklist; every step before them is local, inspectable, and
repeatable without consequence.

## 29. Explicit definition of DONE

Phase 8, and the IncidentDNA planned roadmap as a whole, are DONE when:

- Every item in §27's acceptance criteria is checked and true.
- Every item in §25's regression requirements is checked and true.
- §28's release checklist has been executed at least once, producing a
  real, tagged, published `v0.1.0` GitHub Release with five checksummed
  archives, an SBOM, and benchmark output attached.
- `docs/phase-8-report.md` exists, documenting the real implementation
  (mirroring `docs/phase-1-report.md` through `docs/phase-7-report.md`'s
  role exactly) and explicitly states the tag was cut and published.
- `README.md`'s Roadmap section lists Phase 8 under "Implemented," and its
  "Future work" paragraph is rewritten to state plainly that no further
  phase is currently planned — the same explicit "nothing further is
  implied" discipline `docs/phase-7-plan.md` §22 already models for its
  own narrower boundary, generalized here to the whole roadmap.
- No `internal/` package outside the four new `_bench_test.go` additions
  has changed (§21, §25) — the product's seven-phase core is byte-for-byte
  the same code that existed before Phase 8 began, now released rather
  than only built.

Explicitly, Phase 8 is **not** done when only the code merges — it
requires the tag to exist and the release to be published (§28), since
"release readiness" that never results in an actual release has not
demonstrated the thing it claims to enable.

## 30. Future boundary (restated, for the roadmap as a whole)

Named explicitly, the same way `docs/phase-4-plan.md` §28,
`docs/phase-5-plan.md` §26, `docs/phase-6-plan.md` §20, and
`docs/phase-7-plan.md` §22 each named their own phase's boundary before it
existed — generalized here, once, for the entire project, since Phase 8 is
the last currently-planned phase:

Nothing in this plan, and nothing Phase 8 implements, begins, scopes, or
implies any of: a Phase 9; a hosted/SaaS version of IncidentDNA; remote or
cloud storage for the evidence store, incident library, scenarios, suites,
or policies; network access or telemetry of any kind from the
`incidentdna` binary; AI/LLM-assisted incident authoring or scenario
generation; artifact/evidence/library signing or key management; container,
VM, or seccomp-based sandboxing of scenario execution; multi-tenancy or
access control; automated deployment of built artifacts anywhere; CI-vendor
APIs inside the binary; parallel suite execution; directory/glob-based
scenario discovery; or retention/garbage collection for the incident
library. Every one of these remains exactly what `docs/product-scope.md`
already calls it: a real future need, named, not built, and — as of this
plan — not even scheduled. A future maintainer choosing to pursue any of
them starts a genuinely new, unscoped planning effort, the same discipline
this project has applied to every phase from Phase 1 onward.

## 31. Provenance of this planning document

This plan was produced across two sessions on branch `phase-8-planning`.
The first session performed the repository-wide gap analysis reflected in
§0–§2 and began drafting this file, then was interrupted mid-response by a
connection error before any file was written to disk — `git status`,
`git branch --show-current`, and a direct check for `docs/phase-8-plan.md`
at the start of this session all confirmed no partial file existed on disk
to preserve or reconcile; the working tree was clean and the file was
simply absent. This session re-derived the same gap inventory
independently, by inspecting `main.go`, `go.mod`, `Makefile`,
`.github/workflows/ci.yml`, `.gitignore`, and every existing
`docs/phase-*.md`/`docs/*-model.md`/`docs/product-scope.md` file directly
against the current tree, and found it consistent with the eight gaps and
the design decisions (final phase, `phase-8-release-readiness` branch name,
non-gating benchmarks, dependency-free SBOM) recorded as already-decided in
the interrupted session's brief, before writing this document in full. No
Go source, test, example, policy, `Makefile`, or CI file was read as
untrusted — only as reference for what this plan must remain consistent
with — and none of them was modified in the writing of this plan.
