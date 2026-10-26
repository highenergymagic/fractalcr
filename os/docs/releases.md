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
- A USB drive at least as large as the 2 GiB uncompressed boot image; use a
  nominal 4 GB or larger drive. Check the supplied image manifest for its size.
- At least one separate, unused disk for data0. Setup refuses partitioned disks;
  back up and prepare the intended data disk before installation.
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
5. Complete the serial setup dialog. Select one or more non-USB data disks,
   choose `single` or `raid1`, and type the exact `ERASE` confirmation shown for
   those disks. This destroys their existing partitions and filesystems. Then
   configure accounts and management networking.

The image contains a 1 GiB FAT32 EFI System Partition with the loader and signed
system image. It contains no datastore. The USB provides `bootflash:`; `data0:`
resides on separate disks and holds configuration and container data. The USB
must remain attached for subsequent boots. The development trust anchor is
specific to the build. Keep the matching ROMMON and system image together.

### Verification

Run `show version`, `show storage datastore data0`, `show interfaces`, and
`show platform software service`. Confirm system identity, mounted storage,
management addressing, and enabled service readiness. Save with `write memory`.
See [Troubleshooting](../../docs/troubleshooting.md) for boot or service failures.

## Hypervisor and VPS deployment

### Prerequisites and disk installation

Compare the download with `SHA256SUMS` and decompress `hv.qcow2.gz`. Import
`hv.qcow2` as the VM's system disk. It contains both
bootflash and data; it has no backing file. Select x86-64 UEFI, a serial console,
at least 4 GiB RAM, and firmware Secure Boot disabled. An extra NoCloud seed disk
is removable provisioning media, not a second persistent system disk. Nested
virtualization is required only for Kata, not ordinary containers.

### Optional NoCloud provisioning

FractalCR supports **NoCloud local-media configuration** for initial accounts
and management networking. The hypervisor or VPS provider needs to support
custom qcow2 disks and an attached NoCloud seed. User-data scripts, HTTP metadata
services, and other cloud-init data sources are unsupported.

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
physical ROMMON configuration-recovery workflow. Create each new VM from the
blank release image; cloning a configured disk also copies its accounts, SSH
host keys, and container data.

The datastore is sized when the image is built; increasing the virtual disk does
not automatically expand its GPT partition or Btrfs filesystem.

## Related information

- [CLI reference](../CLI.md)
- [Software releases and image signing](../../docs/releases.md)
- [Publishing development releases](../../docs/publishing.md)
- [cloud-init NoCloud media format](https://docs.cloud-init.io/en/latest/reference/datasources/nocloud.html)
