#!/bin/sh
set -eu

REPO="${KISS_REPO:-wwulfric/kiss}"
VERSION="${KISS_VERSION:-latest}"
INSTALL_DIR="${KISS_INSTALL_DIR:-/usr/local/bin}"
VERIFY_SIGNATURE="${KISS_VERIFY_SIGNATURE:-0}"
FORCE_INSTALL="${KISS_FORCE_INSTALL:-0}"

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

semver_parts() {
  printf '%s\n' "$1" | sed -n 's/^v\{0,1\}\([0-9][0-9]*\)\.\([0-9][0-9]*\)\.\([0-9][0-9]*\)$/\1 \2 \3/p'
}

semver_cmp() {
  left=$(semver_parts "$1")
  right=$(semver_parts "$2")
  if [ -z "$left" ] || [ -z "$right" ]; then
    return 2
  fi
  set -- $left
  left_major=$1
  left_minor=$2
  left_patch=$3
  set -- $right
  right_major=$1
  right_minor=$2
  right_patch=$3
  if [ "$left_major" -lt "$right_major" ]; then echo -1; return 0; fi
  if [ "$left_major" -gt "$right_major" ]; then echo 1; return 0; fi
  if [ "$left_minor" -lt "$right_minor" ]; then echo -1; return 0; fi
  if [ "$left_minor" -gt "$right_minor" ]; then echo 1; return 0; fi
  if [ "$left_patch" -lt "$right_patch" ]; then echo -1; return 0; fi
  if [ "$left_patch" -gt "$right_patch" ]; then echo 1; return 0; fi
  echo 0
}

installed_kiss_path() {
  target="$INSTALL_DIR/kiss"
  if [ -x "$target" ]; then
    printf '%s\n' "$target"
    return 0
  fi
  command -v kiss 2>/dev/null || true
}

if [ "$FORCE_INSTALL" != "1" ]; then
  current_binary=$(installed_kiss_path)
  if [ -n "$current_binary" ]; then
    current_version=$("$current_binary" --version 2>/dev/null | sed -n '1s/[[:space:]]*$//p')
    if [ -n "$current_version" ]; then
      cmp=$(semver_cmp "$current_version" "$artifact_version" 2>/dev/null || true)
      case "$cmp" in
        0)
          echo "kiss $current_version is already installed at $current_binary; target $artifact_version is the same. Skipping download and install."
          exit 0
          ;;
        1)
          echo "Installed kiss $current_version at $current_binary is newer than target $artifact_version. Skipping download and install."
          exit 0
          ;;
        -1)
          echo "Updating kiss from $current_version to $artifact_version."
          ;;
        *)
          if [ "$current_version" = "$artifact_version" ]; then
            echo "kiss $current_version is already installed at $current_binary; target $artifact_version is the same. Skipping download and install."
            exit 0
          fi
          echo "Cannot compare installed version $current_version with target $artifact_version; continuing with install."
          ;;
      esac
    fi
  fi
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
