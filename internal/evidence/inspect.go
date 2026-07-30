package evidence

import "context"

// Inspect looks up digest directly in the store — no incident document
// involved — and reports its metadata: presence, size, and full integrity
// status (re-hashed, unlike List). It never returns or logs the object's
// content; InspectResult has no field capable of holding it, and no code
// path in this function reads bytes into anything other than a hash.Hash.
//
// digest must be a canonical "sha256:<64 lowercase hex>" string; an
// unparseable value is rejected before any filesystem access (ErrKindInvalidDigest
// or ErrKindUnsupportedAlgorithm), never partially interpreted as a path.
func (s *Store) Inspect(ctx context.Context, digest string) (InspectResult, error) {
	if err := ctx.Err(); err != nil {
		return InspectResult{}, &Error{Kind: ErrKindCanceled, Msg: "evidence inspection canceled", Err: err}
	}

	d, perr := Parse(digest)
	if perr != nil {
		return InspectResult{}, perr
	}

	status, size, detail := s.checkObject(ctx, d)
	if status == objectError && detail == "canceled" {
		return InspectResult{}, &Error{Kind: ErrKindCanceled, Msg: "evidence inspection canceled", Err: ctx.Err()}
	}

	result := InspectResult{Digest: d.String(), Size: size}
	switch status {
	case objectOK:
		result.Status = StatusOK
	case objectMissing:
		result.Status = StatusMissing
	case objectError:
		result.Status = StatusError
		result.Detail = detail
	default:
		result.Status = StatusCorrupted
		result.Detail = detail
	}
	return result, nil
}
