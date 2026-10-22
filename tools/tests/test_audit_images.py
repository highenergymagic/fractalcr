"""Untrusted image parsing must not escape the offline audit directory."""
import gzip
import importlib.util
from pathlib import Path
import stat
import struct
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


def module(name, filename):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'tools/security' / filename)
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


cpio = module('cpio_scan', 'extract-initramfs.py')
guest = module('guest_scan', 'guest-partition.py')


def entry(name, data=b'', mode=stat.S_IFREG | 0o644, inode=1, links=1):
    name = name.encode() + b'\0'
    fields = [inode, mode, 0, 0, links, 0, len(data), 0, 0, 0, 0, len(name), 0]
    header = b'070701' + b''.join(f'{n:08x}'.encode() for n in fields)
    return header + name + b'\0' * (-(110+len(name)) % 4) + data + b'\0' * (-len(data) % 4)


class AuditImageTests(unittest.TestCase):
    def run_cpio(self, root, data):
        source = root / 'image.gz'
        source.write_bytes(gzip.compress(data + entry('TRAILER!!!')))
        return cpio.extract(source, root / 'extracted')

    def test_hardlink_data_can_appear_last(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            records = self.run_cpio(root, entry('first', links=2) + entry('second', b'ELF contents', links=2))
            self.assertEqual((root / 'extracted/first').read_bytes(), b'ELF contents')
            self.assertEqual(records[0]['sha256'], records[1]['sha256'])
            self.assertEqual(records[0]['size'], 12)

    def test_no_symlink_or_device_is_created(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            records = self.run_cpio(root, entry('link', b'/etc', stat.S_IFLNK | 0o777) + entry('device', mode=stat.S_IFCHR | 0o600))
            self.assertFalse((root / 'extracted/link').exists())
            self.assertFalse((root / 'extracted/device').exists())
            self.assertEqual(records[0]['symlink'], '/etc')

    def test_traversal_and_duplicates_fail(self):
        for data in [entry('../escaped', b'x'), entry('/escaped', b'x'), entry('same') + entry('same')]:
            with self.subTest(data=data), tempfile.TemporaryDirectory() as directory:
                with self.assertRaises((ValueError, FileExistsError)):
                    self.run_cpio(Path(directory), data)

    def test_signed_numeric_header_is_rejected(self):
        data = bytearray(entry('bad'))
        data[6+6*8:6+7*8] = b'-0000001'
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(ValueError):
                self.run_cpio(Path(directory), bytes(data))

    def test_partition_bounds_and_filesystem_magic(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, output = root / 'disk', root / 'partition'
            data = bytearray(4096)
            data[510:512] = b'\x55\xaa'
            data[446:462] = struct.pack('<B3sB3sII', 0, b'\0'*3, 0x83, b'\0'*3, 2, 6)
            data[1024+1024+56:1024+1024+58] = b'\x53\xef'
            source.write_bytes(data)
            guest.extract(source, output)
            self.assertEqual(output.read_bytes(), data[1024:])
            data[458:462] = struct.pack('<I', 999)
            source.write_bytes(data)
            with self.assertRaises(ValueError):
                guest.extract(source, root / 'bad')


if __name__ == '__main__':
    unittest.main()
