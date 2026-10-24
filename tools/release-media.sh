#!/bin/bash
# Run inside the pinned OS builder; source and signed inputs are read-only.
set -euo pipefail
src=${1:?source directory}
art=${2:?signed artifacts directory}
out=${3:?new output directory}
mkdir "$out"
version=$(cat "$src/VERSION")
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$ ]] || exit 2
python3 "$src/tools/verify-development-artifacts.py" --artifacts "$art" --policy "$src/boot/image-policy.json"
base="fractalcr-${version}-x86_64-devsigned"
for policy in required optional; do
    raw="$out/${base}-usb-${policy}.img"
    python3 "$src/os/tools/make-appliance.py" --efi "$art/rommon.efi" --system "$art/system.bin" \
        --output "$raw" --size-mib 16384 --firmware-policy "$policy"
    sfdisk --verify "$raw"
    if [[ "$policy" == optional ]]; then
        qemu-img convert -f raw -O qcow2 -o compat=1.1,lazy_refcounts=off "$raw" "$out/${base}-vps.qcow2"
        qemu-img check -f qcow2 "$out/${base}-vps.qcow2"
        qemu-img compare -f raw -F qcow2 "$raw" "$out/${base}-vps.qcow2"
    fi
    gzip -n -1 "$raw"
done
cp "$art/firmware.crt" "$out/firmware.crt"
cp "$art/release.pub" "$out/release.pub"
cp "$art/system.bin" "$out/system.bin"
cp "$art/rommon.efi" "$out/rommon.efi"
cp "$art/system.spdx.json" "$art/system.oe-vex.json" "$art/packages.manifest" "$out/"
cp "$art/system-licenses.tar.gz" "$art/buildroot-packages.json" "$art/boot-sources.json" "$art/oe-sources.json" "$art/kernel-security.json" "$out/"
dpkg-query -W -f='${Package}\t${Version}\t${Architecture}\n' > "$out/builder-packages.txt"
cp "$src/os/docs/releases.md" "$out/README.md"
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -C "$src/os/examples" -cf - nocloud | gzip -n > "$out/nocloud-examples.tar.gz"
python3 - "$src" "$out" <<'PY'
import hashlib, json, os, pathlib, subprocess, sys
src, out = map(pathlib.Path, sys.argv[1:])
files = {p.relative_to(out).as_posix(): hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
         for p in sorted(out.rglob('*')) if p.is_file()}
manifest = {'version': (src/'VERSION').read_text().strip(), 'signing': 'development',
            'builder_image':os.environ.get('FCR_BUILDER_ID','unrecorded'),
            'architecture':'x86_64', 'boot':'UEFI; serial 115200 8N1',
            'source_build': subprocess.check_output(['python3',str(src/'tools/build-identity.py')],text=True).strip(),
            'files':files}
(out/'release-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
PY
(cd "$out"; find . -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS)
