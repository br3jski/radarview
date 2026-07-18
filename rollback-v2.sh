#!/usr/bin/env bash
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "Run this rollback as root." >&2
  exit 1
fi

systemctl disable --now adsbpro-feeder.service || true
if [ ! -f /etc/systemd/system/radarview.service ] && [ ! -f /lib/systemd/system/radarview.service ]; then
  echo "Legacy radarview.service is not installed; no automatic rollback is possible." >&2
  exit 1
fi
systemctl daemon-reload
systemctl enable --now radarview.service
echo "Legacy radarview.service is active. Feeder v2 identity files were retained."
