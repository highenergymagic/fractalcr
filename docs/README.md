# Documentation

Begin with [getting started](getting-started.md). Commands run from the root of a
recursive integration checkout unless stated otherwise. `ctr#` denotes appliance
EXEC mode, `(config...)#` configuration mode, and `rommon N >` the recovery monitor.
Do not type prompt prefixes. Replace uppercase placeholders such as `BUILD_HOST`.

## Operate an appliance

| Task | Guide |
| --- | --- |
| Build, boot and configure your first VM | [Getting started](getting-started.md) |
| Understand requirements and boot media | [System platform](../os/README.md) |
| Configure networking, containers, storage and time | [CLI reference](../os/CLI.md) |
| Configure users, SSH and password recovery | [Authentication and management](../os/docs/aaa-and-management.md) |
| Write USB images or provision a VPS | [Image deployment](../os/docs/releases.md) |
| Diagnose boot, networking and persistence | [Troubleshooting](troubleshooting.md) |
| Recover an image over USB or TFTP | [ROMMON guide](https://github.com/highenergymagic/fractalcr-rommon/blob/main/README.md) |
| Inspect services and boot messages | [Boot behavior](../os/BOOT-UX.md) |

## Develop and maintain

| Task | Guide |
| --- | --- |
| Build locally or over SSH; export artifacts | [Build instructions](building.md) |
| Understand repositories and trust boundaries | [Architecture](architecture.md) |
| Submit a change and run checks | [Contributing](../CONTRIBUTING.md) |
| Update pinned dependencies | [Dependency maintenance](dependencies.md) |
| Understand native API integration | [Native services](../os/docs/native-services.md) |
| Understand the supervisor | [Service implementation](https://github.com/highenergymagic/fractalcr-supervisor/blob/main/README.md) |
| Understand image authentication | [Image format](https://github.com/highenergymagic/fractalcr-rommon/blob/main/FORMAT.md) |
| Sign releases or rotate keys | [Release policy](releases.md), [key management](https://github.com/highenergymagic/fractalcr-rommon/blob/main/docs/release-key-management.md) |

## Release status and qualification

[Release status](development-status.md) lists supported environments, known
issues, and outstanding qualification. [Management conformance](ios-management-conformance.md)
maps implemented behavior and restrictions to the IOS XE 17.12 reference.

For build acceptance procedures, use [Firmware qualification](../os/docs/cli-and-container-qualification.md)
and [Three-node Swarm qualification](../os/docs/swarm-qualification.md).
Qualification reports identify the exact images tested; results do not transfer
to another build.

Security and distribution references:

- [Security assessment](security-review.md)
- [Recovery architecture](https://github.com/highenergymagic/fractalcr-rommon/blob/main/docs/isr4331-research.md)
- [Upstream source trust](https://github.com/highenergymagic/fractalcr-rommon/blob/main/upstream-trust/README.md)
- [Go advisory dispositions](../tools/security/go-findings.md)
- [Source distribution and reporting](publication-review.md)

## Terms used in these guides

| Term | Meaning |
| --- | --- |
| ROMMON | The ROM Monitor recovery environment that authenticates and boots the system image. |
| CIP | Civil Infrastructure Platform, the source of the maintained kernel baseline. |
| OE / OpenEmbedded | The build system and metadata used to assemble the appliance userspace and kernel. |
| UKI | Unified Kernel Image: an EFI-bootable kernel and embedded boot resources. |
| Bootflash | Storage for the monitor, boot settings and authenticated system image. |
| Datastore | Persistent Btrfs storage for configuration and container data; currently `data0`. |
| Running / startup configuration | Active settings / settings saved for the next boot. |
| EXEC / VTY | Operational CLI mode / a virtual terminal line used by an SSH session. |
| KVM / WHPX / TCG | Linux virtualization / Windows virtualization / QEMU software emulation. |
| Kata / CLH | VM-isolated containers / Cloud Hypervisor, the supported Kata hypervisor. |
| Source build ID | A content identity recorded by a build; it is not necessarily a Git commit ID. |
