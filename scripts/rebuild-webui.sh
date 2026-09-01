#!/bin/sh
# Rebuild the embedded web UI (Vite/React) into internal/web/frontend/dist
# so `go build` / INSTALL can embed a fresh UI.
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
FRONTEND_DIR="$ROOT_DIR/internal/web/frontend"

die() {
  echo "$*" >&2
  exit 1
}

command -v npm >/dev/null 2>&1 || die "npm is required to rebuild the web UI"

cd "$FRONTEND_DIR"

if [ ! -d node_modules ]; then
  echo "installing frontend dependencies…"
  npm install
fi

echo "building web UI…"
npm run build

echo "web UI ready at $FRONTEND_DIR/dist"
