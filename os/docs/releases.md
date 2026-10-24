# FractalCR release images

Fractal Microsystems — Copyright (c) 2025–2026.
Technical support: https://www.fractalmicro.org/techsupport

Images are x86-64 UEFI firmware with a serial console at 115200 8N1. These
releases are **development signed**. Image signatures remain mandatory even
when firmware Secure Boot is disabled. There is no default password or SSH
private key in an image.

## USB deployment

Verify SHA256SUMS before decompressing the selected `usb-*.img.gz`. Write the
decompressed image as a whole disk using your normal disk-imaging application;
this replaces the destination disk's contents. Use a USB drive of at least
16 GiB (a nominal 16 GB drive is too small). The image has a 1 GiB FAT32 ESP
containing the EFI loader and authenticated OS, and a Btrfs single-disk datastore.
The serial setup dialog configures accounts and networking on first boot.

`usb-optional` works with firmware Secure Boot disabled; `usb-required` requires
enrolling the accompanying firmware certificate into your firmware trust store.
The development key belongs to the build installation, not an offline release
authority. Keep the matching ROMMON and system image together. A build on a
different installation can use a different trust anchor.

## VPS deployment and cloud-init seed

Import the standalone `vps.qcow2` as the VM's only system disk. It contains both
bootflash and data; it has no backing file. Select x86-64 UEFI, a serial console,
at least 4 GiB RAM, and firmware Secure Boot disabled. An extra NoCloud seed disk
is removable provisioning media, not a second persistent system disk. Nested
virtualization is required only for Kata, not ordinary containers.

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

Provisioning atomically saves startup-config before services start. Subsequent
boots ignore seed changes once startup-config exists. Remove the seed after a
successful login. Key-only users cannot log into the serial console with a
password; configure a local secret through SSH if desired, or use the established
physical ROMMON configuration-recovery workflow. Clones must start from the blank
release image so they do not inherit accounts, host keys or container data.

The datastore is sized when the image is built; increasing the virtual disk does
not automatically expand its GPT partition or Btrfs filesystem.

## Cutting 1.0

1. Finish qualification, commit the ROMMON, supervisor and OE layer repositories,
   then commit their exact gitlinks and all integration sources in `fractalcr`.
2. Set `VERSION` to `1.0.0` and regenerate `service/release.h` with
   `python3 tools/version-metadata.py --output service/release.h`. Commit the
   supervisor header and updated gitlink before tagging the integration commit.
3. CI uses ordinary GitHub-hosted Ubuntu runners and development keys only.
   Cold OpenEmbedded builds are substantial; set the repository variable
   `FIRMWARE_BUILD_RUNNER` to an ordinary larger Linux x86-64 runner label if
   needed. There is no release-signing runner, vault, or release-key secret.
   Docker, Python 3, sufficient build storage and at least 16 GiB RAM are needed;
   KVM accelerates tests and TCG is used when it is absent. The three existing
   private-component read-key secrets authorize source checkout only.
4. Run **Release firmware images** manually on the release branch first. Inspect
   its build tests, NoCloud cold-boot/reboot SSH test, and uploaded firmware.
5. Tag the reviewed integration commit `v1.0.0` (or `v1.0`) and push that tag.
   The workflow checks VERSION, builds pinned components in Docker, runs firmware
   tests, packages both USB policies and qcow2, and publishes a development-signed prerelease only after the
   single-disk provisioning qualification passes.

Development keys are generated in the builder's work volume. An ephemeral hosted
runner therefore produces a new development trust anchor; distribute and upgrade
ROMMON and system.bin together. Never upload Docker work volumes or private
signing material.
Source archives, public firmware certificate, SBOM, advisory metadata, image
manifest, qualification result and SHA256SUMS accompany releases. Do not call
these offline-release-signed or HSM-backed images. Full-image reproducibility
and physical motherboard qualification are separate release acceptance checks.

## Display conventions

`show version` follows the layout of Cisco's IOS XE command: software identity,
support and copyright, ROM bootstrap, boot image, uptime, hardware inventory,
memory and storage, build identity, and configuration register. Values come from
the running system and ROMMON handoff; unknown history is not invented.

References: [Cisco IOS XE show commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/xe3se/3850/fundamentals-xe3se-3850-cr-book/show-commands-xe-3se.html),
[Cisco router show version example](https://www.cisco.com/c/en/us/support/docs/ios-nx-os-software/ios-xe-16/217045-troubleshoot-password-recovery-in-cisco.html),
[cloud-init NoCloud](https://docs.cloud-init.io/en/latest/reference/datasources/nocloud.html).
