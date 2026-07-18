#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:?Usage: install-release.sh VERSION}"
: "${MINISIGN_PUBLIC_KEY:?Set MINISIGN_PUBLIC_KEY to the trusted release public key}"
REPOSITORY_URL="${REPOSITORY_URL:-https://github.com/br3jski/radarview/releases/download/v${VERSION}}"

case "$(uname -m)" in
  x86_64|amd64) PLATFORM=linux-amd64 ;;
  aarch64|arm64) PLATFORM=linux-arm64 ;;
  armv7l) PLATFORM=linux-armv7 ;;
  armv6l) PLATFORM=linux-armv6 ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

for command_name in curl minisign sha256sum; do
  command -v "$command_name" >/dev/null || { echo "$command_name is required" >&2; exit 1; }
done

TEMP_DIR=$(mktemp -d)
trap 'rm -rf "$TEMP_DIR"' EXIT
ASSET="adsbpro-feeder-${VERSION}-${PLATFORM}"
curl --fail --location --proto '=https' --tlsv1.2 "$REPOSITORY_URL/$ASSET" -o "$TEMP_DIR/$ASSET"
curl --fail --location --proto '=https' --tlsv1.2 "$REPOSITORY_URL/SHA256SUMS-${VERSION}" -o "$TEMP_DIR/SHA256SUMS-${VERSION}"
curl --fail --location --proto '=https' --tlsv1.2 "$REPOSITORY_URL/SHA256SUMS-${VERSION}.minisig" -o "$TEMP_DIR/SHA256SUMS-${VERSION}.minisig"

minisign -Vm "$TEMP_DIR/SHA256SUMS-${VERSION}" -x "$TEMP_DIR/SHA256SUMS-${VERSION}.minisig" -P "$MINISIGN_PUBLIC_KEY"
(cd "$TEMP_DIR" && grep "  $ASSET\$" "SHA256SUMS-${VERSION}" | sha256sum -c -)
chmod 0755 "$TEMP_DIR/$ASSET"
exec "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/install-v2.sh" "$TEMP_DIR/$ASSET"
