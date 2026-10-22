import importlib.util
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('refresh_vendor', Path(__file__).resolve().parents[1] / 'security/refresh-vendor.py')
refresh = importlib.util.module_from_spec(spec)
spec.loader.exec_module(refresh)


@unittest.skipUnless(shutil.which('git'), 'Git required')
class VendorPatchTests(unittest.TestCase):
    def test_deleted_and_renamed_files_survive_patch_export(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            def git(*args, data=None):
                return subprocess.run(['git', *args], cwd=root, input=data, check=True,
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout
            git('init')
            git('config', 'user.email', 'test@example.invalid')
            git('config', 'user.name', 'Patch test')
            git('config', 'core.autocrlf', 'false')
            vendor = root / 'vendor'
            vendor.mkdir()
            (vendor / 'obsolete.go').write_bytes(b'old implementation\n')
            (vendor / 'renamed.go').write_bytes(b'retained implementation\n')
            (root / '.gitignore').write_bytes(b'*.yaml\n')
            git('add', '.')
            git('commit', '-m', 'fixture')
            (vendor / 'obsolete.go').unlink()
            (vendor / 'renamed.go').rename(vendor / 'replacement.go')
            (vendor / '.lint.yaml').write_bytes(b'ignored upstream file\n')
            patch = refresh.dependency_diff(root, ['vendor'])
            git('reset', '--hard', 'HEAD')
            (vendor / 'replacement.go').unlink(missing_ok=True)
            (vendor / '.lint.yaml').unlink(missing_ok=True)
            git('apply', '-', data=patch)
            self.assertEqual({p.name for p in vendor.iterdir()}, {'replacement.go', '.lint.yaml'})
            self.assertEqual((vendor / 'replacement.go').read_bytes(), b'retained implementation\n')
