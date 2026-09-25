#!/usr/bin/env bash
# One-shot installer for watch on Ubuntu.
# Usage: sudo ./deploy/install.sh [movies folder] [listen address]
#   movies folder defaults to /home/<your user>/Movies
#   listen address defaults to 0.0.0.0:8092 (the TV needs the LAN)
set -euo pipefail

BIN="watch"
BIN_DIR="/opt/watch"
DATA_DIR="/var/lib/watch"
SERVICE="watch.service"

# The service runs as the user who owns the Movies folder, so pick that from
# whoever invoked sudo.
RUN_USER="${SUDO_USER:-andrinoff}"
if ! id -u "$RUN_USER" >/dev/null 2>&1; then
  echo "error: user '$RUN_USER' does not exist" >&2
  exit 1
fi
RUN_HOME="$(getent passwd "$RUN_USER" | cut -d: -f6)"
MOVIES="${1:-$RUN_HOME/Movies}"
ADDR="${2:-0.0.0.0:8092}"
PORT="${ADDR##*:}"

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if [[ $EUID -ne 0 ]]; then
  echo "error: run as root (sudo ./deploy/install.sh)" >&2
  exit 1
fi

if [[ ! -f "$REPO_DIR/$BIN" ]]; then
  echo "error: '$BIN' not found in $REPO_DIR. Run 'make build' first." >&2
  exit 1
fi

if [[ ! -d "$MOVIES" ]]; then
  echo "error: movies folder '$MOVIES' does not exist." >&2
  echo "  Create it, or pass one:  sudo ./deploy/install.sh /path/to/videos" >&2
  exit 1
fi

echo "==> Installing binary"
install -d -m 0755 "$BIN_DIR"
install -m 0755 "$REPO_DIR/$BIN" "$BIN_DIR/$BIN"
install -d -m 0755 -o "$RUN_USER" -g "$RUN_USER" "$DATA_DIR"

echo "==> Installing systemd unit (user $RUN_USER, shelf $MOVIES)"
sed -e "s|^User=.*|User=$RUN_USER|" \
    -e "s|^Group=.*|Group=$RUN_USER|" \
    -e "s|-addr 0.0.0.0:8092|-addr $ADDR|" \
    -e "s|-dir /home/andrinoff/Movies|-dir $MOVIES|" \
    "$REPO_DIR/deploy/$SERVICE" > "/etc/systemd/system/$SERVICE"
chmod 0644 "/etc/systemd/system/$SERVICE"
systemctl daemon-reload

echo "==> Starting service"
systemctl enable --now "$SERVICE"
sleep 2

if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; then
  echo "OK: watch is up on $ADDR, serving $MOVIES"
else
  echo "Started, but the health check failed. Check:"
  echo "  systemctl status $SERVICE --no-pager"
  echo "  journalctl -u $SERVICE -e --no-pager"
fi

echo
echo "On the TV: open any media app (VLC, or the TV's own media browser) and look"
echo "for '$(hostname) watch' on the network. It appears on its own."
echo
echo "For the browser:"
echo "  sudo ./deploy/setup-domain.sh watch.andrinoff.com"
