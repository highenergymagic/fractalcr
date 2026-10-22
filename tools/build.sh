#!/bin/bash
# Portable Linux entry point. All source is pinned before any compiler runs.
set -euo pipefail
src=$(cd "$(dirname "$0")/.." && pwd)
cd "$src"
stage=${1:-all}
action=${2:-All}
python3 -m unittest discover -s tools/tests
if [[ -d .git ]]; then
    if git submodule status --recursive | grep -Eq '^[-+U]'; then
        echo 'Component checkouts do not match integration pins. Run git submodule update --init --recursive.' >&2
        exit 1
    fi
fi
# For ordinary Docker group access: FCR_DOCKER_DIRECT=1 bash tools/build.sh
exec bash tools/remote-build.sh "$stage" "$action" "${FCR_SIGNING_MODE:-development}" "${FCR_TRUST_ID:-development}"
