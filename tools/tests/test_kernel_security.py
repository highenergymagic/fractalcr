import gzip
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('kernel_security', ROOT/'boot/tools/kernel-security.py')
security = importlib.util.module_from_spec(spec)
spec.loader.exec_module(security)


class KernelSecurityTests(unittest.TestCase):
    def test_boot_and_system_use_identical_reviewed_updates(self):
        boot = ROOT/'boot/kernel/security'
        layer = ROOT/'os/meta-fractal/recipes-kernel/linux/files'
        data = (boot/'kernel-security.json').read_bytes()
        self.assertEqual(data, (layer/'kernel-security.json').read_bytes())
        manifest = json.loads(data)
        pins = json.loads((ROOT/'boot/sources.json').read_text())
        self.assertEqual(manifest['base'], pins['linux']['version'])
        for entry in manifest['patches']:
            packed = (boot/entry['file']).read_bytes()
            self.assertEqual(hashlib.sha256(packed).hexdigest(), entry['sha256'])
            self.assertEqual(packed, (layer/entry['file']).read_bytes())
            text = gzip.decompress(packed).decode()
            for excluded in entry['excluded_files']:
                self.assertNotIn('diff --git a/'+excluded+' b/'+excluded+'\n', text)
        for entry in manifest['backports']:
            patch = (boot/entry['file']).read_bytes()
            self.assertEqual(hashlib.sha256(patch).hexdigest(), entry['sha256'])
            self.assertEqual(patch, (layer/entry['file']).read_bytes())
            self.assertIn('https://', entry['reference_url'])

    @unittest.skipUnless(shutil.which('patch'), 'GNU patch required')
    def test_recreate_patched_tree_and_reject_corrupt_update(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            source = work/'sources/linux'
            source.mkdir(parents=True)
            (source/'file').write_bytes(b'old\n')
            (source/'Makefile').write_text('VERSION = 6\nPATCHLEVEL = 12\nSUBLEVEL = 112\n')
            (source/'localversion-cip').write_text('-cip33\n')
            updates = work/'updates'
            updates.mkdir()
            packed = gzip.compress(b'--- a/file\n+++ b/file\n@@ -1 +1 @@\n-old\n+new\n')
            patch = updates/'fix.patch.gz'
            patch.write_bytes(packed)
            (updates/'kernel-security.json').write_text(json.dumps(dict(base='6.12.112-cip33', patches=[dict(
                file=patch.name, sha256=hashlib.sha256(packed).hexdigest())])))
            target = security.prepare(work, updates)
            self.assertEqual((target/'file').read_bytes(), b'new\n')
            (target/'file').write_bytes(b'polluted\n')
            security.prepare(work, updates)
            self.assertEqual((target/'file').read_bytes(), b'new\n')
            self.assertEqual((source/'file').read_bytes(), b'old\n')
            patch.write_bytes(b'bad')
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                security.prepare(work, updates)

    @unittest.skipUnless(shutil.which('patch'), 'GNU patch required')
    def test_plain_backport_applies_after_stable_updates(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            source = work/'sources/linux'
            source.mkdir(parents=True)
            (source/'file').write_bytes(b'old\n')
            (source/'Makefile').write_text('VERSION = 6\nPATCHLEVEL = 12\nSUBLEVEL = 112\n')
            (source/'localversion-cip').write_text('-cip33\n')
            updates = work/'updates'
            updates.mkdir()
            stable = gzip.compress(b'--- a/file\n+++ b/file\n@@ -1 +1 @@\n-old\n+intermediate\n')
            backport = b'--- a/file\n+++ b/file\n@@ -1 +1 @@\n-intermediate\n+fixed\n'
            (updates/'stable.patch.gz').write_bytes(stable)
            (updates/'backport.patch').write_bytes(backport)
            (updates/'kernel-security.json').write_text(json.dumps({
                'base': '6.12.112-cip33',
                'patches': [{'file': 'stable.patch.gz', 'sha256': hashlib.sha256(stable).hexdigest()}],
                'backports': [{'file': 'backport.patch', 'sha256': hashlib.sha256(backport).hexdigest()}],
            }))
            self.assertEqual((security.prepare(work, updates)/'file').read_bytes(), b'fixed\n')
