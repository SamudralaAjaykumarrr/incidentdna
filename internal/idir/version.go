// Package idir defines the Incident Deterministic Intermediate Representation
// (IDIR): a vendor-neutral document format for describing production
// incidents as permanent, executable regression scenarios.
package idir

// SupportedSchemaVersion is the only IDIR schema version accepted in Phase 1.
// A document whose SchemaVersion does not equal this value is rejected by
// validation before any semantic rule runs.
const SupportedSchemaVersion = "0.1"

// MaxDocumentSize is the maximum number of bytes an IDIR document may occupy
// on disk before it is rejected. It bounds memory and CPU spent parsing a
// maliciously or accidentally oversized document (see docs/threat-model.md,
// "Resource exhaustion from maliciously large documents").
const MaxDocumentSize = 5 * 1024 * 1024 // 5 MiB
