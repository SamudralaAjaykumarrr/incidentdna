# Phase 1 Report

This report records what was actually run to verify Phase 1, with real
command output. Nothing below is invented; every block is a transcript from
this implementation session.

## Environment

The sandbox this was built in has no `docker`, `go`, or `podman` binaries,
and no passwordless `sudo`. Per `docs/phase-1-plan.md`, verification was
done with a Go toolchain downloaded to user space instead:

```
$ curl -s https://go.dev/VERSION?m=text
go1.26.5

$ tar -xzf go1.26.5.linux-amd64.tar.gz -C $HOME/.local && mv $HOME/.local/go $HOME/.local/go1.26.5
$ export PATH="$HOME/.local/go1.26.5/bin:$PATH"
$ export GOPATH="$HOME/.local/gopath"
$ go version
go version go1.26.5 linux/amd64
```

`Dockerfile.dev` is pinned to `FROM golang:1.26.5` — the exact version
verified here. **Docker itself was never exercised in this sandbox** —
`compose.yaml` and the Makefile's `docker compose run` invocations were
sanity-checked without a daemon:

```
$ python3 -c "import yaml; print(yaml.safe_load(open('compose.yaml')))"
{'services': {'dev': {'build': {'context': '.', 'dockerfile': 'Dockerfile.dev'}, 'working_dir': '/workspace',
 'volumes': ['.:/workspace', 'gomodcache:/go/pkg/mod', 'gobuildcache:/root/.cache/go-build']}},
 'volumes': {'gomodcache': None, 'gobuildcache': None}}
# valid YAML, expected shape

$ make -n lint
docker compose run --rm --remove-orphans dev sh -c '\
	unformatted="$(gofmt -l .)"; \
	if [ -n "$unformatted" ]; then \
		echo "gofmt: the following files are not formatted:" >&2; \
		echo "$unformatted" >&2; \
		exit 1; \
	fi; \
	go vet ./...'
# (make -n dry-run expansion for every target — format/lint/test/build/example/verify/clean —
#  all expanded to the intended docker compose invocation with no syntax errors)
```

Everything below this point ran for real against the local Go 1.26.5
toolchain, which is what `docker compose run --rm dev <cmd>` would run
inside the container.

## `make format` (gofmt -w)

```
$ gofmt -w .
$ gofmt -l .
(no output — nothing left to reformat)
```

## `make lint` (gofmt -l check + go vet)

```
$ gofmt -l .
(no output — clean)

$ go vet ./...
$ echo "exit: $?"
exit: 0
```

## `make test` (go test ./... -race -count=1)

```
$ go test ./... -race -count=1
ok  	github.com/SamudralaAjaykumarrr/incidentdna/cmd/incidentdna	2.605s
ok  	github.com/SamudralaAjaykumarrr/incidentdna/internal/canonical	1.012s
ok  	github.com/SamudralaAjaykumarrr/incidentdna/internal/compare	1.013s
ok  	github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint	1.026s
ok  	github.com/SamudralaAjaykumarrr/incidentdna/internal/idir	1.055s
ok  	github.com/SamudralaAjaykumarrr/incidentdna/internal/validate	1.019s
```

All 46 test functions/subtests passed, including (full `-v` transcript was
reviewed, not just the summary):

- `internal/validate`: one subtest per VALIDATION REQUIREMENTS bullet
  (unsupported schema version, missing incident id, missing/empty business
  invariants, dangling causal ref, duplicate event id, cyclic causal
  relationship, evidence digest without prefix / wrong length, unknown
  sensitivity, repro step referencing nonexistent event, redacted-but-dirty,
  empty expected corrected behavior) — 15 subtests, all pass — plus a
  positive "valid document has no issues" test, a longer (3-node) cycle
  test, and a context-cancellation test.
- `internal/canonical`: key-order independence, whitespace independence,
  content-change sensitivity, number formatting, trailing-data rejection,
  struct-tag usage, array-order significance.
- `internal/fingerprint`: determinism, identity-metadata independence,
  invariant-order independence, causal-structure sensitivity, invariant/
  side-effect-type/technology-category/recovery-behavior content
  sensitivity, and the golden test against the duplicate-payment example.
- `internal/idir`: YAML load, JSON load, extension sniffing, oversized-
  document rejection, missing-file rejection, malformed-YAML rejection.
- `internal/compare`: identical-despite-different-record-identity,
  invariant-difference reporting, causal-structure-difference reporting.
- `cmd/incidentdna`: builds the real binary and runs it against the real
  example and all 12 invalid fixtures, `init`'s overwrite protection and
  `--force`, and that the `init` template itself validates cleanly.

## `make build`

```
$ go build ./...
$ echo "exit: $?"
exit: 0

$ go build -o bin/incidentdna ./cmd/incidentdna
$ echo "exit: $?"
exit: 0
$ ls -la bin/
-rwxrwxr-x 1 ajay ajay 4428556 ... bin/incidentdna
```

## `make example`

```
$ ./bin/incidentdna validate examples/duplicate-payment/incident.yaml
OK: examples/duplicate-payment/incident.yaml is a valid IDIR v0.1 document

$ ./bin/incidentdna fingerprint examples/duplicate-payment/incident.yaml
sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
```

## `make verify` (golden fingerprint script)

```
$ sh scripts/verify-golden-fingerprint.sh
verify-golden-fingerprint: OK (sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e)
```

## CLI run against the duplicate-payment example: `inspect` and `compare`

```
$ ./bin/incidentdna inspect examples/duplicate-payment/incident.yaml
Incident:     INC-2026-0417 — Duplicate payment charge after message redelivery
Schema:       0.1
Valid:        yes
Occurred:     2026-04-17T09:12:03Z
Application:  orbitcart / payment-worker (production, release 2.14.0)

Trigger:      [message-redelivery] The message broker redelivered an order-created message ...

Event timeline (5):
  [evt-order-created-consumed] message-consumed (caused by: -)
  [evt-charge-committed] external-charge-committed (caused by: evt-order-created-consumed)
  [evt-worker-crash] process-terminated (caused by: evt-charge-committed)
  [evt-message-redelivered] message-redelivered (caused by: evt-worker-crash)
  [evt-second-charge-committed] external-charge-committed (caused by: evt-message-redelivered)
...
Fingerprint:  sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e

$ ./bin/incidentdna compare examples/duplicate-payment/incident.yaml examples/duplicate-payment/incident.yaml
IDENTICAL: examples/duplicate-payment/incident.yaml and examples/duplicate-payment/incident.yaml share the same normalized incident fingerprint
sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e
```

`init` was also run for real in a scratch directory: first `init` succeeds
and creates `.incidentdna/incident.yaml`; a second `init` without `--force`
fails with `... already exists; use --force to overwrite` (exit 1);
`init --force` then succeeds; and `validate` on the generated template
reports it as a valid IDIR v0.1 document.

## Proof 1: formatting-only copy → identical fingerprint

The example was re-serialized as JSON with sorted keys and 4-space
indentation (via Python's `json.dump(doc, f, indent=4, sort_keys=True)`) —
different key order, different whitespace, different format (YAML → JSON)
entirely:

```
$ ./bin/incidentdna fingerprint examples/duplicate-payment/incident.yaml
sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e

$ ./bin/incidentdna fingerprint /tmp/proofs/reformatted.json
sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e

$ diff <(./bin/incidentdna fingerprint examples/duplicate-payment/incident.yaml) \
       <(./bin/incidentdna fingerprint /tmp/proofs/reformatted.json) \
  && echo "MATCH: fingerprints identical"
MATCH: fingerprints identical
```

## Proof 2: causal-structure change → different fingerprint

The causal edge from `evt-worker-crash` to `evt-message-redelivered` was
removed (`caused_by: [evt-worker-crash]` → `caused_by: []`) — a materially
different causal mechanism, same event types otherwise:

```
$ FP_ORIG=$(./bin/incidentdna fingerprint examples/duplicate-payment/incident.yaml)
$ echo "$FP_ORIG"
sha256:fc3dac016d31dcca1a58c0242c45474107dd69c1e7718d9cdebdbe357bca2a8e

$ FP_CHANGED=$(./bin/incidentdna fingerprint /tmp/proofs/causal-changed.yaml)
$ echo "$FP_CHANGED"
sha256:d1f0281ffe2583ee9e477fbad11215ac844acf8feec1eae0e3b467c147bd5524

$ [ "$FP_ORIG" != "$FP_CHANGED" ] && echo "DIFFER: fingerprints differ as expected"
DIFFER: fingerprints differ as expected
```

## Proof 3: cyclic causality → rejected

`testdata/golden/invalid/cyclic-causality.yaml` sets `evt-1.caused_by:
[evt-2]` while `evt-2.caused_by: [evt-1]` already exists — a 2-node cycle:

```
$ ./bin/incidentdna validate testdata/golden/invalid/cyclic-causality.yaml
incidentdna: testdata/golden/invalid/cyclic-causality.yaml is not a valid IDIR document:
  - events: cyclic causal relationship detected: evt-1 -> evt-2 -> evt-1
$ echo "exit code: $?"
exit code: 2
```

Exit code `2` is the documented "semantic validation failure" code (see
`docs/architecture.md`).

## Dependency and network checks

```
$ cat go.mod
module github.com/SamudralaAjaykumarrr/incidentdna
go 1.26
require gopkg.in/yaml.v3 v3.0.1
# `go mod tidy` run — no changes; gopkg.in/yaml.v3 is the sole dependency.

$ grep -rn '"net' cmd/ internal/
no net imports found
$ grep -rln "os.Getenv" cmd/ internal/
no telemetry-like env reads
```

## Git state

```
$ git status --short
?? .github/
?? CLAUDE.md
?? Dockerfile.dev
?? Makefile
?? cmd/
?? compose.yaml
?? docs/
?? examples/
?? go.mod
?? go.sum
?? internal/
?? schemas/
?? scripts/
?? testdata/
```

`bin/` does not appear — it's covered by the pre-existing `.gitignore`. No
`git add`, `git commit`, `git push`, or git-config command was run at any
point in this session. `/home/ajay/projects/omniflow` was never read or
written.

## Known limitations

- **Docker path unverified end-to-end.** `Dockerfile.dev`/`compose.yaml`
  were syntax-checked (YAML parse, `make -n` dry-run expansion) but never
  actually built/run, since this sandbox has no Docker daemon. A real
  contributor's first `make build` will be the first true test of the
  containerized path.
- **Redaction/PII check is a documented heuristic, not general PII
  detection** — see `docs/privacy-model.md`. It only covers three known
  field locations plus three regex patterns over description fields.
- **No property-based/fuzz testing** of the YAML/JSON decoders or the
  canonicalizer — coverage is table-driven/example-based only.
- **`compare`'s per-dimension diff is presentation-only** and uses simple
  string summaries (e.g. joined causal-type signatures); it is not a
  minimal edit-distance diff, and could be misleading for very large
  documents with many simultaneous changes.
- **No `--json` / machine-readable output mode** for any command — all
  output is human-readable plain text, per the spec's literal command
  descriptions and to avoid scope creep beyond what was asked.

## Security risks remaining

See `docs/threat-model.md` for the full analysis; the residual (not fully
closed) risks worth calling out explicitly:

- Semantic tampering that happens to preserve every fingerprint-relevant
  field is undetectable (the fingerprint answers "is this the same failure
  class," not "is this document truthful").
- Evidence digests are format-checked only — there is no evidence storage
  yet to verify them against, so a forged digest that's correctly formatted
  is currently indistinguishable from a real one.
- The YAML-parser-abuse mitigation relies primarily on the pre-parse size
  cap; this project has not independently audited `gopkg.in/yaml.v3`'s
  internal alias/anchor expansion limits as a standalone control.
- No sandboxing beyond normal OS file permissions — a user running this CLI
  has exactly the file-system access the invoking user already has, which
  is appropriate for Phase 1's single-user CLI scope but will need explicit
  reconsideration once IncidentDNA gains a service/multi-tenant mode.

## Recommended Phase 2 scope

Based on what Phase 1 deliberately left out (`docs/product-scope.md`) and
what actually surfaced as friction while building this:

1. **Evidence storage + digest verification** — an addressable store (even
   just local-filesystem-backed to start) so `evidence[].digest` can be
   checked against real bytes, closing the "forged evidence reference" gap.
2. **A release-gate integration point** — some way to run the incident
   library against a release artifact and produce a pass/fail signal; this
   is the "block software that reintroduces a previously observed failure"
   promise from the product description, and Phase 1 built the
   representation/comparison primitives it needs but not the gate itself.
3. **OmniFlow integration** as the first reference application, per the
   original brief — exercising IDIR/validate/fingerprint/compare against a
   real, non-synthetic incident source for the first time.
4. **A real Docker-daemon CI run** (this environment couldn't do it) to
   confirm `Dockerfile.dev`/`compose.yaml`/CI workflow actually work, not
   just parse correctly.
5. Reassess the fingerprint's included-dimensions list (see
   `fingerprint-design.md`) against real incidents once there's a larger
   corpus than one synthetic example — the current six dimensions are a
   reasoned Phase 1 default, not empirically validated yet.

## Proposed commit message

Not executed — provided for the user to use if they choose to commit this
work themselves:

```
Add Phase 1 IDIR foundation: types, validation, fingerprinting, CLI

Implements the vendor-neutral core for representing, validating,
deterministically fingerprinting, and comparing production incidents
as IDIR v0.1 documents: internal/idir (types + loader), internal/validate
(semantic rule engine), internal/canonical (canonical JSON encoding),
internal/fingerprint (identity-payload extraction + sha256 fingerprint),
internal/compare (fingerprint diff), and the incidentdna CLI (init,
validate, fingerprint, inspect, compare).

Includes the duplicate-payment-after-redelivery synthetic example,
golden fingerprint tests, table-driven validation tests covering every
required rejection case, a containerized dev workflow (Dockerfile.dev /
compose.yaml / Makefile), GitHub Actions CI, and the required docs
(product scope, architecture, IDIR spec, fingerprint design, threat
model, privacy model, phase 1 plan and report).
```
