#!/usr/bin/env python3
"""Export a clean recursive Git checkout without Git metadata or local artifacts."""
import argparse
import gzip
import hashlib
import io
import json
import re
from pathlib import Path
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[1]


def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args])


def inventory(root, prefix='', repositories=None):
    if git(root, 'status', '--porcelain').strip():
        raise ValueError(f'{root}: source export requires a clean checkout')
    repositories[prefix or '.'] = git(root, 'rev-parse', 'HEAD').decode().strip()
    for entry in git(root, 'ls-files', '--stage', '-z').split(b'\0'):
        if not entry:
            continue
        meta, name = entry.split(b'\t', 1)
        mode, revision, stage = meta.decode().split()
        name = name.decode('utf-8')
        path = root / name
        if mode == '160000':
            if git(path, 'rev-parse', 'HEAD').decode().strip() != revision:
                raise ValueError('submodule does not match pinned revision: ' + name)
            yield from inventory(path, prefix + name + '/', repositories)
        elif mode in ('100644', '100755') and not path.is_symlink():
            # Export committed bytes, independent of Windows checkout line endings.
            data = git(root, 'cat-file', 'blob', revision)
            if re.search(rb'-----BEGIN (?:RSA |EC |OPENSSH |ENCRYPTED )?PRIVATE KEY-----', data):
                raise ValueError('private key marker in export: ' + name)
            yield prefix + name, data, int(mode[-3:], 8)
        else:
            raise ValueError('unresolved or unsupported file in export: ' + name)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    revisions = {}
    files = sorted(inventory(ROOT, repositories=revisions))
    manifest = dict(schema=1, repositories=revisions,
                    files={name: hashlib.sha256(data).hexdigest() for name, data, _ in files})
    files.append(('source-manifest.json', (json.dumps(manifest, indent=2) + '\n').encode(), 0o644))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open('xb') as output, gzip.GzipFile(fileobj=output, filename='', mtime=0) as zipped:
        with tarfile.open(fileobj=zipped, mode='w') as tar:
            for name, data, mode in files:
                info = tarfile.TarInfo(name)
                info.mode, info.size, info.mtime = mode, len(data), 0
                tar.addfile(info, io.BytesIO(data))
    print(json.dumps(dict(output=str(args.output), sha256=hashlib.sha256(args.output.read_bytes()).hexdigest(), repositories=revisions)))


if __name__ == '__main__':
    main()
