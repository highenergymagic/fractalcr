"""Reject private key material, except byte-pinned public upstream fixtures."""
import hashlib
import re

# This historical upstream file has a nonstandard test filename, so Go vendors
# it. Verified against the go.sum-pinned module cache on the build host. A path
# alone is never an exemption: any change requires reviewing the entire file.
PUBLIC_FIXTURES = {
    'vendor/github.com/digitorus/pkcs7/verify_test_dsa.go':
        'f7bf74cda7cf86c74febd919a6a169fd2062fa17ea0a38f2c303b5491e137872',
}


def check_private_keys(name, data):
    marker = rb'-----BEGIN (?:RSA |EC |OPENSSH |ENCRYPTED )?PRIVATE KEY-----'
    if not re.search(marker, data):
        return
    expected = PUBLIC_FIXTURES.get(name)
    if expected and hashlib.sha256(data).hexdigest() == expected:
        return
    raise ValueError('private key marker in export: ' + name)
