#!/usr/bin/env python3
"""Fetch exact OE commits; reject dirty caches and unexpected origins."""
import argparse
import concurrent.futures
import json
from pathlib import Path
import re
import subprocess


def fetch(repo, sources):
    if not re.fullmatch(r'[a-z][a-z0-9-]+', repo['name']) or not re.fullmatch(r'[a-f0-9]{40}', repo['revision']):
        raise ValueError('invalid repository pin')
    path = sources / repo['name']
    path.mkdir(parents=True, exist_ok=True)

    def git(*args, check=True):
        return subprocess.run(['git', '-C', str(path), *args], check=check,
                              text=True, capture_output=True)

    if not (path / '.git').exists():
        if list(path.iterdir()):
            raise ValueError(f'{path}: nonempty cache is not a Git checkout')
        git('init', '-q')
        git('remote', 'add', 'origin', repo['url'])
    if git('remote', 'get-url', 'origin').stdout.strip() != repo['url']:
        raise ValueError(f'{path}: origin differs from the lockfile')
    # setup.sh creates this exact link. Remove only this generated file before
    # the cleanliness check; setup.sh recreates it after fetching succeeds.
    link = path / 'bitbake'
    if repo['name'] == 'openembedded-core' and link.is_symlink() and link.readlink() == Path('/work/sources/bitbake'):
        link.unlink()
    if git('status', '--porcelain', '--untracked-files=all').stdout.strip():
        raise ValueError(f'{path}: dirty source cache; preserve edits and use a fresh cache')
    if git('rev-parse', 'HEAD', check=False).stdout.strip() != repo['revision']:
        git('fetch', '--depth=1', 'origin', repo['revision'])
        git('checkout', '--detach', repo['revision'])
    if git('rev-parse', 'HEAD').stdout.strip() != repo['revision']:
        raise ValueError('checkout differs from pin')
    print(f"PINNED {repo['name']} {repo['revision']}", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, default=Path('/src/os/sources.json'))
    parser.add_argument('--sources', type=Path, default=Path('/work/sources'))
    args = parser.parse_args()
    manifest = json.loads(args.manifest.read_text())
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:
        list(pool.map(lambda repo: fetch(repo, args.sources), manifest['repositories']))


if __name__ == '__main__':
    main()
