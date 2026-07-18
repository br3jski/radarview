#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
ADS-B.Pro feeder v2 installer

Usage: radarview_setup.sh [options]

Bootstrap options:
  --version VERSION     Install a specific release (default: 2.0.0).
  -h, --help            Show this help.

Feeder options passed to the verified installer:
  --token-file PATH
  --source-host HOST
  --source-mode auto|beast|sbs
  --beast-port PORT
  --sbs-port PORT
  --label LABEL
  --wait-seconds N

Existing radarview.py installations are migrated automatically. The legacy
service is disabled only after v2 reaches ACTIVE and sends an accepted frame.
EOF
}

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

VERSION="${ADSBPRO_VERSION:-2.0.0}"
INSTALLER_ARGS=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version) VERSION="${2:?--version requires a value}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) INSTALLER_ARGS+=("$1"); shift ;;
  esac
done

[ "$(id -u)" -eq 0 ] || fail "Run this installer as root (for example with sudo)."
[[ "$VERSION" =~ ^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$ ]] || fail "Invalid release version."

case "$(uname -m)" in
  x86_64|amd64) PLATFORM=linux-amd64 ;;
  aarch64|arm64) PLATFORM=linux-arm64 ;;
  armv7l) PLATFORM=linux-armv7 ;;
  armv6l) PLATFORM=linux-armv6 ;;
  *) fail "Unsupported architecture: $(uname -m)" ;;
esac

for command_name in sha256sum tar mktemp; do
  command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required."
done

if ! command -v curl >/dev/null 2>&1 || ! command -v openssl >/dev/null 2>&1; then
  if command -v apt-get >/dev/null 2>&1; then
    echo "Installing HTTPS and signature verification tools..."
    apt-get update
    DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl openssl
  else
    fail "curl and openssl are required. Install them and run this installer again."
  fi
fi

REPOSITORY_URL="${REPOSITORY_URL:-https://github.com/br3jski/radarview/releases/download/v${VERSION}}"
ASSET="adsbpro-feeder-${VERSION}-${PLATFORM}.tar.gz"
CHECKSUMS="SHA256SUMS-${VERSION}"
TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT

echo "Downloading ADS-B.Pro feeder v${VERSION} for ${PLATFORM}..."
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
  "$REPOSITORY_URL/$ASSET" -o "$TEMP_DIR/$ASSET"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
  "$REPOSITORY_URL/$CHECKSUMS" -o "$TEMP_DIR/$CHECKSUMS"
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
  "$REPOSITORY_URL/$CHECKSUMS.sig" -o "$TEMP_DIR/$CHECKSUMS.sig"

cat > "$TEMP_DIR/release-signing-public.pem" <<'EOF'
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEwdJsBoq27ca5SsbOv+v6MsiuCx2u
sKMbr69tLhqXBC2X3o6PZGaNg3K2srF0zjb6Y2musEm9opwy42NUVaX83A==
-----END PUBLIC KEY-----
EOF
openssl dgst -sha256 \
  -verify "$TEMP_DIR/release-signing-public.pem" \
  -signature "$TEMP_DIR/$CHECKSUMS.sig" \
  "$TEMP_DIR/$CHECKSUMS"
(
  cd "$TEMP_DIR"
  grep -F "  $ASSET" "$CHECKSUMS" | grep -E "^[0-9a-fA-F]{64}  ${ASSET}$" | sha256sum -c -
)

install -d -m 0700 "$TEMP_DIR/package"
tar -xzf "$TEMP_DIR/$ASSET" -C "$TEMP_DIR/package"
[ -x "$TEMP_DIR/package/install-v2.sh" ] || fail "Verified package does not contain install-v2.sh."
[ -x "$TEMP_DIR/package/adsbpro-feeder" ] || fail "Verified package does not contain the feeder binary."

"$TEMP_DIR/package/install-v2.sh" \
  --binary "$TEMP_DIR/package/adsbpro-feeder" \
  "${INSTALLER_ARGS[@]}"
