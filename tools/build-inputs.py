#!/usr/bin/env python3
"""Record public build inputs; never read/export private signing material."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

stage = sys.argv[1]
assert stage in ('boot', 'os')
def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()
sources = {}
for directory in ('boot', 'os', 'service', 'tools', 'internal', 'cmd', 'vendor'):
    for path in sorted((Path('/src')/directory).rglob('*')):
        if path.is_file() and '.git' not in path.parts and '__pycache__' not in path.parts and path.suffix != '.pyc':
            sources[str(path.relative_to('/src'))] = sha(path)
sources['go.mod'] = sha(Path('/src/go.mod'))
sources['go.sum'] = sha(Path('/src/go.sum'))
sources['VERSION'] = sha(Path('/src/VERSION'))
keys = Path('/work/build/keys' if stage == 'boot' else '/boot-work/build/keys')
inputs = {
    'stage': stage, 'source_date_epoch': 1792454400,
    'builder_image_id': os.environ.get('FCR_BUILDER_ID', 'unrecorded'),
    'source_build': subprocess.check_output(['python3', '/src/tools/build-identity.py'], text=True).strip(),
    'sources': sources,
    'public_keys': {name: sha(keys/name) for name in ('firmware.crt', 'release.pub')},
    'builder_packages': subprocess.check_output(['dpkg-query', '-W', '-f', '${Package}=${Version}\n'], text=True).splitlines(),
}
if stage == 'os':
    inputs['rommon_sha256'] = sha(Path('/boot-work/artifacts/rommon.efi'))
print(json.dumps(inputs, sort_keys=True, indent=2))
