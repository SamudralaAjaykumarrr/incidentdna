// index.go implements read/write helpers for a fingerprint directory's
// index.json: the digest -> incident.id/occurred_at mapping described in
// docs/phase-3-plan.md §5, used by Add (add.go) to detect a same-incident.id
// conflict or idempotent re-add without re-reading and re-canonicalizing
// every occurrence body stored under a fingerprint.
package library

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// indexEntry is one row of a fingerprint directory's index.json.
type indexEntry struct {
	// Digest is the bare (unprefixed) 64-character lowercase hex SHA-256
	// digest of the occurrence's own canonical document bytes — the same
	// value OccurrencePath uses to address that occurrence's file.
	Digest string `json:"digest"`
	// IncidentID echoes the stored document's incident.id, so Add can find
	// "is there already an occurrence for this incident.id under this
	// fingerprint" without opening every occurrence file.
	IncidentID string `json:"incident_id"`
	// OccurredAt echoes the stored document's incident.occurred_at, for a
	// later slice's `library list` summary (docs/phase-3-plan.md §7).
	OccurredAt string `json:"occurred_at"`
}

// index is the decoded content of one fingerprint directory's index.json.
type index struct {
	Entries []indexEntry `json:"entries"`
}

// sortEntries orders Entries by Digest ascending, the deterministic ordering
// docs/phase-3-plan.md §13 requires so repeated writes of an unchanged
// logical index produce byte-identical JSON.
func (idx *index) sortEntries() {
	sort.Slice(idx.Entries, func(i, j int) bool {
		return idx.Entries[i].Digest < idx.Entries[j].Digest
	})
}

// readIndex reads and parses the index.json file at path.
//
// A missing file is not an error: it means the fingerprint directory has no
// recorded occurrences yet (or has not been created), and returns an empty,
// non-nil *index — mirroring evidence.Open's "not-yet-created is not an
// error" stance (docs/phase-3-plan.md §12). A present file that is a
// symlink, not a regular file, not valid JSON, has trailing data after the
// JSON value, or contains a structurally invalid entry (empty digest or
// incident id) is reported as a wrapped ErrMalformedIndex — it is never
// partially trusted or silently repaired.
func readIndex(path string) (*index, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &index{}, nil
		}
		return nil, fmt.Errorf("library: stat index %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("library: %w: index at %q is not a regular file", ErrMalformedIndex, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("library: read index %q: %w", path, err)
	}

	var idx index
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&idx); err != nil {
		return nil, fmt.Errorf("library: %w: index %q is not valid JSON: %v", ErrMalformedIndex, path, err)
	}
	if _, err := dec.Token(); err != io.EOF { //nolint:errcheck // deliberate EOF check
		return nil, fmt.Errorf("library: %w: index %q has trailing data after its JSON value", ErrMalformedIndex, path)
	}

	for i, e := range idx.Entries {
		if e.Digest == "" {
			return nil, fmt.Errorf("library: %w: index %q entry %d has an empty digest", ErrMalformedIndex, path, i)
		}
		if _, perr := parseDigestHex(e.Digest); perr != nil {
			return nil, fmt.Errorf("library: %w: index %q entry %d has an invalid digest %q: %v", ErrMalformedIndex, path, i, e.Digest, perr)
		}
		if e.IncidentID == "" {
			return nil, fmt.Errorf("library: %w: index %q entry %d has an empty incident id", ErrMalformedIndex, path, i)
		}
	}

	return &idx, nil
}

// writeIndex atomically writes idx to path: entries are sorted first for
// deterministic serialization (docs/phase-3-plan.md §13), then written to a
// temporary file created in the same directory as path (so the final rename
// is same-filesystem and atomic), fsync'd and closed, then renamed over
// path. The temporary file is removed on any failure before the rename
// (docs/phase-3-plan.md §12/§14, "atomic-write cleanup after a simulated
// failure").
func writeIndex(path string, idx *index) error {
	idx.sortEntries()
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("library: encode index for %q: %w", path, err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("library: create fingerprint directory %q: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".index-*.tmp")
	if err != nil {
		return fmt.Errorf("library: create temporary index file: %w", err)
	}
	tmpPath := tmp.Name()
	needsCleanup := true
	defer func() {
		if needsCleanup {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("library: write temporary index file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("library: sync temporary index file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("library: close temporary index file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("library: set permissions on temporary index file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("library: finalize index file %q: %w", path, err)
	}
	needsCleanup = false
	return nil
}
