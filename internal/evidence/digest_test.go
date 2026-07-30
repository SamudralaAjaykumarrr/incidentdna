package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func validHex64() string {
	return strings.Repeat("a1", 32) // 64 hex chars
}

func TestParse_AcceptsCanonicalForm(t *testing.T) {
	hex := validHex64()
	d, err := Parse("sha256:" + hex)
	if err != nil {
		t.Fatalf("Parse: unexpected error: %v", err)
	}
	if d.Hex() != hex {
		t.Errorf("Hex() = %q, want %q", d.Hex(), hex)
	}
	if got := d.String(); got != "sha256:"+hex {
		t.Errorf("String() = %q, want %q", got, "sha256:"+hex)
	}
}

func TestParse_RejectsEachInvalidCase(t *testing.T) {
	hex := validHex64()
	cases := map[string]struct {
		in       string
		wantKind ErrorKind
	}{
		"no algorithm prefix":             {hex, ErrKindInvalidDigest},
		"empty algorithm":                 {":" + hex, ErrKindUnsupportedAlgorithm},
		"unsupported algorithm":           {"md5:" + hex, ErrKindUnsupportedAlgorithm},
		"too few hex characters":          {"sha256:" + hex[:63], ErrKindInvalidDigest},
		"too many hex characters":         {"sha256:" + hex + "a", ErrKindInvalidDigest},
		"non-hex characters":              {"sha256:" + strings.Repeat("g", 64), ErrKindInvalidDigest},
		"uppercase hex":                   {"sha256:" + strings.ToUpper(hex), ErrKindInvalidDigest},
		"leading whitespace":              {" sha256:" + hex, ErrKindUnsupportedAlgorithm},
		"trailing whitespace":             {"sha256:" + hex + " ", ErrKindInvalidDigest},
		"whitespace inside hex":           {"sha256:" + hex[:32] + " " + hex[33:], ErrKindInvalidDigest},
		"path separator in hex":           {"sha256:" + hex[:32] + "/" + hex[33:], ErrKindInvalidDigest},
		"relative path components":        {"sha256:../../etc/passwd", ErrKindInvalidDigest},
		"absolute path":                   {"sha256:/etc/passwd/////////////////////////////", ErrKindInvalidDigest},
		"additional algorithm separator":  {"sha256:sha256:" + hex, ErrKindInvalidDigest},
		"valid digest with trailing data": {"sha256:" + hex + ":extra", ErrKindInvalidDigest},
		"empty string":                    {"", ErrKindInvalidDigest},
		"just algorithm prefix":           {"sha256:", ErrKindInvalidDigest},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(tc.in)
			if err == nil {
				t.Fatalf("Parse(%q): expected error, got nil", tc.in)
			}
			if !IsKind(err, tc.wantKind) {
				t.Errorf("Parse(%q): error kind = %v, want %v (err: %v)", tc.in, errKindOf(err), tc.wantKind, err)
			}
		})
	}
}

func errKindOf(err error) ErrorKind {
	if e, ok := err.(*Error); ok {
		return e.Kind
	}
	return ""
}

func TestDigest_Equal(t *testing.T) {
	a, _ := Parse("sha256:" + validHex64())
	b, _ := Parse("sha256:" + validHex64())
	c, _ := Parse("sha256:" + strings.Repeat("b2", 32))

	if !a.Equal(b) {
		t.Error("expected equal digests to compare equal")
	}
	if a.Equal(c) {
		t.Error("expected different digests to compare unequal")
	}
	if (Digest{}).Equal(Digest{}) != true {
		t.Error("two zero digests should be Equal (both empty strings)")
	}
}

func TestDigest_IsZero(t *testing.T) {
	if !(Digest{}).IsZero() {
		t.Error("zero-value Digest should report IsZero")
	}
	d, _ := Parse("sha256:" + validHex64())
	if d.IsZero() {
		t.Error("parsed Digest should not report IsZero")
	}
}

func TestNewHasher_ProducesExpectedDigest(t *testing.T) {
	content := []byte("incidentdna phase 2 test content")

	h := NewHasher()
	if _, err := h.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got := digestFromSum(h.Sum(nil))

	wantSum := sha256.Sum256(content)
	want := "sha256:" + hex.EncodeToString(wantSum[:])

	if got.String() != want {
		t.Errorf("digest = %q, want %q", got.String(), want)
	}
}
