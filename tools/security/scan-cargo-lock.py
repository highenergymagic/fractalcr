#!/usr/bin/env python3
"""Record OSV results for a reviewed upstream Cargo.lock, without executing it.

This is a conservative source dependency inventory, not binary reachability or
proof that an imported executable was produced from this lockfile. Git sources
and local workspace crates are recorded separately as coverage limitations.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import tomllib
import urllib.request


def query(path, body=None):
    data = None if body is None else json.dumps(body).encode()
    request = urllib.request.Request('https://api.osv.dev/v1/'+path, data=data,
                                     headers={'Content-Type':'application/json'})
    with urllib.request.urlopen(request, timeout=120) as response:
        return json.load(response)


def scan(lockfile, source_url):
    data = lockfile.read_bytes()
    packages = tomllib.loads(data.decode())['package']
    registry = [p for p in packages if p.get('source') == 'registry+https://github.com/rust-lang/crates.io-index']
    queries = [dict(package=dict(name=p['name'], ecosystem='crates.io'), version=p['version']) for p in registry]
    results = []
    for start in range(0, len(queries), 200):
        batch = queries[start:start+200]
        response = query('querybatch', dict(queries=batch))['results']
        if len(response) != len(batch):
            raise ValueError('incomplete OSV batch response')
        for request, result in zip(batch, response):
            vulnerabilities = list(result.get('vulns', []))
            while result.get('next_page_token'):
                result = query('query', dict(request, page_token=result['next_page_token']))
                vulnerabilities += result.get('vulns', [])
            results.append(dict(query=request, vulnerabilities=vulnerabilities))
    ids = sorted({v['id'] for r in results for v in r['vulnerabilities']})
    return dict(schema=1, generated_utc=datetime.now(timezone.utc).isoformat(),
                source_url=source_url, lock_sha256=hashlib.sha256(data).hexdigest(),
                registry_results=results, advisories={i:query('vulns/'+i) for i in ids},
                unscanned_sources=[p for p in packages if p not in registry],
                limitation='Source lockfile only: target/features, reachable symbols and imported-binary correspondence require review')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('lockfile', type=Path)
    parser.add_argument('--source-url', required=True)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    result = scan(args.lockfile, args.source_url)
    args.output.write_text(json.dumps(result, indent=2)+'\n')
    print(f"{len(result['registry_results'])} registry dependencies; {len(result['advisories'])} advisory IDs; review required")
