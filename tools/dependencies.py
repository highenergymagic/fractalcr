#!/usr/bin/env python3
"""Discover updates separately from locked builds. Never execute upstream scripts."""
import argparse
import datetime
import hashlib
import http.client
import json
from pathlib import Path
import re
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]


def read_url(url):
    req = urllib.request.Request(url, headers={'User-Agent': 'FractalCR-dependency-review/1'})
    for attempt in range(3):
        try:
            with urllib.request.urlopen(req, timeout=60) as response:
                return response.read().decode()
        except (urllib.error.URLError, http.client.IncompleteRead, TimeoutError):
            if attempt == 2:
                raise
            time.sleep(attempt + 1)


def kernel_versions(text):
    # This is discovery data over HTTPS, not a claim of PGP verification.
    return {version: digest for digest, version in re.findall(
        r'^([a-f0-9]{64})\s+linux-cip-(6\.12\.\d+-cip\d+)\.tar\.xz\s*$', text, re.M)}


def version_key(version):
    return tuple(int(n) for n in re.findall(r'\d+', version))


def discover():
    pins = json.loads((ROOT / 'boot/sources.json').read_text())
    checksums = 'https://cdn.kernel.org/pub/linux/kernel/projects/cip/6.12/sha256sums.asc'
    versions = kernel_versions(read_url(checksums))
    if not versions:
        raise ValueError('no CIP releases found; upstream format may have changed')
    latest = max(versions, key=version_key)
    result = [dict(name='linux-cip', current=pins['linux']['version'], candidate=latest,
                   sha256=versions[latest], evidence=checksums,
                   verification='HTTPS checksum discovery; review upstream developer signature before release')]
    listing = subprocess.check_output(['git', 'ls-remote', '--tags',
        'https://gitlab.com/buildroot.org/buildroot.git', 'refs/tags/2025.02.*'], text=True, timeout=90)
    versions = set(re.findall(r'refs/tags/(2025\.02\.\d+)(?:\^\{\})?\s*$', listing, re.M))
    if not versions:
        raise ValueError('no Buildroot LTS releases found')
    result.append(dict(name='buildroot', current=pins['buildroot']['version'],
                       candidate=max(versions, key=version_key), evidence='https://buildroot.org/downloads/'))
    manifest = json.loads((ROOT / 'os/sources.json').read_text())
    for repo in manifest['repositories']:
        branch = '2.18' if repo['name'] == 'bitbake' else manifest['series']
        output = subprocess.check_output(['git', 'ls-remote', '--exit-code', repo['url'],
                                          'refs/heads/' + branch], text=True, timeout=90)
        revision = output.split()[0]
        if not re.fullmatch('[0-9a-f]{40}', revision):
            raise ValueError('invalid upstream revision')
        result.append(dict(name=repo['name'], current=repo['revision'], candidate=revision,
                           evidence=repo['url'], branch=branch))
    for name, repository, current in [('kata', 'kata-containers/kata-containers', '4.2.0'),
                                     ('compose', 'docker/compose', 'v5.6.1'),
                                     ('buildx', 'docker/buildx', 'v0.38.1'),
                                     ('buildkit', 'moby/buildkit', 'v0.34.1')]:
        # Read the installed versions from recipes, so this report cannot drift
        # after an update. Recipes deliberately pin prebuilt upstream inputs.
        recipes = ROOT / 'os/meta-fractal/recipes-containers'
        if name == 'kata':
            current = next((recipes / 'fractal-kata').glob('fractal-kata_*.bb')).stem.split('_', 1)[1]
        else:
            recipe = (recipes / 'fractal-container-tools/fractal-container-tools_1.0.bb').read_text()
            current = re.search(re.escape(repository) + r'/releases/download/([^/]+)/', recipe)[1]
        release = json.loads(read_url('https://api.github.com/repos/' + repository + '/releases/latest'))
        result.append(dict(name=name, current=current, candidate=release['tag_name'], evidence=release['html_url']))
    for item in result:
        item['update_available'] = item['current'] != item['candidate']
    return dict(schema=1, checked_at=datetime.datetime.now(datetime.timezone.utc).isoformat(), dependencies=result)


def update_archive(name, version, checksum):
    pattern = r'6\.12\.\d+-cip\d+' if name == 'linux' else r'2025\.02\.\d+'
    if not re.fullmatch(pattern, version) or not re.fullmatch(r'[0-9a-f]{64}', checksum):
        raise ValueError('invalid version or SHA-256; only the selected LTS lines are accepted')
    path = ROOT / 'boot/sources.json'
    pins = json.loads(path.read_text())
    if version_key(version) < version_key(pins[name]['version']):
        raise ValueError('dependency updater refuses downgrades')
    prefix = 'https://cdn.kernel.org/pub/linux/kernel/projects/cip/6.12/linux-cip-' if name == 'linux' else 'https://buildroot.org/downloads/buildroot-'
    url = prefix + version + '.tar.xz'
    # Stream the actual archive before changing either repository's pins.
    digest = hashlib.sha256()
    with urllib.request.urlopen(url, timeout=120) as response:
        while chunk := response.read(1024 * 1024):
            digest.update(chunk)
    if digest.hexdigest() != checksum:
        raise ValueError('archive does not match reviewed checksum; no pins changed')
    if name == 'linux':
        directory = ROOT / 'os/meta-fractal/recipes-kernel/linux'
        old = directory / f"linux-fractal-cip_{pins[name]['version']}.bb"
        new = directory / f'linux-fractal-cip_{version}.bb'
        if new != old and new.exists():
            raise ValueError('target recipe already exists')
        content = old.read_text()
        if pins[name]['sha256'] not in content:
            raise ValueError('ROMMON and OE pins differ; reconcile first')
        content = content.replace(pins[name]['sha256'], checksum)
        new.write_text(content)
        if old != new:
            old.unlink()
    pins[name] = dict(version=version, url=url, sha256=checksum)
    path.write_text(json.dumps(pins, indent=2) + '\n')
    print('Updated source pins. Review and commit affected component repositories, then their integration gitlinks.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    check = commands.add_parser('discover')
    check.add_argument('--output', type=Path)
    update = commands.add_parser('update-archive')
    update.add_argument('name', choices=['linux', 'buildroot'])
    update.add_argument('--version', required=True)
    update.add_argument('--sha256', required=True, help='Checksum reviewed against upstream release evidence')
    commands.add_parser('vendor-go')
    args = parser.parse_args()
    if args.command == 'discover':
        text = json.dumps(discover(), indent=2) + '\n'
        if args.output:
            args.output.parent.mkdir(parents=True, exist_ok=True)
            args.output.write_text(text)
        print(text)
    elif args.command == 'update-archive':
        update_archive(args.name, args.version, args.sha256)
    else:
        for command in (['go', 'mod', 'download'], ['go', 'mod', 'verify'], ['go', 'mod', 'vendor']):
            subprocess.run(command, cwd=ROOT, check=True)


if __name__ == '__main__':
    main()
