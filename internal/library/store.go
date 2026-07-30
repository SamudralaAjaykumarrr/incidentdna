// Package library implements Phase 3 Slice 1: a local, filesystem-backed
// store's root resolution and its fingerprint/digest-derived directory
// scheme, per docs/phase-3-plan.md §5/§6. Later slices add the occurrence
// add/check/list operations on top of the layout this file defines.
//
// Nothing in this package performs network access or collects telemetry —
// every operation is local filesystem I/O, consistent with the rest of this
// module (see docs/threat-model.md).
package library

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DefaultLibraryRoot is the library root used when a command is not given an
// explicit --library directory, resolved relative to the current working
// directory — see docs/phase-3-plan.md §7/§20.3. It lives under the same
// .incidentdna root the Phase 2 evidence store already uses.
const DefaultLibraryRoot = ".incidentdna/library/objects"

// shardPrefixLen is the number of leading hex characters used as the shard
// (subdirectory) name, mirroring internal/evidence's existing sharding
// scheme (docs/phase-3-plan.md §5, "Sharding").
const shardPrefixLen = 2

// occurrencesDirName is the fixed subdirectory name, under a fingerprint's
// directory, holding that failure class's digest-addressed occurrence
// objects (docs/phase-3-plan.md §5).
const occurrencesDirName = "occurrences"

// indexFileName is the fixed filename, under a fingerprint's directory, of
// that failure class's digest -> incident.id/occurred_at index. A later
// slice's index.go reads and writes this file's content; this package only
// derives its path.
const indexFileName = "index.json"

// fingerprintPattern matches the exact external form internal/fingerprint.
// Compute produces: "sha256:" followed by 64 lowercase hex characters.
var fingerprintPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// digestHexPattern matches a bare (unprefixed) 64-character lowercase hex
// SHA-256 digest, the form an occurrence's canonical-bytes digest takes
// internally within this package.
var digestHexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Store is a local incident library rooted at a single resolved, absolute
// directory. A Store's root is fixed at construction time (Open) and never
// re-resolved, mirroring internal/evidence.Store's own precedent
// (docs/phase-2-plan.md §6, adopted here per docs/phase-3-plan.md §6).
type Store struct {
	root string
}

// Open resolves root (or DefaultLibraryRoot if root is empty) to an
// absolute, cleaned path, validates it is usable as a library root, and
// returns a Store bound to that resolved path for the remainder of the
// calling command's invocation.
//
// Resolution follows internal/evidence.Open's already-reviewed precedent
// exactly (docs/phase-3-plan.md §10, "Symlink and atomic-write handling
// mirrors internal/evidence's existing, already-reviewed design"): root is
// made absolute and cleaned; a symlink at that location is resolved to its
// real target via filepath.EvalSymlinks and the real path becomes the
// library root; if nothing exists at the resolved location yet, Open
// succeeds anyway (a read-only command against a library that hasn't been
// created yet simply finds nothing, which is not an error — docs/phase-3-plan.md
// §12); if something exists and is not a directory, Open fails immediately.
func Open(root string) (*Store, error) {
	if root == "" {
		root = DefaultLibraryRoot
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve library root %q: %w", root, err)
	}
	resolved := filepath.Clean(abs)

	info, lerr := os.Lstat(resolved)
	switch {
	case lerr == nil && info.Mode()&os.ModeSymlink != 0:
		real, everr := filepath.EvalSymlinks(resolved)
		if everr != nil {
			return nil, fmt.Errorf("library root %q is a symbolic link that could not be resolved: %w", root, everr)
		}
		resolved = real
		realInfo, serr := os.Stat(resolved)
		if serr != nil {
			return nil, fmt.Errorf("stat resolved library root %q: %w", root, serr)
		}
		if !realInfo.IsDir() {
			return nil, fmt.Errorf("library root %q resolves to a non-directory", root)
		}
	case lerr == nil && !info.IsDir():
		return nil, fmt.Errorf("library root %q exists and is not a directory", root)
	case lerr != nil && !errors.Is(lerr, fs.ErrNotExist):
		return nil, fmt.Errorf("stat library root %q: %w", root, lerr)
	}
	// lerr != nil && IsNotExist(lerr): nothing there yet. Slice 1 does not
	// create it — that is a later slice's write path (add.go) — mirroring
	// evidence.Open, which also defers directory creation to its own first
	// write (ensureDirs).

	return &Store{root: resolved}, nil
}

// Root returns the resolved, absolute library root this Store was opened
// against.
func (s *Store) Root() string {
	return s.root
}

// FingerprintDir returns the sharded directory path for the failure class
// identified by fingerprint — a "sha256:" + 64-lowercase-hex string exactly
// as produced by internal/fingerprint.Compute. fingerprint is validated
// before it ever becomes part of a path: an invalid format is rejected
// outright rather than truncated, escaped, or joined as-is (docs/phase-3-plan.md
// §5, "Content-addressed at two levels, neither derived from untrusted
// input").
func (s *Store) FingerprintDir(fingerprint string) (string, error) {
	hex, err := parseFingerprint(fingerprint)
	if err != nil {
		return "", err
	}
	shard, name := hex[:shardPrefixLen], hex[shardPrefixLen:]
	path := filepath.Clean(filepath.Join(s.root, shard, name))
	if err := s.checkContained(path); err != nil {
		return "", err
	}
	return path, nil
}

// OccurrencePath returns the digest-addressed path for one occurrence
// object stored under fingerprint's directory. digestHex must be the bare
// (unprefixed) 64-character lowercase hex SHA-256 digest of the occurrence's
// own canonical document bytes (internal/canonical.Marshal applied to the
// whole validated document) — never the incident's id, or any other
// untrusted string, which is what makes this construction structurally safe
// against path traversal (docs/phase-3-plan.md §5).
func (s *Store) OccurrencePath(fingerprint, digestHex string) (string, error) {
	fpDir, err := s.FingerprintDir(fingerprint)
	if err != nil {
		return "", err
	}
	dHex, err := parseDigestHex(digestHex)
	if err != nil {
		return "", err
	}
	shard, name := dHex[:shardPrefixLen], dHex[shardPrefixLen:]
	path := filepath.Clean(filepath.Join(fpDir, occurrencesDirName, shard, name+".json"))
	if err := s.checkContained(path); err != nil {
		return "", err
	}
	return path, nil
}

// IndexPath returns the path to the index.json file under fingerprint's
// directory (digest -> incident.id/occurred_at; docs/phase-3-plan.md §5).
// This package only derives the path; reading and writing its content is a
// later slice's responsibility (index.go).
func (s *Store) IndexPath(fingerprint string) (string, error) {
	fpDir, err := s.FingerprintDir(fingerprint)
	if err != nil {
		return "", err
	}
	path := filepath.Join(fpDir, indexFileName)
	if err := s.checkContained(path); err != nil {
		return "", err
	}
	return path, nil
}

// checkContained is a belt-and-suspenders containment check: path is always
// built from s.root joined with characters drawn only from a validated
// fingerprint or digest's hex alphabet (enforced by parseFingerprint/
// parseDigestHex), which cannot produce "..", "/", or a null byte, so this
// can never actually fail in practice — but the check is cheap and fails
// closed if that invariant is ever violated by a future change, mirroring
// internal/evidence.Store.objectPath's identical precedent.
func (s *Store) checkContained(path string) error {
	rootWithSep := s.root + string(filepath.Separator)
	if path != s.root && !strings.HasPrefix(path, rootWithSep) {
		return fmt.Errorf("constructed library path %q escaped the resolved library root", path)
	}
	return nil
}

// parseFingerprint validates fp as a canonical fingerprint string and
// returns its bare (unprefixed) 64-character hex portion.
func parseFingerprint(fp string) (string, error) {
	if !fingerprintPattern.MatchString(fp) {
		return "", fmt.Errorf("fingerprint %q is not a valid sha256-prefixed 64-character lowercase hex fingerprint", fp)
	}
	return strings.TrimPrefix(fp, "sha256:"), nil
}

// parseDigestHex validates d as a bare 64-character lowercase hex digest.
func parseDigestHex(d string) (string, error) {
	if !digestHexPattern.MatchString(d) {
		return "", fmt.Errorf("occurrence digest %q must be exactly 64 lowercase hexadecimal characters", d)
	}
	return d, nil
}
