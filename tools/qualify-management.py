#!/usr/bin/env python3
"""Qualify the signed firmware's serial and SSH management on disposable disks."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

HARNESS_FILES = ('tools/qualify-management.py', 'tools/prepare-launcher.py',
                 'os/tests/qemu-ios-workflow.py', 'os/tests/qemu-aaa.py',
                 'os/tests/qemu-clock-save.py',
                 'os/tests/ios_prompts.py', 'os/tools/make-data.py',
                 'boot/tools/make-flash.py', 'boot/tests/qemu_process.py')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--artifacts', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    artifacts, output = args.artifacts.resolve(), args.output.resolve()
    if not Path('/dev/kvm').is_char_device():
        parser.error('Management qualification requires KVM; no qualification is skipped')
    inputs = json.loads((artifacts / 'build-inputs.json').read_text())
    source = inputs['source_build']
    if not isinstance(source, str) or not re.fullmatch(r'[0-9a-f]{64}', source):
        parser.error('Invalid source-build identity')
    with (artifacts / 'system.bin').open('rb') as stream:
        image = hashlib.file_digest(stream, 'sha256').hexdigest()
    with (artifacts / 'rommon.efi').open('rb') as stream:
        rommon = hashlib.file_digest(stream, 'sha256').hexdigest()
    output.mkdir(parents=True, exist_ok=False)
    root = Path(__file__).resolve().parents[1]
    harness = {}
    for name in HARNESS_FILES:
        with (root / name).open('rb') as stream:
            harness[name] = hashlib.file_digest(stream, 'sha256').hexdigest()
    result = {'passed': False, 'source_build': source, 'system_sha256': image,
              'rommon_sha256': rommon, 'harness_sha256': harness, 'checks': {}}
    try:
        subprocess.run([sys.executable, str(root / 'tools/prepare-launcher.py'),
                        '--artifacts', str(artifacts), '--output', str(output / 'templates')], check=True)
        # Sequential VMs fit on an ordinary hosted runner; each needs 4 GiB.
        cases = [
            ('ios', root / 'os/tests/qemu-ios-workflow.py', output / 'ios', output / 'templates',
             ['--accelerator', 'kvm', '--expected-source-build', source], 'result.json'),
            ('aaa', root / 'os/tests/qemu-aaa.py', output / 'aaa', artifacts, [], 'result-os-aaa.json'),
        ]
        for name, script, destination, files, extra, report in cases:
            subprocess.run([sys.executable, str(script), '--artifacts', str(files),
                            '--output', str(destination), *extra], check=True)
            evidence = json.loads((destination / report).read_text())
            if (not evidence.get('passed') or evidence.get('system_sha256') != image
                    or evidence.get('rommon_sha256') != rommon
                    or (name == 'ios' and evidence.get('source_build') != source)):
                raise RuntimeError(name + ' qualification does not match the signed image')
            result['checks'][name] = evidence
        subprocess.run([sys.executable, str(root / 'os/tests/qemu-clock-save.py'),
                        '--kernel', str(artifacts / 'system.efi'),
                        '--output', str(output / 'clock-save'),
                        '--expected-source-build', source], check=True)
        evidence = json.loads((output / 'clock-save/result-clock-save.json').read_text())
        with (artifacts / 'system.efi').open('rb') as stream:
            kernel = hashlib.file_digest(stream, 'sha256').hexdigest()
        if (not evidence.get('passed') or evidence.get('source_build') != source
                or evidence.get('kernel_sha256') != kernel):
            raise RuntimeError('Clock restoration qualification does not match the kernel')
        result['checks']['clock-save'] = evidence
        result['passed'] = True
    except Exception as error:
        result['error'] = str(error)
        raise
    finally:
        (output / 'result.json').write_text(json.dumps(result, indent=2) + '\n')


if __name__ == '__main__':
    main()
