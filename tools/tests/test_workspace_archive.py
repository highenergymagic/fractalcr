import importlib.util
from pathlib import Path
import tarfile
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('workspace_archive', Path(__file__).resolve().parents[1] / 'workspace-archive.py')
snapshot = importlib.util.module_from_spec(spec)
spec.loader.exec_module(snapshot)


class WorkspaceArchiveTests(unittest.TestCase):
    def test_lf_binary_preservation_and_determinism(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in snapshot.INPUTS:
                (root / name).mkdir()
            (root / 'boot/fetch.sh').write_bytes(b'#!/bin/bash\r\nset -eu\r\n')
            (root / 'boot/blob.bin').write_bytes(b'\r\n\0binary')
            (root / 'boot/.git').write_text('git metadata must not ship')
            first, second = root / 'one.tar.gz', root / 'two.tar.gz'
            snapshot.create(root, first)
            snapshot.create(root, second)
            self.assertEqual(first.read_bytes(), second.read_bytes())
            with tarfile.open(first) as archive:
                self.assertEqual(archive.extractfile('boot/fetch.sh').read(), b'#!/bin/bash\nset -eu\n')
                self.assertEqual(archive.extractfile('boot/blob.bin').read(), b'\r\n\0binary')
                self.assertNotIn('boot/.git', archive.getnames())
                self.assertEqual(archive.getmember('boot/fetch.sh').mode, 0o755)

    def test_private_key_rejected(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'innocent.txt'
            path.write_bytes(b'-----BEGIN ' + b'PRIVATE KEY-----\n')
            with self.assertRaises(ValueError):
                snapshot.normalized(path)
