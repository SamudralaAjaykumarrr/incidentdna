// Package fingerprint computes a deterministic identity fingerprint for an
// IDIR document: a value that is the same for two documents describing the
// same underlying failure pattern, and different when the pattern actually
// differs. See docs/fingerprint-design.md for the full rationale.
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/canonical"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// identityPayload holds only the fields that define an incident's identity.
// Field order here is fixed and deliberate; combined with canonical.Marshal
// (which sorts any nested map keys) this makes the payload's JSON encoding
// fully deterministic regardless of the source document's formatting.
//
// Deliberately excluded: incident id/title/summary/timestamps, application
// and release identity, event ids/descriptions/timestamps, evidence and its
// storage locations, and privacy/redaction metadata — none of these change
// what class of failure the incident represents.
type identityPayload struct {
	SchemaVersion             string          `json:"schema_version"`
	Trigger                   triggerIdentity `json:"trigger"`
	Events                    []eventIdentity `json:"events"`
	BusinessInvariants        []string        `json:"business_invariants"`
	SideEffectTypes           []string        `json:"side_effect_types"`
	ExpectedCorrectedBehavior []string        `json:"expected_corrected_behavior"`
	TechnologyCategories      []string        `json:"technology_categories"`
}

type triggerIdentity struct {
	Type          string   `json:"type"`
	Description   string   `json:"description"`
	Preconditions []string `json:"preconditions"`
}

type eventIdentity struct {
	Type          string   `json:"type"`
	CausedByTypes []string `json:"caused_by_types"`
}

// Compute derives the identity payload for doc, canonicalizes it, and
// returns "sha256:" followed by the lowercase hex SHA-256 digest of the
// canonical bytes.
func Compute(doc *idir.Document) (string, error) {
	payload := buildIdentityPayload(doc)

	canonicalBytes, err := canonical.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("fingerprint: canonicalize: %w", err)
	}

	sum := sha256.Sum256(canonicalBytes)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func buildIdentityPayload(doc *idir.Document) identityPayload {
	typeByID := make(map[string]string, len(doc.Events))
	for _, ev := range doc.Events {
		typeByID[ev.ID] = ev.Type
	}

	events := make([]eventIdentity, 0, len(doc.Events))
	for _, ev := range doc.Events {
		var causedByTypes []string
		for _, causeID := range ev.CausedBy {
			if t, ok := typeByID[causeID]; ok {
				causedByTypes = append(causedByTypes, t)
			}
		}
		sort.Strings(causedByTypes)
		events = append(events, eventIdentity{
			Type:          ev.Type,
			CausedByTypes: dedupeSorted(causedByTypes),
		})
	}

	invariants := make([]string, 0, len(doc.BusinessInvariants))
	for _, inv := range doc.BusinessInvariants {
		invariants = append(invariants, inv.Statement)
	}

	sideEffectTypes := make([]string, 0, len(doc.SideEffects))
	for _, se := range doc.SideEffects {
		sideEffectTypes = append(sideEffectTypes, se.Type)
	}

	techCategories := make([]string, 0, len(doc.Services))
	for _, svc := range doc.Services {
		techCategories = append(techCategories, svc.TechnologyCategory)
	}

	return identityPayload{
		SchemaVersion: doc.SchemaVersion,
		Trigger: triggerIdentity{
			Type:          doc.Trigger.Type,
			Description:   doc.Trigger.Description,
			Preconditions: dedupeSorted(doc.Trigger.Preconditions),
		},
		Events:                    events,
		BusinessInvariants:        dedupeSorted(invariants),
		SideEffectTypes:           dedupeSorted(sideEffectTypes),
		ExpectedCorrectedBehavior: dedupeSorted(doc.ExpectedCorrectedBehavior),
		TechnologyCategories:      dedupeSorted(techCategories),
	}
}

// dedupeSorted sorts in and removes duplicate elements. A nil/empty input
// returns an empty (non-nil) slice so it always canonicalizes to "[]" rather
// than "null".
func dedupeSorted(in []string) []string {
	sorted := append([]string(nil), in...)
	sort.Strings(sorted)
	out := make([]string, 0, len(sorted))
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}
