#!/usr/bin/env sh
# scripts/build-release.sh — reproducible multi-platform release build.
#
# Builds all five artifacts named in docs/release-process.md's platform
# matrix. Each platform target is built TWICE in immediate succession and
# the two binaries' SHA-256 digests are compared before anything is
# archived — a build that cannot reproduce itself aborts the whole release
# build rather than silently shipping non-reproducible output
# (docs/phase-8-plan.md §8).
#
# VERSION (env, default "dev") is the caller-supplied version string
# injected via -ldflags "-X main.version=$VERSION" — never derived from git
# or the working tree, preserving the exact discipline the existing
# -buildvcs=false flag already establishes for `make build`.
#
# Output: dist/incidentdna-$VERSION-<os>-<arch>.tar.gz (POSIX platforms) or
# .zip (windows/amd64). Each archive contains exactly two files at its
# root: the binary (incidentdna or incidentdna.exe) and a copy of LICENSE.
#
# windows/amd64 is part of the default matrix: internal/scenario's
# timeout/process-cleanup path (docs/phase-4-plan.md §9's "process tree"
# guarantee) is implemented per-OS via a `//go:build` split
# (internal/scenario/run_unix.go for the pre-existing, unchanged
# Setpgid/Kill-based process-group behavior; internal/scenario/run_windows.go
# for an equivalent Job-Object-based implementation) rather than a single
# Unix-only file, so cmd/incidentdna now compiles for GOOS=windows without
# weakening or disabling that guarantee on any platform. See
# docs/release-process.md's platform matrix section for detail.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
LICENSE_FILE="$ROOT/LICENSE"
VERSION="${VERSION:-dev}"

if [ ! -s "$LICENSE_FILE" ]; then
  echo "build-release: missing or empty $ROOT/LICENSE" >&2
  exit 1
fi

TMPROOT="$(mktemp -d)"
trap 'rm -rf "$TMPROOT"' EXIT

rm -rf "$DIST"
mkdir -p "$DIST"

build_once() {
  goos="$1"
  goarch="$2"
  out="$3"
  ( cd "$ROOT" && CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
      -trimpath -buildvcs=false \
      -ldflags "-s -w -X main.version=$VERSION" \
      -o "$out" ./cmd/incidentdna )
}

# make_zip archives $1 (a staging directory containing exactly the archive
# root's contents) into $2 (a .zip path), using a go run-based writer over
# stdlib archive/zip rather than assuming a system `zip` binary is
# installed in the build container — the same "no new dependency, prefer
# stdlib" discipline scripts/generate-checksums.sh applies to sha256sum
# (docs/phase-8-plan.md §11).
make_zip() {
  stagedir="$1"
  outzip="$2"
  helper="$TMPROOT/mkzip.go"
  cat > "$helper" <<'ZIPEOF'
package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
)

func main() {
	srcDir := os.Args[1]
	dstZip := os.Args[2]

	out, err := os.Create(dstZip)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		f, err := os.Open(filepath.Join(srcDir, e.Name()))
		if err != nil {
			panic(err)
		}
		info, err := e.Info()
		if err != nil {
			panic(err)
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			panic(err)
		}
		hdr.Name = e.Name()
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			panic(err)
		}
		if _, err := io.Copy(w, f); err != nil {
			panic(err)
		}
		f.Close()
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}
}
ZIPEOF
  ( cd "$ROOT" && go run "$helper" "$stagedir" "$outzip" )
}

PLATFORMS="linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64"

for spec in $PLATFORMS; do
  goos="${spec%%/*}"
  goarch="${spec##*/}"
  artifact="incidentdna-$goos-$goarch"

  binname="incidentdna"
  [ "$goos" = "windows" ] && binname="incidentdna.exe"

  work1="$TMPROOT/$artifact-1"
  work2="$TMPROOT/$artifact-2"
  stage="$TMPROOT/$artifact-stage"
  mkdir -p "$work1" "$work2" "$stage"

  build_once "$goos" "$goarch" "$work1/$binname"
  build_once "$goos" "$goarch" "$work2/$binname"

  sum1="$(sha256sum "$work1/$binname" | awk '{print $1}')"
  sum2="$(sha256sum "$work2/$binname" | awk '{print $1}')"
  if [ "$sum1" != "$sum2" ]; then
    echo "build-release: REPRODUCIBILITY FAILURE for $goos/$goarch" >&2
    echo "  build 1: $sum1" >&2
    echo "  build 2: $sum2" >&2
    exit 1
  fi
  echo "build-release: $goos/$goarch reproducible ($sum1)"

  cp "$work1/$binname" "$stage/$binname"
  cp "$LICENSE_FILE" "$stage/LICENSE"

  if [ "$goos" = "windows" ]; then
    out="$DIST/incidentdna-$VERSION-$goos-$goarch.zip"
    make_zip "$stage" "$out"
  else
    out="$DIST/incidentdna-$VERSION-$goos-$goarch.tar.gz"
    ( cd "$stage" && tar -czf "$out" "$binname" LICENSE )
  fi
  echo "build-release: wrote $out"
done

echo "build-release: OK (VERSION=$VERSION)"
