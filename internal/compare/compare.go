// Package compare reports whether two IDIR documents represent the same
// normalized incident, and if not, which identity dimensions differ.
package compare

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// Difference describes one identity dimension that differs between two
// documents, with short human-readable summaries of each side.
type Difference struct {
	Dimension string
	A         string
	B         string
}

// Result is the outcome of comparing two documents.
type Result struct {
	FingerprintA string
	FingerprintB string
	// Identical is true iff FingerprintA == FingerprintB.
	Identical   bool
	Differences []Difference
}

// Documents computes and compares the fingerprints of a and b. If they
// differ, Documents also computes a best-effort, human-readable breakdown of
// which identity dimensions changed (business invariants, trigger, causal
// event structure, side effect types, recovery behavior, technology
// categories) — this is presentation help, not part of the fingerprint
// itself.
func Documents(a, b *idir.Document) (Result, error) {
	fpA, err := fingerprint.Compute(a)
	if err != nil {
		return Result{}, fmt.Errorf("compare: fingerprint a: %w", err)
	}
	fpB, err := fingerprint.Compute(b)
	if err != nil {
		return Result{}, fmt.Errorf("compare: fingerprint b: %w", err)
	}

	res := Result{
		FingerprintA: fpA,
		FingerprintB: fpB,
		Identical:    fpA == fpB,
	}
	if res.Identical {
		return res, nil
	}

	res.Differences = diffDimensions(a, b)
	return res, nil
}

func diffDimensions(a, b *idir.Document) []Difference {
	var diffs []Difference

	if a.Trigger.Type != b.Trigger.Type || a.Trigger.Description != b.Trigger.Description || !reflect.DeepEqual(sortedCopy(a.Trigger.Preconditions), sortedCopy(b.Trigger.Preconditions)) {
		diffs = append(diffs, Difference{
			Dimension: "trigger",
			A:         fmt.Sprintf("%s: %s", a.Trigger.Type, a.Trigger.Description),
			B:         fmt.Sprintf("%s: %s", b.Trigger.Type, b.Trigger.Description),
		})
	}

	if eventStructureSignature(a) != eventStructureSignature(b) {
		diffs = append(diffs, Difference{
			Dimension: "causal event structure",
			A:         eventStructureSignature(a),
			B:         eventStructureSignature(b),
		})
	}

	if invA, invB := sortedStatements(a), sortedStatements(b); !reflect.DeepEqual(invA, invB) {
		diffs = append(diffs, Difference{
			Dimension: "business invariants",
			A:         joinOrNone(invA),
			B:         joinOrNone(invB),
		})
	}

	if seA, seB := sortedSideEffectTypes(a), sortedSideEffectTypes(b); !reflect.DeepEqual(seA, seB) {
		diffs = append(diffs, Difference{
			Dimension: "side effect types",
			A:         joinOrNone(seA),
			B:         joinOrNone(seB),
		})
	}

	if recA, recB := sortedCopy(a.ExpectedCorrectedBehavior), sortedCopy(b.ExpectedCorrectedBehavior); !reflect.DeepEqual(recA, recB) {
		diffs = append(diffs, Difference{
			Dimension: "recovery behavior",
			A:         joinOrNone(recA),
			B:         joinOrNone(recB),
		})
	}

	if techA, techB := sortedTechCategories(a), sortedTechCategories(b); !reflect.DeepEqual(techA, techB) {
		diffs = append(diffs, Difference{
			Dimension: "affected technology categories",
			A:         joinOrNone(techA),
			B:         joinOrNone(techB),
		})
	}

	return diffs
}

// eventStructureSignature builds a short human-readable summary of the
// causal event structure (type + caused-by types, in file order) for
// display in a diff. It is not used for fingerprinting.
func eventStructureSignature(doc *idir.Document) string {
	typeByID := make(map[string]string, len(doc.Events))
	for _, ev := range doc.Events {
		typeByID[ev.ID] = ev.Type
	}
	sig := ""
	for i, ev := range doc.Events {
		if i > 0 {
			sig += " | "
		}
		causes := make([]string, 0, len(ev.CausedBy))
		for _, id := range ev.CausedBy {
			if t, ok := typeByID[id]; ok {
				causes = append(causes, t)
			}
		}
		sort.Strings(causes)
		if len(causes) == 0 {
			sig += ev.Type
		} else {
			sig += fmt.Sprintf("%s<-[%s]", ev.Type, joinOrNone(causes))
		}
	}
	return sig
}

func sortedStatements(doc *idir.Document) []string {
	out := make([]string, 0, len(doc.BusinessInvariants))
	for _, inv := range doc.BusinessInvariants {
		out = append(out, inv.Statement)
	}
	return sortedCopy(out)
}

func sortedSideEffectTypes(doc *idir.Document) []string {
	out := make([]string, 0, len(doc.SideEffects))
	for _, se := range doc.SideEffects {
		out = append(out, se.Type)
	}
	return sortedCopy(out)
}

func sortedTechCategories(doc *idir.Document) []string {
	out := make([]string, 0, len(doc.Services))
	for _, svc := range doc.Services {
		out = append(out, svc.TechnologyCategory)
	}
	return sortedCopy(out)
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func joinOrNone(in []string) string {
	if len(in) == 0 {
		return "(none)"
	}
	out := in[0]
	for _, s := range in[1:] {
		out += ", " + s
	}
	return out
}
