import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]


def load(name):
    spec = importlib.util.spec_from_file_location(name, ROOT/'tools/security'/f'{name}.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class SourceInventoryTests(unittest.TestCase):
    def test_embedded_kernel_is_included_without_hiding_conflicts(self):
        merge = load('merge-oe-vex').merge
        kernel = dict(name='linux-fractal-cip', version='6.12.112-cip33',
                      cpes=['cpe:2.3:*:linux:linux_kernel:6.12.112:*:*:*:*:*:*:*'])
        rootfs = dict(package=[dict(name='glibc', version='2.43', cpes=[])])
        result = merge(rootfs, dict(package=[kernel]))
        self.assertEqual([p['name'] for p in result['package']], ['glibc','linux-fractal-cip'])
        self.assertEqual(load('oe-cpes').cpes(result), kernel['cpes'])
        with self.assertRaises(ValueError):
            merge(rootfs, dict(package=[]))
        with self.assertRaises(ValueError):
            merge(dict(package=[dict(kernel, version='old')]), dict(package=[kernel]))
        with self.assertRaises(ValueError):
            load('oe-cpes').cpes(dict(package=[dict(cpes=['cpe:2.3:foo\ninjected'])]))


if __name__ == '__main__':
    unittest.main()
