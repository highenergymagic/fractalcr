#!/bin/bash
# Run in a disposable Linux audit container. SOURCE is a built initramfs cpio.gz.
set -euo pipefail
export SYFT_GOLANG_CAPTURE_SYMBOLS=all
source_image=${1:?rootfs.cpio.gz}
output=${2:?new audit output directory}
oe_metadata=${3:?system.oe-vex.json from the same image build}
mkdir "$output"
output=$(realpath "$output")
src=$(cd "$(dirname "$0")/../.." && pwd)
python3 "$src/tools/security/install-scanners.py" "$output/tools"
python3 "$src/tools/security/extract-initramfs.py" "$source_image" "$output/rootfs" --inventory "$output/files.json"
sha256sum "$source_image" > "$output/rootfs.sha256"
"$output/tools/syft" scan "dir:$output/rootfs" -o "syft-json=$output/host.syft.json" -o "spdx-json=$output/host.spdx.json"
"$output/tools/grype" "sbom:$output/host.syft.json" -o json > "$output/host.grype.json"
# Embedded rootfs images omit package-manager databases. OE records source
# versions/CPE identities and its explicit patch dispositions before stripping.
cp "$oe_metadata" "$output/system.oe-vex.json"
python3 "$src/tools/security/oe-cpes.py" "$oe_metadata" "$output/oe.cpes"
"$output/tools/grype" "cpes:$output/oe.cpes" -o json > "$output/oe.grype.json"
# Scan the nested guest separately. Upstream may strip package/distro metadata;
# report that coverage gap instead of treating zero matches as a clean guest.
guest="$output/rootfs/opt/kata/share/kata-containers/kata-containers.img"
if [[ -f "$guest" ]]; then
    mkdir "$output/guest"
    python3 "$src/tools/security/guest-partition.py" "$guest" "$output/guest.ext4"
    debugfs -R "rdump / $output/guest" "$output/guest.ext4" > "$output/guest-extraction.log" 2>&1
    test -d "$output/guest/usr"
    "$output/tools/syft" scan "dir:$output/guest" -o "syft-json=$output/guest.syft.json" -o "spdx-json=$output/guest.spdx.json"
    # Stripped Kata images omit os-release. Preserve the original inventory and
    # record exactly which ELF notes support selecting the Azure Linux feed.
    distro=$(python3 "$src/tools/security/guest-distro.py" "$output/guest.syft.json" "$output/guest-distro-evidence.json")
    distro_args=()
    if [[ -n "$distro" ]]; then distro_args=(--distro "$distro"); fi
    "$output/tools/grype" "sbom:$output/guest.syft.json" "${distro_args[@]}" -o json > "$output/guest.grype.json"
fi
"$output/tools/grype" db status -o json > "$output/database.json"
python3 "$src/tools/security/summarize.py" "$output"
echo 'Inventory and vulnerability reports generated; findings require disposition.'
