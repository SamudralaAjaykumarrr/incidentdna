package evidence

// EntryStatus is the outcome of checking one evidence entry (List, Verify)
// or one bare digest (Inspect) against the store. Not every status is
// reachable from every operation — see each function's doc comment for
// which subset it produces.
type EntryStatus string

const (
	// StatusOK means the object is present and, when re-hashed, its bytes
	// match the declared digest exactly. Produced only by Verify and
	// Inspect, never by List (which never re-hashes).
	StatusOK EntryStatus = "ok"

	// StatusPresent means a stat-only check found an object at the declared
	// digest's path. Produced only by List. It is deliberately not called
	// "ok" or "verified" — StatusPresent carries no integrity guarantee at
	// all; it means "something is stored here," not "that something is
	// intact." See docs/evidence-storage.md, "Why list presence does not
	// prove integrity."
	StatusPresent EntryStatus = "present"

	// StatusMissing means no object exists at the declared digest's path.
	StatusMissing EntryStatus = "missing"

	// StatusCorrupted means an object exists at the declared digest's path
	// but is not trustworthy as that evidence: re-hashing it produced a
	// different digest (modified, truncated, or has trailing bytes), or the
	// path is a symlink or other non-regular file rather than the evidence
	// itself. Detail explains which.
	StatusCorrupted EntryStatus = "corrupted"

	// StatusInvalid means the declared digest string itself could not be
	// parsed as a canonical "sha256:<64 lowercase hex>" value (wrong
	// algorithm, wrong length, bad characters). internal/validate already
	// rejects this at the document level for any document that reached
	// Verify/List through the normal CLI path, so this is defense-in-depth
	// for callers that construct a *Store and call these methods directly.
	StatusInvalid EntryStatus = "invalid_digest"

	// StatusError means the entry could not be checked due to an
	// operational failure unrelated to the evidence's own integrity (most
	// commonly a filesystem permission error). It is kept distinct from
	// StatusCorrupted so a summary reader isn't told evidence is corrupted
	// when the real problem is that incidentdna couldn't read the store.
	StatusError EntryStatus = "error"
)

// EntryResult is the per-evidence-entry outcome of List or Verify.
type EntryResult struct {
	// ID and Type echo the evidence[] entry's own fields, for display only.
	ID, Type string
	// Digest is the declared digest string exactly as written in the
	// document (even when it fails to parse, so the report can show what
	// was actually declared).
	Digest string
	Status EntryStatus
	// Size is the object's size in bytes, when known (zero for
	// Missing/Invalid).
	Size int64
	// Detail is a short, human-readable explanation for a non-OK/
	// non-Present status. Never contains evidence content.
	Detail string
}

// ListResult is the outcome of Store.List.
type ListResult struct {
	Entries []EntryResult
}

// VerifyResult is the outcome of Store.Verify.
type VerifyResult struct {
	Entries []EntryResult
	// AggregateBytes is the total size, in bytes, of the distinct stored
	// objects discovered during the pre-pass (see verify.go) — the same
	// total checked against MaxTotalVerifyBytes.
	AggregateBytes int64
}

// AllOK reports whether every entry in r verified successfully. A result
// with zero entries is vacuously true.
func (r VerifyResult) AllOK() bool {
	for _, e := range r.Entries {
		if e.Status != StatusOK {
			return false
		}
	}
	return true
}

// InspectResult is the outcome of Store.Inspect: metadata about one stored
// object, addressed directly by digest. It deliberately has no field
// capable of holding the object's content.
type InspectResult struct {
	Digest string
	Status EntryStatus
	Size   int64
	Detail string
}

// StoreResult is the outcome of Store.Put.
type StoreResult struct {
	Digest Digest
	// AlreadyPresent is true if an object with this digest already existed
	// (and was successfully re-verified) before this call — a deduplicated
	// no-op — rather than being newly written.
	AlreadyPresent bool
	Size           int64
}
