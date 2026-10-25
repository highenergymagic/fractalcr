# Software releases and image signing

FractalCR development releases are published on the
[GitHub releases page](https://github.com/highenergymagic/fractalcr/releases).
Each release contains installation images, checksums, source, and test reports.
See [Image deployment](../os/docs/releases.md) for installation instructions and
[Release status](development-status.md) for known issues.

## Downloads

| File | Description |
| --- | --- |
| `usb.img.gz` | Whole-disk image for physical hardware with firmware Secure Boot disabled |
| `usb-secureboot.img.gz` | Whole-disk image for hardware with the release's firmware certificate enrolled |
| `hv.qcow2.gz` | Standalone hypervisor disk containing bootflash and the datastore |
| `nocloud-examples.tar.gz` | Example seed files for initial SSH-key provisioning |
| `SHA256SUMS` | Checksums for the release assets |
| `release-manifest.json` | Source revision and artifact inventory |
| `source.tar.gz` | Source archive including the pinned component repositories |

The release also supplies public signing certificates, package and advisory
metadata, and qualification reports. The reports identify the image hashes and
the checks performed on those images. GitHub's automatically generated source
ZIP omits the component repositories; use `source.tar.gz` for a complete checkout.

## Development signing

The release workflow generates a new development signing identity for each
build. ROMMON contains the public keys needed to authenticate its matching
system image. Private signing keys are not included in downloads.

With Secure Boot enabled, UEFI verifies ROMMON's EFI signature. ROMMON then
verifies the system image before starting it. With Secure Boot disabled,
ROMMON continues to verify the system image, but firmware no longer
authenticates ROMMON.

## Installing and upgrading

Compare the downloaded image's SHA-256 with its entry in `SHA256SUMS`, then
follow the [USB or hypervisor installation procedure](../os/docs/releases.md).
Checksums detect download corruption; image signatures are checked during boot.

Use the ROMMON, system image, and public trust files from the same release.
Because each development build has a new signing identity, an older ROMMON
normally rejects a system image from a newer build. Upgrading therefore includes
both ROMMON and the system image. Secure Boot installations also need the new
firmware certificate enrolled.

Updates do not provide automatic A/B recovery after a power interruption. Keep
a working image and external recovery media available during an upgrade.
ROMMON's [recovery procedures](https://github.com/highenergymagic/fractalcr-rommon/blob/main/README.md#usb-recovery)
cover USB and TFTP image recovery.

## Using your own signing identity

Custom builds can use an operator-managed signing identity. The build worker
receives public trust material; a separate signing workstation holds the private
keys and signs the completed artifacts. The [key-management guide](https://github.com/highenergymagic/fractalcr-rommon/blob/main/docs/release-key-management.md)
covers key creation, encrypted storage, offline signing, verification, rotation,
and revocation.

ROMMON enforces a minimum signed security epoch set when it is built. Images
within the same accepted epoch remain interchangeable. Raising the minimum
requires a new ROMMON; retiring the old ROMMON also requires revoking its
firmware signer. The minimum does not advance automatically during normal boot.

## Release maintainers

See [Publishing development releases](publishing.md) for runner requirements,
manual builds, and version-tag publication.
