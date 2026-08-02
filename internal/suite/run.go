// run.go implements the local, sequential, offline suite runner
// (docs/phase-5-plan.md §2/§7/§9/§13): sequential execution of every listed
// scenario, in declared order, via internal/scenario.Run (unchanged, once
// per scenario), aggregate outcome classification, and --fail-fast
// short-circuiting.
//
// Run performs the same pre-execution checks suite verify performs
// (Validate) before ever preparing a workspace root or running any scenario
// — a suite that fails those checks (or lists an invalid scenario) is
// classified INVALID and nothing is ever executed (docs/phase-5-plan.md
// §14/§17): "a reviewer's approval of 'this suite is valid' always means
// 'every scenario in it is valid,' never 'most of them are.'"
package suite

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/scenario"
)

// AggregateOutcome is the suite-level classification suite run reports
// (docs/phase-5-plan.md §14).
type AggregateOutcome string

const (
	AggregatePass          AggregateOutcome = "PASS"
	AggregateFail          AggregateOutcome = "FAIL"
	AggregateInvalid       AggregateOutcome = "INVALID"
	AggregateInternalError AggregateOutcome = "INTERNAL_ERROR"
)

// ScenarioOutcome is one listed scenario's outcome within a suite run: the
// same five values scenario.Outcome already reports, plus SKIPPED for a
// scenario never reached because --fail-fast stopped the suite early
// (docs/phase-5-plan.md §4 Workflow C).
type ScenarioOutcome string

const (
	ScenarioOutcomePass                          = ScenarioOutcome(scenario.OutcomePass)
	ScenarioOutcomeFail                          = ScenarioOutcome(scenario.OutcomeFail)
	ScenarioOutcomeTimeout                       = ScenarioOutcome(scenario.OutcomeTimeout)
	ScenarioOutcomeInvalid                       = ScenarioOutcome(scenario.OutcomeInvalid)
	ScenarioOutcomeInternalError                 = ScenarioOutcome(scenario.OutcomeInternalError)
	ScenarioOutcomeSkipped       ScenarioOutcome = "SKIPPED"
)

// ScenarioResult is one listed scenario's outcome within a suite run.
// Result is nil only when Outcome is ScenarioOutcomeSkipped (the scenario
// was never executed because --fail-fast stopped the suite first).
type ScenarioResult struct {
	Path    string
	Outcome ScenarioOutcome
	Result  *scenario.RunResult
}

// RunOptions configures Run's optional, non-default behavior. The zero value
// is the safe default: every scenario gets its own fresh os.MkdirTemp
// workspace (via scenario.Run's own default), each removed after that
// scenario's run completes, and every listed scenario runs regardless of
// earlier outcomes.
type RunOptions struct {
	// WorkspaceRoot, if non-empty, is created (if it does not yet exist) or
	// used as-is (if it exists and is empty), and each scenario gets its own
	// subdirectory under it instead of an independent os.MkdirTemp
	// directory. It must not already exist as a non-empty directory
	// (docs/phase-5-plan.md §9).
	WorkspaceRoot string
	// KeepWorkspaces, if true, leaves every scenario's workspace directory
	// (and WorkspaceRoot itself, if given) in place after the run completes
	// instead of removing them.
	KeepWorkspaces bool
	// FailFast, if true, stops running further scenarios immediately after
	// the first non-PASS outcome; scenarios never reached are recorded
	// ScenarioOutcomeSkipped rather than silently omitted.
	FailFast bool
}

// SuiteRunResult is the aggregate outcome of one Run call.
type SuiteRunResult struct {
	SuiteID    string
	Outcome    AggregateOutcome
	Scenarios  []ScenarioResult
	DurationMs int64
	// Error is non-empty only for AggregateInvalid/AggregateInternalError.
	Error string
}

// Run executes every scenario doc lists, in declared order, exactly once
// each (unless --fail-fast stops the suite early), and aggregates the
// result. suiteDir is the directory containing the suite manifest file
// itself, used both by the pre-execution Validate call and to resolve
// scenarios[].path entries.
func Run(ctx context.Context, doc *Document, suiteDir string, opts RunOptions) SuiteRunResult {
	start := time.Now()
	result := SuiteRunResult{SuiteID: doc.Suite.ID}

	verify := Validate(ctx, doc, suiteDir)
	if !verify.Valid() {
		result.Outcome = AggregateInvalid
		result.Error = verify.Error()
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}

	workspaceRoot, cleanupRoot, err := prepareWorkspaceRoot(opts.WorkspaceRoot)
	if err != nil {
		result.Outcome = AggregateInternalError
		result.Error = err.Error()
		result.DurationMs = time.Since(start).Milliseconds()
		return result
	}
	if !opts.KeepWorkspaces {
		defer cleanupRoot()
	}

	skipRest := false
	for i, entry := range doc.Scenarios {
		if skipRest {
			result.Scenarios = append(result.Scenarios, ScenarioResult{Path: entry.Path, Outcome: ScenarioOutcomeSkipped})
			continue
		}

		resolvedPath := filepath.Clean(filepath.Join(suiteDir, entry.Path))
		sdoc, lerr := scenario.LoadFile(resolvedPath)
		if lerr != nil {
			// A TOCTOU race: Validate already confirmed this file loaded and
			// validated moments earlier (docs/phase-5-plan.md §17).
			rr := scenario.RunResult{Outcome: scenario.OutcomeInternalError, Error: lerr.Error()}
			result.Scenarios = append(result.Scenarios, ScenarioResult{Path: entry.Path, Outcome: ScenarioOutcome(rr.Outcome), Result: &rr})
			if opts.FailFast {
				skipRest = true
			}
			continue
		}

		scenarioWorkspaceDir := ""
		if workspaceRoot != "" {
			scenarioWorkspaceDir = filepath.Join(workspaceRoot, scenarioSubdirName(i, sdoc.Scenario.ID))
		}

		rr := scenario.Run(ctx, sdoc, filepath.Dir(resolvedPath), scenario.RunOptions{
			WorkspaceDir:  scenarioWorkspaceDir,
			KeepWorkspace: opts.KeepWorkspaces,
		})
		result.Scenarios = append(result.Scenarios, ScenarioResult{Path: entry.Path, Outcome: ScenarioOutcome(rr.Outcome), Result: &rr})

		if rr.Outcome != scenario.OutcomePass && opts.FailFast {
			skipRest = true
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	result.Outcome = aggregateOutcome(result.Scenarios)
	return result
}

// aggregateOutcome is a pure function of the per-scenario outcomes
// (docs/phase-5-plan.md §7.4): PASS only if every listed scenario's own
// outcome was PASS; FAIL otherwise (FAIL, TIMEOUT, INVALID, INTERNAL_ERROR,
// or SKIPPED all count as a materially negative finding for the suite as a
// whole — docs/phase-5-plan.md §17's "one scenario's environmental failure
// does not abort the whole suite's report generation" applies identically to
// every non-PASS outcome).
func aggregateOutcome(results []ScenarioResult) AggregateOutcome {
	for _, r := range results {
		if r.Outcome != ScenarioOutcomePass {
			return AggregateFail
		}
	}
	return AggregatePass
}

// prepareWorkspaceRoot resolves opts.WorkspaceRoot: empty means "no shared
// root," in which case each scenario uses its own independent os.MkdirTemp
// workspace via scenario.Run's own default. A non-empty root must either not
// yet exist (it is created) or exist as an empty, non-symlinked directory —
// otherwise ErrSuiteWorkspaceRootNotEmpty is returned and nothing is written
// (docs/phase-5-plan.md §9).
func prepareWorkspaceRoot(root string) (resolved string, cleanup func(), err error) {
	if root == "" {
		return "", func() {}, nil
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, fmt.Errorf("resolve --workspace-root %q: %w", root, err)
	}
	abs = filepath.Clean(abs)

	info, statErr := os.Lstat(abs)
	switch {
	case statErr == nil:
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("--workspace-root %q must not be a symbolic link", root)
		}
		if !info.IsDir() {
			return "", nil, fmt.Errorf("--workspace-root %q exists and is not a directory", root)
		}
		entries, rerr := os.ReadDir(abs)
		if rerr != nil {
			return "", nil, fmt.Errorf("read --workspace-root %q: %w", root, rerr)
		}
		if len(entries) > 0 {
			return "", nil, fmt.Errorf("%w: %q", ErrSuiteWorkspaceRootNotEmpty, root)
		}
	case errors.Is(statErr, fs.ErrNotExist):
		if merr := os.MkdirAll(abs, 0o755); merr != nil {
			return "", nil, fmt.Errorf("create --workspace-root %q: %w", root, merr)
		}
	default:
		return "", nil, fmt.Errorf("stat --workspace-root %q: %w", root, statErr)
	}

	return abs, func() { os.RemoveAll(abs) }, nil
}

// scenarioSubdirName derives one scenario's subdirectory name under a shared
// --workspace-root: a zero-padded, 1-based index (guaranteeing uniqueness
// even if two scenario IDs collide or are empty) plus a sanitized form of
// the scenario's own id, for readability (docs/phase-5-plan.md §12).
func scenarioSubdirName(index int, scenarioID string) string {
	sanitized := sanitizeForDirName(scenarioID)
	if sanitized == "" {
		return fmt.Sprintf("%03d", index+1)
	}
	return fmt.Sprintf("%03d-%s", index+1, sanitized)
}

// sanitizeForDirName replaces every character outside [A-Za-z0-9._-] with
// '_' and bounds the result's length, so a scenario.id can never inject a
// path separator or otherwise unexpected byte into a workspace subdirectory
// name.
func sanitizeForDirName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}
