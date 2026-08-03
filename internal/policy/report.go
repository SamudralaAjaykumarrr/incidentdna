// report.go implements the machine-readable JSON verdict report
// (docs/phase-7-plan.md §12): a fixed Go struct (no map iteration, so field
// order is deterministic across repeated marshals of the same value), and
// the --report file-write behavior (always overwritten unconditionally, no
// append, no versioning — mirrors scenario run --report/suite run --report
// exactly).
package policy

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReportSchemaVersion is the fixed schema_version literal every Report
// carries.
const ReportSchemaVersion = "policy-report/v0.1"

// RuleReport is one declared rule's entry in the JSON verdict report.
type RuleReport struct {
	Type   string `json:"type"`
	Value  string `json:"value,omitempty"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// Report is the deterministic JSON verdict report policy evaluate writes to
// --report <path>. Verdict holds the overall outcome: PASS, FAIL, INVALID,
// or INTERNAL_ERROR (docs/phase-7-plan.md §11) — a superset of Evaluate's
// own Verdict.Result (PASS/FAIL only), since INVALID/INTERNAL_ERROR are
// pre-evaluation or environmental outcomes the cmd/incidentdna layer
// determines before Evaluate is ever called. Rules is empty for INVALID/
// INTERNAL_ERROR, since nothing was evaluated. Error is populated only for
// INVALID/INTERNAL_ERROR, with a short, actionable message.
type Report struct {
	SchemaVersion   string       `json:"schema_version"`
	PolicyID        string       `json:"policy_id"`
	PolicyFile      string       `json:"policy_file"`
	InputReportKind string       `json:"input_report_kind"`
	InputReportFile string       `json:"input_report_file"`
	Verdict         string       `json:"verdict"`
	Rules           []RuleReport `json:"rules"`
	Error           *string      `json:"error"`
}

// BuildReport builds a Report from the evaluation inputs/outputs the CLI
// layer already has: the policy's own id and file path as named on the
// command line, the input report's kind and file path as named on the
// command line, the overall verdict string, every rule's own outcome (nil
// or empty when nothing was evaluated), and a possibly-empty error message
// for the INVALID/INTERNAL_ERROR case.
func BuildReport(policyID, policyFile, inputReportKind, inputReportFile, verdict string, rules []RuleOutcome, errMsg string) Report {
	r := Report{
		SchemaVersion:   ReportSchemaVersion,
		PolicyID:        policyID,
		PolicyFile:      policyFile,
		InputReportKind: inputReportKind,
		InputReportFile: inputReportFile,
		Verdict:         verdict,
	}
	for _, ro := range rules {
		r.Rules = append(r.Rules, RuleReport{
			Type:   ro.Type,
			Value:  ro.Value,
			Status: string(ro.Status),
			Detail: ro.Detail,
		})
	}
	if errMsg != "" {
		e := errMsg
		r.Error = &e
	}
	return r
}

// WriteReport marshals r as indented JSON (deterministic field order, fixed
// struct — no map iteration) and writes it to path, overwriting any
// existing content unconditionally: no append, no versioning, no
// timestamped filename. Running policy evaluate twice with the same
// --report path leaves exactly one, current report on disk.
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
