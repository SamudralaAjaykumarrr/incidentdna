// report_load.go loads an already-produced scenario.Report or suite.Report
// JSON artifact (Phase 4/5, unchanged irs-report/v0.1 and suite-report/v0.1
// shapes) into the caller-facing LoadedReport shape Evaluate consumes
// (docs/phase-7-plan.md §6). internal/policy imports internal/scenario and
// internal/suite only for these existing, unchanged Report struct
// definitions — it never calls LoadFile, Validate, or Run from either
// package, and neither package is modified to accommodate this
// (docs/phase-7-plan.md §16).
package policy

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/scenario"
	"github.com/SamudralaAjaykumarrr/incidentdna/internal/suite"
)

// ReportKind identifies which already-produced report artifact a
// LoadedReport was extracted from.
type ReportKind string

const (
	ReportKindScenario ReportKind = "scenario"
	ReportKindSuite    ReportKind = "suite"
)

// LoadedReport is the caller-facing shape Evaluate consumes: an
// already-produced report's own top-level result classification, and the
// distinct linked_fingerprint values found in it (docs/phase-7-plan.md §7
// point 4) — extracted once here so Evaluate itself never has to know about
// either report package's own struct shape.
type LoadedReport struct {
	Kind                 ReportKind
	Result               string
	DistinctFingerprints []string
}

// LoadScenarioReport loads an already-produced scenario.Report JSON file
// (unchanged irs-report/v0.1 shape, Phase 4) from path, size-capped at
// MaxReportDocumentSize, and rejects it unless its schema_version equals
// scenario.ReportSchemaVersion exactly.
func LoadScenarioReport(path string) (LoadedReport, error) {
	data, err := readCappedReportFile(path)
	if err != nil {
		return LoadedReport{}, err
	}

	var rep scenario.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return LoadedReport{}, fmt.Errorf("policy: parse scenario report %s: %w", path, err)
	}
	if rep.SchemaVersion != scenario.ReportSchemaVersion {
		return LoadedReport{}, fmt.Errorf("policy: %s: schema_version must be %q, got %q", path, scenario.ReportSchemaVersion, rep.SchemaVersion)
	}

	var fps []string
	if rep.LinkedFingerprint != "" {
		fps = append(fps, rep.LinkedFingerprint)
	}
	if err := checkDistinctFingerprintCount(len(fps)); err != nil {
		return LoadedReport{}, fmt.Errorf("policy: %s: %w", path, err)
	}

	return LoadedReport{Kind: ReportKindScenario, Result: rep.Result, DistinctFingerprints: fps}, nil
}

// LoadSuiteReport loads an already-produced suite.Report JSON file
// (unchanged suite-report/v0.1 shape, Phase 5) from path, size-capped at
// MaxReportDocumentSize, and rejects it unless its schema_version equals
// suite.ReportSchemaVersion exactly. Distinct linked_fingerprint values are
// collected in the order their owning scenarios[] entry appears,
// deduplicated to first occurrence (docs/phase-7-plan.md §7 point 4) — the
// identical discipline docs/library-crossref.md already established for
// suite verify --library. A scenarios[] entry whose embedded report is nil
// (a SKIPPED scenario) or whose linked_fingerprint is empty (a scenario that
// never reached execution, e.g. INTERNAL_ERROR before the scenario document
// loaded) contributes no fingerprint.
func LoadSuiteReport(path string) (LoadedReport, error) {
	data, err := readCappedReportFile(path)
	if err != nil {
		return LoadedReport{}, err
	}

	var rep suite.Report
	if err := json.Unmarshal(data, &rep); err != nil {
		return LoadedReport{}, fmt.Errorf("policy: parse suite report %s: %w", path, err)
	}
	if rep.SchemaVersion != suite.ReportSchemaVersion {
		return LoadedReport{}, fmt.Errorf("policy: %s: schema_version must be %q, got %q", path, suite.ReportSchemaVersion, rep.SchemaVersion)
	}

	seen := make(map[string]bool, len(rep.Scenarios))
	var fps []string
	for _, entry := range rep.Scenarios {
		if entry.Report == nil {
			continue
		}
		fp := entry.Report.LinkedFingerprint
		if fp == "" || seen[fp] {
			continue
		}
		seen[fp] = true
		fps = append(fps, fp)
	}
	if err := checkDistinctFingerprintCount(len(fps)); err != nil {
		return LoadedReport{}, fmt.Errorf("policy: %s: %w", path, err)
	}

	return LoadedReport{Kind: ReportKindSuite, Result: rep.Result, DistinctFingerprints: fps}, nil
}

// readCappedReportFile reads path's full contents, rejecting it outright if
// its on-disk size exceeds MaxReportDocumentSize before any parsing occurs —
// the identical stat-then-limited-read discipline scenario.LoadFile/
// suite.LoadFile/policy.LoadFile already establish. Report artifacts are
// always JSON (scenario.WriteReport/suite.WriteReport never write YAML), so
// no format sniffing is needed here.
func readCappedReportFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s is a directory, not a file", path)
	}
	if err := checkReportDocumentSize(info.Size()); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	limited := io.LimitReader(f, MaxReportDocumentSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := checkReportDocumentSize(int64(len(data))); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return data, nil
}
