// validate.go implements every ISM v0.1 semantic rule from
// docs/phase-5-plan.md §5/§17: schema_version, non-empty suite.id, a
// non-empty scenarios list bounded by MaxScenariosPerSuite, scenarios[].path
// safety/existence/duplicate rejection, and the aggregate timeout bound
// against MaxSuiteTotalTimeoutSeconds. For every listed scenario that
// resolves to a real file, Validate additionally calls scenario.LoadFile and
// scenario.Validate, unchanged — a suite is valid only if every scenario it
// lists is itself valid; Validate never executes anything.
//
// Validate's result depends only on the suite manifest's own decoded fields
// and the bytes of the scenario files it lists — never on any *workspace*
// filesystem state, which does not exist yet at verify time
// (docs/phase-5-plan.md §7.3).
package suite

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/scenario"
)

// Issue is one semantic validation failure: the document field it applies
// to and an actionable, human-readable message. Mirrors
// internal/scenario.Issue.
type Issue struct {
	Field   string
	Message string
}

func (i Issue) String() string {
	if i.Field == "" {
		return i.Message
	}
	return i.Field + ": " + i.Message
}

// ScenarioCheck is the per-listed-scenario outcome of Validate: whether that
// one entry (its declared path, and — if it resolved and loaded — the
// scenario document itself) is valid, and its scenario.id if it loaded
// successfully far enough to have one. Populated in declared order,
// regardless of whether earlier entries were valid.
type ScenarioCheck struct {
	Path       string
	ScenarioID string
	Valid      bool
	Issues     []string
}

// Result collects every suite-level Issue found while validating a suite
// manifest, plus the per-listed-scenario check results. A document with no
// suite-level issues and every scenario Valid is itself valid. Mirrors
// internal/scenario.Result, extended with per-scenario detail so `suite
// verify` can print one line per listed scenario (docs/phase-5-plan.md §4
// Workflow A).
type Result struct {
	Issues    []Issue
	Scenarios []ScenarioCheck
}

// Valid reports whether the suite manifest had zero suite-level issues and
// every listed scenario is itself valid.
func (r Result) Valid() bool {
	if len(r.Issues) > 0 {
		return false
	}
	for _, s := range r.Scenarios {
		if !s.Valid {
			return false
		}
	}
	return true
}

// Error implements the error interface so a Result can be returned directly
// where an error is expected, listing every suite-level issue.
func (r Result) Error() string {
	lines := make([]string, len(r.Issues))
	for i, issue := range r.Issues {
		lines[i] = issue.String()
	}
	return strings.Join(lines, "\n")
}

func (r *Result) add(field, format string, args ...any) {
	r.Issues = append(r.Issues, Issue{Field: field, Message: fmt.Sprintf(format, args...)})
}

// Validate runs every ISM v0.1 semantic rule against doc, and validates every
// listed scenario file it resolves to. suiteDir is the directory containing
// the suite manifest file itself (typically filepath.Dir(suiteFilePath)),
// used to resolve scenarios[].path entries.
func Validate(ctx context.Context, doc *Document, suiteDir string) Result {
	var res Result
	if doc == nil {
		res.add("", "suite document is nil")
		return res
	}

	if doc.SchemaVersion != SupportedSchemaVersion {
		res.add("schema_version", "must be %q, got %q", SupportedSchemaVersion, doc.SchemaVersion)
	}

	if strings.TrimSpace(doc.Suite.ID) == "" {
		res.add("suite.id", "must not be empty")
	}

	if len(doc.Scenarios) == 0 {
		res.add("scenarios", "must declare at least one scenario")
		return res
	}

	if err := checkScenarioCount(len(doc.Scenarios)); err != nil {
		res.add("scenarios", "%v", err)
		return res
	}

	seenPaths := make(map[string]bool, len(doc.Scenarios))
	totalTimeoutSeconds := 0

	for i, entry := range doc.Scenarios {
		if err := ctx.Err(); err != nil {
			res.add("scenarios", "validation canceled: %v", err)
			return res
		}
		field := fmt.Sprintf("scenarios[%d]", i)
		check := ScenarioCheck{Path: entry.Path}

		cleanPath, err := safeRelativePath(entry.Path)
		if err != nil {
			res.add(field+".path", "%v", err)
			check.Issues = append(check.Issues, err.Error())
			res.Scenarios = append(res.Scenarios, check)
			continue
		}

		if seenPaths[cleanPath] {
			msg := fmt.Sprintf("duplicate scenario path %q: every scenarios[] entry must reference a distinct file", cleanPath)
			res.add(field+".path", "%s", msg)
			check.Issues = append(check.Issues, msg)
			res.Scenarios = append(res.Scenarios, check)
			continue
		}
		seenPaths[cleanPath] = true

		if suiteDir == "" {
			res.Scenarios = append(res.Scenarios, check)
			continue
		}

		resolvedPath := filepath.Clean(filepath.Join(suiteDir, cleanPath))
		rootWithSep := filepath.Clean(suiteDir) + string(filepath.Separator)
		if resolvedPath != filepath.Clean(suiteDir) && !strings.HasPrefix(resolvedPath, rootWithSep) {
			msg := "resolves outside the suite manifest's own directory"
			res.add(field+".path", "%s", msg)
			check.Issues = append(check.Issues, msg)
			res.Scenarios = append(res.Scenarios, check)
			continue
		}

		info, lerr := os.Lstat(resolvedPath)
		if lerr != nil {
			msg := fmt.Sprintf("%v: %v", ErrSuiteScenarioMissing, lerr)
			res.add(field+".path", "%s", msg)
			check.Issues = append(check.Issues, msg)
			res.Scenarios = append(res.Scenarios, check)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			msg := fmt.Sprintf("%v: %q is a symbolic link, not a regular file", ErrSuiteScenarioMissing, entry.Path)
			res.add(field+".path", "%s", msg)
			check.Issues = append(check.Issues, msg)
			res.Scenarios = append(res.Scenarios, check)
			continue
		}
		if !info.Mode().IsRegular() {
			msg := fmt.Sprintf("%v: %q is not a regular file", ErrSuiteScenarioMissing, entry.Path)
			res.add(field+".path", "%s", msg)
			check.Issues = append(check.Issues, msg)
			res.Scenarios = append(res.Scenarios, check)
			continue
		}

		sdoc, lerr := scenario.LoadFile(resolvedPath)
		if lerr != nil {
			msg := lerr.Error()
			res.add(field, "failed to load: %s", msg)
			check.Issues = append(check.Issues, msg)
			res.Scenarios = append(res.Scenarios, check)
			continue
		}
		check.ScenarioID = sdoc.Scenario.ID

		sres := scenario.Validate(ctx, sdoc, filepath.Dir(resolvedPath))
		if !sres.Valid() {
			for _, issue := range sres.Issues {
				msg := issue.String()
				res.add(field, "scenario is invalid: %s", msg)
				check.Issues = append(check.Issues, msg)
			}
			res.Scenarios = append(res.Scenarios, check)
			continue
		}

		check.Valid = true
		res.Scenarios = append(res.Scenarios, check)
		totalTimeoutSeconds += scenario.EffectiveTimeoutSeconds(sdoc)
	}

	if err := checkTotalTimeoutSeconds(totalTimeoutSeconds); err != nil {
		res.add("scenarios", "%v", err)
	}

	return res
}

// safeRelativePath validates p as a non-empty, non-absolute, "..".free
// relative path and returns its filepath.Clean form. This structurally
// prevents a scenarios[].path from escaping its resolved root (the suite
// manifest's own directory), mirroring internal/scenario's identical
// workspace_files path-safety discipline, applied here to a declared
// scenario-file path instead.
func safeRelativePath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("must not be empty")
	}
	if filepath.IsAbs(p) {
		return "", fmt.Errorf("must not be an absolute path, got %q", p)
	}
	cleaned := filepath.Clean(p)
	if cleaned == "." {
		return "", fmt.Errorf("must not resolve to its root directory itself, got %q", p)
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("must not escape its root directory (contains \"..\"), got %q", p)
	}
	return cleaned, nil
}
