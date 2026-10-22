#!/usr/bin/env python3
"""Verify Buildroot's clearsigned checksum with the pinned maintainer key."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
FINGERPRINT = '18C7DF2819C1733D822D599EA500D6EE9CB0E540'


def verify(version, archive):
    if not re.fullmatch(r'2025\.02\.\d+', version):
        raise ValueError('only the selected Buildroot LTS line is accepted')
    name = f'buildroot-{version}.tar.xz'
    url = f'https://buildroot.org/downloads/{name}.sign'
    with urllib.request.urlopen(url, timeout=60) as response:
        signature = response.read(16385)
    if len(signature) > 16384:
        raise ValueError('release signature exceeds size limit')
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        root.chmod(0o700)
        signed, clear = root / 'release.asc', root / 'verified.txt'
        signed.write_bytes(signature)
        gpg = ['gpg', '--batch', '--no-options', '--homedir', str(root)]
        subprocess.run([*gpg, '--import', str(ROOT/'boot/upstream-trust/buildroot-maintainer.asc')], check=True, capture_output=True)
        checked = subprocess.run([*gpg, '--status-fd', '1', '--output', str(clear), '--decrypt', str(signed)], check=True, capture_output=True, text=True)
        signatures = re.findall(r'^\[GNUPG:\] VALIDSIG ([0-9A-F]+) ', checked.stdout, re.M)
        if signatures != [FINGERPRINT]:
            raise ValueError('release signature does not use the pinned maintainer key')
        matches = re.findall(r'^SHA256: ([a-f0-9]{64})  ' + re.escape(name) + r'$', clear.read_text(), re.M)
        with archive.open('rb') as source:
            actual = hashlib.file_digest(source, 'sha256').hexdigest()
        if matches != [actual]:
            raise ValueError('archive does not match authenticated release checksum')
        return dict(version=version, sha256=actual, signer=FINGERPRINT, signature_url=url)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--version', required=True)
    parser.add_argument('--archive', type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(verify(args.version, args.archive), indent=2))
