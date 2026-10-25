import gzip
import hashlib
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('finalize_release', Path(__file__).resolve().parents[1] / 'finalize-release.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class FinalizeReleaseTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        for name in ('usb.img.gz', 'usb-secureboot.img.gz', 'system.bin', 'rommon.efi'):
            (self.root / name).write_bytes(name.encode())
        with gzip.open(self.root / 'hv.qcow2.gz', 'wb') as stream:
            stream.write(b'qualified disk')
        self.write('qualification.json', {'passed': True, 'image_sha256': hashlib.sha256(b'qualified disk').hexdigest()})
        self.write('usb-qualification.json', {'passed': True, 'images': {name: module.digest(self.root / name) for name in ('usb.img.gz', 'usb-secureboot.img.gz')}})
        self.write('management-qualification.json', {'passed': True, 'system_sha256': module.digest(self.root / 'system.bin'), 'rommon_sha256': module.digest(self.root / 'rommon.efi')})
        self.write('release-manifest.json', {'files': {'old-name': 'old-hash'}})

    def write(self, name, data):
        (self.root / name).write_text(json.dumps(data))

    def test_inventory_includes_reports_and_source(self):
        (self.root / 'source.tar.gz').write_bytes(b'source')
        module.finalize(self.root, 'a' * 40)
        manifest = json.loads((self.root / 'release-manifest.json').read_text())
        self.assertEqual(manifest['source_commit'], 'a' * 40)
        self.assertIn('source.tar.gz', manifest['files'])
        self.assertIn('qualification.json', manifest['files'])
        self.assertNotIn('old-name', manifest['files'])
        self.assertNotIn('release-manifest.json', manifest['files'])

    def test_rejects_replaced_downloads_and_firmware(self):
        for name in ('hv.qcow2.gz', 'usb.img.gz', 'usb-secureboot.img.gz', 'system.bin', 'rommon.efi'):
            with self.subTest(name=name):
                p = self.root / name
                original = p.read_bytes()
                p.write_bytes(gzip.compress(b'replaced') if name == 'hv.qcow2.gz' else b'replaced')
                with self.assertRaises(ValueError):
                    module.finalize(self.root, 'a' * 40)
                p.write_bytes(original)

    def test_rejects_failed_qualification(self):
        self.write('qualification.json', {'passed': False})
        with self.assertRaises(ValueError):
            module.finalize(self.root, 'a' * 40)
