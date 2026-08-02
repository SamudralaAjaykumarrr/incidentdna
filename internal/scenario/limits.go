package scenario

import "fmt"

// Resource limits enforced by this package. Each is a fixed constant, not a
// configurable flag (docs/phase-4-plan.md §10), consistent with the
// fixed-constants discipline internal/evidence and internal/library already
// established. Each has its own distinct, actionable error and its own
// passing/failing boundary test pair.
const (
	// MaxScenarioDocumentSize is the maximum number of bytes a scenario
	// file's own on-disk size may occupy, enforced by LoadFile the same way
	// idir.LoadFile enforces idir.MaxDocumentSize.
	MaxScenarioDocumentSize = 1 * 1024 * 1024 // 1 MiB

	// MinScenarioTimeoutSeconds is the lower bound on a declared
	// timeout_seconds: 0 or negative is rejected as invalid rather than
	// silently treated as "no timeout".
	MinScenarioTimeoutSeconds = 1

	// DefaultScenarioTimeoutSeconds is used when timeout_seconds is
	// omitted.
	DefaultScenarioTimeoutSeconds = 30

	// MaxScenarioTimeoutSeconds is the upper bound a declared
	// timeout_seconds must not exceed, checked at verify time, not only at
	// run time.
	MaxScenarioTimeoutSeconds = 300

	// MaxWorkspaceFiles is the maximum number of execution.workspace_files
	// entries a scenario may declare.
	MaxWorkspaceFiles = 50

	// MaxWorkspaceFileSize is the maximum size of any single declared
	// workspace_files source file.
	MaxWorkspaceFileSize = 10 * 1024 * 1024 // 10 MiB

	// MaxWorkspaceTotalBytes is the maximum sum of all staged
	// workspace_files source file sizes.
	MaxWorkspaceTotalBytes = 50 * 1024 * 1024 // 50 MiB

	// MaxScenarioOutputBytes is the maximum number of bytes captured per
	// output stream (stdout, stderr independently) before truncation.
	MaxScenarioOutputBytes = 1 * 1024 * 1024 // 1 MiB
)

// checkDocumentSize enforces MaxScenarioDocumentSize against sizeBytes, the
// scenario file's on-disk size (os.FileInfo.Size()), before any parsing
// occurs.
func checkDocumentSize(sizeBytes int64) error {
	if sizeBytes > MaxScenarioDocumentSize {
		return &Error{
			Kind: ErrKindDocumentTooLarge,
			Msg: fmt.Sprintf(
				"scenario document is %d bytes, which exceeds the maximum scenario document size of %d bytes (%d MiB)",
				sizeBytes, MaxScenarioDocumentSize, MaxScenarioDocumentSize/(1024*1024),
			),
		}
	}
	return nil
}

// checkWorkspaceFileCount enforces MaxWorkspaceFiles against n
// (len(doc.Execution.WorkspaceFiles)).
func checkWorkspaceFileCount(n int) error {
	if n > MaxWorkspaceFiles {
		return &Error{
			Kind: ErrKindTooManyWorkspaceFiles,
			Msg: fmt.Sprintf(
				"execution.workspace_files declares %d entries, which exceeds the maximum of %d (MaxWorkspaceFiles)",
				n, MaxWorkspaceFiles,
			),
		}
	}
	return nil
}

// checkWorkspaceFileSize enforces MaxWorkspaceFileSize against one declared
// workspace_files source file's size.
func checkWorkspaceFileSize(source string, sizeBytes int64) error {
	if sizeBytes > MaxWorkspaceFileSize {
		return &Error{
			Kind: ErrKindWorkspaceFileTooLarge,
			Msg: fmt.Sprintf(
				"workspace_files source %q is %d bytes, which exceeds the maximum workspace file size of %d bytes (%d MiB)",
				source, sizeBytes, MaxWorkspaceFileSize, MaxWorkspaceFileSize/(1024*1024),
			),
		}
	}
	return nil
}

// checkWorkspaceTotalBytes enforces MaxWorkspaceTotalBytes against the sum
// of every declared workspace_files source file's size.
func checkWorkspaceTotalBytes(totalBytes int64) error {
	if totalBytes > MaxWorkspaceTotalBytes {
		return &Error{
			Kind: ErrKindWorkspaceTotalTooLarge,
			Msg: fmt.Sprintf(
				"execution.workspace_files sources total %d bytes, which exceeds the maximum aggregate workspace size of %d bytes (%d MiB)",
				totalBytes, MaxWorkspaceTotalBytes, MaxWorkspaceTotalBytes/(1024*1024),
			),
		}
	}
	return nil
}

// checkTimeoutBounds enforces MinScenarioTimeoutSeconds and
// MaxScenarioTimeoutSeconds against an explicitly declared timeout_seconds
// value. Returns a plain error (not *Error) since this is a semantic
// validation rule reported through Result/Issue, not a resource-limit
// failure a caller needs to dispatch on by Kind.
func checkTimeoutBounds(seconds int) error {
	if seconds < MinScenarioTimeoutSeconds {
		return fmt.Errorf("timeout_seconds %d is below the minimum of %d seconds (MinScenarioTimeoutSeconds)", seconds, MinScenarioTimeoutSeconds)
	}
	if seconds > MaxScenarioTimeoutSeconds {
		return fmt.Errorf("timeout_seconds %d exceeds the maximum of %d seconds (MaxScenarioTimeoutSeconds)", seconds, MaxScenarioTimeoutSeconds)
	}
	return nil
}

// EffectiveTimeoutSeconds returns the timeout a scenario document's
// execution.timeout_seconds resolves to: the declared value if present,
// otherwise DefaultScenarioTimeoutSeconds. Callers must have already run
// Validate (which enforces checkTimeoutBounds) before trusting this value as
// within bounds.
func EffectiveTimeoutSeconds(doc *Document) int {
	if doc.Execution.TimeoutSeconds != nil {
		return *doc.Execution.TimeoutSeconds
	}
	return DefaultScenarioTimeoutSeconds
}
