#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-2.2.0}"
OUTPUT_DIR="${OUTPUT_DIR:-dist}"
SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPOSITORY_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd)

[[ "$VERSION" =~ ^[0-9A-Za-z][0-9A-Za-z._-]{0,63}$ ]] || {
  echo "Invalid VERSION." >&2
  exit 1
}

mkdir -p "$OUTPUT_DIR"
OUTPUT_DIR=$(CDPATH='' cd -- "$OUTPUT_DIR" && pwd)
PACKAGE_TEMP=$(mktemp -d)
trap 'rm -rf "$PACKAGE_TEMP"' EXIT

build() {
  local goarch="$1"
  local goarm="${2:-}"
  local platform
  if [ "$goarch" = arm ]; then
    platform="linux-armv${goarm}"
  else
    platform="linux-${goarch}"
  fi
  local binary="$OUTPUT_DIR/adsbpro-feeder-${VERSION}-${platform}"
  local package_dir="$PACKAGE_TEMP/$platform"
  local archive="$OUTPUT_DIR/adsbpro-feeder-${VERSION}-${platform}.tar.gz"

  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" GOARM="$goarm" \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$binary" ./cmd/adsbpro-feeder

  install -d -m 0755 "$package_dir/packaging"
  install -m 0755 "$binary" "$package_dir/adsbpro-feeder"
  install -m 0755 "$REPOSITORY_DIR/install-v2.sh" "$package_dir/install-v2.sh"
  install -m 0755 "$REPOSITORY_DIR/rollback-v2.sh" "$package_dir/rollback-v2.sh"
  install -m 0644 "$REPOSITORY_DIR/packaging/adsbpro-feeder.service" "$package_dir/packaging/adsbpro-feeder.service"
  COPYFILE_DISABLE=1 tar --no-xattrs -C "$package_dir" -czf "$archive" .
  rm -f "$binary"
}

build_windows() {
  local goarch="$1"
  local platform="windows-${goarch}"
  local binary="$OUTPUT_DIR/adsbpro-feeder-${VERSION}-${platform}.exe"
  local package_dir="$PACKAGE_TEMP/$platform"
  local archive="$OUTPUT_DIR/adsbpro-feeder-${VERSION}-${platform}.zip"

  CGO_ENABLED=0 GOOS=windows GOARCH="$goarch" \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$binary" ./cmd/adsbpro-feeder

  install -d -m 0755 "$package_dir"
  install -m 0755 "$binary" "$package_dir/adsbpro-feeder.exe"
  install -m 0644 "$REPOSITORY_DIR/install-windows.ps1" "$package_dir/install-windows.ps1"
  install -m 0644 "$REPOSITORY_DIR/rollback-windows.ps1" "$package_dir/rollback-windows.ps1"
  (
    cd "$package_dir"
    zip -q -X "$archive" adsbpro-feeder.exe install-windows.ps1 rollback-windows.ps1
  )
  rm -f "$binary"
}

cd "$REPOSITORY_DIR"
build amd64
build arm64
build arm 6
build arm 7
build_windows amd64
build_windows arm64

(
  cd "$OUTPUT_DIR"
  sha256sum adsbpro-feeder-"$VERSION"-*.tar.gz adsbpro-feeder-"$VERSION"-*.zip > "SHA256SUMS-${VERSION}"
)

if [ "${RELEASE:-false}" = true ]; then
  : "${RELEASE_SIGNING_KEY:?RELEASE_SIGNING_KEY is required for release artifacts}"
  OPENSSL_BIN="${OPENSSL_BIN:-openssl}"
  command -v "$OPENSSL_BIN" >/dev/null 2>&1 || { echo "openssl is required" >&2; exit 1; }
  "$OPENSSL_BIN" dgst -sha256 \
    -sign "$RELEASE_SIGNING_KEY" \
    -out "$OUTPUT_DIR/SHA256SUMS-${VERSION}.sig" \
    "$OUTPUT_DIR/SHA256SUMS-${VERSION}"
fi
