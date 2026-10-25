# FractalCR

**A container host that behaves like an appliance.**

FractalCR turns an x86-64 machine into a dedicated container platform with an
IOS-style command line, an immutable operating system, and a recovery monitor.
Configure storage, networking, users and workloads over serial or SSH. Save your
configuration. Replace the firmware image to update the system.

Built by **Fractal Microsystems** for people who want to operate a container host
like a piece of infrastructure, with a consistent management interface from
first boot through recovery.

## Try it

[**Development downloads**](https://github.com/highenergymagic/fractalcr/releases)
· [Installation guide](os/docs/releases.md)
· [Documentation](docs/README.md)
· [Current status](docs/development-status.md)

| Image | Target |
| --- | --- |
| `usb.img.gz` | Write to a USB drive and boot physical hardware |
| `hv.qcow2.gz` | Import into a hypervisor as a single firmware-and-data disk |

Both use x86-64 UEFI and a **serial console at 115200 baud, 8N1**. First boot
offers a setup dialog for storage, management networking and accounts. The VM
image also accepts a NoCloud seed for initial SSH configuration. Use a 32 GB or
larger drive for the USB image. There is no local monitor/keyboard setup.

Images are published as **development prereleases after CI qualification**.
Check the downloads page for available builds; an unsuccessful build publishes
no release. Physical hardware qualification is ongoing. Read the
[known issues](docs/development-status.md) before deploying.

## What makes it different

- **One management interface.** Context-sensitive help, command abbreviations,
  configuration modes, local AAA and persistent startup configuration give host
  administration an appliance workflow.
- **Firmware you replace as a unit.** The system runs from a read-only image.
  Configuration and container data live on the datastore, separate from the OS.
- **Recovery built in.** A serial ROM Monitor can recover images from USB or
  IPv4/TFTP. It authenticates the system image before booting it.
- **Containers are the purpose.** Docker Engine, Compose, Buildx/BuildKit and
  Swarm have appliance CLI interfaces, with Kata available for VM isolation.
- **Local setup or automated provisioning.** Configure a standalone machine
  interactively, or seed a hypervisor deployment with SSH public keys.

```text
ctr# configure terminal
ctr(config)# hostname edge01
edge01(config)# end
edge01# write memory
```

The [CLI reference](os/CLI.md) covers workloads, Docker networks, interfaces,
storage, accounts, time synchronization and recovery. Feature qualification is
tracked in the [conformance record](docs/ios-management-conformance.md).
FractalCR is an independent project, not Cisco software.

## Trust and updates

Development images use development keys. Firmware Secure Boot is optional;
system-image authentication is always enforced. A separate
`usb-secureboot.img.gz` image supports firmware certificate enrollment.
Keep each build's recovery monitor, system image and public trust files together.
Operators can manage their own production keys using the
[signing procedure](docs/releases.md).

## Build and contribute

This repository is the project's entry point, documentation and integration
build. It pins the recovery monitor, supervisor and system layer so that a source
checkout identifies the components used together.

```sh
git clone --recurse-submodules https://github.com/highenergymagic/fractalcr.git
cd fractalcr
```

See [building from source](docs/building.md) for Linux Docker builds and remote
builds from Windows, and [CONTRIBUTING.md](CONTRIBUTING.md) for the component
workflow. GitHub's automatic source ZIP omits component repositories; release
`source.tar.gz` archives include them.

Report vulnerabilities through [SECURITY.md](SECURITY.md). First-party code is
covered by [LICENSE](LICENSE); included upstream software retains its own licenses.

Fractal Microsystems — Copyright (c) 2025–2026.
[Technical support](https://www.fractalmicro.org/techsupport).
