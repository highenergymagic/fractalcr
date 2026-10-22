#!/bin/bash
# Run on carbon from a source checkout. Only disposable keys are generated.
set -euo pipefail
src=$(cd "$(dirname "$0")/.." && pwd)
evidence=${1:?Supply a new evidence directory}
mkdir -p "$(dirname "$evidence")"
mkdir "$evidence"
evidence=$(realpath "$evidence")
docker() { doas -n docker "$@"; }
docker build --build-arg "BOOT_BUILDER=$(docker image inspect --format '{{.Id}}' fractalcr-boot-builder)" \
    -t fractalcr-release-test -f "$src/boot/Dockerfile.release-test" "$src"
image=$(docker image inspect --format '{{.Id}}' fractalcr-release-test)
printf '%s\n' "$image" > "$evidence/builder-image-id.txt"
docker run --rm --name fractalcr-release-tests --network none \
    --tmpfs /offline:rw,mode=0700,size=128m --tmpfs /work:rw,mode=0755,size=128m \
    --mount "type=bind,source=$src,target=/src,readonly" \
    --mount type=volume,source=fractalcr-boot-work,target=/boot-work,readonly \
    --mount "type=bind,source=$evidence,target=/evidence" \
    "$image" python3 /src/boot/tests/test-release.py --qemu-rotation
echo "Release qualification evidence: $evidence"
