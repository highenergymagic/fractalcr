# FractalCR release images

Fractal Microsystems — Copyright (c) 2025–2026.
Technical support: https://www.fractalmicro.org/techsupport

Select a build from [Development releases](https://github.com/highenergymagic/fractalcr/releases)
and read its release notes and [known issues](../../docs/development-status.md).
For source builds, use [Building from source](../../docs/building.md).

Images are x86-64 UEFI firmware with a serial console at 115200 8N1. These
releases are **development signed**. Image signatures remain mandatory even
when firmware Secure Boot is disabled. There is no default password or SSH
private key in an image.

## USB deployment

### Prerequisites

- An x86-64 UEFI machine with a serial console and supported storage/network devices.
- A USB drive at least as large as the uncompressed image. The release workflow
  defaults to 16 GiB; use a nominal 32 GB or larger drive. Manual media defaults
  to 7 GiB. Check the supplied image manifest for the actual size.
- A disk-imaging application and a backup of any data on the destination drive.

### Procedure

1. Download `usb.img.gz` and `SHA256SUMS` from the same release. Compare the
   compressed file's SHA-256 against the corresponding entry before decompression.
2. Decompress the image and write it to the whole USB device. **This erases the
   destination's partition table and data.**
3. Disable firmware Secure Boot for `usb.img.gz`. To use Secure Boot, select
   `usb-secureboot.img.gz` instead and enroll its accompanying firmware certificate
   in the target machine's firmware trust store.
4. Connect the serial console at 115200 8N1, select UEFI USB boot, and power on.
5. Complete the serial setup dialog to configure accounts and management networking.

The image contains a 1 GiB FAT32 EFI System Partition with the loader and signed
system image, followed by a Btrfs datastore. The development trust anchor is
specific to the build. Keep the matching ROMMON and system image together.

### Verification

Run `show version`, `show storage datastore data0`, `show interfaces`, and
`show platform software service`. Confirm system identity, mounted storage,
management addressing, and enabled service readiness. Save with `write memory`.
See [Troubleshooting](../../docs/troubleshooting.md) for boot or service failures.

## Hypervisor and VPS deployment

### Prerequisites and disk installation

Verify SHA256SUMS and decompress `hv.qcow2.gz`. Import the standalone `hv.qcow2` as the VM's only system disk. It contains both
bootflash and data; it has no backing file. Select x86-64 UEFI, a serial console,
at least 4 GiB RAM, and firmware Secure Boot disabled. An extra NoCloud seed disk
is removable provisioning media, not a second persistent system disk. Nested
virtualization is required only for Kata, not ordinary containers.

### Optional NoCloud provisioning

The appliance consumes cloud-init **NoCloud local-media configuration**. It does
not run the cloud-init Python program, execute user-data scripts, or contact
provider metadata HTTP services. Providers must allow a custom qcow2 and a
NoCloud seed disk; EC2, OpenStack config-drive, and provider-specific metadata
are not interchangeable with NoCloud.

Extract `nocloud-examples.tar.gz`. Replace the public-key placeholder in
`user-data`, set a unique `instance-id` and hostname in `meta-data`, then create
an ISO9660 or FAT seed labelled `CIDATA`, containing those files at its root.
For example, on a provisioning workstation:

```
cloud-localds seed.img user-data meta-data
```

Attach it before first boot. With one physical/virtual NIC the appliance selects
that NIC for management DHCP. With multiple NICs, also provide `network-config`
(see `network-config.example`) selecting exactly one management MAC with DHCPv4.
Other interfaces are not enabled for management. Connect using `ssh -t admin@IP`;
SSH opens the appliance CLI, not a Unix shell. Supplied users receive privilege
15 and key-only authentication. Top-level `ssh_authorized_keys` creates `admin`.

Supported fields are `instance-id`, `local-hostname`, `hostname`, `users[].name`,
`users[].lock_passwd: true`, `users[].ssh_authorized_keys`, top-level
`ssh_authorized_keys`, and `ssh_pwauth: false`. Unknown fields, scripts, ambiguous
interfaces, malformed keys, and password-unlocking requests fail provisioning.
Network configuration supports the documented version-2 MAC-matched DHCP form.
Static addresses can be set later through the serial CLI; arbitrary cloud-init
network configurations are not supported.

### Verify provisioning

After SSH login, inspect `show users`, `show interfaces`, and `show startup-config`.
Confirm the expected account and management address before removing the seed.
Treat configuration output as sensitive when collecting diagnostics.

Provisioning atomically saves startup-config before services start. Subsequent
boots ignore seed changes once startup-config exists. Remove the seed after a
successful login. Key-only users cannot log into the serial console with a
password; configure a local secret through SSH if desired, or use the established
physical ROMMON configuration-recovery workflow. Clones must start from the blank
release image so they do not inherit accounts, host keys or container data.

The datastore is sized when the image is built; increasing the virtual disk does
not automatically expand its GPT partition or Btrfs filesystem.

## Development releases

Run **Release firmware images** from the repository's Actions tab on the desired
branch. After all qualification gates pass, CI publishes a uniquely named
`dev-YYYYMMDD-RUN-ATTEMPT` prerelease for that exact commit. It includes
`usb.img.gz`, `hv.qcow2.gz`, and the optional `usb-secureboot.img.gz`, checksums,
public trust files, source, and qualification reports. Failed builds publish no
release. Manual runs publish prereleases too; they are not artifact-only dry runs.

Builds are deliberately on demand or version-tag triggered: documentation pushes
do not start a cold firmware build. Physical hardware testing follows CI's QEMU
qualification; passing CI does not establish motherboard compatibility.

## Publish a versioned development release

The following procedure uses `1.0.0` as an example version. Substitute the
intended version consistently in `VERSION`, generated metadata, and the tag.

1. Finish qualification, commit the ROMMON, supervisor and OE layer repositories,
   then commit their exact gitlinks and all integration sources in `fractalcr`.
2. Set `VERSION` to `1.0.0` and regenerate `service/release.h` with
   `python3 tools/version-metadata.py --output service/release.h`. Commit the
   supervisor header and updated gitlink before tagging the integration commit.
3. CI uses ordinary GitHub-hosted Ubuntu runners and development keys only.
   Cold OpenEmbedded builds are substantial; set the repository variable
   `FIRMWARE_BUILD_RUNNER` to an ordinary larger Linux x86-64 runner label if
   needed. The workflow generates development keys; it does not use production
   signing credentials.
   Docker, Python 3, sufficient build storage, at least 16 GiB RAM, and KVM are
   needed. Media tests can use TCG, but the serial/SSH management gate requires
   KVM and will fail rather than skip qualification. Public component checkouts
   require no deploy-key secrets.
4. Run **Release firmware images** manually on the release branch first. Inspect
   its build tests, NoCloud cold-boot/reboot SSH test, serial IOS and AAA reports,
   and uploaded firmware.
5. Tag the reviewed integration commit `v1.0.0` (or `v1.0`) and push that tag.
   The workflow checks VERSION, builds pinned components in Docker, runs firmware
   tests, packages both USB policies and qcow2, and publishes a development-signed prerelease only after the
   single-disk provisioning, both USB images, and serial/SSH management
   qualification pass. `management-qualification.json` binds the management
   reports to the released system image hash.

Development keys are generated in the builder's work volume. An ephemeral hosted
runner therefore produces a new development trust anchor; distribute and upgrade
ROMMON and system.bin together. Never upload Docker work volumes or private
signing material.
Source archives, public firmware certificate, SBOM, advisory metadata, image
manifest, qualification result and SHA256SUMS accompany releases. Full-image
reproducibility and physical motherboard qualification are separate acceptance
checks. See [Signing and release policy](../../docs/releases.md) for operator-managed
signing identities.

## Related information

- [CLI reference](../CLI.md)
- [Signing and release policy](../../docs/releases.md)
- [cloud-init NoCloud media format](https://docs.cloud-init.io/en/latest/reference/datasources/nocloud.html)
