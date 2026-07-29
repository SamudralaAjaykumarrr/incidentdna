# Threat Model

Scope: the `incidentdna` CLI and the `internal/*` libraries it's built on,
as they exist at the end of Phase 1 — local file input, local file/stdout
output, no network, no multi-user or multi-tenant concerns yet.

## Maliciously modified incident documents

A document could be edited to misrepresent what happened (e.g. remove a
causal step, soften a business invariant) while still passing structural
validation. Phase 1 cannot detect *semantic* tampering — validation checks
internal coherence (no dangling refs, no cycles, non-empty required fields),
not truthfulness against some external source of record. What it does
guarantee: any such edit that changes a fingerprint-relevant field (see
[`fingerprint-design.md`](fingerprint-design.md)) produces a **different**
fingerprint, so a tampered document cannot silently masquerade as the
original incident's regression scenario under the same identity. Evidence
digests (`sha256:`-prefixed, format-checked) let a *future* phase verify
evidence integrity against externally stored material, once evidence
storage/retrieval is built — Phase 1 only enforces the digest's format, not
that it matches any actual bytes (there is nothing to fetch and check
against yet).

## Fingerprint collisions or canonicalization ambiguity

Mitigated by: (1) a dedicated, independently-tested canonical JSON encoder
(`internal/canonical`) rather than relying on incidental struct-marshaling
behavior; (2) SHA-256, whose collision resistance is adequate for this use
case; (3) golden tests pinning the exact canonical bytes' hash for a known
document, so a canonicalization regression is caught immediately rather than
silently changing what "the same incident" means. Residual risk: the
identity payload's field selection is itself a judgment call (see
`fingerprint-design.md`) — two failures that a human would consider
different could theoretically map to the same fingerprint if they happen to
share every included dimension. This is a design tradeoff, not a bug, and is
the reason `compare` explains *which* dimensions matched rather than only
reporting a boolean.

## Sensitive information entering incident evidence

Mitigated by the `privacy` block: a fixed set of "known sensitive
locations" (`trigger.raw_payload_excerpt`, `events[].raw_payload_excerpt`,
`evidence[].raw_excerpt`) that validation requires to be empty when
`privacy.redacted` is `true`, plus a regex backstop (email/phone/long-digit
patterns) over free-text description fields. This is **not** general
PII detection — see [`privacy-model.md`](privacy-model.md) for the explicit
scope and limitation. A document that is *not* marked `redacted` is not
scanned at all; authors are responsible for setting `redacted` accurately
and for what they write into non-"known-sensitive" free-text fields
(`description`, `summary`, etc.) even when redacted, beyond what the regex
backstop happens to catch.

## Forged evidence references

`evidence[].digest` must be `sha256:`-prefixed and exactly 64 lowercase hex
characters; a malformed digest is rejected. Phase 1 has no evidence storage
or retrieval, so there is nothing yet to verify a digest *against* — this is
a format precondition for a future phase's integrity check, not integrity
verification itself. `evidence[].location` is treated as inert metadata:
the CLI never opens, fetches, or otherwise acts on it (see below).

## Path traversal through CLI inputs

`incidentdna validate/fingerprint/inspect` open exactly the file path(s)
given directly on the command line by the invoking user — the same trust
boundary as any CLI tool that takes a filename argument (`cat`, `jq`, etc.).
The one place this could go wrong is if the tool followed a path *embedded
inside* a document (e.g. `evidence[].location`) — it deliberately never
does; that field is presentational only. `incidentdna init` writes to a
fixed relative path (`.incidentdna/incident.yaml`) under the current
directory, never a user- or document-supplied path, and refuses to
overwrite an existing file unless `--force` is passed (see next item).

## Resource exhaustion from maliciously large documents

`internal/idir.LoadFile` checks the file's size via `os.Stat` before opening
it, and independently caps the actual bytes read via `io.LimitReader` (belt
and suspenders against a file that grows between stat and read, or a
non-regular file with an unreliable reported size) at `MaxDocumentSize` (5
MiB). Beyond that, every CLI subcommand runs under a 30-second
`context.Context` timeout (`cmd/incidentdna/main.go`), and
`internal/validate`'s loops check `ctx.Err()` periodically, so a
pathological-but-under-the-size-cap document (e.g. a huge `events` array)
cannot hang the process indefinitely.

## YAML parser abuse

Decoding always targets a concrete typed `idir.Document` struct (`yaml.v3`
via `Decode`), never `interface{}` or `map[string]interface{}`. This rules
out the classic "YAML decodes into arbitrary Go types/interfaces and code
acts on them unexpectedly" class of issue, since unknown fields and unknown
YAML tags simply have nowhere to go. Combined with the size cap above (which
bounds alias/anchor expansion in absolute terms, since the *source* bytes
are capped before parsing even starts) and `gopkg.in/yaml.v3`'s own
built-in alias-count limits, a "billion laughs"-style expansion attack is
bounded. Residual risk: this project does not independently verify yaml.v3's
internal expansion limits are sufficient on their own without the size cap —
the size cap is treated as the primary control, not a backstop.

## Dependency compromise

The dependency surface is deliberately minimal: `gopkg.in/yaml.v3` is the
**only** non-stdlib Go dependency in the module (`go.mod` — verified via
`go mod tidy`, see `docs/phase-1-report.md`). `go.sum` pins exact checksums.
No lint/build tooling is fetched as a Go dependency either (see
`architecture.md`, "golangci-lint → go vet + gofmt"). This does not
eliminate supply-chain risk, but it minimizes the attack surface to review.

## Generated files overwriting existing user files

`incidentdna init` checks whether `.incidentdna/incident.yaml` already
exists and refuses to proceed (non-zero exit, no write) unless `--force` is
passed — verified in `cmd/incidentdna/cli_test.go` and manually in
`phase-1-report.md`. No other subcommand writes any file.

## Future multi-tenant risks (explicitly out of scope for Phase 1)

Phase 1 has no concept of a tenant, user account, or shared incident
library — every invocation operates on files the invoking user already has
filesystem access to, with no access control layer of its own. Risks that
become relevant once a shared/multi-tenant incident library exists,
deliberately deferred to a later phase:

- Cross-tenant fingerprint/identity leakage (can one tenant infer another
  tenant's incident existed, from a shared fingerprint namespace?).
- Authorization for who may write, read, or mark an incident "resolved" in
  a shared library.
- Rate limiting / quota enforcement once ingestion is not "a person runs a
  CLI against a file they already have."
- Tenant isolation for evidence storage, once evidence storage exists.

None of these are mitigated today because none of the underlying
capabilities (network service, storage, multiple callers) exist yet in this
codebase.
