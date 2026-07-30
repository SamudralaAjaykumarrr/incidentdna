// privacy.go implements Phase 3 Slice 4: the §10 privacy gate Add enforces
// before any fingerprint/digest/storage work happens (docs/phase-3-plan.md
// §12), and the --allow-unredacted override's warning signal.
//
// privacy.redacted is author-declared, not verified. Setting it to true
// records only that the document's author asserts the document was reviewed
// and sanitized before authoring it — nothing in this package, or anywhere
// else in this codebase, confirms that assertion is actually true (see
// docs/privacy-model.md). AddOptions.AllowUnredacted is the internal
// equivalent of the future CLI's explicit --allow-unredacted flag: an
// explicit, exceptional override an operator chooses deliberately, never a
// default and never a silent bypass. When it is used, the caller (the
// future CLI) MUST warn the user; this package never prints anything itself
// and never exposes any part of a document's content in a privacy-gate
// error or result — only the fact that the declaration was missing.
package library

import (
	"errors"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// ErrPrivacyNotRedacted means doc's privacy.redacted field is false (or
// unset) and AddOptions.AllowUnredacted was not set (docs/phase-3-plan.md
// §10/§12). The error carries no document content beyond the fact of the
// missing declaration — never the document's title, summary, or any other
// field.
var ErrPrivacyNotRedacted = errors.New("library: document is not declared redacted (privacy.redacted != true); pass --allow-unredacted to store it anyway")

// AddOptions configures Add's optional, non-default behavior. The zero
// value is the safe default: AllowUnredacted false means Add refuses any
// document that does not declare privacy.redacted == true.
type AddOptions struct {
	// AllowUnredacted permits Add to proceed with a document whose
	// privacy.redacted field is false or unset (docs/phase-3-plan.md §10).
	// This is an explicit, exceptional override corresponding to the future
	// CLI's --allow-unredacted flag — not a default, and never silent: when
	// it is the reason Add proceeded, AddResult.PrivacyOverridden is true so
	// the caller can display a warning. Setting privacy.redacted to true on
	// the document itself is always preferable; this flag exists for the
	// operator who has decided, explicitly, to store a document anyway.
	AllowUnredacted bool
}

// checkPrivacyGate enforces docs/phase-3-plan.md §10's privacy policy: doc
// must declare privacy.redacted == true unless allowUnredacted is set. It
// returns whether the caller must display an override warning (true only
// when the gate was bypassed via allowUnredacted) and a non-nil error when
// the document is refused outright.
//
// checkPrivacyGate never prints anything and never returns any part of
// doc's content — not its title, summary, free-text fields, or any other
// value — in either its error or its overridden result. It reads exactly
// one field of doc: Privacy.Redacted.
func checkPrivacyGate(doc *idir.Document, allowUnredacted bool) (overridden bool, err error) {
	if doc.Privacy.Redacted {
		return false, nil
	}
	if !allowUnredacted {
		return false, ErrPrivacyNotRedacted
	}
	return true, nil
}
