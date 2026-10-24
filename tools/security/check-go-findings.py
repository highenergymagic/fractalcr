#!/usr/bin/env python3
"""Keep symbol-level govulncheck enforcement with narrowly reviewed false positives.

Input must be the complete JSON output of a successful govulncheck invocation.
Scanner failures must be checked separately by the caller.
"""
import hashlib
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
NAME_SOURCE = 'vendor/github.com/docker/docker/pkg/namesgenerator/names-generator.go'
NAME_HASH = 'cd2f9528ad47ae7519d78e6381e0b2313fa9b5ff541814e95e06ebb187aa9b64'


def messages(text):
    decoder = json.JSONDecoder()
    while text.strip():
        value, end = decoder.raw_decode(text.lstrip())
        yield value
        text = text.lstrip()[end:]


def reviewed(finding, source):
    trace = finding.get('trace', [])
    if not trace:
        return False
    frame = trace[0]
    return (finding.get('osv') in {'GO-2026-4883', 'GO-2026-4887'}
            and frame.get('module') == 'github.com/docker/docker'
            and frame.get('version') == 'v28.5.2+incompatible'
            and frame.get('package') == 'github.com/docker/docker/pkg/namesgenerator'
            and frame.get('function') in {'init', 'GetRandomName'}
            and hashlib.sha256(source.replace(b'\r\n', b'\n')).hexdigest() == NAME_HASH)


def check(text, source):
    events = list(messages(text))
    configs = [e['config'] for e in events if 'config' in e]
    if len(configs) != 1 or configs[0].get('scan_level') != 'symbol' or configs[0].get('scan_mode') != 'source':
        raise ValueError('Expected a complete source/symbol govulncheck report')
    failed = False
    for event in events:
        if 'error' in event:
            raise ValueError('Scanner reported an error')
        finding = event.get('finding')
        if not finding:
            continue
        trace = finding.get('trace', [])
        if not trace:
            raise ValueError('Finding has no trace')
        frame = trace[0]
        if not frame.get('function'):
            print('INFORMATIONAL (no reachable symbol):', finding['osv'], frame)
        elif reviewed(finding, source):
            print('REVIEWED NOT AFFECTED:', finding['osv'], frame['package'], frame['function'])
        else:
            print('FAIL:', json.dumps(finding))
            failed = True
    return not failed


if __name__ == '__main__':
    sys.exit(0 if check(Path(sys.argv[1]).read_text(encoding='utf-8'), (ROOT / NAME_SOURCE).read_bytes()) else 1)
