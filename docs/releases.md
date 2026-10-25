# Software signing and release policy

This guide describes development-image trust and the requirements for publishing
FractalCR firmware. For installation, see [Image deployment](../os/docs/releases.md).

## Development images

GitHub releases contain development-signed images. Each ephemeral build creates
a firmware key and an image-signing key. Distribute the matching `rommon.efi`,
`system.bin`, `firmware.crt`, and `release.pub` together. Private keys are excluded
from release assets and CI caches.

UEFI verifies ROMMON when Secure Boot is enabled. ROMMON always verifies the
system image. Disabling firmware Secure Boot does not disable system-image
authentication; it removes firmware authentication of ROMMON itself.

## Operator-managed signing

Operators can build with their own public trust bundle and sign the resulting
images on a separate signing workstation. The
[key-management guide](https://github.com/highenergymagic/fractalcr-rommon/blob/main/docs/release-key-management.md)
provides procedures for key creation, encrypted storage, offline signing,
verification, rotation, and revocation. Keep private keys and passphrases off
build workers and source-control services.

## Release contents

Each release must identify the integration revision, component revisions,
dependency inputs, builder identity, image hashes, public trust, and qualification
results. Include the recursive source archive, package inventory, license
materials, SBOM, and advisory metadata. GitHub's automatic source ZIP omits
component repositories; use the supplied `source.tar.gz`.

Only publish the exact images covered by the qualification reports. A rebuild
or a change of signing key produces different artifacts and requires new tests.
The release workflow runs on demand and on version tags. It publishes a
development prerelease only after the required image tests pass.

## Verification

1. Compare downloaded files with `SHA256SUMS` before installation.
2. Inspect `release-manifest.json` for the source revision and artifact inventory.
3. Inspect the USB, hypervisor, and management qualification reports for passed
   checks and matching image hashes.
4. Verify that ROMMON and the system image use the trust bundle intended for
   the target appliance.

Checksums detect changed files; the embedded signatures authenticate firmware.
A Git tag alone does not provide an independent cryptographic source signature.

## Restrictions

- ROMMON enforces a signed security epoch against its embedded minimum. Retiring
  an epoch requires an updated ROMMON and revocation of the old firmware signer.
  The minimum does not advance automatically after each boot.
- Power-loss-qualified A/B installation is not implemented. Retain a working
  image and external recovery media when updating.
- Production acceptance requires hardware qualification, independent security
  review, complete advisory and license disposition, and reproducibility evidence.
  See [Release status and restrictions](development-status.md).
