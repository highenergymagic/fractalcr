import importlib.util
import os
from pathlib import Path
import sys
import tempfile
import unittest


@unittest.skipUnless(sys.platform == 'linux', 'Linux signing station descriptors')
class SigningFileTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        spec = importlib.util.spec_from_file_location('release_files', Path(__file__).resolve().parents[1] / 'release.py')
        cls.release = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cls.release)

    def test_private_file_rejects_special_shared_and_public_inputs(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            key = root / 'private'
            key.write_bytes(b'fixture')
            key.chmod(0o600)
            self.assertEqual(self.release.private_file(key), b'fixture')
            fifo = root / 'fifo'
            os.mkfifo(fifo, 0o600)
            with self.assertRaises(ValueError):self.release.private_file(fifo)
            link = root / 'link'
            link.symlink_to(key)
            with self.assertRaises(OSError):self.release.private_file(link)
            key.chmod(0o644)
            with self.assertRaises(ValueError):self.release.private_file(key)
            key.chmod(0o600)
            with self.assertRaises(ValueError):self.release.private_file(key, 2)
            os.link(key, root / 'shared')
            with self.assertRaises(ValueError):self.release.private_file(key)

    def test_signing_accounting_rejects_fifo_without_blocking(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            os.mkfifo(root / 'signing-audit.jsonl', 0o600)
            with self.assertRaises(OSError):self.release.audit(root, {'event': 'test'})
