# FractalCR development firmware

An x86-64 container appliance from **Fractal Microsystems**. These images are
development signed and intended for evaluation and hardware qualification.

| Download | Use |
| --- | --- |
| `usb.img.gz` | Physical hardware, with firmware Secure Boot disabled |
| `hv.qcow2.gz` | Hypervisors and VPSes; one disk holds firmware and container data |
| `usb-secureboot.img.gz` | Physical hardware with the included firmware certificate enrolled |

Verify `SHA256SUMS`, then decompress your image. Write the USB image to an entire
drive (32 GB or larger); writing it erases the destination. Select x86-64 UEFI
boot and connect a **115200 baud, 8N1 serial console**. A monitor and keyboard
are not supported. First boot offers an interactive setup dialog.

For the hypervisor image, use UEFI with Secure Boot disabled, a serial console,
at least 4 GiB RAM, and a network adapter. Setup can be interactive or provisioned
with a NoCloud seed containing SSH public keys. Nested KVM is needed for Kata
isolation, not ordinary containers. There is no default password.

[Installation and NoCloud instructions](https://github.com/highenergymagic/fractalcr/blob/main/os/docs/releases.md)
explain firmware enrollment, disk layout, supported seed fields and first login.
[CLI reference](https://github.com/highenergymagic/fractalcr/blob/main/os/CLI.md)
and [troubleshooting](https://github.com/highenergymagic/fractalcr/blob/main/docs/troubleshooting.md)
cover operation and recovery.

CI checks USB boot, NoCloud provisioning and console/SSH management before
publishing. The included reports record what passed; they do not qualify your
physical motherboard. See [known limitations](https://github.com/highenergymagic/fractalcr/blob/main/docs/development-status.md)
and [release policy](https://github.com/highenergymagic/fractalcr/blob/main/docs/releases.md).

Each ephemeral build generates development signing keys. Keep the matching
ROMMON, system image and public trust files together when upgrading. Image
authentication remains mandatory even with firmware Secure Boot disabled.
Private signing keys are never included. `release-manifest.json` identifies the
source commit and asset hashes; `source.tar.gz` includes pinned component sources.

Fractal Microsystems — Copyright (c) 2025–2026.
[Technical support](https://www.fractalmicro.org/techsupport).
