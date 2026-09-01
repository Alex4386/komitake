#!/bin/sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname "$0")" && pwd)"
SUBMODULE_DIR="$ROOT_DIR/third_party/openkart-hostapd"
HOSTAP_ROOT="$SUBMODULE_DIR/hostap"
HOSTAPD_DIR="$HOSTAP_ROOT/hostapd"
PATCH_FILE="$SUBMODULE_DIR/patches/0001-CCMP-Add-a-static_ccmp_key-configuration-option.patch"
CONFIG_FILE="$SUBMODULE_DIR/config"
OUTPUT_BIN="${HOSTAPD_BINARY_OUTPUT:-$ROOT_DIR/hostapd}"

die() {
  echo "$*" >&2
  exit 1
}

# True when $1 is a usable git work tree (handles .git files that point at a
# missing modules dir after a partial clone / copy).
git_worktree_ok() {
  dir="$1"
  git -C "$dir" rev-parse --git-dir >/dev/null 2>&1
}

ensure_openkart_hostapd() {
  if ! git -C "$ROOT_DIR" rev-parse --git-dir >/dev/null 2>&1; then
    die "komitake root is not a git checkout; cannot initialize submodules"
  fi

  if ! git_worktree_ok "$SUBMODULE_DIR"; then
    echo "repairing broken submodule at $SUBMODULE_DIR"
    # Stale gitfile pointing at a missing .git/modules/... path is common after
    # a non-recursive clone or an incomplete copy. Wipe the checkout + module
    # metadata, then re-init from the parent repo.
    rm -rf "$SUBMODULE_DIR"
    rm -rf "$ROOT_DIR/.git/modules/third_party/openkart-hostapd"
    git -C "$ROOT_DIR" submodule sync -- third_party/openkart-hostapd
    git -C "$ROOT_DIR" submodule update --init --recursive third_party/openkart-hostapd
  fi

  if ! git_worktree_ok "$SUBMODULE_DIR"; then
    echo "missing submodule at $SUBMODULE_DIR" >&2
    echo "run: git submodule update --init --recursive" >&2
    exit 1
  fi

  # Nested hostap submodule may still need sync/init (and URL override).
  git -C "$SUBMODULE_DIR" config -f .gitmodules submodule.hostap.url https://w1.fi/hostap.git
  git -C "$SUBMODULE_DIR" submodule sync -- hostap
  git -C "$ROOT_DIR" submodule update --init --recursive third_party/openkart-hostapd
}

ensure_openkart_hostapd

if [ ! -d "$HOSTAPD_DIR" ]; then
  echo "missing hostapd sources at $HOSTAPD_DIR" >&2
  echo "make sure third_party/openkart-hostapd submodules are initialized" >&2
  exit 1
fi

mkdir -p "$(dirname "$OUTPUT_BIN")"

cp "$CONFIG_FILE" "$HOSTAPD_DIR/.config"

if ! grep -q "static_ccmp_key" "$HOSTAP_ROOT/src/ap/ap_config.h"; then
  git -C "$HOSTAP_ROOT" apply "$PATCH_FILE"
fi

make -C "$HOSTAPD_DIR"
cp "$HOSTAPD_DIR/hostapd" "$OUTPUT_BIN"
chmod +x "$OUTPUT_BIN"

echo "built patched hostapd at $OUTPUT_BIN"
