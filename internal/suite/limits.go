package suite

import "fmt"

// Resource limits enforced by this package. Each is a fixed constant, not a
// configurable flag (docs/phase-5-plan.md §10), consistent with the
// fixed-constants discipline internal/evidence, internal/library, and
// internal/scenario already established. Each has its own distinct,
// actionable error and its own passing/failing boundary test pair.
const (
	// MaxSuiteDocumentSize is the maximum number of bytes a suite manifest
	// file's own on-disk size may occupy, enforced by LoadFile the same way
	// scenario.LoadFile enforces scenario.MaxScenarioDocumentSize.
	MaxSuiteDocumentSize = 256 * 1024 // 256 KiB

	// MaxScenariosPerSuite is the maximum number of scenarios[] entries a
	// suite manifest may declare.
	MaxScenariosPerSuite = 100

	// MaxSuiteTotalTimeoutSeconds is the maximum sum of every listed
	// scenario's declared (or default) execution.timeout_seconds, checked at
	// verify time — a structural backstop bounding a suite's total
	// wall-clock budget, not a separately-configurable suite timeout
	// (docs/phase-5-plan.md §9/§10).
	MaxSuiteTotalTimeoutSeconds = 1800 // 30 minutes
)

// checkDocumentSize enforces MaxSuiteDocumentSize against sizeBytes, the
// suite manifest's on-disk size (os.FileInfo.Size()), before any parsing
// occurs.
func checkDocumentSize(sizeBytes int64) error {
	if sizeBytes > MaxSuiteDocumentSize {
		return &Error{
			Kind: ErrKindDocumentTooLarge,
			Msg: fmt.Sprintf(
				"suite manifest is %d bytes, which exceeds the maximum suite document size of %d bytes (%d KiB)",
				sizeBytes, MaxSuiteDocumentSize, MaxSuiteDocumentSize/1024,
			),
		}
	}
	return nil
}

// checkScenarioCount enforces MaxScenariosPerSuite against n
// (len(doc.Scenarios)).
func checkScenarioCount(n int) error {
	if n > MaxScenariosPerSuite {
		return &Error{
			Kind: ErrKindTooManyScenarios,
			Msg: fmt.Sprintf(
				"scenarios declares %d entries, which exceeds the maximum of %d (MaxScenariosPerSuite)",
				n, MaxScenariosPerSuite,
			),
		}
	}
	return nil
}

// checkTotalTimeoutSeconds enforces MaxSuiteTotalTimeoutSeconds against the
// sum of every listed scenario's declared/default timeout_seconds.
func checkTotalTimeoutSeconds(totalSeconds int) error {
	if totalSeconds > MaxSuiteTotalTimeoutSeconds {
		return &Error{
			Kind: ErrKindTotalTimeoutTooLarge,
			Msg: fmt.Sprintf(
				"the sum of every listed scenario's timeout_seconds is %d, which exceeds the maximum aggregate suite timeout of %d seconds (MaxSuiteTotalTimeoutSeconds)",
				totalSeconds, MaxSuiteTotalTimeoutSeconds,
			),
		}
	}
	return nil
}
