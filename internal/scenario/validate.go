// validate.go implements every IRS v0.1 semantic rule from
// docs/phase-4-plan.md §5/§17: schema_version, command shape (non-empty,
// non-bare command[0]), workspace_files path safety/existence/duplicate
// destinations, timeout bounds, linked_fingerprint format, and the
// expected-assertion shape. Validate never executes anything — it is the
// structural/semantic gate scenario verify and the pre-execution phase of
// scenario run both call (docs/phase-4-plan.md §13).
//
// Validate's result depends only on the scenario document's own decoded
// fields and, for workspace_files, the declared source files' existence
// relative to the scenario file's own directory — never on any *workspace*
// filesystem state, which does not exist yet at verify time
// (docs/phase-4-plan.md §7.8).
package scenario

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Issue is one semantic validation failure: the document field it applies
// to and an actionable, human-readable message. Mirrors
// internal/validate.Issue.
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

// Result collects every Issue found while validating a scenario document. A
// document with no issues is valid. Mirrors internal/validate.Result.
type Result struct {
	Issues []Issue
}

// Valid reports whether the document had zero validation issues.
func (r Result) Valid() bool {
	return len(r.Issues) == 0
}

// Error implements the error interface so a Result can be returned directly
// where an error is expected.
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

// linkedFingerprintPattern matches internal/fingerprint.Compute's exact
// external form: "sha256:" followed by 64 lowercase hex characters — the
// same pattern internal/library.fingerprintPattern already checks.
var linkedFingerprintPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Validate runs every IRS v0.1 semantic rule against doc. scenarioDir is the
// directory containing the scenario file itself (typically
// filepath.Dir(scenarioFilePath)), used to resolve and check
// execution.workspace_files[].source entries; it is only dereferenced when
// doc declares at least one workspace_files entry.
func Validate(ctx context.Context, doc *Document, scenarioDir string) Result {
	var res Result
	if doc == nil {
		res.add("", "scenario document is nil")
		return res
	}

	if doc.SchemaVersion != SupportedSchemaVersion {
		res.add("schema_version", "must be %q, got %q", SupportedSchemaVersion, doc.SchemaVersion)
	}

	if strings.TrimSpace(doc.Scenario.ID) == "" {
		res.add("scenario.id", "must not be empty")
	}

	validateCommand(&res, doc.Execution.Command)
	validateWorkspaceFiles(ctx, &res, doc.Execution.WorkspaceFiles, scenarioDir)
	validateTimeout(&res, doc.Execution.TimeoutSeconds)
	validateLinkedFingerprint(&res, doc.LinkedFingerprint)
	validateExpected(&res, doc.Expected)

	return res
}

func validateCommand(res *Result, command []string) {
	if len(command) == 0 {
		res.add("execution.command", "must not be empty")
		return
	}
	cmd0 := command[0]
	if cmd0 == "" {
		res.add("execution.command[0]", "must not be empty")
		return
	}
	if isBareCommand(cmd0) {
		res.add("execution.command[0]", "must be an absolute path or a path relative to the workspace root (containing a %q), not a bare command name %q that would require an implicit $PATH lookup", "/", cmd0)
	}
}

// isBareCommand reports whether cmd has no path separator and is not
// absolute — the shape docs/phase-4-plan.md §5/§11 rejects, since resolving
// it would require an implicit, host-dependent $PATH lookup.
func isBareCommand(cmd string) bool {
	return !filepath.IsAbs(cmd) && !strings.ContainsRune(cmd, '/')
}

func validateWorkspaceFiles(ctx context.Context, res *Result, files []WorkspaceFile, scenarioDir string) {
	if err := checkWorkspaceFileCount(len(files)); err != nil {
		res.add("execution.workspace_files", "%v", err)
		return
	}

	seenDestinations := make(map[string]bool, len(files))
	var totalBytes int64

	for i, wf := range files {
		if err := ctx.Err(); err != nil {
			res.add("execution.workspace_files", "validation canceled: %v", err)
			return
		}
		field := fmt.Sprintf("execution.workspace_files[%d]", i)

		cleanSource, srcErr := safeRelativePath(wf.Source)
		if srcErr != nil {
			res.add(field+".source", "%v", srcErr)
			continue
		}
		cleanDest, dstErr := safeRelativePath(wf.Destination)
		if dstErr != nil {
			res.add(field+".destination", "%v", dstErr)
			continue
		}

		if seenDestinations[cleanDest] {
			res.add(field+".destination", "duplicate destination %q: every workspace_files entry must stage to a distinct path", cleanDest)
			continue
		}
		seenDestinations[cleanDest] = true

		if scenarioDir == "" {
			continue
		}
		resolvedSource := filepath.Clean(filepath.Join(scenarioDir, cleanSource))
		rootWithSep := filepath.Clean(scenarioDir) + string(filepath.Separator)
		if resolvedSource != filepath.Clean(scenarioDir) && !strings.HasPrefix(resolvedSource, rootWithSep) {
			res.add(field+".source", "resolves outside the scenario file's own directory")
			continue
		}

		info, err := os.Lstat(resolvedSource)
		if err != nil {
			res.add(field+".source", "%v: %v", ErrWorkspaceFileMissing, err)
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			res.add(field+".source", "%v: %q is a symbolic link, not a regular file", ErrWorkspaceFileMissing, wf.Source)
			continue
		}
		if !info.Mode().IsRegular() {
			res.add(field+".source", "%v: %q is not a regular file", ErrWorkspaceFileMissing, wf.Source)
			continue
		}
		if err := checkWorkspaceFileSize(wf.Source, info.Size()); err != nil {
			res.add(field+".source", "%v", err)
			continue
		}
		totalBytes += info.Size()
	}

	if scenarioDir != "" {
		if err := checkWorkspaceTotalBytes(totalBytes); err != nil {
			res.add("execution.workspace_files", "%v", err)
		}
	}
}

// safeRelativePath validates p as a non-empty, non-absolute, "..".free
// relative path and returns its filepath.Clean form. This structurally
// prevents a workspace_files source or destination from escaping its
// resolved root, mirroring internal/library.Store's fingerprint/digest path
// safety applied to declared relative paths instead.
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

func validateTimeout(res *Result, timeoutSeconds *int) {
	if timeoutSeconds == nil {
		return
	}
	if err := checkTimeoutBounds(*timeoutSeconds); err != nil {
		res.add("execution.timeout_seconds", "%v", err)
	}
}

func validateLinkedFingerprint(res *Result, fp string) {
	if fp == "" {
		res.add("linked_fingerprint", "must not be empty")
		return
	}
	if !linkedFingerprintPattern.MatchString(fp) {
		res.add("linked_fingerprint", "must be \"sha256:\" followed by exactly 64 lowercase hexadecimal characters, got %q", fp)
	}
}

func validateExpected(res *Result, expected Expected) {
	if expected.ExitCode == nil {
		res.add("expected.exit_code", "must be declared")
	}
	validateStreamAssertion(res, "expected.stdout", expected.Stdout)
	validateStreamAssertion(res, "expected.stderr", expected.Stderr)
}

func validateStreamAssertion(res *Result, field string, assertion *StreamAssertion) {
	if assertion == nil {
		return
	}
	switch assertion.Mode {
	case StreamModeExact, StreamModeContains:
		// value is compared verbatim; any string, including empty, is valid.
	case StreamModeRegex:
		if _, err := regexp.Compile(assertion.Value); err != nil {
			res.add(field+".value", "is not a valid regular expression: %v", err)
		}
	default:
		res.add(field+".mode", "must be one of %q, %q, %q, got %q", StreamModeExact, StreamModeContains, StreamModeRegex, assertion.Mode)
	}
}
