package evidence

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// Verify checks every evidence[] entry in doc against the store: is an
// object present at the declared digest, and does re-hashing its actual
// bytes still reproduce that digest? See docs/phase-2-plan.md §5.
//
// doc is expected to have already passed validate.Validate — Verify does
// not re-run semantic validation — but it re-parses each declared digest
// itself rather than trusting the string, so a document checked without
// going through the CLI's normal validate-then-verify path still fails
// safely (an unparseable digest is reported StatusInvalid, never handed to
// a filesystem operation).
//
// Two resource limits are enforced before any object content is read:
// MaxEntriesPerCommand against len(doc.Evidence), and MaxTotalVerifyBytes
// against the sum of stat'd sizes of the distinct objects that will actually
// be read (a cheap, content-free pre-pass) — if the aggregate would exceed
// the limit, Verify returns an error without reading a single byte of any
// object. Each distinct digest is re-hashed at most once, even if the same
// digest appears in multiple evidence[] entries.
func (s *Store) Verify(ctx context.Context, doc *idir.Document) (VerifyResult, error) {
	if err := checkEntryCount(len(doc.Evidence)); err != nil {
		return VerifyResult{}, err
	}

	entries := make([]EntryResult, len(doc.Evidence))
	type candidate struct {
		idx    int
		digest Digest
	}
	var candidates []candidate
	sizes := make(map[string]int64) // digest hex -> stat'd size, deduplicated

	for i, ev := range doc.Evidence {
		if err := ctx.Err(); err != nil {
			return VerifyResult{}, &Error{Kind: ErrKindCanceled, Msg: "evidence verification canceled", Err: err}
		}
		entries[i] = EntryResult{ID: ev.ID, Type: ev.Type, Digest: ev.Digest}

		d, perr := Parse(ev.Digest)
		if perr != nil {
			entries[i].Status = StatusInvalid
			entries[i].Detail = perr.Error()
			continue
		}
		path, perr2 := s.objectPath(d)
		if perr2 != nil {
			entries[i].Status = StatusInvalid
			entries[i].Detail = perr2.Error()
			continue
		}

		info, lerr := os.Lstat(path)
		switch {
		case lerr != nil && errors.Is(lerr, fs.ErrNotExist):
			entries[i].Status = StatusMissing
			continue
		case lerr != nil:
			entries[i].Status = StatusError
			entries[i].Detail = wrapIOError(lerr, "stat evidence object").Error()
			continue
		case info.Mode()&os.ModeSymlink != 0:
			entries[i].Status = StatusCorrupted
			entries[i].Detail = "stored path is a symbolic link, not a regular file; refusing to trust it as evidence"
			continue
		case !info.Mode().IsRegular():
			entries[i].Status = StatusCorrupted
			entries[i].Detail = "stored path is not a regular file"
			continue
		}

		if _, seen := sizes[d.Hex()]; !seen {
			sizes[d.Hex()] = info.Size()
		}
		candidates = append(candidates, candidate{idx: i, digest: d})
	}

	var total int64
	for _, sz := range sizes {
		total += sz
	}
	if err := checkTotalBytes(total); err != nil {
		return VerifyResult{}, err
	}

	// Re-hash each distinct digest at most once.
	type outcome struct {
		status objectCheck
		size   int64
		detail string
	}
	checked := make(map[string]outcome, len(sizes))
	for _, c := range candidates {
		if err := ctx.Err(); err != nil {
			return VerifyResult{}, &Error{Kind: ErrKindCanceled, Msg: "evidence verification canceled", Err: err}
		}
		key := c.digest.Hex()
		if _, done := checked[key]; done {
			continue
		}
		status, size, detail := s.checkObject(ctx, c.digest)
		if status == objectError && detail == "canceled" {
			return VerifyResult{}, &Error{Kind: ErrKindCanceled, Msg: "evidence verification canceled", Err: ctx.Err()}
		}
		checked[key] = outcome{status: status, size: size, detail: detail}
	}

	for _, c := range candidates {
		o := checked[c.digest.Hex()]
		entries[c.idx].Size = o.size
		switch o.status {
		case objectOK:
			entries[c.idx].Status = StatusOK
		case objectMissing:
			entries[c.idx].Status = StatusMissing
		case objectError:
			entries[c.idx].Status = StatusError
			entries[c.idx].Detail = o.detail
		default:
			entries[c.idx].Status = StatusCorrupted
			entries[c.idx].Detail = o.detail
		}
	}

	return VerifyResult{Entries: entries, AggregateBytes: total}, nil
}
