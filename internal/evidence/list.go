package evidence

import (
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// List reports, for each evidence[] entry in doc, whether an object is
// present in the store at its declared digest. List never opens or
// re-hashes object content — it is a cheap, stat-only presence check, not an
// integrity check. StatusPresent means only "something is stored at this
// digest"; use Verify for an actual integrity guarantee. See
// docs/evidence-storage.md, "Why list presence does not prove integrity."
//
// MaxEntriesPerCommand is enforced before any filesystem work begins.
// MaxTotalVerifyBytes does not apply to List, since List never reads object
// content.
func (s *Store) List(ctx context.Context, doc *idir.Document) (ListResult, error) {
	if err := checkEntryCount(len(doc.Evidence)); err != nil {
		return ListResult{}, err
	}

	entries := make([]EntryResult, len(doc.Evidence))
	for i, ev := range doc.Evidence {
		if err := ctx.Err(); err != nil {
			return ListResult{}, &Error{Kind: ErrKindCanceled, Msg: "evidence listing canceled", Err: err}
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
		case lerr != nil:
			entries[i].Status = StatusError
			entries[i].Detail = wrapIOError(lerr, "stat evidence object").Error()
		case info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular():
			// A symlink or special file at the expected path is not a
			// trustworthy stored object; List reports "not present" rather
			// than following it or reporting it as a size-bearing object.
			entries[i].Status = StatusMissing
			entries[i].Detail = "path exists but is not a regular file; treated as not present"
		default:
			entries[i].Status = StatusPresent
			entries[i].Size = info.Size()
		}
	}
	return ListResult{Entries: entries}, nil
}
