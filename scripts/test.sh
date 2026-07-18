#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPOSITORY_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd)
cd "$REPOSITORY_DIR"

shellcheck \
  radarview_setup.sh \
  install-v2.sh \
  rollback-v2.sh \
  scripts/build-release.sh \
  scripts/test-installer.sh \
  scripts/testdata/fake-systemctl \
  scripts/testdata/install-v2-case.sh \
  scripts/test.sh

bash -n \
  radarview_setup.sh \
  install-v2.sh \
  rollback-v2.sh \
  scripts/build-release.sh \
  scripts/test-installer.sh \
  scripts/testdata/fake-systemctl \
  scripts/testdata/install-v2-case.sh \
  scripts/test.sh

go test ./...

CURRENT_VERSION=2.2.1
CURRENT_RELEASE_DIR="$REPOSITORY_DIR/releases/v${CURRENT_VERSION}"
openssl dgst -sha256 \
  -verify packaging/release-signing-public.pem \
  -signature "$CURRENT_RELEASE_DIR/SHA256SUMS-${CURRENT_VERSION}.sig" \
  "$CURRENT_RELEASE_DIR/SHA256SUMS-${CURRENT_VERSION}"
(
  cd "$CURRENT_RELEASE_DIR"
  sha256sum -c "SHA256SUMS-${CURRENT_VERSION}"
)

RELEASE_TEST_DIR=$(mktemp -d)
trap 'rm -rf "$RELEASE_TEST_DIR"' EXIT
VERSION=2.2.1-test OUTPUT_DIR="$RELEASE_TEST_DIR" ./scripts/build-release.sh
(
  cd "$RELEASE_TEST_DIR"
  sha256sum -c SHA256SUMS-2.2.1-test
)

for platform in linux-amd64 linux-arm64 linux-armv6 linux-armv7; do
  archive="$RELEASE_TEST_DIR/adsbpro-feeder-2.2.1-test-${platform}.tar.gz"
  archive_listing=$(tar -tzf "$archive")
  for required_path in \
    ./adsbpro-feeder \
    ./install-v2.sh \
    ./rollback-v2.sh \
    ./packaging/adsbpro-feeder.service; do
    grep -Fxq "$required_path" <<< "$archive_listing"
  done
done

for platform in windows-amd64 windows-arm64; do
  archive="$RELEASE_TEST_DIR/adsbpro-feeder-2.2.1-test-${platform}.zip"
  archive_listing=$(unzip -Z1 "$archive")
  for required_path in \
    adsbpro-feeder.exe \
    install-windows.ps1 \
    rollback-windows.ps1; do
    grep -Fxq "$required_path" <<< "$archive_listing"
  done
done

if command -v docker >/dev/null 2>&1; then
  ./scripts/test-installer.sh
fi

echo "All feeder tests passed."
