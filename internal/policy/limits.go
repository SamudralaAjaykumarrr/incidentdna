package policy

import "fmt"

// Resource limits enforced by this package. Each is a fixed constant, not a
// configurable flag (docs/phase-7-plan.md §9), consistent with the
// fixed-constants discipline internal/scenario and internal/suite already
// established. Each has its own distinct, actionable error and its own
// passing/failing boundary test pair.
const (
	// MaxPolicyDocumentSize is the maximum number of bytes a policy file's
	// own on-disk size may occupy, enforced by LoadFile the same way
	// scenario.LoadFile enforces scenario.MaxScenarioDocumentSize.
	MaxPolicyDocumentSize = 64 * 1024 // 64 KiB

	// MaxRulesPerPolicy is the maximum number of rules[] entries a policy
	// document may declare.
	MaxRulesPerPolicy = 20

	// MaxReportDocumentSize is the maximum number of bytes an input report
	// file's (a scenario run --report or suite run --report JSON file) own
	// on-disk size may occupy. Sized above the worst-case suite-report/v0.1
	// document (suite.MaxScenariosPerSuite (100) x 2 output streams x
	// scenario.MaxScenarioOutputBytes (1 MiB), plus structural overhead), so
	// a legitimate maximal suite report is never rejected.
	MaxReportDocumentSize = 256 * 1024 * 1024 // 256 MiB

	// MaxDistinctFingerprintsPerEvaluation is the maximum number of
	// distinct linked_fingerprint values one policy evaluate --library call
	// will look up — bounded by, and equal to, suite.MaxScenariosPerSuite,
	// the same reasoning docs/library-crossref.md already applied to suite
	// verify --library.
	MaxDistinctFingerprintsPerEvaluation = 100
)

// checkDocumentSize enforces MaxPolicyDocumentSize against sizeBytes, the
// policy file's on-disk size (os.FileInfo.Size()), before any parsing
// occurs.
func checkDocumentSize(sizeBytes int64) error {
	if sizeBytes > MaxPolicyDocumentSize {
		return &Error{
			Kind: ErrKindDocumentTooLarge,
			Msg: fmt.Sprintf(
				"policy document is %d bytes, which exceeds the maximum policy document size of %d bytes (%d KiB)",
				sizeBytes, MaxPolicyDocumentSize, MaxPolicyDocumentSize/1024,
			),
		}
	}
	return nil
}

// checkRuleCount enforces MaxRulesPerPolicy against n (len(doc.Rules)).
func checkRuleCount(n int) error {
	if n > MaxRulesPerPolicy {
		return &Error{
			Kind: ErrKindTooManyRules,
			Msg: fmt.Sprintf(
				"rules declares %d entries, which exceeds the maximum of %d (MaxRulesPerPolicy)",
				n, MaxRulesPerPolicy,
			),
		}
	}
	return nil
}

// checkReportDocumentSize enforces MaxReportDocumentSize against sizeBytes,
// an input report file's on-disk size, before any parsing occurs.
func checkReportDocumentSize(sizeBytes int64) error {
	if sizeBytes > MaxReportDocumentSize {
		return &Error{
			Kind: ErrKindReportTooLarge,
			Msg: fmt.Sprintf(
				"report document is %d bytes, which exceeds the maximum report document size of %d bytes (%d MiB)",
				sizeBytes, MaxReportDocumentSize, MaxReportDocumentSize/(1024*1024),
			),
		}
	}
	return nil
}

// checkDistinctFingerprintCount enforces
// MaxDistinctFingerprintsPerEvaluation against n, the number of distinct
// linked_fingerprint values extracted from an input report.
func checkDistinctFingerprintCount(n int) error {
	if n > MaxDistinctFingerprintsPerEvaluation {
		return &Error{
			Kind: ErrKindTooManyDistinctFingerprints,
			Msg: fmt.Sprintf(
				"report names %d distinct linked fingerprint(s), which exceeds the maximum of %d (MaxDistinctFingerprintsPerEvaluation)",
				n, MaxDistinctFingerprintsPerEvaluation,
			),
		}
	}
	return nil
}
