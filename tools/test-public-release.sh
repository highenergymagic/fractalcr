#!/bin/bash
# End-to-end public-only builder and separate disposable offline signing station.
set -euo pipefail
src=$(cd "$(dirname "$0")/.." && pwd)
evidence=${1:?Supply a new evidence directory}
mkdir -p "$(dirname "$evidence")"
mkdir "$evidence"
evidence=$(realpath "$evidence")
docker() { doas -n docker "$@"; }
identity="fractalcr-public-chain-$(date -u +%Y%m%dT%H%M%S)-$$"
station="$identity-signer"
volume="$identity-build"
docker volume create "$volume" >/dev/null
docker run -d --name "$station" --network none \
    --tmpfs /offline:rw,mode=0700,size=256m \
    --mount "type=bind,source=$src,target=/src,readonly" \
    --mount "type=bind,source=$evidence,target=/evidence" \
    fractalcr-boot-builder sleep 3600 >/dev/null
trap 'docker rm -f "$station" >/dev/null' EXIT
docker exec "$station" python3 -c 'from pathlib import Path; p=Path("/offline/passphrase"); p.write_text("disposable-public-chain-test-only"); p.chmod(0o600)'
docker exec "$station" python3 /src/tools/release.py init --vault /offline/vault --name public-chain-test --password-file /offline/passphrase
mkdir "$evidence/public"
docker exec "$station" tar -C /offline/vault/public -cf - firmware.crt release.pub trust.json | tar -xf - -C "$evidence/public"
# Reuse public sources and compiler/kernel caches, explicitly excluding keys.
# The separate builder never mounts the signing station's tmpfs.
docker run --rm --network none \
    --mount type=volume,source=fractalcr-boot-work,target=/cache,readonly \
    --mount "type=volume,source=$volume,target=/work" \
    fractalcr-boot-builder bash -c 'set -eu; mkdir -p /work/build; cp -a /cache/sources /cache/downloads /work/; cp -a /cache/build/rommon-root /cache/build/kernel-rommon /cache/build/kernel-witness /work/build/; test ! -e /work/build/keys'
docker run --rm --network none \
    --mount "type=volume,source=$volume,target=/work" \
    --mount "type=bind,source=$src,target=/src,readonly" \
    --mount "type=bind,source=$evidence/public,target=/release-public,readonly" \
    -e FCR_SIGNING_MODE=external -e FCR_TRUST_DIR=/release-public -e JOBS=2 \
    fractalcr-boot-builder bash /src/boot/tools/build.sh > "$evidence/public-build.log" 2>&1
docker run --rm --network none \
    --mount "type=volume,source=$volume,target=/work,readonly" \
    --mount "type=bind,source=$evidence,target=/evidence" \
    fractalcr-boot-builder bash -c 'set -eu; test ! -e /work/build/keys/firmware.key; test ! -e /work/build/keys/release.key; mkdir /evidence/artifacts; cp /work/artifacts/{rommon.unsigned.efi,witness.unsigned.efi,signing-request.json} /evidence/artifacts/'
for kind in rommon system; do
    unsigned=rommon.unsigned.efi
    signed=rommon.efi
    if [[ "$kind" == system ]]; then unsigned=witness.unsigned.efi; signed=system.bin; fi
    hash=$(sha256sum "$evidence/artifacts/$unsigned" | cut -d' ' -f1)
    docker exec "$station" python3 /src/tools/release.py sign --vault /offline/vault --password-file /offline/passphrase --kind "$kind" --input "/evidence/artifacts/$unsigned" --expected-sha256 "$hash" --output "/evidence/artifacts/$signed"
    docker exec "$station" python3 /src/tools/release.py verify --public /evidence/public --kind "$kind" --input "/evidence/artifacts/$signed" --unsigned "/evidence/artifacts/$unsigned"
done
docker exec "$station" python3 /src/boot/tests/test-public-chain.py
echo "Public-only build and offline-signed boot evidence: $evidence"
echo "Public build cache retained in volume: $volume"
