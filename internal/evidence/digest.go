// Package evidence implements local, content-addressable storage for
// incident evidence and verification of stored evidence against the SHA-256
// digests declared in an IDIR document's evidence[] entries. See
// docs/evidence-storage.md for the full design and docs/phase-2-plan.md for
// the approved plan this package implements.
//
// Nothing in this package performs network access or collects telemetry —
// every operation is local filesystem I/O, consistent with the rest of this
// module (see docs/threat-model.md).
package evidence

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
)

// Algorithm is the only digest algorithm Phase 2 supports. A future phase
// could add others; this package deliberately does not build an extensible
// algorithm registry ahead of that actually being needed.
const Algorithm = "sha256"

// hexLength is the number of hex characters in a SHA-256 digest (32 bytes).
const hexLength = sha256.Size * 2

// Digest is a validated, canonical SHA-256 content digest: internally,
// exactly 64 lowercase hexadecimal characters with no prefix. The zero value
// is not a valid Digest and must never be used to construct a store path;
// every Digest in a running program came from either Parse (untrusted input:
// a CLI argument, a value read from a document) or from actually hashing
// bytes (trusted: internal/evidence computed it itself). This split is what
// makes object-path construction in store.go structurally safe against path
// traversal — no code path ever turns an arbitrary string directly into a
// filesystem path.
type Digest struct {
	hex string
}

// String returns the canonical external form: "sha256:" followed by 64
// lowercase hex characters. This is the form documents declare in
// evidence[].digest and the form printed by the CLI.
func (d Digest) String() string {
	return Algorithm + ":" + d.hex
}

// Hex returns the bare 64-character lowercase hex digest, with no algorithm
// prefix. Used internally to derive the sharded object path (see
// store.go's objectPath).
func (d Digest) Hex() string {
	return d.hex
}

// IsZero reports whether d is the unconstructed zero value.
func (d Digest) IsZero() bool {
	return d.hex == ""
}

// Equal reports whether d and other represent the same digest. The
// comparison runs in constant time with respect to the digest bytes
// themselves: digests are not secret, so this is defense-in-depth rather
// than a load-bearing security control, but it removes any need for a
// future caller to reason about whether their particular use is
// timing-sensitive.
func (d Digest) Equal(other Digest) bool {
	if len(d.hex) != len(other.hex) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(d.hex), []byte(other.hex)) == 1
}

// Parse validates s as a canonical digest string — "sha256:" followed by
// exactly 64 lowercase hexadecimal characters, nothing more and nothing
// less — and returns the resulting Digest.
//
// Parse is the only entry point by which untrusted input (a CLI argument, a
// value read from an IDIR document) becomes a Digest. Every rejection case
// required by docs/phase-2-plan.md is covered by construction, not by a list
// of special cases:
//   - missing/empty algorithm prefix, or no ":" at all: rejected by the
//     strings.Cut/prefix check below.
//   - unsupported algorithm: rejected by the exact-match check against
//     Algorithm.
//   - too few/too many hex characters, or a valid digest followed by
//     trailing data (which simply appears as extra length or extra
//     characters after the split): rejected by the exact length check.
//   - non-hexadecimal characters, uppercase hexadecimal characters,
//     whitespace inside the digest, path separators, "..", and absolute
//     paths: all rejected by the lowercase-hex-only character class check,
//     since none of those characters are in that class. A digest can
//     therefore never contain "/", "\", "..", or a null byte — path
//     traversal via a parsed Digest is not just defended against, it is
//     unrepresentable.
//   - whitespace around the whole string: rejected by the length check
//     (leading/trailing whitespace makes the hex portion the wrong length)
//     or the character class check (embedded whitespace is not hex).
//   - additional algorithm separators (e.g. "sha256:sha256:<hex>"): the
//     split is on the *first* ":" only, so anything after it becomes part
//     of the "hex" portion and fails the character class check.
func Parse(s string) (Digest, error) {
	algo, hexPart, ok := strings.Cut(s, ":")
	if !ok {
		return Digest{}, &Error{
			Kind: ErrKindInvalidDigest,
			Msg:  fmt.Sprintf("digest %q is missing the required %q algorithm prefix", s, Algorithm+":"),
		}
	}
	if algo != Algorithm {
		return Digest{}, &Error{
			Kind: ErrKindUnsupportedAlgorithm,
			Msg:  fmt.Sprintf("digest %q uses unsupported algorithm %q; only %q is supported in Phase 2", s, algo, Algorithm),
		}
	}
	if len(hexPart) != hexLength {
		return Digest{}, &Error{
			Kind: ErrKindInvalidDigest,
			Msg:  fmt.Sprintf("digest %q has %d character(s) after the %q prefix; exactly %d lowercase hex characters are required", s, len(hexPart), Algorithm+":", hexLength),
		}
	}
	for i, c := range []byte(hexPart) {
		if !isLowerHexDigit(c) {
			return Digest{}, &Error{
				Kind: ErrKindInvalidDigest,
				Msg:  fmt.Sprintf("digest %q contains a non-lowercase-hex character %q at position %d", s, c, i),
			}
		}
	}
	return Digest{hex: hexPart}, nil
}

func isLowerHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')
}

// NewHasher returns a fresh hash.Hash implementing Algorithm, for streaming
// content through while computing its Digest (see store.go, verify.go).
func NewHasher() hash.Hash {
	return sha256.New()
}

// digestFromSum converts the output of hasher.Sum(nil) into a Digest without
// re-running Parse's validation: a hash.Hash's Sum is always exactly
// hexLength/2 bytes, so hex-encoding it always produces a string that would
// pass Parse — re-validating would be redundant work on a value this package
// itself just computed, not untrusted input.
func digestFromSum(sum []byte) Digest {
	return Digest{hex: hex.EncodeToString(sum)}
}
