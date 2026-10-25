#!/bin/bash
# Preserve the build's exit status while periodically logging runner resources.
set -euo pipefail
src=$(cd "$(dirname "$0")/.." && pwd)
python3 -u "$src/tools/ci-resource-monitor.py" &
monitor=$!
cleanup() {
    kill "$monitor" 2>/dev/null || true
    wait "$monitor" 2>/dev/null || true
}
trap cleanup EXIT
bash "$src/tools/build.sh" "$@"
