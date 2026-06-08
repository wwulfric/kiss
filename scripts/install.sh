#!/bin/sh
set -eu

REPO="${KISS_REPO:-wwulfric/kiss}"
VERSION="${KISS_VERSION:-latest}"
INSTALL_DIR="${KISS_INSTALL_DIR:-/usr/local/bin}"
VERIFY_SIGNATURE="${KISS_VERIFY_SIGNATURE:-0}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  arm64 | aarch64) arch="arm64" ;;
  *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
esac

case "$os" in
  darwin | linux) ;;
  *) echo "unsupported OS: $os" >&2; exit 1 ;;
esac

if [ "$VERSION" = "latest" ]; then
  base_url="https://github.com/$REPO/releases/latest/download"
  artifact_version="latest"
else
  base_url="https://github.com/$REPO/releases/download/$VERSION"
  artifact_version="$VERSION"
fi

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT INT TERM

archive="kiss_${artifact_version}_${os}_${arch}.tar.gz"
if [ "$artifact_version" = "latest" ]; then
  latest_url=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest")
  artifact_version=${latest_url##*/}
  archive="kiss_${artifact_version}_${os}_${arch}.tar.gz"
fi

curl -fsSLo "$tmp_dir/$archive" "$base_url/$archive"
curl -fsSLo "$tmp_dir/checksums.txt" "$base_url/checksums.txt"

if [ "$VERIFY_SIGNATURE" = "1" ]; then
  if ! command -v cosign >/dev/null 2>&1; then
    echo "KISS_VERIFY_SIGNATURE=1 requires cosign in PATH" >&2
    exit 1
  fi
  curl -fsSLo "$tmp_dir/checksums.txt.sig" "$base_url/checksums.txt.sig"
  curl -fsSLo "$tmp_dir/checksums.txt.pem" "$base_url/checksums.txt.pem"
  cosign verify-blob \
    --certificate "$tmp_dir/checksums.txt.pem" \
    --signature "$tmp_dir/checksums.txt.sig" \
    --certificate-identity "https://github.com/$REPO/.github/workflows/release.yml@refs/tags/$artifact_version" \
    --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
    "$tmp_dir/checksums.txt"
fi

(
  cd "$tmp_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    grep "  $archive\$" checksums.txt | sha256sum -c -
  else
    grep "  $archive\$" checksums.txt | shasum -a 256 -c -
  fi
)

tar -C "$tmp_dir" -xzf "$tmp_dir/$archive"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmp_dir/kiss" "$INSTALL_DIR/kiss"

echo "Installed kiss to $INSTALL_DIR/kiss"
