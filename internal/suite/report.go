// report.go implements the machine-readable JSON aggregate report
// (docs/phase-5-plan.md §15): a fixed Go struct (no map iteration, so field
// order is deterministic across repeated marshals of the same value),
// embedding each listed scenario's own unmodified scenario.Report, and the
// --report file-write behavior (always overwritten unconditionally, no
// append, no versioning — mirrors scenario run --report exactly,
// docs/phase-5-plan.md §16).
package suite

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/scenario"
)

// ReportSchemaVersion is the fixed schema_version literal every Report
// carries.
const ReportSchemaVersion = "suite-report/v0.1"

// Counts is the per-outcome tally across every listed scenario in a suite
// run, a pure function of the per-scenario outcomes (docs/phase-5-plan.md
// §7.4).
type Counts struct {
	Pass          int `json:"pass"`
	Fail          int `json:"fail"`
	Timeout       int `json:"timeout"`
	Invalid       int `json:"invalid"`
	InternalError int `json:"internal_error"`
	Skipped       int `json:"skipped"`
}

// ScenarioReportEntry is one listed scenario's entry in the aggregate
// report: its declared path, and its own full scenario.Report — exactly the
// JSON object scenario.BuildReport already produces for that scenario,
// unchanged (docs/phase-5-plan.md §15). Report is nil only for a scenario
// that was never executed because --fail-fast stopped the suite first
// (ScenarioOutcomeSkipped).
type ScenarioReportEntry struct {
	Path   string           `json:"path"`
	Report *scenario.Report `json:"report"`
}

// Report is the deterministic JSON aggregate report suite run writes to
// --report <path>, mirroring SuiteRunResult's fields in the fixed field
// order docs/phase-5-plan.md §15 documents.
type Report struct {
	SchemaVersion string                `json:"schema_version"`
	SuiteID       string                `json:"suite_id"`
	SuiteFile     string                `json:"suite_file"`
	Result        string                `json:"result"`
	Counts        Counts                `json:"counts"`
	DurationMs    int64                 `json:"duration_ms"`
	Scenarios     []ScenarioReportEntry `json:"scenarios"`
	// Error is non-empty only for AggregateInvalid/AggregateInternalError.
	Error *string `json:"error"`
}

// CountOutcomes tallies every listed scenario's outcome, a pure function of
// the per-scenario outcomes (docs/phase-5-plan.md §7.4). Shared by BuildReport
// and the CLI's printed summary so both report identical counts.
func CountOutcomes(results []ScenarioResult) Counts {
	var c Counts
	for _, sr := range results {
		switch sr.Outcome {
		case ScenarioOutcomePass:
			c.Pass++
		case ScenarioOutcomeFail:
			c.Fail++
		case ScenarioOutcomeTimeout:
			c.Timeout++
		case ScenarioOutcomeInvalid:
			c.Invalid++
		case ScenarioOutcomeInternalError:
			c.InternalError++
		case ScenarioOutcomeSkipped:
			c.Skipped++
		}
	}
	return c
}

// BuildReport builds a Report from a SuiteRunResult and the suite manifest
// file path as named on the command line (suiteFile).
func BuildReport(res SuiteRunResult, suiteFile string) Report {
	r := Report{
		SchemaVersion: ReportSchemaVersion,
		SuiteID:       res.SuiteID,
		SuiteFile:     suiteFile,
		Result:        string(res.Outcome),
		Counts:        CountOutcomes(res.Scenarios),
		DurationMs:    res.DurationMs,
	}

	for _, sr := range res.Scenarios {
		entry := ScenarioReportEntry{Path: sr.Path}
		if sr.Result != nil {
			rep := scenario.BuildReport(*sr.Result, sr.Path)
			entry.Report = &rep
		}
		r.Scenarios = append(r.Scenarios, entry)
	}

	if res.Error != "" {
		e := res.Error
		r.Error = &e
	}
	return r
}

// WriteReport marshals r as indented JSON (deterministic field order, fixed
// struct — no map iteration) and writes it to path, overwriting any existing
// content unconditionally: no append, no versioning, no timestamped filename
// (docs/phase-5-plan.md §16). Running suite run twice with the same --report
// path leaves exactly one, current report on disk.
func WriteReport(path string, r Report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("encode report: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write report %q: %w", path, err)
	}
	return nil
}
