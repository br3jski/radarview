#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
VERSION="${1:?Usage: install-release.sh VERSION [installer options]}"
shift
exec "$SCRIPT_DIR/radarview_setup.sh" --version "$VERSION" "$@"
