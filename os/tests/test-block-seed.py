#!/usr/bin/env python3
"""The seed encoding must depend on bytes, never host allocation layout."""
import gzip
from pathlib import Path
import struct
import subprocess
import sys
import tempfile

packer = Path(__file__).resolve().parents[1] / 'meta-fractal/recipes-containers/fractal-blockstore/files/pack-sparse.py'
size = 32 * 1024 * 1024 + 13
with tempfile.TemporaryDirectory() as directory:
    root = Path(directory)
    expected = bytearray(size)
    for offset, data in ((1024, b'superblock'), (8191, b'cross-page'), (size-7, b'trailer')):
        expected[offset:offset+len(data)] = data
    for name in ('sparse', 'allocated'):
        path = root / name
        with path.open('wb') as target:
            if name == 'allocated':
                target.write(bytes(size))
            else:
                target.truncate(size)
            for offset, data in ((1024, b'superblock'), (8191, b'cross-page'), (size-7, b'trailer')):
                target.seek(offset)
                target.write(data)
        subprocess.run([sys.executable, str(packer), str(path), str(path)+'.gz'], check=True)
    assert (root/'sparse.gz').read_bytes() == (root/'allocated.gz').read_bytes()
    data = gzip.decompress((root/'sparse.gz').read_bytes())
    assert data[:8] == b'FCRBLK01' and struct.unpack_from('<Q', data, 8)[0] == size
    decoded = bytearray(size)
    pos, last = 16, 0
    while True:
        offset, length = struct.unpack_from('<QQ', data, pos)
        pos += 16
        if not length:
            assert offset == 0 and pos == len(data)
            break
        assert offset >= last and offset % 4096 == 0
        assert length == min(4096, size-offset)
        decoded[offset:offset+length] = data[pos:pos+length]
        last, pos = offset+length, pos+length
    assert decoded == expected
print('BLOCK_SEED_ALLOCATION_INDEPENDENT')
