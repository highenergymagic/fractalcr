import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa
from cryptography.exceptions import InvalidSignature

spec = importlib.util.spec_from_file_location('development_artifacts', Path(__file__).resolve().parents[1]/'verify-development-artifacts.py')
verify = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verify)

class ReleaseEnvelopeTests(unittest.TestCase):
    def test_signature_epoch_and_embedded_image_must_all_match(self):
        with tempfile.TemporaryDirectory() as directory:
            art = Path(directory)
            key = rsa.generate_private_key(public_exponent=65537,key_size=2048)
            (art/'release.pub').write_bytes(key.public_key().public_bytes(serialization.Encoding.PEM,serialization.PublicFormat.SubjectPublicKeyInfo))
            kernel = b'MZ' + bytes(range(256))*4
            (art/'system.efi').write_bytes(kernel)
            signed = kernel + (3).to_bytes(8,'big')
            envelope = signed + key.sign(b'FractalCR system image v2\n'+signed,padding.PKCS1v15(),hashes.SHA256()) + b'FCRBIN02'
            (art/'system.bin').write_bytes(envelope)
            with patch.object(verify.subprocess,'run') as run:
                verify.verify(art,{'minimum_security_epoch':3})
                self.assertEqual(run.call_count,2)
                with self.assertRaises(ValueError): verify.verify(art,{'minimum_security_epoch':4})
                (art/'system.efi').write_bytes(kernel+b'x')
                with self.assertRaises(ValueError): verify.verify(art,{'minimum_security_epoch':3})
                (art/'system.efi').write_bytes(kernel)
                (art/'system.bin').write_bytes(b'X'+envelope[1:])
                with self.assertRaises(InvalidSignature): verify.verify(art,{'minimum_security_epoch':3})
