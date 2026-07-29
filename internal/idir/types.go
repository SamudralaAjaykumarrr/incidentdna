package idir

// Document is the root of an IDIR document: a complete, self-contained
// description of one production incident, turned into a permanent regression
// scenario. Field order here is documentation order, not fingerprint order —
// see internal/fingerprint for the identity-relevant subset.
type Document struct {
	SchemaVersion             string       `json:"schema_version" yaml:"schema_version"`
	Incident                  Incident     `json:"incident" yaml:"incident"`
	Application               Application  `json:"application" yaml:"application"`
	Trigger                   Trigger      `json:"trigger" yaml:"trigger"`
	Events                    []Event      `json:"events" yaml:"events"`
	Services                  []Service    `json:"services" yaml:"services"`
	SideEffects               []SideEffect `json:"side_effects" yaml:"side_effects"`
	BusinessInvariants        []Invariant  `json:"business_invariants" yaml:"business_invariants"`
	ExpectedCorrectedBehavior []string     `json:"expected_corrected_behavior" yaml:"expected_corrected_behavior"`
	Evidence                  []Evidence   `json:"evidence" yaml:"evidence"`
	ReproductionSequence      []ReproStep  `json:"reproduction_sequence" yaml:"reproduction_sequence"`
	Privacy                   Privacy      `json:"privacy" yaml:"privacy"`
}

// Incident identifies the incident record itself: what happened, and when.
// None of these fields participate in the fingerprint — they identify the
// specific record, not the class of failure it represents.
type Incident struct {
	ID         string `json:"id" yaml:"id"`
	Title      string `json:"title" yaml:"title"`
	Summary    string `json:"summary" yaml:"summary"`
	OccurredAt string `json:"occurred_at" yaml:"occurred_at"`
	DetectedAt string `json:"detected_at,omitempty" yaml:"detected_at,omitempty"`
	ResolvedAt string `json:"resolved_at,omitempty" yaml:"resolved_at,omitempty"`
}

// Application identifies the application, service, environment, and release
// in which the incident occurred. Excluded from the fingerprint so the same
// underlying failure pattern is recognized across apps and releases.
type Application struct {
	Name           string `json:"name" yaml:"name"`
	Service        string `json:"service" yaml:"service"`
	Environment    string `json:"environment" yaml:"environment"`
	ReleaseVersion string `json:"release_version" yaml:"release_version"`
	CommitRef      string `json:"commit_ref,omitempty" yaml:"commit_ref,omitempty"`
}

// Trigger describes the condition(s) that set the incident in motion.
//
// RawPayloadExcerpt is a "known sensitive location" (see internal/validate
// and docs/privacy-model.md): when Privacy.Redacted is true, this field must
// be empty.
type Trigger struct {
	Type              string   `json:"type" yaml:"type"`
	Description       string   `json:"description" yaml:"description"`
	Preconditions     []string `json:"preconditions,omitempty" yaml:"preconditions,omitempty"`
	RawPayloadExcerpt string   `json:"raw_payload_excerpt,omitempty" yaml:"raw_payload_excerpt,omitempty"`
}

// Event is one node in the incident's causal graph. CausedBy references the
// IDs of events that causally precede this one; the resulting graph must be
// acyclic and every referenced ID must resolve to a defined event.
//
// RawPayloadExcerpt is a known sensitive location, see Trigger.
type Event struct {
	ID                string   `json:"id" yaml:"id"`
	Type              string   `json:"type" yaml:"type"`
	Description       string   `json:"description" yaml:"description"`
	CausedBy          []string `json:"caused_by,omitempty" yaml:"caused_by,omitempty"`
	RawPayloadExcerpt string   `json:"raw_payload_excerpt,omitempty" yaml:"raw_payload_excerpt,omitempty"`
}

// Service is a service or dependency implicated in the incident.
type Service struct {
	Name               string `json:"name" yaml:"name"`
	Role               string `json:"role" yaml:"role"`
	TechnologyCategory string `json:"technology_category" yaml:"technology_category"`
}

// SideEffect is an externally observable business consequence of the
// incident, such as a duplicate charge or a lost order.
type SideEffect struct {
	Type           string `json:"type" yaml:"type"`
	Description    string `json:"description" yaml:"description"`
	AffectedEntity string `json:"affected_entity" yaml:"affected_entity"`
	Reversible     bool   `json:"reversible" yaml:"reversible"`
}

// Invariant is a business rule the incident violated, e.g. "at most one
// completed charge per order". At least one is required.
type Invariant struct {
	ID        string `json:"id" yaml:"id"`
	Statement string `json:"statement" yaml:"statement"`
}

// Evidence references material substantiating the incident record (a log
// excerpt, a trace, a ticket). Digest must be a "sha256:"-prefixed SHA-256
// hex digest. Location is excluded from the fingerprint and is never
// dereferenced or opened by this tool — it is inert metadata.
//
// RawExcerpt is a known sensitive location, see Trigger.
type Evidence struct {
	ID         string `json:"id" yaml:"id"`
	Type       string `json:"type" yaml:"type"`
	Location   string `json:"location" yaml:"location"`
	Digest     string `json:"digest" yaml:"digest"`
	RawExcerpt string `json:"raw_excerpt,omitempty" yaml:"raw_excerpt,omitempty"`
}

// ReproStep is one step of the minimum sequence of actions that reproduces
// the incident. RefEventID, if set, must resolve to a defined Event.ID.
type ReproStep struct {
	Step        int    `json:"step" yaml:"step"`
	Description string `json:"description" yaml:"description"`
	RefEventID  string `json:"ref_event_id,omitempty" yaml:"ref_event_id,omitempty"`
}

// Sensitivity classification values accepted for Privacy.Sensitivity.
const (
	SensitivityPublic       = "public"
	SensitivityInternal     = "internal"
	SensitivityConfidential = "confidential"
	SensitivityRestricted   = "restricted"
)

// Privacy records the incident's sensitivity classification and redaction
// status. See docs/privacy-model.md for the "known sensitive locations"
// checked when Redacted is true.
type Privacy struct {
	Sensitivity string `json:"sensitivity" yaml:"sensitivity"`
	Redacted    bool   `json:"redacted" yaml:"redacted"`
	Notes       string `json:"notes,omitempty" yaml:"notes,omitempty"`
}
