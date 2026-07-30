package evidence

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestCheckEntryCount(t *testing.T) {
	if err := checkEntryCount(MaxEntriesPerCommand); err != nil {
		t.Errorf("checkEntryCount(%d) = %v, want nil (at limit is allowed)", MaxEntriesPerCommand, err)
	}
	err := checkEntryCount(MaxEntriesPerCommand + 1)
	if err == nil {
		t.Fatal("checkEntryCount(limit+1): expected error, got nil")
	}
	if !IsKind(err, ErrKindTooManyEntries) {
		t.Errorf("checkEntryCount(limit+1): kind = %v, want %v", errKindOf(err), ErrKindTooManyEntries)
	}
	if !strings.Contains(err.Error(), "101") || !strings.Contains(err.Error(), "100") {
		t.Errorf("error message %q should name both the actual count and the limit", err.Error())
	}
}

func TestCheckTotalBytes(t *testing.T) {
	if err := checkTotalBytes(MaxTotalVerifyBytes); err != nil {
		t.Errorf("checkTotalBytes(limit) = %v, want nil (at limit is allowed)", err)
	}
	err := checkTotalBytes(MaxTotalVerifyBytes + 1)
	if err == nil {
		t.Fatal("checkTotalBytes(limit+1): expected error, got nil")
	}
	if !IsKind(err, ErrKindTotalBytesExceeded) {
		t.Errorf("checkTotalBytes(limit+1): kind = %v, want %v", errKindOf(err), ErrKindTotalBytesExceeded)
	}
}

func TestHashLimitedCopy_WithinLimit(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 1024)
	var dst bytes.Buffer
	n, err := hashLimitedCopy(context.Background(), &dst, bytes.NewReader(data), 2048)
	if err != nil {
		t.Fatalf("hashLimitedCopy: unexpected error: %v", err)
	}
	if n != int64(len(data)) {
		t.Errorf("n = %d, want %d", n, len(data))
	}
	if dst.Len() != len(data) {
		t.Errorf("copied %d bytes, want %d", dst.Len(), len(data))
	}
}

func TestHashLimitedCopy_ExactlyAtLimit(t *testing.T) {
	data := bytes.Repeat([]byte("y"), 100)
	var dst bytes.Buffer
	_, err := hashLimitedCopy(context.Background(), &dst, bytes.NewReader(data), 100)
	if err != nil {
		t.Fatalf("hashLimitedCopy at exact limit: unexpected error: %v", err)
	}
}

func TestHashLimitedCopy_OverLimit(t *testing.T) {
	data := bytes.Repeat([]byte("z"), 101)
	var dst bytes.Buffer
	_, err := hashLimitedCopy(context.Background(), &dst, bytes.NewReader(data), 100)
	if err == nil {
		t.Fatal("hashLimitedCopy over limit: expected error, got nil")
	}
	if !IsKind(err, ErrKindObjectTooLarge) {
		t.Errorf("kind = %v, want %v", errKindOf(err), ErrKindObjectTooLarge)
	}
}

// slowReader trickles bytes one at a time so a canceled context has multiple
// opportunities to be observed mid-copy, proving cancellation is checked at
// chunk granularity rather than only before/after the whole copy.
type slowReader struct {
	remaining int
}

func (r *slowReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, nil
	}
	p[0] = 'a'
	r.remaining--
	return 1, nil
}

func TestHashLimitedCopy_RespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled before the copy even starts
	var dst bytes.Buffer
	_, err := hashLimitedCopy(ctx, &dst, &slowReader{remaining: 1000}, MaxObjectSize)
	if err == nil {
		t.Fatal("expected an error from a canceled context, got nil")
	}
}

func TestHashLimitedCopy_CancelsDuringLongRead(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	var dst bytes.Buffer
	// An effectively-infinite source; the copy must stop once the context
	// deadline passes rather than running to completion.
	_, err := hashLimitedCopy(ctx, &dst, &infiniteReader{}, MaxObjectSize)
	if err == nil {
		t.Fatal("expected the copy to be interrupted by context deadline, got nil error")
	}
}

type infiniteReader struct{}

func (infiniteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'a'
	}
	return len(p), nil
}
