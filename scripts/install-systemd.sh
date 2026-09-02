#!/bin/sh
# Install the komitake systemd unit. The unit text is embedded below, so this
# script is self-contained and does not depend on a separate .service template.
#
# Configuration via environment:
#   KOMITAKE_BIN   path to the installed komitake binary (default: BINDIR/komitake)
#   CONFIG_PATH    path to config.json                   (default: SYSCONFDIR/komitake/config.json)
#   DESTDIR        staging root for packaging            (default: empty)
#   PREFIX         install prefix                        (default: /usr)
#   BINDIR         binary dir                            (default: PREFIX/bin)
#   SYSCONFDIR     config root                           (default: /etc)
#   SYSTEMD_UNIT_DIR  unit dir  (default: SYSCONFDIR/systemd/system)
set -eu

DESTDIR="${DESTDIR:-}"
PREFIX="${PREFIX:-/usr}"
SYSCONFDIR="${SYSCONFDIR:-/etc}"
BINDIR="${BINDIR:-$PREFIX/bin}"
SYSTEMD_UNIT_DIR="${SYSTEMD_UNIT_DIR:-$SYSCONFDIR/systemd/system}"

KOMITAKE_BIN="${KOMITAKE_BIN:-$BINDIR/komitake}"
CONFIG_PATH="${CONFIG_PATH:-$SYSCONFDIR/komitake/config.json}"
SERVICE_DST="$DESTDIR$SYSTEMD_UNIT_DIR/komitake.service"

die() {
  echo "$*" >&2
  exit 1
}

# install_file copies with the requested mode, escalating to sudo only if the
# unprivileged attempt fails.
install_file() {
  src="$1"
  dst="$2"
  mode="$3"
  dir="$(dirname "$dst")"
  mkdir -p "$dir" 2>/dev/null || {
    command -v sudo >/dev/null 2>&1 && sudo mkdir -p "$dir"
  } || die "cannot create $dir (re-run with sudo or set DESTDIR/PREFIX/SYSCONFDIR)"

  if install -m "$mode" "$src" "$dst" 2>/dev/null; then
    return 0
  fi
  if command -v sudo >/dev/null 2>&1; then
    sudo install -m "$mode" "$src" "$dst"
    return 0
  fi
  die "cannot install $dst (re-run with sudo or set DESTDIR/PREFIX/SYSCONFDIR)"
}

tmp_service="$(mktemp)"
trap 'rm -f "$tmp_service"' EXIT INT TERM

# The embedded unit. ExecStart/config are interpolated from the resolved paths
# above; everything else is fixed.
cat > "$tmp_service" <<EOF
[Unit]
Description=Komitake Fuji connectivity daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
# Restart=always so `komitake daemon` can exit for a supervised restart
# (triggered via the admin API / Web UI) and systemd brings it right back.
ExecStart=$KOMITAKE_BIN daemon --config $CONFIG_PATH
Restart=always
RestartSec=2
User=root
WorkingDirectory=/

[Install]
WantedBy=multi-user.target
EOF

install_file "$tmp_service" "$SERVICE_DST" 644
echo "installed systemd unit to $SERVICE_DST"

# Reload systemd only for a live install (not when staging into DESTDIR).
# Prefer sudo so an unprivileged INSTALL still refreshes unit files.
if [ -z "$DESTDIR" ] && command -v systemctl >/dev/null 2>&1; then
  if [ "$(id -u)" = "0" ]; then
    systemctl daemon-reload
  elif command -v sudo >/dev/null 2>&1; then
    sudo systemctl daemon-reload
  else
    echo "warning: could not daemon-reload (install sudo or re-run as root)" >&2
  fi
  echo "next: systemctl enable --now komitake.service"
fi
