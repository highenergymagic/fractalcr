from pathlib import Path
import os
import shutil
import subprocess
import tempfile
import unittest


@unittest.skipUnless(os.name == 'posix' and shutil.which('rsync'), 'Linux build synchronization')
class BuildSyncTests(unittest.TestCase):
    def test_same_size_same_mtime_change_reaches_bitbake(self):
        script = (Path(__file__).resolve().parents[2] / 'os/tools/setup.sh').read_text()
        # Exercise the real source-staging portion without invoking BitBake or
        # changing ownership. Absolute production paths map into this fixture.
        script = script.split('cat > build/conf/bblayers.conf', 1)[0]
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for name in ('src/os/meta-fractal/recipes-kernel/linux/files',
                         'src/os/meta-fractal/recipes-core/fractal-init/files',
                         'src/service', 'src/internal', 'src/cmd', 'src/vendor', 'src/pkg', 'src/tools',
                         'work/sources/openembedded-core', 'boot-work/build/keys'):
                (root/name).mkdir(parents=True, exist_ok=True)
            for name in ('src/go.mod', 'src/go.sum', 'boot-work/build/keys/firmware.crt'):
                (root/name).write_text('fixture\n')
            (root/'src/VERSION').write_text('1.0.0-dev\n')
            shutil.copy(Path(__file__).resolve().parents[1]/'version-metadata.py', root/'src/tools/version-metadata.py')
            (root/'src/tools/build-identity.py').write_text('print("fixture-build-identity")\n')
            source = root/'src/os/meta-fractal/security.patch'
            source.write_bytes(b'old-content\n')
            os.utime(source, (0, 0))
            script = script.replace('/boot-work', str(root/'boot-work')).replace('/src', str(root/'src'))
            # Avoid rewriting the /work suffix in the already expanded root.
            script = script.replace('/work', str(root/'work'))
            subprocess.run(['bash', '-c', script], check=True, capture_output=True)
            copied = root/'work/meta-fractal/security.patch'
            os.utime(copied, (0, 0))
            source.write_bytes(b'new-content\n')
            os.utime(source, (0, 0))
            subprocess.run(['bash', '-c', script], check=True, capture_output=True)
            self.assertEqual(copied.read_bytes(), b'new-content\n')
            self.assertNotEqual(copied.stat().st_mtime, 0)
