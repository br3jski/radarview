#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
if [ -x "$SCRIPT_DIR/radarview_setup.sh" ]; then
  exec "$SCRIPT_DIR/radarview_setup.sh" "$@"
fi

TEMP_SCRIPT=$(mktemp)
trap 'rm -f "$TEMP_SCRIPT"' EXIT
SETUP_URL=https://raw.githubusercontent.com/br3jski/radarview/main/radarview_setup.sh
if command -v curl >/dev/null 2>&1; then
  curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
    "$SETUP_URL" -o "$TEMP_SCRIPT"
elif command -v wget >/dev/null 2>&1; then
  wget --https-only --quiet -O "$TEMP_SCRIPT" "$SETUP_URL"
else
  echo "ERROR: curl or wget is required." >&2
  exit 1
fi
chmod 0700 "$TEMP_SCRIPT"
"$TEMP_SCRIPT" "$@"
