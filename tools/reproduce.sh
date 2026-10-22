#!/bin/bash
# Run inside the corresponding builder with its ordinary /src and /work mounts.
# Dependency toolchains are retained; appliance packages, rootfs and kernels are
# regenerated twice. This intentionally does not claim a clean-room toolchain build.
set -euo pipefail
stage=${1:?boot or os}
: "${FCR_BUILDER_ID:?Launch through remote-build so the immutable builder ID is recorded}"
case "$stage" in boot|os) ;; *) exit 2 ;; esac
export SOURCE_DATE_EPOCH=1792454400
mkdir -p /work/reproducibility /work/artifacts
rm -f /work/artifacts/reproducibility.json
python3 /src/tools/build-inputs.py "$stage" > /work/reproducibility/inputs.json
for pass in 1 2; do
    echo "REPRODUCIBILITY $stage pass $pass: regenerating appliance artifacts"
    if [[ "$stage" == boot ]]; then
        # Fixed absolute output directories; never touch downloads or keys.
        for target in /work/build/kernel-rommon /work/build/kernel-witness; do
            if [[ -d "$target" ]]; then
                test "$(realpath "$target")" = "$target"
                rm -rf -- "$target"
            fi
        done
        if [[ -d /work/build/rommon-root ]]; then
            make -C /work/sources/buildroot O=/work/build/rommon-root rommon-dirclean
        fi
    else
        bash /src/os/tools/setup.sh
        runuser -u builder -- bash /src/os/tools/bitbake.sh -c cleansstate fractal-init fractal-management fractal-kata fractal-container-tools fractal-system linux-fractal-cip
    fi
    bash "/src/$stage/tools/build.sh"
    if [[ "$stage" == os ]]; then
        # Retain intermediate evidence so any mismatch can be localized without
        # discarding either independent rootfs/kernel generation.
        mkdir -p "/work/reproducibility/components-$pass"
        deploy=/work/build/tmp/deploy/images/fractal-x86-64
        cp "$deploy/fractal-system-fractal-x86-64.cpio.gz" "/work/reproducibility/components-$pass/rootfs.cpio.gz"
        cp "$deploy/bzImage-initramfs-fractal-x86-64.bin" "/work/reproducibility/components-$pass/kernel.unsigned.efi"
    fi
    python3 /src/tools/build-inputs.py "$stage" > "/work/reproducibility/inputs-$pass.json"
    cmp /work/reproducibility/inputs.json "/work/reproducibility/inputs-$pass.json"
    if [[ "$stage" == boot ]]; then
        files=(rommon.unsigned.efi rommon.efi witness.efi system.bin)
    else
        files=(system.efi system.bin)
    fi
    (cd /work/artifacts && sha256sum "${files[@]}") > "/work/reproducibility/pass-$pass.sha256"
    if [[ "$pass" == 1 ]]; then
        mkdir -p /work/reproducibility/first
        for file in "${files[@]}"; do cp "/work/artifacts/$file" "/work/reproducibility/first/$file"; done
    else
        for file in "${files[@]}"; do cmp "/work/reproducibility/first/$file" "/work/artifacts/$file"; done
    fi
done
cmp /work/reproducibility/pass-1.sha256 /work/reproducibility/pass-2.sha256
python3 - "$stage" <<'PY'
import json, sys
from pathlib import Path
root = Path('/work/reproducibility')
result = {
    'stage': sys.argv[1], 'passed': True,
    'scope': 'Two forced appliance/rootfs/kernel rebuilds; dependency toolchain caches retained',
    'inputs': json.loads((root/'inputs.json').read_text()),
    'artifacts': dict((line.split()[1], line.split()[0]) for line in (root/'pass-2.sha256').read_text().splitlines()),
}
Path('/work/artifacts/reproducibility.json').write_text(json.dumps(result, indent=2)+'\n')
print('REPRODUCIBILITY_PASSED', sys.argv[1])
PY
