#!/usr/bin/env python3
"""Snapshot development sources with LF text, including uncommitted edits.

Release builders should use source-archive.py's clean, committed source export.
This helper implements the workspace's text=auto/eol=lf policy for Windows
development builds while preserving binary bytes and excluding local secrets.
"""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import re
import tarfile
from archive_policy import check_private_keys

ROOT = Path(__file__).resolve().parents[1]
INPUTS = ('boot', 'os', 'service', 'tools', 'internal', 'cmd', 'pkg', 'vendor',
          'go.mod', 'go.sum', 'VERSION', '.dockerignore', 'README.md', 'LICENSE')
BINARY = {'.png', '.jpg', '.efi', '.bin'}


def normalized(path, archive_name=None):
    data = path.read_bytes()
    if path.suffix not in BINARY and b'\0' not in data[:8000]:
        data = data.replace(b'\r\n', b'\n')
    check_private_keys(archive_name or path.name, data)
    return data


def files(root):
    for name in INPUTS:
        base = root / name
        for path in sorted(base.rglob('*')) if base.is_dir() else [base]:
            relative = path.relative_to(root)
            if any(p in ('.git', '__pycache__') for p in relative.parts) or path.suffix == '.pyc':
                continue
            if path.is_symlink() or (hasattr(path, 'is_junction') and path.is_junction()):
                raise ValueError('links are not permitted in build snapshots: ' + str(relative))
            if path.is_file():
                if path.suffix == '.key' or path.name.endswith('.encrypted.pem'):
                    raise ValueError('private key filename in build snapshot: ' + str(relative))
                yield relative.as_posix(), normalized(path, relative.as_posix())


def create(root, output, public=None):
    entries = list(files(root))
    if public:
        expected = {'firmware.crt', 'release.pub', 'trust.json'}
        if {p.name for p in public.iterdir()} != expected:
            raise ValueError('public trust must contain exactly three public files')
        for name in sorted(expected):
            path = public / name
            if path.is_symlink() or not path.is_file():
                raise ValueError('public trust must contain regular files')
            entries.append(('release-trust/' + name, normalized(path, 'release-trust/' + name)))
    manifest = dict(schema=1, kind='development-workspace',
                    files={name: hashlib.sha256(data).hexdigest() for name, data in entries})
    entries.append(('workspace-manifest.json', (json.dumps(manifest, indent=2)+'\n').encode()))
    with output.open('xb') as raw, gzip.GzipFile(fileobj=raw, filename='', mode='wb', mtime=0) as zipped:
        with tarfile.open(fileobj=zipped, mode='w') as archive:
            for name, data in sorted(entries):
                info = tarfile.TarInfo(name)
                info.size, info.mode, info.mtime = len(data), 0o755 if Path(name).suffix in ('.sh', '.py') else 0o644, 0
                archive.addfile(info, io.BytesIO(data))


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--public-trust', type=Path)
    args = parser.parse_args()
    create(ROOT, args.output, args.public_trust)
