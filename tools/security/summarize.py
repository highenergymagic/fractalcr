#!/usr/bin/env python3
"""Summarize scanner findings and coverage without silently approving a release."""
import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path


def summarize(root):
    reports, gaps = {}, []
    for name in ('host', 'guest'):
        path = root / (name+'.grype.json')
        if not path.exists():
            gaps.append(name + ': no vulnerability report')
            continue
        report = json.loads(path.read_text())
        inventory = json.loads((root/(name+'.syft.json')).read_text())
        packages = inventory['artifacts']
        distro = report.get('distro') or {}
        if not distro.get('name'):
            gaps.append(name + ': distribution not identified; OS package advisories may be missing')
        # ELF build notes may describe RPMs even after the RPM database is
        # removed. Their package type alone does not imply complete coverage.
        if not any(p.get('foundBy') in ('rpm-db-cataloger', 'dpkg-db-cataloger',
                                       'apk-db-cataloger', 'alpm-db-cataloger') for p in packages):
            gaps.append(name + ': no distribution package database; reconcile OE/guest source inventory')
        unknown = [p['name'] for p in packages if p.get('version') in (None, '', 'UNKNOWN', '(devel)')]
        if unknown:
            gaps.append(name + ': some executable/package versions are unknown')
        reports[name] = dict(packages=len(packages), matches=len(report['matches']),
            severities=dict(Counter(m['vulnerability']['severity'] for m in report['matches'])),
            unknown_versions=unknown, sha256=hashlib.sha256(path.read_bytes()).hexdigest())
    source_report = root/'oe.grype.json'
    if source_report.exists():
        report = json.loads(source_report.read_text())
        reports['oe_sources'] = dict(matches=len(report['matches']),
            severities=dict(Counter(m['vulnerability']['severity'] for m in report['matches'])),
            sha256=hashlib.sha256(source_report.read_bytes()).hexdigest(),
            note='Reconcile source-version matches with system.oe-vex.json patch dispositions; no automatic suppression')
    else:
        gaps.append('host: no OE source-identity advisory report')
    result = dict(schema=1, release_approved=False, reports=reports, coverage_gaps=gaps,
                  note='Matches require review; absence of matches does not establish absence of vulnerabilities.')
    (root/'summary.json').write_text(json.dumps(result, indent=2)+'\n')
    return result


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', type=Path)
    args = parser.parse_args()
    print(json.dumps(summarize(args.directory), indent=2))
