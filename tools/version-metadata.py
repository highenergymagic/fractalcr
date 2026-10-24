#!/usr/bin/env python3
"""Generate deterministic firmware branding; tag builds must match VERSION."""
import argparse
from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]

def main():
    p = argparse.ArgumentParser()
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--tag')
    a = p.parse_args()
    version = (ROOT/'VERSION').read_text().strip()
    if not re.fullmatch(r'\d+\.\d+\.\d+(?:-[a-zA-Z0-9.]+)?', version):
        p.error('invalid VERSION')
    if a.tag and a.tag not in ('v'+version, 'v'+version.removesuffix('.0')):
        p.error('release tag does not match VERSION')
    kind = 'DEVELOPMENT SOFTWARE' if version.endswith('-dev') else 'RELEASE CANDIDATE' if '-' in version else 'RELEASE SOFTWARE'
    a.output.write_text('#ifndef FCR_RELEASE_H\n#define FCR_RELEASE_H\n'
        f'#define FCR_VERSION "{version}"\n#define FCR_RELEASE_KIND "{kind}"\n'
        '#define FCR_SUPPORT "https://www.fractalmicro.org/techsupport"\n'
        '#define FCR_COPYRIGHT "Copyright (c) 2025-2026 by Fractal Microsystems"\n'
        '#endif\n')

if __name__ == '__main__':
    main()
