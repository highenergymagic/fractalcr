#!/usr/bin/env python3
"""Reject stale embedded userland before signing the appliance's gzip bzImage."""
import argparse
import gzip
import hashlib
from pathlib import Path
import re
import zlib


def cpio_file(archive, wanted):
    offset = 0
    found = None
    while offset + 110 <= len(archive):
        header = archive[offset:offset+110]
        if header[:6] != b'070701':
            raise ValueError('expected a newc initramfs')
        size = int(header[54:62], 16)
        namesize = int(header[94:102], 16)
        if namesize < 1 or offset+110+namesize > len(archive):
            raise ValueError('invalid cpio name')
        name = archive[offset+110:offset+110+namesize]
        if name[-1:] != b'\0':
            raise ValueError('unterminated cpio name')
        data_start = offset + ((110+namesize+3) & ~3)
        if data_start+size > len(archive):
            raise ValueError('truncated cpio entry')
        if name[:-1].removeprefix(b'./') == wanted:
            if found is not None or int(header[14:22], 16) & 0o170000 != 0o100000:
                raise ValueError('build identity must be one regular file')
            found = archive[data_start:data_start+size]
        if name == b'TRAILER!!!\0':
            if found is None:
                raise ValueError('initramfs has no build identity')
            return found
        offset = (data_start+size+3) & ~3
    raise ValueError('missing cpio trailer')


def verify(kernel, archive, expected):
    if not re.fullmatch(r'[0-9a-f]{64}', expected):
        raise ValueError('invalid expected source identity')
    if cpio_file(archive, b'etc/fractal/build-id') != (expected+'\n').encode():
        raise ValueError('root filesystem does not match source build')
    if not kernel.startswith(b'MZ'):
        raise ValueError('expected a PE bzImage')
    # This kernel uses gzip and embeds a prebuilt newc archive verbatim. Check
    # the complete archive, not merely a source-id string that could coexist
    # with stale executable files. No extraction or execution is involved.
    limit = len(archive) + 128*1024*1024
    offset = 0
    while True:
        offset = kernel.find(b'\x1f\x8b\x08', offset)
        if offset < 0:
            raise ValueError('kernel does not embed the current root filesystem')
        try:
            stream = zlib.decompressobj(31)
            payload = stream.decompress(kernel[offset:], limit)
            if stream.eof and archive in payload:
                return
        except zlib.error:
            pass
        offset += 1


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--kernel', type=Path, required=True)
    p.add_argument('--initramfs', type=Path, required=True)
    p.add_argument('--expected-build', required=True)
    a = p.parse_args()
    if a.kernel.stat().st_size > 512*1024*1024:
        p.error('kernel exceeds appliance image limit')
    kernel = a.kernel.read_bytes()
    with gzip.open(a.initramfs, 'rb') as f:
        archive = f.read(1536*1024*1024+1)
    if len(archive) > 1536*1024*1024:
        p.error('initramfs exceeds appliance root size limit')
    verify(kernel, archive, a.expected_build)
    print('BUNDLED_USERLAND_VERIFIED source='+a.expected_build+
          ' initramfs_sha256='+hashlib.sha256(archive).hexdigest())


if __name__ == '__main__':
    main()
