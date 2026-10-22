#!/usr/bin/env python3
"""Finish a comparison interrupted ONLY by the unsigned UKI timestamp bug.

Keep both forced-build inputs intact. Verify their recorded hashes and require
every compiled source/configuration/builder/key input to be unchanged. Apply the
final packaging correction to both outputs and compare the complete results.
"""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess

root = Path('/work/reproducibility')
original = json.loads((root/'inputs.json').read_text())
current = json.loads(subprocess.check_output(['python3', '/src/tools/build-inputs.py', 'boot'], text=True))
assert original['builder_image_id'] != 'unrecorded'
for key in original:
    if key != 'sources':
        assert original[key] == current[key], f'Changed build input: {key}'
allowed = {'boot/tools/package.sh', 'boot/tools/normalize-pe.py', 'tools/finish-boot-reproduction.py'}
changed = {name for name in original['sources'].keys() | current['sources'].keys()
           if original['sources'].get(name) != current['sources'].get(name)}
assert changed <= allowed, f'Compiled or unrelated inputs changed: {sorted(changed-allowed)}'
def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()
files = ('rommon.unsigned.efi', 'rommon.efi', 'witness.efi', 'system.bin')
for attempt in (1, 2):
    source = root/'first' if attempt == 1 else Path('/work/artifacts')
    expected = {line.split()[1]: line.split()[0]
                for line in (root/f'pass-{attempt}.sha256').read_text().splitlines()}
    target = root/f'final-pass-{attempt}'
    target.mkdir(exist_ok=True)
    for name in files:
        assert digest(source/name) == expected[name], f'Original pass {attempt} changed: {name}'
        shutil.copyfile(source/name, target/name)
    subprocess.run(['python3', '/src/boot/tools/normalize-pe.py', str(target/'rommon.unsigned.efi')], check=True)
    subprocess.run(['bash', '/src/boot/tools/sign-pe.sh', '--key', '/work/build/keys/firmware.key',
                    '--cert', '/work/build/keys/firmware.crt', '--output', str(target/'rommon.efi'),
                    str(target/'rommon.unsigned.efi')], check=True)
hashes = {}
for name in files:
    first, second = root/'final-pass-1'/name, root/'final-pass-2'/name
    assert first.read_bytes() == second.read_bytes(), f'Final image differs: {name}'
    hashes[name] = digest(second)
subprocess.run(['sbverify', '--cert', '/work/build/keys/firmware.crt', str(root/'final-pass-2/rommon.efi')], check=True)
for name in files:
    shutil.copyfile(root/'final-pass-2'/name, Path('/work/artifacts')/name)
Path('/work/artifacts/SHA256SUMS').write_text(''.join(f'{hashes[name]}  {name}\n' for name in ('rommon.efi', 'system.bin')))
result = {
    'stage': 'boot', 'passed': True,
    'scope': 'Two forced appliance/rootfs/kernel rebuilds, each repackaged with the final COFF timestamp normalization; dependency toolchain caches retained',
    'inputs': current, 'artifacts': hashes,
    'packaging_correction': {'changed_source_files': sorted(changed), 'original_build_inputs': original,
                             'original_hashes': {str(n): (root/f'pass-{n}.sha256').read_text() for n in (1, 2)}},
}
Path('/work/artifacts/reproducibility.json').write_text(json.dumps(result, indent=2)+'\n')
print('REPRODUCIBILITY_PASSED boot: both forced builds match after final packaging')
