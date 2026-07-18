#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "Run this installer as root." >&2
  exit 1
fi

BINARY_PATH="${1:-./adsbpro-feeder}"
SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
if [ ! -x "$BINARY_PATH" ]; then
  echo "Pass the path to a verified adsbpro-feeder binary." >&2
  exit 1
fi

install -d -m 0755 /etc/adsbpro-feeder
if ! id adsbpro-feeder >/dev/null 2>&1; then
  useradd --system --home /var/lib/adsbpro-feeder --shell /usr/sbin/nologin adsbpro-feeder
fi
install -d -o adsbpro-feeder -g adsbpro-feeder -m 0700 /var/lib/adsbpro-feeder
install -m 0755 "$BINARY_PATH" /usr/local/bin/adsbpro-feeder
install -m 0644 "$SCRIPT_DIR/packaging/config.env" /etc/adsbpro-feeder/config.env
install -m 0644 "$SCRIPT_DIR/packaging/adsbpro-feeder.service" /etc/systemd/system/adsbpro-feeder.service

TOKEN=""
if [ -f /opt/radarview.py ]; then
  TOKEN=$(sed -n "s/^USER_TOKEN = ['\"]\(ADS-[A-Za-z0-9]*\)['\"].*/\1/p" /opt/radarview.py | head -n 1)
fi
if [ -z "$TOKEN" ]; then
  read -r -s -p "ADS-B.Pro feeder token: " TOKEN
  echo
fi
if ! printf '%s' "$TOKEN" | grep -Eq '^ADS-[A-Za-z0-9]{32}$'; then
  echo "Invalid token format." >&2
  exit 1
fi
printf '%s' "$TOKEN" > /var/lib/adsbpro-feeder/pairing-token
chown adsbpro-feeder:adsbpro-feeder /var/lib/adsbpro-feeder/pairing-token
chmod 0600 /var/lib/adsbpro-feeder/pairing-token
unset TOKEN

systemctl daemon-reload
systemctl enable --now adsbpro-feeder.service

ACTIVE=false
for _ in $(seq 1 90); do
  if [ -f /var/lib/adsbpro-feeder/status.json ] && grep -q '"state":"active"' /var/lib/adsbpro-feeder/status.json; then
    ACTIVE=true
    break
  fi
  sleep 1
done

if [ "$ACTIVE" != true ]; then
  systemctl stop adsbpro-feeder.service
  echo "Feeder v2 did not become active. The legacy service was left unchanged." >&2
  exit 1
fi

if systemctl list-unit-files radarview.service >/dev/null 2>&1; then
  systemctl disable --now radarview.service || true
fi
echo "Feeder v2 is active. Legacy files were retained for rollback."
