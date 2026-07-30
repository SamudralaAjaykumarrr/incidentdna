package evidence

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultStoreRoot is the store root used when a command is not given an
// explicit --store directory, resolved relative to the current working
// directory — see docs/phase-2-plan.md §4/§6.
const DefaultStoreRoot = ".incidentdna/evidence/objects"

// shardPrefixLen is the number of leading hex characters used as the shard
// (subdirectory) name, mirroring the layout git's own object store uses.
const shardPrefixLen = 2

// tmpDirName is the reserved staging subdirectory name for atomic writes
// (see Put). It can never collide with a real shard directory: shard
// directories are always exactly two lowercase hex characters, and a hex
// digest can never begin with ".".
const tmpDirName = ".tmp"

// Store is a content-addressable evidence store rooted at a single resolved,
// absolute directory. A Store's root is fixed at construction time (Open)
// and never re-resolved — see docs/phase-2-plan.md §6 for why a command
// resolves its store root exactly once per invocation.
type Store struct {
	root string
}

// Open resolves root (or DefaultStoreRoot if root is empty) to an absolute,
// cleaned path, validates it is usable as a store root, and returns a Store
// bound to that resolved path for the remainder of the calling command's
// invocation.
//
// Resolution: root is made absolute (filepath.Abs, relative to the current
// working directory) and cleaned (filepath.Clean). If a symlink exists at
// that location, it is resolved to its real target via filepath.EvalSymlinks
// and the real path is used as the store root from then on — this avoids a
// symlink at the root itself creating ambiguity about what "under the store
// root" means for the containment check in objectPath. If nothing exists at
// the resolved location yet, it is created (with parents) on first write; if
// something exists and is not a directory, Open fails immediately rather
// than attempting to write through it.
func Open(root string) (*Store, error) {
	if root == "" {
		root = DefaultStoreRoot
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, &Error{Kind: ErrKindStoreRoot, Msg: fmt.Sprintf("resolve store root %q", root), Err: err}
	}
	resolved := filepath.Clean(abs)

	info, lerr := os.Lstat(resolved)
	switch {
	case lerr == nil && info.Mode()&os.ModeSymlink != 0:
		real, everr := filepath.EvalSymlinks(resolved)
		if everr != nil {
			return nil, &Error{Kind: ErrKindStoreRoot, Msg: fmt.Sprintf("store root %q is a symbolic link that could not be resolved", root), Err: everr}
		}
		resolved = real
		realInfo, serr := os.Stat(resolved)
		if serr != nil {
			return nil, &Error{Kind: ErrKindStoreRoot, Msg: fmt.Sprintf("stat resolved store root %q", root), Err: serr}
		}
		if !realInfo.IsDir() {
			return nil, &Error{Kind: ErrKindStoreRoot, Msg: fmt.Sprintf("store root %q resolves to a non-directory", root)}
		}
	case lerr == nil && !info.IsDir():
		return nil, &Error{Kind: ErrKindStoreRoot, Msg: fmt.Sprintf("store root %q exists and is not a directory", root)}
	case lerr != nil && !errors.Is(lerr, fs.ErrNotExist):
		return nil, &Error{Kind: ErrKindStoreRoot, Msg: fmt.Sprintf("stat store root %q", root), Err: lerr}
	}
	// lerr != nil && IsNotExist(lerr): nothing there yet, created lazily on
	// first write (see ensureDirs); a read-only command (verify/list/
	// inspect) against a store root that doesn't exist yet simply finds
	// every object missing, which is correct, not an error.

	return &Store{root: resolved}, nil
}

// Root returns the resolved, absolute store root this Store was opened
// against.
func (s *Store) Root() string {
	return s.root
}

func (s *Store) tmpDir() string {
	return filepath.Join(s.root, tmpDirName)
}

// objectPath derives the on-disk path for d: the store root, a shard
// subdirectory named by the digest's first two hex characters, and a file
// named by the remaining 62. This is the only function in this package that
// turns a Digest into a filesystem path, and it accepts nothing but a
// Digest — never an evidence id, type, location, original filename, or any
// other document-supplied string — which is what makes path traversal
// structurally unreachable here (see Digest.Parse's doc comment and
// docs/phase-2-plan.md §7).
func (s *Store) objectPath(d Digest) (string, error) {
	if d.IsZero() {
		return "", &Error{Kind: ErrKindInvalidDigest, Msg: "cannot derive an object path from an unvalidated digest"}
	}
	h := d.Hex()
	if len(h) != hexLength {
		// Unreachable in practice: Digest's only constructors (Parse,
		// digestFromSum) guarantee this length. Checked anyway as
		// belt-and-suspenders, matching idir.LoadFile's own precedent of
		// re-checking an invariant right before it matters.
		return "", &Error{Kind: ErrKindInvalidDigest, Msg: fmt.Sprintf("digest has unexpected internal length %d", len(h))}
	}
	shard, name := h[:shardPrefixLen], h[shardPrefixLen:]
	path := filepath.Clean(filepath.Join(s.root, shard, name))

	// Belt-and-suspenders containment check: shard/name can only ever be
	// lowercase hex characters (enforced by Digest's constructors), which
	// cannot produce "..", "/", or a null byte, so this can never actually
	// fail — but the check is cheap and fails closed if that invariant is
	// ever violated by a future change.
	rootWithSep := s.root + string(filepath.Separator)
	if path != s.root && !strings.HasPrefix(path, rootWithSep) {
		return "", &Error{Kind: ErrKindStoreRoot, Msg: "constructed object path escaped the resolved store root"}
	}
	return path, nil
}

func (s *Store) ensureDirs() error {
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return &Error{Kind: ErrKindStoreRoot, Msg: fmt.Sprintf("create store root %q", s.root), Err: err}
	}
	if err := os.MkdirAll(s.tmpDir(), 0o755); err != nil {
		return &Error{Kind: ErrKindStoreRoot, Msg: "create temporary-staging directory", Err: err}
	}
	return nil
}

// Put ingests the file at sourcePath into the store: it streams the file's
// bytes through a SHA-256 hasher and a temporary file simultaneously,
// enforcing MaxObjectSize as it goes (not merely via a preliminary stat),
// then atomically finalizes the temporary file at its digest-derived path.
//
// Deduplication is structural: if an object already exists at the computed
// digest's path, Put re-verifies that existing object's actual bytes against
// the digest before declaring success (rather than trusting the filename),
// and reports AlreadyPresent — the newly-read source bytes are discarded,
// never used to silently overwrite the existing object. If the existing
// object fails that re-verification, Put fails with ErrKindObjectCorrupted
// rather than silently replacing it or silently accepting it.
//
// Put never opens sourcePath if it is a symbolic link, a directory, or any
// non-regular file — see docs/evidence-storage.md, "Local trust boundaries"
// for why source symlinks are rejected even though other Phase 1 file
// arguments (e.g. `validate <file>`) follow them.
func (s *Store) Put(ctx context.Context, sourcePath string) (StoreResult, error) {
	if err := ctx.Err(); err != nil {
		return StoreResult{}, &Error{Kind: ErrKindCanceled, Msg: "evidence ingestion canceled", Err: err}
	}

	info, err := os.Lstat(sourcePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return StoreResult{}, &Error{Kind: ErrKindSourceMissing, Msg: fmt.Sprintf("evidence file %q does not exist", sourcePath), Err: err}
		}
		return StoreResult{}, wrapIOError(err, fmt.Sprintf("stat evidence file %q", sourcePath))
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return StoreResult{}, &Error{Kind: ErrKindUnsafeSource, Msg: fmt.Sprintf("evidence file %q is a symbolic link; pass the real file instead", sourcePath)}
	case info.IsDir():
		return StoreResult{}, &Error{Kind: ErrKindSourceIsDirectory, Msg: fmt.Sprintf("evidence source %q is a directory, not a file", sourcePath)}
	case !info.Mode().IsRegular():
		return StoreResult{}, &Error{Kind: ErrKindUnsafeSource, Msg: fmt.Sprintf("evidence source %q is not a regular file", sourcePath)}
	}

	f, err := os.Open(sourcePath)
	if err != nil {
		return StoreResult{}, wrapIOError(err, fmt.Sprintf("open evidence file %q", sourcePath))
	}
	defer f.Close()

	if err := s.ensureDirs(); err != nil {
		return StoreResult{}, err
	}

	tmp, err := os.CreateTemp(s.tmpDir(), "ingest-*")
	if err != nil {
		return StoreResult{}, &Error{Kind: ErrKindTempFile, Msg: "create temporary file for evidence ingestion", Err: err}
	}
	tmpPath := tmp.Name()
	needsCleanup := true
	defer func() {
		if needsCleanup {
			tmp.Close()
			os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return StoreResult{}, &Error{Kind: ErrKindTempFile, Msg: "restrict permissions on temporary file", Err: err}
	}

	hasher := NewHasher()
	n, err := hashLimitedCopy(ctx, io.MultiWriter(tmp, hasher), f, MaxObjectSize)
	if err != nil {
		var evErr *Error
		if errors.As(err, &evErr) {
			return StoreResult{}, evErr
		}
		if ctx.Err() != nil {
			return StoreResult{}, &Error{Kind: ErrKindCanceled, Msg: "evidence ingestion canceled", Err: ctx.Err()}
		}
		return StoreResult{}, wrapIOError(err, fmt.Sprintf("read evidence file %q", sourcePath))
	}
	if err := tmp.Sync(); err != nil {
		return StoreResult{}, &Error{Kind: ErrKindWriteFailure, Msg: "flush temporary evidence object to disk", Err: err}
	}
	if err := tmp.Close(); err != nil {
		return StoreResult{}, &Error{Kind: ErrKindWriteFailure, Msg: "close temporary evidence object", Err: err}
	}

	digest := digestFromSum(hasher.Sum(nil))
	finalPath, err := s.objectPath(digest)
	if err != nil {
		return StoreResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		return StoreResult{}, &Error{Kind: ErrKindStoreRoot, Msg: "create shard directory", Err: err}
	}

	if existingInfo, lerr := os.Lstat(finalPath); lerr == nil {
		if existingInfo.Mode()&os.ModeSymlink != 0 || !existingInfo.Mode().IsRegular() {
			return StoreResult{}, &Error{Kind: ErrKindObjectCorrupted, Msg: fmt.Sprintf("an existing entry at digest %s is not a regular file; refusing to trust or replace it", digest)}
		}
		status, size, detail := s.checkObject(ctx, digest)
		switch status {
		case objectOK:
			return StoreResult{Digest: digest, AlreadyPresent: true, Size: size}, nil
		case objectError:
			return StoreResult{}, &Error{Kind: ErrKindPermission, Msg: fmt.Sprintf("could not re-verify existing object at digest %s", digest), Err: errors.New(detail)}
		default:
			return StoreResult{}, &Error{Kind: ErrKindObjectCorrupted, Msg: fmt.Sprintf("an existing object at digest %s does not match its own digest when re-hashed; the store may be corrupted: %s", digest, detail)}
		}
	} else if !errors.Is(lerr, fs.ErrNotExist) {
		return StoreResult{}, wrapIOError(lerr, fmt.Sprintf("stat existing object at digest %s", digest))
	}

	if err := os.Chmod(tmpPath, 0o444); err != nil {
		return StoreResult{}, &Error{Kind: ErrKindFinalize, Msg: "set read-only permissions on evidence object before finalizing", Err: err}
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return StoreResult{}, &Error{Kind: ErrKindFinalize, Msg: fmt.Sprintf("finalize evidence object at digest %s", digest), Err: err}
	}
	needsCleanup = false

	return StoreResult{Digest: digest, AlreadyPresent: false, Size: n}, nil
}

// objectCheck is checkObject's internal result category.
type objectCheck int

const (
	objectOK objectCheck = iota
	objectMissing
	objectCorrupted
	objectError
)

// checkObject is the shared "does the stored bytes at digest's path still
// hash to digest" primitive used by Put (dedup re-verification), Verify, and
// Inspect. It never returns the object's content — only a status, the
// object's size when known, and a short detail string safe to print
// (never evidence content, never more of the path than the digest itself
// implies).
func (s *Store) checkObject(ctx context.Context, digest Digest) (status objectCheck, size int64, detail string) {
	path, perr := s.objectPath(digest)
	if perr != nil {
		return objectError, 0, perr.Error()
	}

	info, lerr := os.Lstat(path)
	switch {
	case lerr != nil && errors.Is(lerr, fs.ErrNotExist):
		return objectMissing, 0, ""
	case lerr != nil:
		return objectError, 0, wrapIOError(lerr, "stat evidence object").Error()
	case info.Mode()&os.ModeSymlink != 0:
		return objectCorrupted, 0, "stored path is a symbolic link, not a regular file; refusing to trust it as evidence"
	case !info.Mode().IsRegular():
		return objectCorrupted, 0, "stored path is not a regular file"
	}

	f, err := os.Open(path)
	if err != nil {
		return objectError, 0, wrapIOError(err, "open evidence object").Error()
	}
	defer f.Close()

	hasher := NewHasher()
	n, err := hashLimitedCopy(ctx, hasher, f, MaxObjectSize)
	if err != nil {
		var evErr *Error
		if errors.As(err, &evErr) && evErr.Kind == ErrKindObjectTooLarge {
			return objectCorrupted, n, fmt.Sprintf("stored object exceeds the maximum object size of %d bytes; treated as corrupted rather than partially checked", MaxObjectSize)
		}
		if ctx.Err() != nil {
			return objectError, n, "canceled"
		}
		return objectError, n, wrapIOError(err, "read evidence object").Error()
	}

	got := digestFromSum(hasher.Sum(nil))
	if !got.Equal(digest) {
		return objectCorrupted, n, fmt.Sprintf("re-hashing the stored object produced %s, which does not match the declared digest — the stored bytes were modified, truncated, or contain trailing data", got)
	}
	return objectOK, n, ""
}
