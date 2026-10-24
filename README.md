# FractalCR

FractalCR is a serial-managed container appliance from **Fractal Microsystems**.
It combines an immutable Linux system image, a recovery monitor, persistent
storage, and an IOS-style CLI for containers and host management.

**Development preview.** No production-supported 1.0 firmware release is
available. Read [current status and known issues](docs/development-status.md)
before deploying. The startup/reload fix still requires image qualification.
FractalCR is an independent project, not Cisco software.

## Start here

- [Getting started](docs/getting-started.md): requirements, builds, first boot,
  administrator setup and a first container.
- [Documentation index](docs/README.md): operator guides, developer references
  and historical engineering evidence.
- [CLI reference](os/CLI.md): configuration, networking, storage and workloads.
- [Troubleshooting](docs/troubleshooting.md): boot, connectivity and recovery.

```sh
git clone --recurse-submodules https://github.com/highenergymagic/fractalcr.git
cd fractalcr
```

The three component repositories are pinned by this checkout. GitHub's automatic
source ZIP omits them; use a recursive clone. Builds require a Linux x86-64 Docker
host. Windows users can build over SSH and run the VM with QEMU/WHPX.
See [building](docs/building.md) for commands and resource requirements.

## What it provides

- A Buildroot recovery monitor with serial, USB and IPv4/TFTP recovery.
- An OpenEmbedded system with a CIP kernel and embedded, read-only userspace.
- A native supervisor with health checks and dependency-aware restarts.
- Local users, privileges, SSH, persistent configuration and appliance networking.
- Docker Engine, Compose, Buildx/BuildKit, Swarm and Kata container interfaces.

The initial target is x86-64 UEFI with a **115200 baud, 8N1 serial console**.
There is no local monitor/keyboard setup. Ordinary containers do not require KVM;
Kata requires working KVM, including nested virtualization when the host is a VM.
Features and external providers have different qualification coverage; see the
[conformance record](docs/ios-management-conformance.md).

Firmware verifies the recovery monitor when Secure Boot is enabled. The monitor
always authenticates the system image. Development builds create their own keys;
keep each build's ROMMON, system image and public trust files together.
[Signing policy](docs/releases.md) explains operator-managed production keys.

## Contributing and security

Read [CONTRIBUTING.md](CONTRIBUTING.md) before changing a component or its pin.
Report vulnerabilities through the private channel in [SECURITY.md](SECURITY.md).
First-party code is covered by [LICENSE](LICENSE); upstream components retain
their own licenses.

Fractal Microsystems — Copyright (c) 2025–2026.
[Technical support](https://www.fractalmicro.org/techsupport).
