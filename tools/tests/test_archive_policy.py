import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('archive_policy', ROOT/'tools/archive_policy.py')
policy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(policy)


class ArchivePolicyTests(unittest.TestCase):
    def test_public_fixture_is_byte_pinned(self):
        name = next(iter(policy.PUBLIC_FIXTURES))
        data = (ROOT/name).read_bytes().replace(b'\r\n', b'\n')
        policy.check_private_keys(name, data)
        for path, content in [(name, data+b'\n'), ('elsewhere.go', data)]:
            with self.assertRaises(ValueError):
                policy.check_private_keys(path, content)

    def test_private_material_rejected(self):
        for name in ['key.pem', next(iter(policy.PUBLIC_FIXTURES))]:
            with self.assertRaises(ValueError):
                policy.check_private_keys(name, b'-----BEGIN '+b'PRIVATE KEY-----\nnew key')
