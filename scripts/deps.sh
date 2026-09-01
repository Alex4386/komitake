#!/bin/sh
# Install Komitake build and runtime dependencies.
# Detects the system package manager; override with PKG or install manually.
set -eu

# Packages by ecosystem:
#   hostapd build — C toolchain, libnl headers, pkg-config
#   komitake build — git (submodules)
#   komitake runtime — ffmpeg (live camera transcode; ffplay usually bundled)
#   video hwaccel — VA-API userspace drivers where packaged separately
# Node.js / npm — install separately via NodeSource before ./INSTALL
apt_pkgs="build-essential libnl-3-dev libnl-genl-3-dev pkg-config git ffmpeg mesa-va-drivers"
dnf_pkgs="gcc make libnl3-devel pkgconf-pkg-config git ffmpeg mesa-va-drivers"
pacman_pkgs="base-devel libnl pkgconf git ffmpeg libva-mesa-driver"
zypper_pkgs="gcc make libnl3-devel pkg-config git ffmpeg libva-mesa-driver"
apk_pkgs="build-base libnl3-dev pkgconf git ffmpeg mesa-va-gallium"

die() {
  echo "$*" >&2
  exit 1
}

warn() {
  echo "warning: $*" >&2
}

# run_root executes a command as root, via sudo when available and not already
# root, so the script works both under sudo and as an unprivileged user.
run_root() {
  if [ "$(id -u)" = "0" ]; then
    "$@"
    return
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo "$@"
    return
  fi
  die "need root to install packages; re-run as root or install sudo"
}

detect_pkg() {
  if [ -n "${PKG:-}" ]; then
    echo "$PKG"
    return
  fi
  for candidate in apt-get dnf yum pacman zypper apk; do
    if command -v "$candidate" >/dev/null 2>&1; then
      echo "$candidate"
      return
    fi
  done
  echo ""
}

check_cmd() {
  name="$1"
  hint="$2"
  if command -v "$name" >/dev/null 2>&1; then
    return 0
  fi
  warn "$name was not found on PATH. $hint"
  return 1
}

pkg="$(detect_pkg)"
case "$pkg" in
  apt-get)
    run_root apt-get update
    run_root apt-get install -y $apt_pkgs
    ;;
  dnf)
    run_root dnf install -y $dnf_pkgs
    ;;
  yum)
    run_root yum install -y $dnf_pkgs
    ;;
  pacman)
    run_root pacman -Sy --needed --noconfirm $pacman_pkgs
    ;;
  zypper)
    run_root zypper install -y $zypper_pkgs
    ;;
  apk)
    run_root apk add $apk_pkgs
    ;;
  "")
    die "no supported package manager found; install these manually:
  netlink dev headers (libnl-3, libnl-genl-3), pkg-config, a C toolchain, make,
  git, ffmpeg, and VA-API drivers for your GPU"
    ;;
  *)
    die "unsupported package manager: $pkg"
    ;;
esac

missing=0
check_cmd git "git is required to fetch hostapd submodules during ./INSTALL." || missing=1
check_cmd ffmpeg "ffmpeg is required at runtime for live camera video." || missing=1
check_cmd npm "install Node.js from NodeSource (https://github.com/nodesource/distributions) before ./INSTALL." || missing=1
if ! command -v ffplay >/dev/null 2>&1; then
  warn "ffplay was not found on PATH. Install it if you want \`komitake video\`; the web UI does not need it."
fi
if ! command -v go >/dev/null 2>&1; then
  warn "Go was not found on PATH. Install Go 1.26+ from https://go.dev/dl/ before running ./INSTALL."
  missing=1
else
  go_minor="$(go env GOVERSION 2>/dev/null | sed -n 's/^go1\.\([0-9][0-9]*\).*/\1/p')"
  if [ -z "$go_minor" ] || [ "$go_minor" -lt 26 ] 2>/dev/null; then
    warn "found $(go env GOVERSION 2>/dev/null || go version); Komitake needs Go 1.26+. Install a newer toolchain from https://go.dev/dl/."
    missing=1
  fi
fi

if [ "$missing" -ne 0 ]; then
  echo
  echo "some dependencies are still missing; fix the warnings above before ./INSTALL."
  exit 1
fi

echo "dependencies installed"
