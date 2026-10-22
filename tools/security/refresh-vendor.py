#!/usr/bin/env python3
"""Review-time Go dependency refresh; builds consume its frozen patch offline.

Run in an otherwise clean checkout at an explicitly reviewed upstream commit.
Downloads use Go's checksum database and the upstream go.sum. This never runs
automatically during an appliance build or advances source pins by itself.
"""
import argparse
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile


def run(args, cwd, env=None):
    return subprocess.check_output(args, cwd=cwd, env=env)


def dependency_diff(source, paths):
    # Upstream ignore rules can hide files emitted by `go mod vendor`.
    run(['git', 'add', '--force', '--intent-to-add', *paths], source)
    # Intent-to-add stages deletions; compare against HEAD to include them too.
    # Quilt consumes unified patches, so represent renames as delete/add pairs.
    return run(['git', 'diff', 'HEAD', '--binary', '--no-renames', '--no-ext-diff', '--', *paths], source)


def refresh(source, revision, modules, output, go='go', vendor_archive=None):
    if not re.fullmatch('[0-9a-f]{40}', revision):
        raise ValueError('reviewed full Git revision required')
    if run(['git', 'rev-parse', 'HEAD'], source).decode().strip() != revision:
        raise ValueError('upstream checkout differs from reviewed revision')
    if run(['git', 'status', '--porcelain'], source).strip():
        raise ValueError('dependency refresh requires a clean checkout')
    if not vendor_archive and not (source / 'vendor/modules.txt').is_file():
        raise ValueError('upstream must already vendor its dependencies')
    for module in modules:
        if not re.fullmatch(r'[A-Za-z0-9._~/-]+@v[0-9][A-Za-z0-9.+-]*', module):
            raise ValueError('explicit module@version required')
    env = dict(os.environ, GOTOOLCHAIN='local', GOWORK='off', GOFLAGS='-mod=mod',
               GOPROXY='https://proxy.golang.org', GOSUMDB='sum.golang.org',
               GONOSUMDB='', GOPRIVATE='', GONOPROXY='')
    run([go, 'get', *modules], source, env)
    run([go, 'mod', 'tidy'], source, env)
    run([go, 'mod', 'verify'], source, env)
    run([go, 'mod', 'vendor'], source, env)
    paths = ['go.mod', 'go.sum'] if vendor_archive else ['go.mod', 'go.sum', 'vendor']
    patch = dependency_diff(source, paths)
    header = (f'Subject: [PATCH] Refresh vendored dependencies for appliance security review\n\n'
              f'Upstream-Status: Inappropriate [distribution dependency security refresh]\n'
              f'Base-Revision: {revision}\n\n').encode()
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open('xb') as raw, gzip.GzipFile(fileobj=raw, filename='', mode='wb', mtime=0) as zipped:
        zipped.write(header + patch)
    manifest = dict(schema=1, revision=revision, requested=modules,
                    toolchain=run([go, 'version'], source, env).decode().strip(),
                    patch_sha256=hashlib.sha256(output.read_bytes()).hexdigest(),
                    modules_txt_sha256=hashlib.sha256((source/'vendor/modules.txt').read_bytes()).hexdigest())
    if vendor_archive:
        with vendor_archive.open('xb') as raw, gzip.GzipFile(fileobj=raw, filename='', mode='wb', mtime=0) as zipped:
            with tarfile.open(fileobj=zipped, mode='w') as archive:
                for path in sorted((source/'vendor').rglob('*')):
                    if path.is_symlink():raise ValueError('unexpected symlink in generated vendor tree')
                    if not path.is_file():continue
                    info = tarfile.TarInfo(path.relative_to(source).as_posix())
                    info.mode, info.mtime, info.size = 0o644, 0, path.stat().st_size
                    with path.open('rb') as data:archive.addfile(info, data)
        manifest['vendor_archive_sha256'] = hashlib.sha256(vendor_archive.read_bytes()).hexdigest()
    Path(str(output)+'.json').write_text(json.dumps(manifest, indent=2)+'\n')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', type=Path, required=True)
    parser.add_argument('--revision', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--go', default='go')
    parser.add_argument('--vendor-archive', type=Path)
    parser.add_argument('modules', nargs='+')
    args = parser.parse_args()
    refresh(args.source.resolve(), args.revision, args.modules, args.output, args.go, args.vendor_archive)
