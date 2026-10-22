#!/usr/bin/env python3
"""Reproduce each reviewed patch body from its checksum-pinned stable update.

Download the upstream .xz files to a separate audit directory first. This check
does not fetch updates, change pins, or approve any new exclusions.
"""
import argparse
import gzip
import hashlib
import json
import lzma
from pathlib import Path
import re
from urllib.parse import urlparse


def verify(security, archives):
    manifest = json.loads((security/'kernel-security.json').read_text())
    for entry in manifest['patches']:
        upstream = archives/Path(urlparse(entry['upstream_url']).path).name
        original = upstream.read_bytes()
        if hashlib.sha256(original).hexdigest() != entry['upstream_sha256']:
            raise ValueError('upstream patch checksum mismatch: '+upstream.name)
        blocks = re.split(rb'(?=^diff --git )', lzma.decompress(original), flags=re.M)
        excluded = {('diff --git a/'+name+' b/'+name+'\n').encode()
                    for name in entry['excluded_files']}
        kept, seen = [], set()
        for block in blocks:
            if not block:
                continue
            header = block.splitlines(keepends=True)[0]
            if header in excluded:
                seen.add(header)
            else:
                kept.append(block)
        if seen != excluded:
            raise ValueError('declared exclusions do not match upstream patch')
        packed = (security/entry['file']).read_bytes()
        if hashlib.sha256(packed).hexdigest() != entry['sha256']:
            raise ValueError('derived patch checksum mismatch')
        derived = gzip.decompress(packed)
        start = derived.index(b'diff --git ')
        if derived[start:] != b''.join(kept):
            raise ValueError('derived patch has changes beyond declared exclusions')
    return len(manifest['patches'])


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('security', type=Path)
    parser.add_argument('archives', type=Path)
    args = parser.parse_args()
    print(f'{verify(args.security, args.archives)} kernel backports exactly reproduced')
