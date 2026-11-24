#!/usr/bin/env bash
# Fetch official Latin-subsetted Atkinson Hyperlegible Next WOFF2 fonts (400, 400i, 700, 700i)
# into static/fonts/ for self-hosted, zero-third-party-request deployment.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FONTS_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)/static/fonts"
mkdir -p "$FONTS_DIR"

BASE_URL="https://cdn.jsdelivr.net/npm/@fontsource/atkinson-hyperlegible-next@5.2.5/files"

FILES=(
  "atkinson-hyperlegible-next-latin-400-normal.woff2"
  "atkinson-hyperlegible-next-latin-400-italic.woff2"
  "atkinson-hyperlegible-next-latin-700-normal.woff2"
  "atkinson-hyperlegible-next-latin-700-italic.woff2"
)

fetch_one() {
  local name="$1"
  local dest="${FONTS_DIR}/${name}"
  local url="${BASE_URL}/${name}"
  echo "[fetch-fonts] Downloading ${name}..."
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --retry 3 --retry-delay 2 -o "${dest}.tmp" "$url"
  elif command -v wget >/dev/null 2>&1; then
    wget -q --tries=3 -O "${dest}.tmp" "$url"
  else
    echo "fetch-fonts.sh: error: curl or wget is required" >&2
    exit 1
  fi
  # Verify WOFF2 magic header ("wOF2")
  local magic
  magic="$(head -c 4 "${dest}.tmp")"
  if [ "$magic" != "wOF2" ]; then
    rm -f "${dest}.tmp"
    echo "fetch-fonts.sh: error: ${name} is not a valid WOFF2 file" >&2
    exit 1
  fi
  mv -f "${dest}.tmp" "$dest"
}

for f in "${FILES[@]}"; do
  fetch_one "$f"
done

echo "[fetch-fonts] Done:"
ls -lh "${FONTS_DIR}"/*.woff2
