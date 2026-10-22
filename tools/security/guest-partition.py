#!/usr/bin/env python3
"""Copy the sole Linux MBR partition from a pinned Kata image without mounting."""
import argparse
from pathlib import Path
import struct


def extract(source, destination):
    length = source.stat().st_size
    with source.open('rb') as image:
        mbr = image.read(512)
        if len(mbr) != 512 or mbr[510:] != b'\x55\xaa':
            raise ValueError('expected MBR guest image')
        partitions = [struct.unpack('<B3sB3sII', mbr[446+i*16:462+i*16]) for i in range(4)]
        used = [p for p in partitions if p[2] != 0]
        if len(used) != 1 or used[0][2] != 0x83:
            raise ValueError('expected exactly one Linux primary partition')
        offset, size = used[0][4]*512, used[0][5]*512
        if offset < 512 or size < 2048 or offset+size > length:
            raise ValueError('guest partition outside image')
        image.seek(offset+1024+56)
        if image.read(2) != b'\x53\xef':
            raise ValueError('expected ext filesystem in guest partition')
        image.seek(offset)
        with destination.open('xb') as output:
            while size:
                data = image.read(min(size, 1024*1024))
                if not data:
                    raise ValueError('truncated guest partition')
                output.write(data)
                size -= len(data)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('destination', type=Path)
    args = parser.parse_args()
    extract(args.source, args.destination)
