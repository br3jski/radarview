#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: install-v2.sh --binary PATH [options]

Options:
  --token-file PATH     Read the pairing token from PATH without displaying it.
  --source-host HOST    ADS-B source host (default: 127.0.0.1).
  --source-mode MODE    auto, beast or sbs (default: auto).
  --beast-port PORT     Beast source port (default: 30005).
  --sbs-port PORT       SBS source port (default: 30003).
  --label LABEL         Installation label (default: ADS-B feeder).
  --status-listen ADDR  Local status page address (default: 127.0.0.1:54321).
  --aircraft-json VALUE Optional aircraft.json file path or URL.
  --wait-seconds N      Time to wait for the first accepted frame (default: 90).
  -h, --help            Show this help.

For compatibility, a single positional argument is accepted as the binary path.
EOF
}

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

is_safe_line() {
  local value="$1"
  [ -n "$value" ] && [ "${#value}" -le 255 ] && [[ "$value" != *$'\n'* ]] && [[ "$value" != *$'\r'* ]]
}

valid_port() {
  [[ "$1" =~ ^[0-9]+$ ]] && [ "$1" -ge 1 ] && [ "$1" -le 65535 ]
}

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
EXISTING_CONFIG=/etc/adsbpro-feeder/config.env

existing_config_value() {
  local key="$1"
  local fallback="$2"
  local value=""
  if [ -f "$EXISTING_CONFIG" ]; then
    value=$(sed -n "/^${key}=/{s/^${key}=//;p;q;}" "$EXISTING_CONFIG")
  fi
  printf '%s' "${value:-$fallback}"
}

BINARY_PATH=""
PAIRING_TOKEN_SOURCE="${ADSBPRO_PAIRING_TOKEN_FILE:-}"
SOURCE_HOST="${ADSBPRO_SOURCE_HOST:-$(existing_config_value SOURCE_HOST 127.0.0.1)}"
SOURCE_MODE="${ADSBPRO_SOURCE_MODE:-$(existing_config_value SOURCE_MODE auto)}"
BEAST_PORT="${ADSBPRO_BEAST_PORT:-$(existing_config_value BEAST_PORT 30005)}"
SBS_PORT="${ADSBPRO_SBS_PORT:-$(existing_config_value SBS_PORT 30003)}"
FEEDER_LABEL="${ADSBPRO_FEEDER_LABEL:-$(existing_config_value FEEDER_LABEL 'ADS-B feeder')}"
STATUS_LISTEN="${ADSBPRO_STATUS_LISTEN:-$(existing_config_value STATUS_LISTEN 127.0.0.1:54321)}"
AIRCRAFT_JSON="${ADSBPRO_AIRCRAFT_JSON:-$(existing_config_value AIRCRAFT_JSON '')}"
WAIT_SECONDS="${ADSBPRO_WAIT_SECONDS:-90}"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --binary) BINARY_PATH="${2:?--binary requires a path}"; shift 2 ;;
    --token-file) PAIRING_TOKEN_SOURCE="${2:?--token-file requires a path}"; shift 2 ;;
    --source-host) SOURCE_HOST="${2:?--source-host requires a value}"; shift 2 ;;
    --source-mode) SOURCE_MODE="${2:?--source-mode requires a value}"; shift 2 ;;
    --beast-port) BEAST_PORT="${2:?--beast-port requires a value}"; shift 2 ;;
    --sbs-port) SBS_PORT="${2:?--sbs-port requires a value}"; shift 2 ;;
    --label) FEEDER_LABEL="${2:?--label requires a value}"; shift 2 ;;
    --status-listen) STATUS_LISTEN="${2:?--status-listen requires a value}"; shift 2 ;;
    --aircraft-json) AIRCRAFT_JSON="${2:?--aircraft-json requires a value}"; shift 2 ;;
    --wait-seconds) WAIT_SECONDS="${2:?--wait-seconds requires a value}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    --*) fail "Unknown option: $1" ;;
    *)
      if [ -n "$BINARY_PATH" ]; then
        fail "Unexpected argument: $1"
      fi
      BINARY_PATH="$1"
      shift
      ;;
  esac
done

[ "$(id -u)" -eq 0 ] || fail "Run this installer as root."
[ -n "$BINARY_PATH" ] || fail "Pass the path to a verified adsbpro-feeder binary."
[ -x "$BINARY_PATH" ] || fail "The feeder binary is missing or not executable: $BINARY_PATH"
[ -f "$SCRIPT_DIR/packaging/adsbpro-feeder.service" ] || fail "Missing systemd unit in the installer package."
[ -f "$SCRIPT_DIR/rollback-v2.sh" ] || fail "Missing rollback script in the installer package."
is_safe_line "$SOURCE_HOST" || fail "Invalid source host."
[[ "$SOURCE_HOST" =~ ^[A-Za-z0-9._:-]+$ ]] || fail "Invalid source host."
case "$SOURCE_MODE" in auto|beast|sbs) ;; *) fail "Source mode must be auto, beast or sbs." ;; esac
valid_port "$BEAST_PORT" || fail "Invalid Beast port."
valid_port "$SBS_PORT" || fail "Invalid SBS port."
is_safe_line "$FEEDER_LABEL" || fail "Invalid feeder label."
is_safe_line "$STATUS_LISTEN" || fail "Invalid status listen address."
[[ "$STATUS_LISTEN" =~ ^(\[[0-9A-Fa-f:]+\]|[A-Za-z0-9._-]+):([0-9]{1,5})$ ]] || fail "Status listen address must be HOST:PORT."
valid_port "${BASH_REMATCH[2]}" || fail "Invalid status page port."
if [ -n "$AIRCRAFT_JSON" ]; then is_safe_line "$AIRCRAFT_JSON" || fail "Invalid aircraft.json location."; fi
if ! [[ "$WAIT_SECONDS" =~ ^[0-9]+$ ]]; then
  fail "Wait time must be between 5 and 600 seconds."
fi
if [ "$WAIT_SECONDS" -lt 5 ] || [ "$WAIT_SECONDS" -gt 600 ]; then
  fail "Wait time must be between 5 and 600 seconds."
fi

for command_name in systemctl install getent useradd groupadd sed grep date; do
  command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required."
done

DATA_DIR=/var/lib/adsbpro-feeder
CONFIG_DIR=/etc/adsbpro-feeder
SERVICE_PATH=/etc/systemd/system/adsbpro-feeder.service
LEGACY_SERVICE=radarview.service
TIMESTAMP=$(date -u +%Y%m%dT%H%M%SZ)
BACKUP_DIR=/var/backups/adsbpro-feeder/before-install-$TIMESTAMP
PREVIOUS_V2_ACTIVE=false
PREVIOUS_V2_ENABLED=false

systemctl is-active --quiet adsbpro-feeder.service 2>/dev/null && PREVIOUS_V2_ACTIVE=true
systemctl is-enabled --quiet adsbpro-feeder.service 2>/dev/null && PREVIOUS_V2_ENABLED=true

install -d -m 0700 "$BACKUP_DIR"
for existing_path in \
  /opt/radarview.py \
  /opt/radarview \
  /etc/systemd/system/radarview.service \
  /usr/local/bin/adsbpro-feeder \
  /etc/adsbpro-feeder/config.env \
  /etc/systemd/system/adsbpro-feeder.service; do
  if [ -e "$existing_path" ]; then
    cp -a "$existing_path" "$BACKUP_DIR/"
  fi
done
printf '%s\n' "$PREVIOUS_V2_ACTIVE" > "$BACKUP_DIR/adsbpro-feeder.was-active"
printf '%s\n' "$PREVIOUS_V2_ENABLED" > "$BACKUP_DIR/adsbpro-feeder.was-enabled"
systemctl is-active "$LEGACY_SERVICE" > "$BACKUP_DIR/radarview.was-active" 2>/dev/null || true
systemctl is-enabled "$LEGACY_SERVICE" > "$BACKUP_DIR/radarview.was-enabled" 2>/dev/null || true

if ! getent group adsbpro-feeder >/dev/null; then
  groupadd --system adsbpro-feeder
fi
if ! id adsbpro-feeder >/dev/null 2>&1; then
  useradd --system --gid adsbpro-feeder --home-dir "$DATA_DIR" --shell /usr/sbin/nologin adsbpro-feeder
fi

install -d -m 0755 "$CONFIG_DIR"
install -d -o adsbpro-feeder -g adsbpro-feeder -m 0700 "$DATA_DIR"
install -o root -g root -m 0755 "$BINARY_PATH" /usr/local/bin/adsbpro-feeder
install -o root -g root -m 0644 "$SCRIPT_DIR/packaging/adsbpro-feeder.service" "$SERVICE_PATH"
install -o root -g root -m 0750 "$SCRIPT_DIR/rollback-v2.sh" /usr/local/sbin/adsbpro-feeder-rollback

CONFIG_TEMP=$(mktemp)
trap 'rm -f "$CONFIG_TEMP"' EXIT
{
  printf 'SERVER_ADDR=feed.ads-b.pro:48582\n'
  printf 'SERVER_NAME=feed.ads-b.pro\n'
  printf 'SOURCE_HOST=%s\n' "$SOURCE_HOST"
  printf 'SOURCE_MODE=%s\n' "$SOURCE_MODE"
  printf 'BEAST_PORT=%s\n' "$BEAST_PORT"
  printf 'SBS_PORT=%s\n' "$SBS_PORT"
  printf 'DATA_DIR=%s\n' "$DATA_DIR"
  printf 'TOKEN_FILE=%s/pairing-token\n' "$DATA_DIR"
  printf 'FEEDER_LABEL=%s\n' "$FEEDER_LABEL"
  printf 'STATUS_LISTEN=%s\n' "$STATUS_LISTEN"
  printf 'AIRCRAFT_JSON=%s\n' "$AIRCRAFT_JSON"
  printf 'UPDATE_URL=https://raw.githubusercontent.com/br3jski/radarview/main/latest.json\n'
} > "$CONFIG_TEMP"
install -o root -g root -m 0644 "$CONFIG_TEMP" "$CONFIG_DIR/config.env"

for identity_file in identity-key.pem installation-id paired status.json status.json.new pairing-token; do
  if [ -e "$DATA_DIR/$identity_file" ]; then
    chown adsbpro-feeder:adsbpro-feeder "$DATA_DIR/$identity_file"
    chmod 0600 "$DATA_DIR/$identity_file"
  fi
done

if [ -s "$DATA_DIR/paired" ]; then
  rm -f "$DATA_DIR/pairing-token"
else
  TOKEN=""
  if [ -n "$PAIRING_TOKEN_SOURCE" ]; then
    [ -r "$PAIRING_TOKEN_SOURCE" ] || fail "Cannot read the pairing token file."
    TOKEN=$(tr -d '\r\n' < "$PAIRING_TOKEN_SOURCE")
  else
    for legacy_script in /opt/radarview.py /opt/radarview/radarview.py; do
      if [ ! -f "$legacy_script" ]; then
        continue
      fi
      TOKEN=$(sed -nE "s/^[[:space:]]*USER_TOKEN[[:space:]]*=[[:space:]]*['\"]([^'\"]+)['\"].*/\1/p" "$legacy_script" | head -n 1)
      if [ -n "$TOKEN" ]; then
        echo "Using the token from the existing legacy installation."
        break
      fi
    done
  fi
  if [ -z "$TOKEN" ]; then
    if [ ! -r /dev/tty ]; then
      fail "No interactive terminal. Re-run with --token-file PATH."
    fi
    read -r -s -p "ADS-B.Pro feeder token: " TOKEN < /dev/tty
    echo > /dev/tty
  fi
  if ! printf '%s' "$TOKEN" | LC_ALL=C grep -Eq '^[[:graph:]]{1,255}$'; then
    fail "Invalid token format."
  fi
  umask 077
  printf '%s' "$TOKEN" > "$DATA_DIR/pairing-token"
  chown adsbpro-feeder:adsbpro-feeder "$DATA_DIR/pairing-token"
  chmod 0600 "$DATA_DIR/pairing-token"
  unset TOKEN
fi

restore_previous_v2() {
  systemctl disable --now adsbpro-feeder.service >/dev/null 2>&1 || true
  if [ -f "$BACKUP_DIR/adsbpro-feeder" ]; then
    install -o root -g root -m 0755 "$BACKUP_DIR/adsbpro-feeder" /usr/local/bin/adsbpro-feeder
  fi
  if [ -f "$BACKUP_DIR/config.env" ]; then
    install -o root -g root -m 0644 "$BACKUP_DIR/config.env" "$CONFIG_DIR/config.env"
  fi
  if [ -f "$BACKUP_DIR/adsbpro-feeder.service" ]; then
    install -o root -g root -m 0644 "$BACKUP_DIR/adsbpro-feeder.service" "$SERVICE_PATH"
  fi
  systemctl daemon-reload
  if [ "$PREVIOUS_V2_ENABLED" = true ]; then
    systemctl enable adsbpro-feeder.service >/dev/null 2>&1 || true
  fi
  if [ "$PREVIOUS_V2_ACTIVE" = true ]; then
    systemctl start adsbpro-feeder.service >/dev/null 2>&1 || true
  fi
}

rm -f "$DATA_DIR/status.json" "$DATA_DIR/status.json.new"
systemctl daemon-reload
systemctl reset-failed adsbpro-feeder.service >/dev/null 2>&1 || true
systemctl enable adsbpro-feeder.service >/dev/null
systemctl restart adsbpro-feeder.service

ACTIVE=false
PERMANENT_ERROR=""
for ((attempt = 0; attempt < WAIT_SECONDS; attempt++)); do
  if [ -f "$DATA_DIR/status.json" ]; then
    if grep -q '"state":"active"' "$DATA_DIR/status.json" && systemctl is-active --quiet adsbpro-feeder.service; then
      ACTIVE=true
      break
    fi
    if grep -q 'PAIRING_WINDOW_REQUIRED' "$DATA_DIR/status.json"; then
      PERMANENT_ERROR=PAIRING_WINDOW_REQUIRED
      break
    fi
    if grep -q 'INVALID_TOKEN' "$DATA_DIR/status.json"; then
      PERMANENT_ERROR=INVALID_TOKEN
      break
    fi
  fi
  sleep 1
done

if [ "$ACTIVE" != true ]; then
  restore_previous_v2
  case "$PERMANENT_ERROR" in
    PAIRING_WINDOW_REQUIRED)
      fail "This account already has an installation. Open a pairing window in the ADS-B.Pro account panel and run the installer again. Legacy was left unchanged."
      ;;
    INVALID_TOKEN)
      fail "The feeder token was rejected. Check the token and run the installer again. Legacy was left unchanged."
      ;;
    *)
      fail "Feeder v2 did not reach ACTIVE within ${WAIT_SECONDS}s. Legacy was left unchanged. Check: journalctl -u adsbpro-feeder.service"
      ;;
  esac
fi

if systemctl cat "$LEGACY_SERVICE" >/dev/null 2>&1; then
  if ! systemctl disable --now "$LEGACY_SERVICE" >/dev/null 2>&1; then
    restore_previous_v2
    fail "Feeder v2 reached ACTIVE, but legacy could not be disabled. Feeder v2 was stopped to avoid duplicate upload."
  fi
  if systemctl is-active --quiet "$LEGACY_SERVICE" || systemctl is-enabled --quiet "$LEGACY_SERVICE"; then
    restore_previous_v2
    fail "Legacy remained active or enabled. Feeder v2 was stopped to avoid duplicate upload."
  fi
fi
systemctl is-active --quiet adsbpro-feeder.service || {
  restore_previous_v2
  fail "Feeder v2 stopped while disabling legacy. The previous v2 state was restored."
}

echo "Feeder v2 is ACTIVE. Legacy files were retained for rollback."
echo "Backup: $BACKUP_DIR"
echo "Status: /usr/local/bin/adsbpro-feeder status"
echo "Status page: http://$STATUS_LISTEN"
echo "Rollback: /usr/local/sbin/adsbpro-feeder-rollback"
