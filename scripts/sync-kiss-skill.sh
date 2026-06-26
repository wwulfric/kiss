#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE_FILE="$ROOT_DIR/skills/kiss/SKILL.md"
TARGET_FILE="$ROOT_DIR/internal/kiss/setup_assets/kiss/SKILL.md"

usage() {
  cat <<'EOF'
usage: scripts/sync-kiss-skill.sh [--check]

Sync the public KISS bridge skill into the Go embedded setup asset.

Options:
  --check   only verify that the embedded asset is already in sync
EOF
}

mode="sync"
case "${1:-}" in
  "")
    ;;
  "--check")
    mode="check"
    ;;
  "-h" | "--help")
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

if [ ! -f "$SOURCE_FILE" ]; then
  echo "missing source skill: $SOURCE_FILE" >&2
  exit 1
fi

if [ "$mode" = "check" ]; then
  if [ ! -f "$TARGET_FILE" ]; then
    echo "missing embedded skill asset: $TARGET_FILE" >&2
    echo "run scripts/sync-kiss-skill.sh to generate it" >&2
    exit 1
  fi
  if cmp -s "$SOURCE_FILE" "$TARGET_FILE"; then
    echo "KISS bridge skill asset is in sync."
    exit 0
  fi
  echo "KISS bridge skill asset is out of sync." >&2
  echo "run scripts/sync-kiss-skill.sh and commit the result" >&2
  exit 1
fi

mkdir -p "$(dirname "$TARGET_FILE")"

if [ -f "$TARGET_FILE" ] && cmp -s "$SOURCE_FILE" "$TARGET_FILE"; then
  echo "KISS bridge skill asset is already in sync."
  exit 0
fi

cp "$SOURCE_FILE" "$TARGET_FILE"
chmod 0644 "$TARGET_FILE"
echo "Synced internal/kiss/setup_assets/kiss/SKILL.md from skills/kiss/SKILL.md."
