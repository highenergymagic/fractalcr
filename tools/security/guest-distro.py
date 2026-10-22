#!/usr/bin/env python3
"""Recover the pinned Kata guest's distro from consistent ELF package notes.

This does not recover a package database or establish inventory completeness.
Azure Linux 3 kept 'mariner' in some ELF notes; its advisory feed is azurelinux.
https://anchore.com/blog/grype-support-for-azure-linux-3-released/
"""
import argparse
import json
from pathlib import Path


def identify(inventory):
    if (inventory.get('distro') or {}).get('id'):
        return None
    notes = [p for p in inventory['artifacts']
             if p.get('metadataType') == 'elf-binary-package-note-json-payload']
    identities = {(p['metadata'].get('os'), p['metadata'].get('osVersion')) for p in notes}
    if identities != {('mariner', '3.0')} or not any('.azl3' in p.get('version', '') for p in notes):
        return None
    return dict(distro='azurelinux:3.0', evidence='consistent ELF package notes',
                packages=[dict(name=p['name'], version=p['version'], locations=p['locations']) for p in notes],
                limitation='ELF notes cover only recognized binaries; missing RPM database still limits coverage')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('inventory', type=Path)
    parser.add_argument('evidence', type=Path)
    args = parser.parse_args()
    result = identify(json.loads(args.inventory.read_text()))
    args.evidence.write_text(json.dumps(result, indent=2)+'\n')
    if result:
        print(result['distro'])
