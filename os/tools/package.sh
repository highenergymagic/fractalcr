#!/bin/bash
set -euo pipefail
cd /work
deploy=build/tmp/deploy/images/fractal-x86-64
kernel="$deploy/bzImage-initramfs-fractal-x86-64.bin"
test -f "$kernel" || { echo "Bundled kernel missing: $kernel"; ls -l "$deploy"; exit 1; }
python3 /src/os/tools/verify-bundled-image.py --kernel "$kernel" \
    --initramfs "$deploy/fractal-system-fractal-x86-64.cpio.gz" \
    --expected-build "$(python3 /src/tools/build-identity.py)"
mkdir -p artifacts
cp /src/boot/kernel/security/kernel-security.json artifacts/kernel-security.json
spdx="$deploy/fractal-system-fractal-x86-64.spdx.json"
test -s "$spdx" || { echo "Image SPDX missing; build fractal-system before packaging." >&2; exit 1; }
cp "$spdx" artifacts/system.spdx.json
vex="$deploy/fractal-system-fractal-x86-64.vex.json"
test -s "$vex" || { echo "Image VEX/source advisory metadata missing." >&2; exit 1; }
python3 /src/tools/security/merge-oe-vex.py "$vex" \
    build/tmp/deploy/cve/linux-fractal-cip_cve.json artifacts/system.oe-vex.json
if [[ "${FCR_SIGNING_MODE:-development}" == external ]]; then
    test -f /boot-work/build/keys/trust.json
    test ! -e /boot-work/build/keys/firmware.key
    test ! -e /boot-work/build/keys/release.key
    cp "$kernel" artifacts/system.unsigned.efi
    cp /boot-work/build/keys/{trust.json,firmware.crt,release.pub} artifacts/
    cp "$deploy"/fractal-system-fractal-x86-64.manifest artifacts/packages.manifest
    python3 /src/tools/signing-request.py --artifacts /work/artifacts --trust /boot-work/build/keys --stage os
    echo 'OS_UNSIGNED_IMAGE_BUILT: offline signatures required before boot or distribution'
    exit 0
fi
bash /src/boot/tools/sign-pe.sh --key /boot-work/build/keys/firmware.key --cert /boot-work/build/keys/firmware.crt \
    --output artifacts/system.efi "$kernel"
python3 /src/boot/tools/sign-image.py --key /boot-work/build/keys/release.key \
    --kernel artifacts/system.efi --output artifacts/system.bin
cp /boot-work/artifacts/rommon.efi /boot-work/artifacts/firmware.crt \
    /boot-work/artifacts/OVMF_CODE_4M.secboot.fd /boot-work/artifacts/OVMF_VARS-trusted.fd artifacts/
cp /boot-work/build/keys/release.pub artifacts/release.pub
cp /boot-work/artifacts/buildroot-packages.json /boot-work/artifacts/boot-sources.json artifacts/
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner \
    -C build/tmp/deploy -cf - licenses | gzip -n > artifacts/system-licenses.tar.gz
cp /src/os/sources.json artifacts/oe-sources.json
cp /usr/share/OVMF/OVMF_VARS_4M.fd artifacts/OVMF_VARS-unenrolled.fd
cp "$deploy"/fractal-system-fractal-x86-64.manifest artifacts/packages.manifest
(cd artifacts && sha256sum rommon.efi system.bin > SHA256SUMS)
echo FRACTALCR_OS_IMAGES_BUILT
