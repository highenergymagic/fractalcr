#!/usr/bin/env python3
"""Extract regular files from a newc initramfs for offline scanning; never execute it."""
import argparse
import gzip
import hashlib
import json
import shutil
from pathlib import Path, PurePosixPath
import stat


def exact(stream, count):
    data = stream.read(count)
    if len(data) != count:
        raise ValueError('truncated cpio archive')
    return data


def extract(source, destination):
    destination.mkdir(parents=True, exist_ok=False)
    inventory = []
    hardlinks = {}
    total = 0
    with gzip.open(source, 'rb') as stream:
        while True:
            header = exact(stream, 110)
            if header[:6] != b'070701':
                raise ValueError('expected newc cpio')
            if any(byte not in b'0123456789abcdefABCDEF' for byte in header[6:]):
                raise ValueError('invalid hexadecimal cpio header')
            fields = [int(header[6+i*8:14+i*8], 16) for i in range(13)]
            mode, size, namesize = fields[1], fields[6], fields[11]
            total += size + namesize + 110
            if total > 8*1024**3 or len(inventory) >= 1000000:
                raise ValueError('cpio archive exceeds scan limits')
            if not 1 <= namesize <= 4096 or size > 4*1024**3:
                raise ValueError('cpio entry exceeds scan limits')
            raw = exact(stream, namesize)
            if raw[-1:] != b'\0' or b'\0' in raw[:-1]:
                raise ValueError('invalid cpio name')
            name = raw[:-1].decode('utf-8')
            exact(stream, (-(110+namesize)) % 4)
            if name == 'TRAILER!!!':
                break
            path = PurePosixPath(name)
            if path.is_absolute() or '..' in path.parts:
                raise ValueError('unsafe cpio path')
            target = destination / path
            record = dict(path=name, mode=oct(mode), size=size)
            if stat.S_ISREG(mode):
                target.parent.mkdir(parents=True, exist_ok=True)
                digest = hashlib.sha256()
                with target.open('xb') as output:
                    remaining = size
                    while remaining:
                        chunk = exact(stream, min(remaining, 1024*1024))
                        output.write(chunk)
                        digest.update(chunk)
                        remaining -= len(chunk)
                record['sha256'] = digest.hexdigest()
                target.chmod(mode & 0o777)
                if fields[4] > 1:
                    # newc carries hardlink contents in only one member, which
                    # may appear after its empty aliases. Resolve after parsing.
                    key = (fields[7], fields[8], fields[0])
                    hardlinks.setdefault(key, []).append((target, record))
            else:
                data = exact(stream, size)
                if stat.S_ISDIR(mode):
                    target.mkdir(parents=True, exist_ok=True)
                if stat.S_ISLNK(mode):
                    record['symlink'] = data.decode('utf-8')
                # Devices and links are inventory data only. The scanner must
                # not follow an image's absolute links into its own host.
            exact(stream, (-size) % 4)
            inventory.append(record)
    for group in hardlinks.values():
        populated = [(p, r) for p, r in group if r['size']]
        if len(populated) > 1:
            raise ValueError('multiple data members for cpio hardlink')
        if populated:
            original, source_record = populated[0]
            for target, record in group:
                if target != original:
                    target.chmod(0o600)
                    shutil.copyfile(original, target)
                    target.chmod(int(record['mode'], 8) & 0o777)
                    record.update(size=source_record['size'], sha256=source_record['sha256'])
                record['hardlink_group'] = source_record['path']
    return inventory


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source', type=Path)
    parser.add_argument('destination', type=Path)
    parser.add_argument('--inventory', required=True, type=Path)
    args = parser.parse_args()
    args.inventory.write_text(json.dumps(extract(args.source, args.destination), indent=2) + '\n')
