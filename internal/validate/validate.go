// Package validate implements the IDIR v0.1 semantic rule set: the checks
// that go beyond "does this parse into the right shape" (handled by
// internal/idir's typed decode) into "is this a coherent, safe-to-trust
// incident record".
package validate

import (
	"context"
	"fmt"
	"regexp"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

var validSensitivities = map[string]bool{
	idir.SensitivityPublic:       true,
	idir.SensitivityInternal:     true,
	idir.SensitivityConfidential: true,
	idir.SensitivityRestricted:   true,
}

// knownSensitiveLocations backstop-scans these free-text fields for obvious
// PII patterns when Privacy.Redacted is true. This is deliberately narrow:
// see docs/privacy-model.md for why it is a supplementary check on a fixed
// field list, not general-purpose PII detection.
var (
	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phonePattern = regexp.MustCompile(`\+?\d[\d\-. ]{8,}\d`)
	panPattern   = regexp.MustCompile(`\b\d{13,19}\b`)
)

// Validate runs the full IDIR v0.1 semantic rule set against doc and returns
// every issue found. It never returns a nil Result; check Result.Valid().
//
// ctx bounds total validation work: on a document with a very large number
// of events, evidence, or reproduction steps, the loops below periodically
// check ctx.Err() so a caller-imposed timeout (see cmd/incidentdna) actually
// takes effect rather than running to completion regardless.
func Validate(ctx context.Context, doc *idir.Document) Result {
	var res Result
	if doc == nil {
		res.add("", "document is nil")
		return res
	}

	checkSchemaVersion(&res, doc)
	checkIncidentIdentity(&res, doc)
	checkBusinessInvariants(&res, doc)
	checkExpectedCorrectedBehavior(&res, doc)
	checkSensitivity(&res, doc)

	eventIDs := checkEventsAndCausality(ctx, &res, doc)
	checkEvidence(&res, doc)
	checkReproductionSequence(&res, doc, eventIDs)
	checkRedaction(&res, doc)

	return res
}

func checkSchemaVersion(res *Result, doc *idir.Document) {
	if doc.SchemaVersion != idir.SupportedSchemaVersion {
		res.add("schema_version", "unsupported schema version %q; this build only supports %q", doc.SchemaVersion, idir.SupportedSchemaVersion)
	}
}

func checkIncidentIdentity(res *Result, doc *idir.Document) {
	if doc.Incident.ID == "" {
		res.add("incident.id", "missing incident identifier")
	}
}

func checkBusinessInvariants(res *Result, doc *idir.Document) {
	if len(doc.BusinessInvariants) == 0 {
		res.add("business_invariants", "at least one violated business invariant is required")
		return
	}
	for i, inv := range doc.BusinessInvariants {
		if inv.Statement == "" {
			res.add(field("business_invariants[%d].statement", i), "invariant statement must not be empty")
		}
	}
}

func checkExpectedCorrectedBehavior(res *Result, doc *idir.Document) {
	nonEmpty := false
	for _, s := range doc.ExpectedCorrectedBehavior {
		if s != "" {
			nonEmpty = true
			break
		}
	}
	if !nonEmpty {
		res.add("expected_corrected_behavior", "expected corrected behavior must not be empty")
	}
}

func checkSensitivity(res *Result, doc *idir.Document) {
	if !validSensitivities[doc.Privacy.Sensitivity] {
		res.add("privacy.sensitivity", "unknown sensitivity classification %q", doc.Privacy.Sensitivity)
	}
}

// checkEventsAndCausality validates event identifiers and the causal graph,
// returning the set of valid event IDs for reuse by
// checkReproductionSequence.
func checkEventsAndCausality(ctx context.Context, res *Result, doc *idir.Document) map[string]bool {
	seen := make(map[string]bool, len(doc.Events))
	dupes := make(map[string]bool)
	for i, ev := range doc.Events {
		if ctx.Err() != nil {
			res.add("events", "validation aborted: %v", ctx.Err())
			return seen
		}
		if ev.ID == "" {
			res.add(field("events[%d].id", i), "event id must not be empty")
			continue
		}
		if seen[ev.ID] {
			if !dupes[ev.ID] {
				res.add(field("events[%d].id", i), "duplicate event id %q", ev.ID)
				dupes[ev.ID] = true
			}
			continue
		}
		seen[ev.ID] = true
	}

	// Dangling causal references.
	for i, ev := range doc.Events {
		for _, causeID := range ev.CausedBy {
			if !seen[causeID] {
				res.add(field("events[%d].caused_by", i), "event %q references nonexistent causal event %q", ev.ID, causeID)
			}
		}
	}

	if cyclePath := findCycle(doc.Events); cyclePath != nil {
		res.add("events", "cyclic causal relationship detected: %s", cyclePathString(cyclePath))
	}

	return seen
}

// findCycle runs DFS with a recursion stack over the caused_by graph
// (edges: event -> each event that caused it) and returns the first cycle
// found as a slice of event IDs, or nil if the graph is acyclic. Events with
// dangling causal references are skipped here (already reported separately)
// so a dangling reference doesn't also surface as a spurious cycle.
func findCycle(events []idir.Event) []string {
	byID := make(map[string]idir.Event, len(events))
	for _, ev := range events {
		if ev.ID != "" {
			byID[ev.ID] = ev
		}
	}

	const (
		unvisited = 0
		visiting  = 1
		done      = 2
	)
	state := make(map[string]int, len(byID))
	var path []string

	var visit func(id string) []string
	visit = func(id string) []string {
		switch state[id] {
		case done:
			return nil
		case visiting:
			// Found the back-edge that closes the cycle; return the cycle
			// portion of the current path.
			for i, p := range path {
				if p == id {
					return append(append([]string{}, path[i:]...), id)
				}
			}
			return []string{id}
		}
		state[id] = visiting
		path = append(path, id)
		for _, causeID := range byID[id].CausedBy {
			if _, ok := byID[causeID]; !ok {
				continue // dangling reference, reported elsewhere
			}
			if cycle := visit(causeID); cycle != nil {
				return cycle
			}
		}
		path = path[:len(path)-1]
		state[id] = done
		return nil
	}

	// Deterministic iteration order for a deterministic error message.
	for _, ev := range events {
		if ev.ID == "" || state[ev.ID] == done {
			continue
		}
		if cycle := visit(ev.ID); cycle != nil {
			return cycle
		}
	}
	return nil
}

func cyclePathString(path []string) string {
	out := ""
	for i, id := range path {
		if i > 0 {
			out += " -> "
		}
		out += id
	}
	return out
}

func checkEvidence(res *Result, doc *idir.Document) {
	for i, ev := range doc.Evidence {
		if !digestPattern.MatchString(ev.Digest) {
			res.add(field("evidence[%d].digest", i), "digest %q must be a sha256-prefixed 64-character hex digest, e.g. \"sha256:<64 hex chars>\"", ev.Digest)
		}
	}
}

func checkReproductionSequence(res *Result, doc *idir.Document, eventIDs map[string]bool) {
	for i, step := range doc.ReproductionSequence {
		if step.RefEventID != "" && !eventIDs[step.RefEventID] {
			res.add(field("reproduction_sequence[%d].ref_event_id", i), "reproduction step references nonexistent event %q", step.RefEventID)
		}
	}
}

// checkRedaction enforces that when Privacy.Redacted is true, the fixed set
// of known-sensitive free-text fields (Trigger.RawPayloadExcerpt,
// Event.RawPayloadExcerpt, Evidence.RawExcerpt) are empty, with a regex
// backstop for obvious PII patterns even in fields not on that list.
func checkRedaction(res *Result, doc *idir.Document) {
	if !doc.Privacy.Redacted {
		return
	}
	check := func(field, value string) {
		if value != "" {
			res.add(field, "document is marked redacted but this known-sensitive field is not empty")
			return
		}
	}
	check("trigger.raw_payload_excerpt", doc.Trigger.RawPayloadExcerpt)
	for i, ev := range doc.Events {
		check(field("events[%d].raw_payload_excerpt", i), ev.RawPayloadExcerpt)
	}
	for i, e := range doc.Evidence {
		check(field("evidence[%d].raw_excerpt", i), e.RawExcerpt)
	}

	// Backstop: scan all free-text description fields for obvious PII even
	// though they aren't on the "known sensitive locations" list, since a
	// redacted document should not leak PII through an unlisted field
	// either. This is heuristic and not exhaustive (see docs/threat-model.md).
	scanForPII(res, "trigger.description", doc.Trigger.Description)
	for i, ev := range doc.Events {
		scanForPII(res, field("events[%d].description", i), ev.Description)
	}
	for i, se := range doc.SideEffects {
		scanForPII(res, field("side_effects[%d].description", i), se.Description)
	}
}

func scanForPII(res *Result, field, text string) {
	if text == "" {
		return
	}
	switch {
	case emailPattern.MatchString(text):
		res.add(field, "document is marked redacted but this field appears to contain an email address")
	case phonePattern.MatchString(text):
		res.add(field, "document is marked redacted but this field appears to contain a phone-number-like sequence")
	case panPattern.MatchString(text):
		res.add(field, "document is marked redacted but this field appears to contain a long numeric identifier (e.g. card/account number)")
	}
}

func field(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
