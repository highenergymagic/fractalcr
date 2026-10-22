#!/usr/bin/env python3
"""Export OE's recorded source identities for an additional advisory scan.

Keep the original VEX metadata alongside the report: upstream versions alone
cannot account for distribution backports or compile-time feature exclusions.
"""
import argparse
import json
from pathlib import Path


def cpes(metadata):
    values = sorted({cpe for package in metadata['package'] for cpe in package['cpes']})
    if not values or any(not c.startswith('cpe:2.3:') or '\n' in c or '\r' in c for c in values):
        raise ValueError('missing or invalid OE source CPE identities')
    return values


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('metadata', type=Path)
    parser.add_argument('output', type=Path)
    args = parser.parse_args()
    args.output.write_text('\n'.join(cpes(json.loads(args.metadata.read_text())))+'\n')
