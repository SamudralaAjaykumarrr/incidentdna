package scenario

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/fingerprint"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// minimalIncidentDoc builds the smallest idir.Document that passes
// validate.Validate, mirroring cmd/incidentdna's
// minimalRedactedLibraryDoc fixture pattern.
func minimalIncidentDoc(triggerType string) *idir.Document {
	return &idir.Document{
		SchemaVersion: idir.SupportedSchemaVersion,
		Incident: idir.Incident{
			ID:         "INC-SCENARIO-TEST-0001",
			Title:      "Synthetic incident for scenario fingerprint-link tests",
			OccurredAt: "2026-01-01T00:00:00Z",
		},
		Trigger: idir.Trigger{
			Type:        triggerType,
			Description: "Synthetic trigger, fictional, for scenario tests only.",
		},
		BusinessInvariants: []idir.Invariant{
			{ID: "inv-1", Statement: "at most one synthetic invariant for testing"},
		},
		ExpectedCorrectedBehavior: []string{"n/a - synthetic test fixture"},
		Privacy: idir.Privacy{
			Sensitivity: idir.SensitivityInternal,
		},
	}
}

func writeIncidentJSON(t *testing.T, dir, name string, doc *idir.Document) string {
	t.Helper()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCheckSourceFingerprint_Match(t *testing.T) {
	dir := t.TempDir()
	doc := minimalIncidentDoc("synthetic-trigger")
	path := writeIncidentJSON(t, dir, "incident.json", doc)

	want, err := fingerprint.Compute(doc)
	if err != nil {
		t.Fatal(err)
	}

	res, err := CheckSourceFingerprint(context.Background(), path, want)
	if err != nil {
		t.Fatalf("CheckSourceFingerprint: %v", err)
	}
	if res.SourceFingerprint != want {
		t.Fatalf("expected source fingerprint %q, got %q", want, res.SourceFingerprint)
	}
	if !res.Match {
		t.Fatal("expected Match to be true")
	}
}

func TestCheckSourceFingerprint_Mismatch(t *testing.T) {
	dir := t.TempDir()
	doc := minimalIncidentDoc("synthetic-trigger")
	path := writeIncidentJSON(t, dir, "incident.json", doc)

	declared := "sha256:" + strings.Repeat("0", 64)
	res, err := CheckSourceFingerprint(context.Background(), path, declared)
	if err != nil {
		t.Fatalf("CheckSourceFingerprint: %v", err)
	}
	if res.Match {
		t.Fatal("expected Match to be false for a materially different declared fingerprint")
	}
}

func TestCheckSourceFingerprint_SourceFailsValidation(t *testing.T) {
	dir := t.TempDir()
	doc := minimalIncidentDoc("synthetic-trigger")
	doc.BusinessInvariants = nil // fails validate.Validate: at least one required
	path := writeIncidentJSON(t, dir, "incident.json", doc)

	_, err := CheckSourceFingerprint(context.Background(), path, "sha256:"+strings.Repeat("0", 64))
	if err == nil {
		t.Fatal("expected an error when --source fails validation")
	}
}

func TestCheckSourceFingerprint_SourceMissing(t *testing.T) {
	_, err := CheckSourceFingerprint(context.Background(), filepath.Join(t.TempDir(), "missing.json"), "sha256:"+strings.Repeat("0", 64))
	if err == nil {
		t.Fatal("expected an error when --source cannot be loaded")
	}
}
