#!/bin/sh
# Expand docs/banner.svg text into paths as docs/banner.expanded.svg.
# Uses Pretendard JP (downloaded into docs/fonts/ on first run).
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
EXPAND_DIR="$ROOT_DIR/scripts/expand-banner"
FONT_DIR="$ROOT_DIR/docs/fonts"
PRETENDARD_TAG="v1.3.9"
PRETENDARD_BASE="https://github.com/orioncactus/pretendard/raw/${PRETENDARD_TAG}/packages/pretendard-jp/dist/public/static"

ensure_pretendard_jp() {
  mkdir -p "$FONT_DIR"
  for file in PretendardJP-Bold.otf PretendardJP-Medium.otf PretendardJP-Regular.otf; do
    if [ -f "$FONT_DIR/$file" ]; then
      continue
    fi
    echo "downloading $file..."
    curl -fsSL "$PRETENDARD_BASE/$file" -o "$FONT_DIR/$file"
  done
}

command -v curl >/dev/null 2>&1 || {
  echo "curl is required to download Pretendard JP fonts" >&2
  exit 1
}

command -v npm >/dev/null 2>&1 || {
  echo "npm is required to expand banner.svg (install Node.js from NodeSource)" >&2
  exit 1
}

ensure_pretendard_jp

if [ ! -d "$EXPAND_DIR/node_modules/opentype.js" ]; then
  echo "installing expand-banner dependencies..."
  npm install --prefix "$EXPAND_DIR" --no-fund --no-audit
fi

node "$EXPAND_DIR/expand.mjs"
