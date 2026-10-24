#!/usr/bin/env python3
import gzip
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('bundled', Path(__file__).resolve().parents[1]/'tools/verify-bundled-image.py')
bundled = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bundled)


def entry(name, data):
    name = name.encode()+b'\0'
    fields = [1, 0o100644, 0, 0, 1, 0, len(data), 0, 0, 0, 0, len(name), 0]
    header = b'070701'+''.join(f'{x:08x}' for x in fields).encode()+name
    header += b'\0'*(-len(header) % 4)
    return header+data+b'\0'*(-len(data) % 4)


def archive(identity, binary=b'current executable'):
    return (entry('etc/fractal/build-id', (identity+'\n').encode())+
            entry('usr/libexec/fractal-management', binary)+entry('TRAILER!!!', b''))


class BundledImageTest(unittest.TestCase):
    def test_current_embedded_archive(self):
        root = archive('a'*64)
        bundled.verify(b'MZ'+gzip.compress(b'kernel prefix'+root+b'suffix'), root, 'a'*64)

    def test_stale_kernel_even_with_matching_build_id(self):
        root = archive('a'*64)
        stale = archive('a'*64, b'old executable')
        with self.assertRaises(ValueError):
            bundled.verify(b'MZ'+gzip.compress(stale), root, 'a'*64)

    def test_wrong_source_and_missing_archive(self):
        root = archive('b'*64)
        for kernel, expected in [(b'MZ'+gzip.compress(root), 'a'*64), (b'MZno kernel payload', 'b'*64)]:
            with self.assertRaises(ValueError): bundled.verify(kernel, root, expected)

    def test_truncated_and_duplicate_metadata(self):
        root = archive('a'*64)
        for invalid in [root[:115], entry('etc/fractal/build-id', b'a'*64+b'\n')+root]:
            with self.assertRaises(ValueError): bundled.cpio_file(invalid, b'etc/fractal/build-id')
        with self.assertRaises(ValueError):
            bundled.verify(b'MZ'+gzip.compress(root)[:-8], root, 'a'*64)


if __name__ == '__main__': unittest.main()
