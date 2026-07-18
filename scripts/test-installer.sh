#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPOSITORY_DIR=$(CDPATH='' cd -- "$SCRIPT_DIR/.." && pwd)
IMAGE=debian:11-slim@sha256:cba95a21c96c1f5fc2470081829363eed57706634f7dc26e8c6712934303d57a

for test_case in active upgrade-preserves invalid pairing-window legacy-disable; do
  docker run --rm \
    -v "$REPOSITORY_DIR:/repo:ro" \
    "$IMAGE" \
    bash /repo/scripts/testdata/install-v2-case.sh "$test_case"
done

echo "Installer migration tests passed."
