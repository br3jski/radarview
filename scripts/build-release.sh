#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-2.0.0}"
OUTPUT_DIR="${OUTPUT_DIR:-dist}"
mkdir -p "$OUTPUT_DIR"

build() {
  local goarch="$1"
  local goarm="${2:-}"
  local suffix="linux-${goarch}${goarm:+v$goarm}"
  CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" GOARM="$goarm" \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o "$OUTPUT_DIR/adsbpro-feeder-${VERSION}-${suffix}" ./cmd/adsbpro-feeder
}

build amd64
build arm64
build arm 6
build arm 7

(cd "$OUTPUT_DIR" && sha256sum adsbpro-feeder-* > "SHA256SUMS-${VERSION}")
if [ "${RELEASE:-false}" = true ]; then
  : "${MINISIGN_SECRET_KEY:?MINISIGN_SECRET_KEY is required for release artifacts}"
  command -v minisign >/dev/null
  minisign -Sm "$OUTPUT_DIR/SHA256SUMS-${VERSION}" -s "$MINISIGN_SECRET_KEY"
fi
