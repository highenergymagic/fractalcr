#!/usr/bin/env python3
"""Apply reviewed native-API overrides after go mod vendor; fail on upstream drift."""
import hashlib
import json
from pathlib import Path


def apply(root):
    root = root.resolve()
    base = root / 'tools/vendor-api'
    pending = []
    for entry in json.loads((base / 'manifest.json').read_text()):
        target = (root / entry['target']).resolve()
        source = (base / entry['source']).resolve()
        if not target.is_relative_to(root / 'vendor') or not source.is_relative_to(base):
            raise ValueError('Invalid native API override path')
        replacement = source.read_bytes().replace(b'\r\n', b'\n')
        current = target.read_bytes().replace(b'\r\n', b'\n')
        if current == replacement:
            continue
        if hashlib.sha256(current).hexdigest() != entry['upstream_sha256']:
            raise ValueError('Upstream API changed; review required: ' + entry['target'])
        pending.append((target, replacement))
    # Validate every upstream input before changing any generated source.
    for target, replacement in pending:
        target.write_bytes(replacement)


if __name__ == '__main__':
    apply(Path(__file__).resolve().parents[1])
