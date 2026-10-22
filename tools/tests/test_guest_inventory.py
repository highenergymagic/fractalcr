import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('guest_distro', ROOT/'tools/security/guest-distro.py')
guest = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guest)


class GuestInventoryTests(unittest.TestCase):
    def test_selects_feed_only_from_consistent_binary_evidence(self):
        note = dict(name='glibc', version='2.38-21.azl3', locations=[{'path':'/usr/lib/libc.so.6'}],
                    metadataType='elf-binary-package-note-json-payload', metadata={'os':'mariner','osVersion':'3.0'})
        inventory = dict(artifacts=[note], distro={})
        self.assertEqual(guest.identify(inventory)['distro'], 'azurelinux:3.0')
        inventory['distro'] = {'id':'other'}
        self.assertIsNone(guest.identify(inventory))
        inventory['distro'] = {}
        inventory['artifacts'].append(dict(note, metadata={'os':'mariner','osVersion':'2.0'}))
        self.assertIsNone(guest.identify(inventory))
        self.assertIsNone(guest.identify(dict(artifacts=[])))


if __name__ == '__main__':
    unittest.main()
