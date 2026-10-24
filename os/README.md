# FractalCR system platform

The system image combines a CIP kernel and OpenEmbedded glibc userspace in one
authenticated `system.bin`. Its root filesystem is read-only. Configuration,
container data and logs that require persistence live on a Btrfs datastore.

New users should start with [getting started](../docs/getting-started.md).
See [build instructions](../docs/building.md), [CLI commands](CLI.md) and
[known issues](../docs/development-status.md). All commands run from the
integration checkout root, not this directory.

## Requirements

- x86-64 UEFI and a serial console on the first UART, 115200 baud, 8N1.
- A datastore; prepared media supplies a single-device Btrfs filesystem.
- At least 4 GiB RAM for the standard evaluation VM, plus workload memory.
- A supported network adapter for registry access and remote management.
- Working KVM for Kata; nested virtualization if FractalCR itself runs in a VM.

There is no graphical setup, local keyboard console, Telnet server or host shell
login. SSH opens the appliance CLI. Privileged shell diagnostics are an explicit
[debug-container operation](CLI.md#debug-service-container).

## Physical media

After building both ROMMON and the OS on your SSH build host:

```powershell
.\tools\make-media.ps1 -BuildHost YOUR_BUILD_HOST -FirmwarePolicy optional
# With this build's certificate enrolled in firmware:
.\tools\make-media.ps1 -BuildHost YOUR_BUILD_HOST -FirmwarePolicy required
# Recovery loader and blank datastore, without a system image:
.\tools\make-media.ps1 -BuildHost YOUR_BUILD_HOST -LoaderOnly -FirmwarePolicy optional
```

These commands create image files on your build host and export them under
`out/media`. The manual-media default is 7 GiB (use an 8 GB or larger device).
Release-workflow media uses a different, larger default; consult its manifest
and [deployment guide](docs/releases.md). `-SizeMiB` changes manual media size.

Write the decompressed `.img` to the **entire destination device** with a disk
image writer. This replaces its partition table and data. It contains an EFI
System Partition with `EFI/BOOT/BOOTX64.EFI`, ROMMON settings and `system.bin`,
plus an empty Btrfs datastore. Loader-only media omits `system.bin` and requires
[recovery transfer](https://github.com/highenergymagic/fractalcr-rommon/blob/main/README.md). No accounts, host keys or containers are
preconfigured. Larger disks do not automatically expand the image's partitions.

Required-policy media needs its `firmware.crt` enrolled in firmware db. Optional
policy permits Secure Boot disabled but still requires authenticated system
images. Keep ROMMON, system image and trust files from the same signing identity.

Built-in device families include virtio, Intel e1000/e1000e/igb/igc, Realtek
r8169, USB CDC Ethernet/NCM, RTL8152 and ASIX USB Ethernet, USB mass storage/UAS,
AHCI and NVMe. A built-in driver does not establish compatibility with every
device revision or motherboard; devices needing external firmware need review.

## Boot and storage behavior

1. ROMMON verifies `system.bin` and hands its authenticated kernel to kexec.
2. PID 1 starts the supervisor and runtime mounts.
3. The datastore service mounts Btrfs labelled `FRDATA` at `/datastore/data0`.
4. Configuration and networking load saved settings.
5. Docker and other enabled services start after their dependencies are ready.

Boot never formats a disk. Missing storage or damaged configuration leaves
dependent services stopped. Use [setup and storage commands](CLI.md) or
[physical recovery](docs/aaa-and-management.md#physical-console-password-recovery).
The system currently supports one datastore, `data0`.

`show platform software service` reports readiness and failure state.
`show logging SERVICE` reads bounded diagnostic logs. Most service logs are in
RAM and disappear at reboot; AAA accounting is persistent. See
[boot and service behavior](BOOT-UX.md) for details.

## Windows VM lifecycle

Use [blank launcher templates](../docs/getting-started.md#2-prepare-a-blank-vm)
for a new installation. `os/run.ps1` retains interactive bootflash, datastore and
firmware variables. `-UpdateImage` replaces bootflash, including saved ROMMON
settings, while retaining the datastore. `-Fresh` resets the interactive disks
and discards their data. Specify `-ArtifactDirectory` to keep evaluations separate.

WHPX requires `-NoSecureBoot`; the SMM Secure Boot firmware uses TCG.
There is no silent fallback if acceleration fails. Allow several minutes for
authenticated handoff under TCG. See [troubleshooting](../docs/troubleshooting.md).

## Build and test references

Pinned OE metadata is in `os/sources.json`; the layer is in `os/meta-fractal`.
The build uses persistent Docker caches and source-built container tools.
[Build instructions](../docs/building.md) explain resource limits and input pins.

`tools/qualify-management.py` runs serial, AAA/SSH and no-RTC tests on Linux KVM.
`os/tests/qemu-physical.py` tests emulated USB/AHCI/NVMe paths and setup.
The [Swarm test](docs/swarm-qualification.md) and
[container qualification record](docs/cli-and-container-qualification.md) cover
additional runtime scenarios. Results are bound to exact image hashes; emulated
tests do not certify physical hardware. Generated `validation.md` and JSON
reports live in exported artifact directories, not in the source tree.
