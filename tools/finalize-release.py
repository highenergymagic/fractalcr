#!/usr/bin/env python3
"""Bind qualification and the final published asset inventory to one source commit."""
import argparse
import gzip
import hashlib
import json
from pathlib import Path


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def finalize(directory, commit):
    cloud = json.loads((directory / 'qualification.json').read_text())
    usb = json.loads((directory / 'usb-qualification.json').read_text())
    management = json.loads((directory / 'management-qualification.json').read_text())
    if not all(report.get('passed') is True for report in (cloud, usb, management)):
        raise ValueError('All release qualification reports must pass')
    for name, field in (('system.bin', 'system_sha256'), ('rommon.efi', 'rommon_sha256')):
        if digest(directory / name) != management[field]:
            raise ValueError(f'{name} does not match management qualification')
    with gzip.open(directory / 'hv.qcow2.gz', 'rb') as stream:
        unpacked = hashlib.file_digest(stream, 'sha256').hexdigest()
    if unpacked != cloud['image_sha256']:
        raise ValueError('Hypervisor download does not match qualified image')
    for name in ('usb.img.gz', 'usb-secureboot.img.gz'):
        if digest(directory / name) != usb['images'][name]:
            raise ValueError(f'{name} does not match qualified image')
    manifest_path = directory / 'release-manifest.json'
    manifest = json.loads(manifest_path.read_text())
    manifest['source_commit'] = commit
    manifest['files'] = {p.name: digest(p) for p in sorted(directory.iterdir())
                         if p.is_file() and p.name not in ('release-manifest.json', 'SHA256SUMS')}
    manifest_path.write_text(json.dumps(manifest, indent=2) + '\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    parser.add_argument('--source-commit', required=True)
    args = parser.parse_args()
    finalize(args.directory, args.source_commit)
