#!/usr/bin/env python3
"""Check the packaged envelope, embedded PE and public trust before distribution."""
import argparse
import json
from pathlib import Path
import subprocess
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa

def verify(art, policy):
    data = (art/'system.bin').read_bytes()
    if len(data) <= 272 or data[-8:] != b'FCRBIN02':
        raise ValueError('invalid system envelope')
    key = serialization.load_pem_public_key((art/'release.pub').read_bytes())
    if not isinstance(key, rsa.RSAPublicKey) or key.key_size != 2048:
        raise ValueError('unexpected image signing key')
    key.verify(data[-264:-8], b'FractalCR system image v2\n'+data[:-264], padding.PKCS1v15(), hashes.SHA256())
    if int.from_bytes(data[-272:-264],'big') < policy['minimum_security_epoch']:
        raise ValueError('retired system image epoch')
    if data[:-272] != (art/'system.efi').read_bytes():
        raise ValueError('system envelope does not contain the qualified signed kernel')
    for name in ('rommon.efi','system.efi'):
        subprocess.run(['sbverify','--cert',str(art/'firmware.crt'),str(art/name)],check=True)

if __name__ == '__main__':
    p=argparse.ArgumentParser()
    p.add_argument('--artifacts',type=Path,required=True)
    p.add_argument('--policy',type=Path,required=True)
    a=p.parse_args()
    verify(a.artifacts,json.loads(a.policy.read_text()))
