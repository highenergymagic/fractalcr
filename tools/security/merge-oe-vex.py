#!/usr/bin/env python3
"""Include the boot kernel alongside the embedded rootfs's source identities."""
import argparse
import json
from pathlib import Path


def merge(rootfs, kernel):
    if not any(p['name'] == 'linux-fractal-cip' for p in kernel['package']):
        raise ValueError('kernel source metadata is missing')
    packages = {}
    for package in rootfs['package'] + kernel['package']:
        name = package['name']
        if name in packages and packages[name] != package:
            raise ValueError('conflicting source metadata: '+name)
        packages[name] = package
    return dict(version='1', package=[packages[n] for n in sorted(packages)])


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('rootfs', type=Path)
    parser.add_argument('kernel', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    result = merge(json.loads(args.rootfs.read_text()), json.loads(args.kernel.read_text()))
    args.output.write_text(json.dumps(result, indent=2)+'\n')
