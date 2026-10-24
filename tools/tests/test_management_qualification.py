import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location('qualify_management', Path(__file__).parents[1] / 'qualify-management.py')
qualify = importlib.util.module_from_spec(spec)
spec.loader.exec_module(qualify)


class ManagementQualificationTests(unittest.TestCase):
    def test_failed_or_mismatched_reports_never_qualify(self):
        for failure in ('process', 'ios-image', 'ios-source', 'aaa-rommon', 'aaa-failed',
                        'clock-process', 'clock-kernel', 'clock-source', 'clock-failed', 'clock-empty', None):
            with self.subTest(failure=failure), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                source_root = root / 'source'
                for name in qualify.HARNESS_FILES:
                    fixture = source_root / name
                    fixture.parent.mkdir(parents=True, exist_ok=True)
                    fixture.write_text('qualification fixture: ' + name)
                artifacts = root / 'artifacts'
                artifacts.mkdir()
                (artifacts / 'system.bin').write_bytes(b'signed-image-fixture')
                (artifacts / 'system.efi').write_bytes(b'signed-kernel-fixture')
                (artifacts / 'rommon.efi').write_bytes(b'rommon-fixture')
                source = 'a' * 64
                (artifacts / 'build-inputs.json').write_text(json.dumps({'source_build': source}))
                image = hashlib.sha256(b'signed-image-fixture').hexdigest()
                rommon = hashlib.sha256(b'rommon-fixture').hexdigest()
                kernel = hashlib.sha256(b'signed-kernel-fixture').hexdigest()
                output = root / 'result'

                def execute(argv, check):
                    self.assertTrue(check)
                    name = Path(argv[1]).name
                    if name == 'prepare-launcher.py':
                        return subprocess.CompletedProcess(argv, 0)
                    if failure == 'process':
                        raise subprocess.CalledProcessError(1, argv)
                    if name == 'qemu-clock-save.py' and failure == 'clock-process':
                        raise subprocess.CalledProcessError(1, argv)
                    destination = Path(argv[argv.index('--output') + 1])
                    destination.mkdir()
                    if name == 'qemu-clock-save.py':
                        self.assertEqual(Path(argv[argv.index('--kernel') + 1]), artifacts / 'system.efi')
                        self.assertEqual(argv[argv.index('--expected-source-build') + 1], source)
                        report = {'passed': failure != 'clock-failed', 'source_build': source, 'kernel_sha256': kernel}
                        if failure == 'clock-kernel': report['kernel_sha256'] = 'b' * 64
                        if failure == 'clock-source': report['source_build'] = 'b' * 64
                        if failure == 'clock-empty': report = {}
                        (destination / 'result-clock-save.json').write_text(json.dumps(report))
                        return subprocess.CompletedProcess(argv, 0)
                    ios = name == 'qemu-ios-workflow.py'
                    report = {'passed': True, 'source_build': source,
                              'system_sha256': image, 'rommon_sha256': rommon}
                    if ios and failure == 'ios-image': report['system_sha256'] = 'b' * 64
                    if ios and failure == 'ios-source': report['source_build'] = 'b' * 64
                    if not ios and failure == 'aaa-rommon': report['rommon_sha256'] = 'b' * 64
                    if not ios and failure == 'aaa-failed': report['passed'] = False
                    (destination / ('result.json' if ios else 'result-os-aaa.json')).write_text(json.dumps(report))
                    return subprocess.CompletedProcess(argv, 0)

                with patch.object(qualify.sys, 'argv', ['qualify', '--artifacts', str(artifacts), '--output', str(output)]), \
                     patch.object(qualify, '__file__', str(source_root / 'tools/qualify-management.py')), \
                     patch.object(qualify.Path, 'is_char_device', return_value=True), \
                     patch.object(qualify.subprocess, 'run', side_effect=execute):
                    if failure is None:
                        qualify.main()
                    else:
                        with self.assertRaises((RuntimeError, subprocess.CalledProcessError)):
                            qualify.main()
                result = json.loads((output / 'result.json').read_text())
                self.assertEqual(result['passed'], failure is None)
                self.assertEqual(result['system_sha256'], image)
                self.assertEqual(result['rommon_sha256'], rommon)
                self.assertEqual(set(result['harness_sha256']), set(qualify.HARNESS_FILES))
                if failure is None:
                    self.assertEqual(set(result['checks']), {'ios', 'aaa', 'clock-save'})
                else:
                    self.assertIn('error', result)


if __name__ == '__main__':
    unittest.main()
