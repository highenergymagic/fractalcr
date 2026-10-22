#!/usr/bin/env python3
"""Freeze a reviewed Cargo update/vendor result into deterministic OE inputs.

Run cargo update and cargo vendor --locked explicitly during review. Builds use
this immutable lock patch and vendor archive with --frozen --offline. The helper
permits changes only to Cargo.toml, Cargo.lock and generated vendor/config inputs.
"""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import re
import subprocess
import tarfile


def freeze(source, revision, output, cargo):
    def git(*args):
        return subprocess.check_output(['git','-C',str(source),*args])
    if not re.fullmatch(r'[0-9a-f]{40}', revision) or git('rev-parse','HEAD').decode().strip() != revision:
        raise ValueError('reviewed full source commit required')
    changes = git('diff','HEAD','--name-only').decode().splitlines()
    if 'Cargo.lock' not in changes or not set(changes) <= {'Cargo.toml','Cargo.lock'}:
        raise ValueError('expected only reviewed Cargo manifest/lock changes')
    if any(p not in ('Cargo-vendor.toml',) and not p.startswith('vendor/')
           for p in git('ls-files','--others','--exclude-standard').decode().splitlines()):
        raise ValueError('unexpected untracked source input')
    output.parent.mkdir(parents=True, exist_ok=True)
    patch = output.with_name(output.name+'-security.patch.gz')
    archive = output.with_name(output.name+'-vendor.tar.gz')
    header = ('Subject: [PATCH] Freeze reviewed Rust dependency security updates\n\n'
              'Upstream-Status: Inappropriate [distribution dependency security refresh]\n'
              f'Base-Revision: {revision}\n\n').encode()
    with patch.open('xb') as raw, gzip.GzipFile(fileobj=raw,filename='',mode='wb',mtime=0) as zipped:
        zipped.write(header+git('diff','HEAD','--no-ext-diff','--','Cargo.toml','Cargo.lock'))
    paths = [source/'Cargo-vendor.toml'] + sorted((source/'vendor').rglob('*'))
    with archive.open('xb') as raw, gzip.GzipFile(fileobj=raw,filename='',mode='wb',mtime=0) as zipped:
        with tarfile.open(fileobj=zipped,mode='w') as tar:
            for path in paths:
                if path.is_symlink():raise ValueError('unexpected link in vendor input')
                if not path.is_file():continue
                info = tarfile.TarInfo(path.relative_to(source).as_posix())
                info.size = path.stat().st_size
                info.mode = 0o755 if path.stat().st_mode & 0o111 else 0o644
                info.mtime = 0
                with path.open('rb') as stream:tar.addfile(info,stream)
    manifest = dict(schema=1,revision=revision,
        cargo=subprocess.check_output([cargo,'--version'],text=True).strip(),
        cargo_lock_sha256=hashlib.sha256((source/'Cargo.lock').read_bytes()).hexdigest(),
        patch_sha256=hashlib.sha256(patch.read_bytes()).hexdigest(),
        vendor_archive_sha256=hashlib.sha256(archive.read_bytes()).hexdigest())
    output.with_name(output.name+'-security.json').write_text(json.dumps(manifest,indent=2)+'\n')
    return manifest


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source',required=True,type=Path)
    parser.add_argument('--revision',required=True)
    parser.add_argument('--output-prefix',required=True,type=Path)
    parser.add_argument('--cargo',default='cargo')
    args = parser.parse_args()
    print(json.dumps(freeze(args.source.resolve(),args.revision,args.output_prefix,args.cargo),indent=2))
