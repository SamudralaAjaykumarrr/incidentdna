// report.go implements the machine-readable JSON execution report
// (docs/phase-4-plan.md §15): a fixed Go struct (no map iteration, so field
// order is deterministic across repeated marshals of the same value), and
// the --report file-write behavior (always overwritten unconditionally, no
// append, no versioning; docs/phase-4-plan.md §16).
package scenario

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReportSchemaVersion is the fixed schema_version literal every Report
// carries.
const ReportSchemaVersion = "irs-report/v0.1"

// StreamReport is one captured output stream's report representation.
type StreamReport struct {
	Excerpt   string `json:"excerpt"`
	Truncated bool   `json:"truncated"`
}

// Report is the deterministic JSON execution report scenario run writes to
// --report <path>, mirroring RunResult's fields in the fixed field order
// docs/phase-4-plan.md §15 documents. WorkspacePath is populated only when
// --keep-workspace was given; Error is populated only for
// INVALID/INTERNAL_ERROR.
type Report struct {
	SchemaVersion     string       `json:"schema_version"`
	ScenarioID        string       `json:"scenario_id"`
	ScenarioFile      string       `json:"scenario_file"`
	LinkedFingerprint string       `json:"linked_fingerprint"`
	Result            string       `json:"result"`
	ExitCodeExpected  int          `json:"exit_code_expected"`
	ExitCodeActual    int          `json:"exit_code_actual"`
	DurationMs        int64        `json:"duration_ms"`
	Stdout            StreamReport `json:"stdout"`
	Stderr            StreamReport `json:"stderr"`
	WorkspacePath     *string      `json:"workspace_path"`
	Error             *string      `json:"error"`
}

// BuildReport builds a Report from a RunResult, the scenario file path as
// named on the command line (scenarioFile), and a possibly-empty
// linkedFingerprint override for the case RunResult itself could not be
// fully populated (e.g. the scenario document failed to load at all — see
// cmd/incidentdna/cmd_scenario.go).
func BuildReport(res RunResult, scenarioFile string) Report {
	r := Report{
		SchemaVersion:     ReportSchemaVersion,
		ScenarioID:        res.ScenarioID,
		ScenarioFile:      scenarioFile,
		LinkedFingerprint: res.LinkedFingerprint,
		Result:            string(res.Outcome),
		ExitCodeExpected:  res.ExitCodeExpected,
		ExitCodeActual:    res.ExitCodeActual,
		DurationMs:        res.DurationMs,
		Stdout:            StreamReport{Excerpt: res.Stdout.Excerpt, Truncated: res.Stdout.Truncated},
		Stderr:            StreamReport{Excerpt: res.Stderr.Excerpt, Truncated: res.Stderr.Truncated},
	}
	if res.WorkspacePath != "" {
		p := res.WorkspacePath
		r.WorkspacePath = &p
	}
	if res.Error != "" {
		e := res.Error
		r.Error = &e
	}
	return r
}

// WriteReport marshals r as indented JSON (deterministic field order, fixed
// struct — no map iteration) and writes it to path, overwriting any
// existing content unconditionally: no append, no versioning, no timestamped
// filename (docs/phase-4-plan.md §16). Running scenario run twice with the
// same --report path leaves exactly one, current report on disk.
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
