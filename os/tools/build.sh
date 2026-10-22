#!/bin/bash
set -euo pipefail
memory_kib=$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)
if (( memory_kib < 3584 * 1024 )); then
    echo "The OE toolchain build needs a Docker/Linux build VM with at least 4 GiB allocated." >&2
    echo "Use a build host with at least 4 GiB available. Cached work is retained." >&2
    exit 1
fi
# Metadata and BitBake use the same owner on first and incremental builds.
# setup.sh previously handed root-fetched repositories to builder, causing the
# next root fetch to fail Git's ownership check.
install -d -o builder -g builder /work/sources
chown -R builder:builder /work/sources
runuser -u builder -- python3 /src/os/tools/fetch.py
bash /src/os/tools/setup.sh
runuser -u builder -- bash /src/os/tools/bitbake.sh linux-fractal-cip
bash /src/os/tools/package.sh
