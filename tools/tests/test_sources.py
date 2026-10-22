import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


def module(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


archives = module('archives', 'boot/tools/sources.py')
repositories = module('repositories', 'os/tools/fetch.py')


class SourceTests(unittest.TestCase):
    def test_toolchain_change_invalidates_outputs_but_preserves_keys_and_downloads(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            external = work/'external'
            (external/'configs').mkdir(parents=True)
            (external/'patches').mkdir()
            (external/'configs/fractalcr_rommon_defconfig').write_text('old toolchain')
            override = external/'security-overrides.mk'
            override.write_text('old override')
            archives.prepare_configuration(external, work)
            for name in ('rommon-root', 'kernel-rommon', 'kernel-witness', 'keys'):
                (work/'build'/name).mkdir(parents=True)
                (work/'build'/name/'retained').write_text('fixture')
            (work/'downloads').mkdir()
            (work/'downloads/archive').write_text('cached archive')
            self.assertFalse(archives.prepare_configuration(external, work))
            override.write_text('new override')
            self.assertTrue(archives.prepare_configuration(external, work))
            for name in ('rommon-root', 'kernel-rommon', 'kernel-witness'):
                self.assertFalse((work/'build'/name).exists())
            self.assertEqual((work/'build/keys/retained').read_text(), 'fixture')
            self.assertEqual((work/'downloads/archive').read_text(), 'cached archive')

    def archive(self, work, content, filename='linux/version'):
        path = work / 'downloads/linux.tar.xz'
        path.parent.mkdir(exist_ok=True)
        with tarfile.open(path, 'w:xz') as tar:
            info = tarfile.TarInfo(filename)
            info.size = len(content)
            tar.addfile(info, io.BytesIO(content))
        return dict(url='https://example.invalid/linux.tar.xz', sha256=archives.digest(path))

    def test_pin_change_replaces_old_source_and_invalidates_outputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            old = self.archive(work, b'old')
            self.assertTrue(archives.prepare('linux', old, work, download=False))
            self.assertFalse(archives.prepare('linux', old, work, download=False))
            output = work / 'build/kernel-rommon'
            output.mkdir(parents=True)
            (output / 'stale').write_text('old binary')
            new = self.archive(work, b'new')
            self.assertTrue(archives.prepare('linux', new, work, download=False))
            self.assertEqual((work / 'sources/linux/version').read_bytes(), b'new')
            self.assertFalse(output.exists())

    def test_corrupt_archive_and_traversal_do_not_replace_good_source(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            good = self.archive(work, b'good')
            archives.prepare('linux', good, work, download=False)
            bad = self.archive(work, b'escape', '../escape')
            with self.assertRaises(ValueError):
                archives.prepare('linux', good, work, download=False)
            with self.assertRaises(archives.UnsafeArchive):
                archives.prepare('linux', bad, work, download=False)
            self.assertEqual((work / 'sources/linux/version').read_bytes(), b'good')
            self.assertFalse((work / 'sources/escape').exists())

    def test_extracted_archive_cache_rejects_modified_and_untracked_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            pin = self.archive(work, b'good')
            archives.prepare('linux', pin, work, download=False)
            source = work/'sources/linux/version'
            source.write_bytes(b'evil')
            with self.assertRaisesRegex(ValueError, 'modified source cache'):
                archives.prepare('linux', pin, work, download=False)
            source.write_bytes(b'good')
            (source.parent/'extra.c').write_bytes(b'untracked')
            with self.assertRaisesRegex(ValueError, 'untracked'):
                archives.prepare('linux', pin, work, download=False)

    def test_buildroot_cache_accepts_final_directory_mode_and_migrates_downloads(self):
        with tempfile.TemporaryDirectory() as tmp:
            work = Path(tmp)
            (work/'downloads').mkdir()
            archive = work/'downloads/buildroot.tar.xz'
            with tarfile.open(archive, 'w:xz') as tar:
                for mode in (0o775, 0o755):
                    info = tarfile.TarInfo('buildroot-1/docs')
                    info.type, info.mode = tarfile.DIRTYPE, mode
                    tar.addfile(info)
                info = tarfile.TarInfo('buildroot-1/implicit/nested/file')
                info.size = 4
                tar.addfile(info, io.BytesIO(b'good'))
            pin = dict(version='1', url='https://example.invalid/buildroot.tar.xz', sha256=archives.digest(archive))
            archives.prepare('buildroot', pin, work, download=False)
            legacy = work/'sources/buildroot/dl/package'
            legacy.mkdir(parents=True)
            (legacy/'source.tar').write_bytes(b'cached')
            self.assertFalse(archives.prepare('buildroot', pin, work, download=False))
            self.assertEqual((work/'downloads/buildroot-packages/package/source.tar').read_bytes(), b'cached')
            self.assertFalse((work/'sources/buildroot/dl').exists())
            (work/'sources/buildroot/dl').mkdir()
            self.assertFalse(archives.prepare('buildroot', pin, work, download=False))
            self.assertFalse((work/'sources/buildroot/dl').exists())
            (work/'sources/buildroot/dl').mkdir()
            (work/'sources/buildroot/dl/conflicting.tar').write_bytes(b'preserve')
            with self.assertRaisesRegex(ValueError, 'ambiguous legacy'):
                archives.prepare('buildroot', pin, work, download=False)
            self.assertEqual((work/'sources/buildroot/dl/conflicting.tar').read_bytes(), b'preserve')

    def test_dirty_checkout_and_wrong_origin_are_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            upstream = root / 'upstream'
            upstream.mkdir()
            def git(path, *args):
                return subprocess.check_output(['git', '-C', str(path), *args], text=True).strip()
            git(upstream, 'init', '-q')
            (upstream / 'file').write_text('original')
            git(upstream, 'add', 'file')
            git(upstream, '-c', 'user.name=Test', '-c', 'user.email=test@example.invalid', 'commit', '-qm', 'fixture')
            pin = dict(name='test-source', url=str(upstream), revision=git(upstream, 'rev-parse', 'HEAD'))
            sources = root / 'sources'
            repositories.fetch(pin, sources)
            repositories.fetch(pin, sources)
            checkout = sources / pin['name']
            (checkout / 'file').write_text('tampered')
            with self.assertRaisesRegex(ValueError, 'dirty'):
                repositories.fetch(pin, sources)
            git(checkout, 'checkout', '--', 'file')
            (checkout / 'untracked').write_text('unexpected')
            with self.assertRaisesRegex(ValueError, 'dirty'):
                repositories.fetch(pin, sources)
            (checkout / 'untracked').unlink()
            git(checkout, 'remote', 'set-url', 'origin', 'https://example.invalid/other')
            with self.assertRaisesRegex(ValueError, 'origin'):
                repositories.fetch(pin, sources)

    def test_escaping_symlinks_and_forward_hardlinks_are_rejected(self):
        for kind in (tarfile.SYMTYPE, tarfile.LNKTYPE):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as tmp:
                archive = Path(tmp) / 'links.tar'
                with tarfile.open(archive, 'w') as tar:
                    member = tarfile.TarInfo('linux/link')
                    member.type = kind
                    member.linkname = '../../escape' if kind == tarfile.SYMTYPE else 'linux/later'
                    tar.addfile(member)
                with tarfile.open(archive) as tar:
                    with self.assertRaises(archives.UnsafeArchive):
                        archives.extract_source(tar, Path(tmp) / 'extracted')

    @unittest.skipIf(__import__('os').name == 'nt', 'absolute Unix skeleton link')
    def test_allowed_skeleton_link_cannot_be_followed_by_a_later_member(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            outside = root / 'outside'
            outside.mkdir()
            archive = root / 'links.tar'
            with tarfile.open(archive, 'w') as tar:
                link = tarfile.TarInfo('source/link')
                link.type = tarfile.SYMTYPE
                link.linkname = str(outside)
                tar.addfile(link)
                file = tarfile.TarInfo('source/link/escape')
                file.size = 1
                tar.addfile(file, io.BytesIO(b'x'))
            with tarfile.open(archive) as tar:
                with self.assertRaises(archives.UnsafeArchive):
                    archives.extract_source(tar, root / 'extracted', {('source/link', str(outside))})
            self.assertFalse((outside / 'escape').exists())

    def test_rommon_and_system_kernel_pins_match(self):
        pin = json.loads((ROOT / 'boot/sources.json').read_text())['linux']
        recipes = list((ROOT / 'os/meta-fractal/recipes-kernel/linux').glob('linux-fractal-cip_*.bb'))
        self.assertEqual(len(recipes), 1)
        self.assertEqual(recipes[0].name, f"linux-fractal-cip_{pin['version']}.bb")
        self.assertIn(pin['sha256'], recipes[0].read_text())


if __name__ == '__main__':
    unittest.main()
