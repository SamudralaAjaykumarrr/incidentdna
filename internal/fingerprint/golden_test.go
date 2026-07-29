package fingerprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SamudralaAjaykumarrr/incidentdna/internal/idir"
)

// TestGolden_DuplicatePaymentExample guards the duplicate-payment example's
// fingerprint against accidental change. If this test fails because you
// deliberately changed the example or the fingerprint algorithm, update
// testdata/golden/duplicate-payment.fingerprint to the new value printed
// below and explain why in your commit message — a silent golden-file
// update defeats the point of this test.
func TestGolden_DuplicatePaymentExample(t *testing.T) {
	root := repoRoot(t)
	docPath := filepath.Join(root, "examples", "duplicate-payment", "incident.yaml")
	goldenPath := filepath.Join(root, "testdata", "golden", "duplicate-payment.fingerprint")

	doc, err := idir.LoadFile(docPath)
	if err != nil {
		t.Fatalf("LoadFile(%s): %v", docPath, err)
	}
	got, err := Compute(doc)
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v", goldenPath, err)
	}
	want := strings.TrimSpace(string(wantBytes))

	if got != want {
		t.Fatalf("fingerprint mismatch for %s:\n  got:  %s\n  want: %s (from %s)", docPath, got, want, goldenPath)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// internal/fingerprint -> repo root is two levels up.
	return filepath.Join(wd, "..", "..")
}
