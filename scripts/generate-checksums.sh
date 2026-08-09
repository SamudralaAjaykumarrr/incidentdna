#!/usr/bin/env sh
# scripts/generate-checksums.sh — SHA-256 checksums for every release
# archive (docs/phase-8-plan.md §11).
#
# Reads dist/*.tar.gz and dist/*.zip (produced by scripts/build-release.sh),
# sorts them by filename for determinism, and writes dist/SHA256SUMS in the
# standard `sha256sum`-compatible flat-file format (one
# "<digest>␠␠<filename>" line per artifact), so `sha256sum -c SHA256SUMS`
# works unmodified for anyone who downloads it alongside the archives.
set -eu

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"
OUT="$DIST/SHA256SUMS"

if [ ! -d "$DIST" ]; then
  echo "generate-checksums: $DIST does not exist — run scripts/build-release.sh first" >&2
  exit 1
fi

# List archive filenames, sorted, with no path prefix (sha256sum -c
# resolves filenames relative to its own working directory, so entries
# must be bare filenames, not absolute paths).
archives="$(cd "$DIST" && ls -1 *.tar.gz *.zip 2>/dev/null | sort || true)"

if [ -z "$archives" ]; then
  echo "generate-checksums: no *.tar.gz or *.zip files found in $DIST" >&2
  exit 1
fi

sha256_of() {
  # Prefer sha256sum (GNU coreutils, present in the golang:1.26.5 build
  # container); fall back to shasum -a 256 (macOS); fall back to a
  # temporary go run-based stdlib crypto/sha256 helper if neither system
  # tool is present — no new checked-in dependency either way
  # (docs/phase-8-plan.md §11).
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    helper="$(mktemp -d)/sha256sum.go"
    cat > "$helper" <<'EOF'
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		panic(err)
	}
	fmt.Println(hex.EncodeToString(h.Sum(nil)))
}
EOF
    ( cd "$ROOT" && go run "$helper" "$1" )
  fi
}

: > "$OUT"
for name in $archives; do
  digest="$(sha256_of "$DIST/$name")"
  printf '%s  %s\n' "$digest" "$name" >> "$OUT"
done

echo "generate-checksums: wrote $OUT"
cat "$OUT"
