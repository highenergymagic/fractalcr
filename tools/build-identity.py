#!/usr/bin/env python3
"""Stable identity of OS build inputs, independent of checkout path and mtimes."""
import hashlib
from pathlib import Path

root = Path(__file__).resolve().parents[1]
digest = hashlib.sha256()
for name in ('internal', 'cmd', 'pkg', 'vendor', 'service', 'go.mod', 'go.sum', 'VERSION',
             'os/meta-fractal', 'os/sources.json', 'os/tools', 'tools/build-identity.py'):
    base = root / name
    for path in sorted(base.rglob('*'), key=lambda p: p.relative_to(root).as_posix()) if base.is_dir() else [base]:
        if not path.is_file() or any(p in ('.git', '__pycache__') for p in path.parts):
            continue
        data = path.read_bytes()
        if b'\0' not in data[:8000]:
            data = data.replace(b'\r\n', b'\n')
        digest.update(path.relative_to(root).as_posix().encode() + b'\0')
        digest.update(len(data).to_bytes(8, 'big'))
        digest.update(data)
print(digest.hexdigest())
