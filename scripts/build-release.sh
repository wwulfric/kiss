#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
DIST_DIR="${DIST_DIR:-$ROOT_DIR/dist}"
VERSION="${KISS_VERSION:-$(git -C "$ROOT_DIR" describe --tags --always --dirty 2>/dev/null || echo dev)}"
LDFLAGS="-s -w -X github.com/wwulfric/kiss/internal/kiss.Version=$VERSION"

mkdir -p "$DIST_DIR"
rm -f "$DIST_DIR"/kiss_* "$DIST_DIR"/checksums.txt "$DIST_DIR"/checksums.txt.sig "$DIST_DIR"/checksums.txt.pem "$DIST_DIR"/kiss.rb

build_one() {
  goos="$1"
  goarch="$2"
  ext=""
  if [ "$goos" = "windows" ]; then
    ext=".exe"
  fi
  name="kiss_${VERSION}_${goos}_${goarch}"
  work_dir="$DIST_DIR/$name"
  mkdir -p "$work_dir"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags="$LDFLAGS" -o "$work_dir/kiss$ext" "$ROOT_DIR/cmd/kiss"
  tar_path="$DIST_DIR/$name.tar.gz"
  tar -C "$work_dir" -czf "$tar_path" "kiss$ext"
  rm -rf "$work_dir"
}

build_one darwin amd64
build_one darwin arm64
build_one linux amd64
build_one linux arm64
build_one windows amd64

(
  cd "$DIST_DIR"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum kiss_*.tar.gz > checksums.txt
  else
    shasum -a 256 kiss_*.tar.gz > checksums.txt
  fi
)

DIST_DIR="$DIST_DIR" KISS_VERSION="$VERSION" sh "$ROOT_DIR/scripts/generate-homebrew-formula.sh"

echo "Built release artifacts in $DIST_DIR"
