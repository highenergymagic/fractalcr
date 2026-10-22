#!/usr/bin/env python3
"""Install only checksum-pinned audit tools into an explicit work directory."""
import argparse
import hashlib
import io
import json
from pathlib import Path
import tarfile
import urllib.request

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('destination', type=Path)
args = parser.parse_args()
args.destination.mkdir(parents=True, exist_ok=True)
pins = json.loads(Path(__file__).with_name('scanners.json').read_text())
for name, pin in pins.items():
    with urllib.request.urlopen(pin['url'], timeout=120) as response:
        data = response.read()
    if hashlib.sha256(data).hexdigest() != pin['sha256']:
        raise ValueError('scanner download checksum mismatch: ' + name)
    with tarfile.open(fileobj=io.BytesIO(data)) as archive:
        entry = archive.getmember(name)
        if not entry.isfile():
            raise ValueError('expected regular scanner executable')
        binary = args.destination / name
        binary.write_bytes(archive.extractfile(entry).read())
        binary.chmod(0o755)
    print(name, pin['version'], pin['sha256'])
