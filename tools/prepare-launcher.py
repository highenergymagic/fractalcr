#!/usr/bin/env python3
"""Prepare Windows VM templates from signed artifacts, without user data or tests.

Run on a Linux build/test host with mtools, dosfstools and btrfs-progs. The output
directory must be new. Copy these templates into the launcher's artifact directory;
run.ps1 -UpdateImage replaces flash only and retains the interactive datastore.
"""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--artifacts', type=Path, required=True)
parser.add_argument('--output', type=Path, required=True)
args = parser.parse_args()
source, output = args.artifacts.resolve(), args.output.resolve()
names = ('system.bin', 'rommon.efi', 'firmware.crt', 'OVMF_CODE_4M.secboot.fd',
         'OVMF_VARS-trusted.fd', 'OVMF_VARS-unenrolled.fd')
for name in names:
    if not (source / name).is_file():
        parser.error('missing artifact: ' + name)
output.mkdir(parents=True, exist_ok=False)
for name in names:
    shutil.copyfile(source / name, output / name)
root = Path(__file__).resolve().parents[1]
flash = output / 'flash-os-containers.img'
subprocess.run(['python3', str(root / 'boot/tools/make-flash.py'),
                '--efi', str(source / 'rommon.efi'), '--system', str(source / 'system.bin'),
                '--output', str(flash), '--size-mib', '1024', '--no-tampered-fixture'], check=True)
optional = output / 'flash-os-optional-firmware.img'
shutil.copyfile(flash, optional)
body = (b'BOOT=bootflash:system.bin\nCONFREG=0x2102\nIP_ADDRESS=\n'
        b'IP_SUBNET_MASK=\nDEFAULT_GATEWAY=\nTFTP_SERVER=\nTFTP_FILE=\n'
        b'ETHER_PORT=0\nSECURE_BOOT=optional\n')
with tempfile.TemporaryDirectory(dir=output) as tmp:
    env = Path(tmp) / 'rommon.env1'
    env.write_bytes(b'FCR-ENV-1 1 ' + hashlib.sha256(body).hexdigest().encode() + b'\n' + body)
    subprocess.run(['mcopy', '-i', str(optional) + '@@1048576', str(env), '::/rommon.env1'], check=True)
data = output / 'data-os-containers.img'
with data.open('xb') as stream:
    stream.truncate(4 * 1024**3)
subprocess.run(['mkfs.btrfs', '-L', 'FRDATA', '-d', 'single', '-m', 'dup', str(data)], check=True)
manifest = {'schema': 1, 'datastore': 'empty; serial setup on first boot', 'files': {}}
for path in sorted(output.iterdir()):
    with path.open('rb') as stream:
        manifest['files'][path.name] = hashlib.file_digest(stream, 'sha256').hexdigest()
(output / 'launcher-manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
print(output)
