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
import sys
import tempfile
import time
import tomllib
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
    for name, repository, current in [('kata', 'kata-containers/kata-containers', '4.3.1'),
                                     ('compose', 'docker/compose', 'v5.6.1'),
                                     ('buildx', 'docker/buildx', 'v0.38.1'),
                                     ('buildkit', 'moby/buildkit', 'v0.34.1'),
                                     ('cni', 'containernetworking/plugins', 'v1.9.1')]:
        # Read the installed versions from recipes, so this report cannot drift
        # after an update. Source revisions and vendor patches remain locked.
        recipes = ROOT / 'os/meta-fractal/recipes-containers'
        if name == 'kata':
            current = next((recipes / 'fractal-kata').glob('fractal-kata_*.bb')).stem.split('_', 1)[1]
        else:
            recipe = (recipes / 'fractal-container-tools/fractal-container-tools_1.0.bb').read_text()
            match = re.search(r'^FCR_' + name.upper() + r'_VERSION = "([^"]+)"$', recipe, re.M)
            if not match:
                raise ValueError('missing tool version pin: ' + name)
            current = 'v' + match[1]
        release = json.loads(read_url('https://api.github.com/repos/' + repository + '/releases/latest'))
        result.append(dict(name=name, current=current, candidate=release['tag_name'], evidence=release['html_url']))
    # Stay on the selected maintenance lines; a new major/minor runtime is a
    # separate compatibility decision. Read current pins from the layer.
    recipes = ROOT / 'os/meta-fractal/recipes-containers'
    for name, repository, recipe_path, variable, prefix, pattern in [
        ('containerd', 'containerd/containerd', 'containerd/containerd_%.bbappend', 'CONTAINERD_VERSION', 'v', r'v2\.2\.\d+'),
        ('runc', 'opencontainers/runc', 'runc/runc_%.bbappend', 'RUNC_VERSION', 'v', r'v1\.5\.\d+'),
        ('docker', 'moby/moby', 'docker/docker-moby_%.bbappend', 'DOCKER_VERSION', 'docker-', r'docker-v29\.\d+\.\d+'),
    ]:
        match = re.search(r'^' + variable + r' = "([^"]+)"$', (recipes/recipe_path).read_text(), re.M)
        if not match:
            raise ValueError('missing runtime version pin: ' + name)
        releases = json.loads(read_url('https://api.github.com/repos/' + repository + '/releases?per_page=100'))
        candidates = [r for r in releases if not r['prerelease'] and not r['draft'] and re.fullmatch(pattern, r['tag_name'])]
        if not candidates:
            raise ValueError('no supported maintenance release found: ' + name)
        latest = max(candidates, key=lambda r: version_key(r['tag_name']))
        result.append(dict(name=name, current=prefix+match[1], candidate=latest['tag_name'], evidence=latest['html_url']))
    go_recipe = next((ROOT/'os/meta-fractal/recipes-devtools/go').glob('go-1.26.*.inc'))
    current = go_recipe.stem.removeprefix('go-')
    releases = json.loads(read_url('https://go.dev/dl/?mode=json'))
    candidates = [r for r in releases if r['stable'] and re.fullmatch(r'go1\.26\.\d+', r['version'])]
    if not candidates:
        raise ValueError('Go 1.26 is absent from supported releases; review the supported compiler series')
    latest = max(candidates, key=lambda r: version_key(r['version']))
    result.append(dict(name='go', current='go'+current, candidate=latest['version'], evidence='https://go.dev/dl/'))
    rust_recipe = next((ROOT/'os/meta-fractal/recipes-devtools/rust').glob('fractal-rust-native_*.bb'))
    current = rust_recipe.stem.split('_', 1)[1]
    rust = tomllib.loads(read_url('https://static.rust-lang.org/dist/channel-rust-stable.toml'))
    result.append(dict(name='rust-build-tools', current=current,
                       candidate=rust['pkg']['rust']['version'].split()[0],
                       evidence='https://static.rust-lang.org/dist/channel-rust-stable.toml'))
    vmm_recipes = ROOT/'os/meta-fractal/recipes-containers/fractal-vmm'
    current = next(vmm_recipes.glob('fractal-cloud-hypervisor_*.bb')).stem.split('_', 1)[1]
    release = json.loads(read_url('https://api.github.com/repos/cloud-hypervisor/cloud-hypervisor/releases/latest'))
    result.append(dict(name='cloud-hypervisor', current='v'+current, candidate=release['tag_name'], evidence=release['html_url']))
    current = next(vmm_recipes.glob('fractal-virtiofsd_*.bb')).stem.split('_', 1)[1]
    listing = subprocess.check_output(['git','ls-remote','--tags','https://gitlab.com/virtio-fs/virtiofsd.git'],text=True,timeout=90)
    versions = set(re.findall(r'refs/tags/(v\d+\.\d+\.\d+)(?:\^\{\})?\s*$',listing,re.M))
    if not versions:
        raise ValueError('no virtiofsd releases found')
    result.append(dict(name='virtiofsd',current='v'+current,candidate=max(versions,key=version_key),
                       evidence='https://gitlab.com/virtio-fs/virtiofsd/-/tags'))
    override = (ROOT/'boot/buildroot/security-overrides.mk').read_text()
    current = re.search(r'^override LIBOPENSSL_VERSION = ([0-9.]+)$', override, re.M)[1]
    releases = json.loads(read_url('https://api.github.com/repos/openssl/openssl/releases?per_page=100'))
    candidates = [r for r in releases if not r['prerelease'] and not r['draft'] and re.fullmatch(r'openssl-3\.5\.\d+', r['tag_name'])]
    if not candidates:
        raise ValueError('no OpenSSL 3.5 maintenance release found')
    latest = max(candidates, key=lambda r: version_key(r['tag_name']))
    result.append(dict(name='rommon-openssl', current='openssl-'+current, candidate=latest['tag_name'], evidence=latest['html_url']))
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
    with tempfile.TemporaryDirectory() as directory:
        archive = Path(directory) / 'source.tar.xz'
        digest = hashlib.sha256()
        with urllib.request.urlopen(url, timeout=120) as response, archive.open('xb') as output:
            while chunk := response.read(1024 * 1024):
                digest.update(chunk)
                output.write(chunk)
        if digest.hexdigest() != checksum:
            raise ValueError('archive does not match reviewed checksum; no pins changed')
        if name == 'buildroot':
            subprocess.run([sys.executable, str(ROOT/'tools/security/verify-buildroot-release.py'),
                            '--version', version, '--archive', str(archive)], check=True)
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
