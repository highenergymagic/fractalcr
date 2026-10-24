# Underlying container OS

This is the next layer after the tested ROM Monitor. OpenEmbedded builds the
glibc userspace and CIP SLTS kernel, then bundles the complete root into the
kernel. The existing firmware and release keys sign the resulting `system.bin`.
The boot chain and image format are unchanged.

The acceptance suite includes Linux QEMU scenarios, physical-media emulation,
and a native Windows QEMU smoke test. See `out/carbon/os/validation.md` for the recorded sizes,
image hashes, completed results and scope. `system.bin` includes the embedded
root and Docker.

Use `show version` to read the running image's source-build identifier. An
updated `system.bin` alone does not replace an existing VM's flash image:
the launcher needs a matching flash template and `-UpdateImage`. On a Linux
build host, `python3 tools/prepare-launcher.py --artifacts SIGNED_ARTIFACTS
--output NEW_DIRECTORY` produces both firmware-policy templates, an empty
datastore template, and a manifest of their hashes. It never opens an existing
output directory. Copy the prepared files into the launcher's artifact directory;
`-UpdateImage` preserves the separate interactive data disk and saved configuration.
The launcher rejects unknown switches and warns when an existing bootflash does
not match the current prepared template. That can mean an older OS or intentional
ROMMON edits; it never silently overwrites bootflash to resolve the difference.

The metadata is pinned to exact commits from the Wrynose LTS series in
`sources.json`. Yocto identifies Wrynose as its 6.0 LTS series, supported until
April 2030. [Release catalogue](https://www.yoctoproject.org/development/releases/)

The distro disables Wrynose's automatically appended feature defaults and
selects its own small feature set. Desktop features and package test suites
are excluded; locale generation is limited to `en_US.UTF-8`. The offline
container acceptance probe is built separately as part of the appliance.

## Build

### Physical boot media

After a successful boot and OS build, create a new compressed GPT disk image:

```powershell
.\tools\make-media.ps1 -FirmwarePolicy optional
# Firmware Secure Boot with this build's enrolled certificate:
.\tools\make-media.ps1 -FirmwarePolicy required
# ROMMON plus empty datastore, ready for recovery image transfer:
.\tools\make-media.ps1 -LoaderOnly -FirmwarePolicy optional
```

These commands write image **files** on carbon and export them under `out/media`.
The default raw size is 7 GiB, suitable for an 8 GB or larger USB drive, SATA disk
or NVMe device. Write the decompressed `.img` to the entire selected device with
an image writer; this replaces that device's partition table. The image contains
an EFI System Partition with `EFI/BOOT/BOOTX64.EFI`, ROMMON environment and
`system.bin`, followed by an empty single-device Btrfs datastore. Loader-only
media omits `system.bin`. No test users, startup configuration, containers or SSH
host keys are included. Filesystem and partition UUIDs are generated per image.

Boot x86-64 UEFI and connect the first UART at **115200 baud, 8N1**. The appliance
has no VGA console or keyboard setup. Required-policy images need the exported
`firmware.crt` enrolled in firmware db; optional-policy media supports firmware
Secure Boot disabled while retaining mandatory OS signatures. The setup dialog
starts on the blank datastore. Larger destination disks retain the image's
7 GiB layout; automatic partition expansion is not currently performed.

Included device families cover virtio, Intel e1000/e1000e/igb/igc and Realtek
r8169 Ethernet, USB CDC Ethernet/NCM, RTL8152 and ASIX USB Ethernet, USB mass
storage/UAS, AHCI and NVMe. Hardware needing additional firmware or drivers must
be added to the pinned kernel configuration and qualified on that machine.

`os/tests/qemu-physical.py` exercises USB, AHCI and NVMe device paths with a
single GPT disk, serial setup, two NICs, DHCP, Docker networks, NTP and cold
reboot. These are emulated hardware tests, not a motherboard certification.

### Development builds

First build the boot milestone so that the signed ROMMON, keys and firmware
fixtures exist in `fractalcr-boot-work`. Then:

```powershell
.\os\build.ps1
.\tools\remote-build.ps1 -Action Status
# After the remote build and tests finish:
.\os\build.ps1 -Action Export
```

The wrapper uses SSH and passwordless `doas` to run Docker on `carbon`
(four CPUs, 16 GiB RAM). Docker Desktop is not involved. At least 4 GiB of
build-host RAM is required; the appliance VM's test allocation is 4096 MiB.
To build and test both layers in order on a fresh host, run
`.\tools\remote-build.ps1` instead. Jobs survive SSH disconnects.

The build uses a separate `fractalcr-os-work` Docker volume. The boot volume is
mounted read-only; private keys stay there. BitBake runs as an unprivileged
build user. The builder disables Docker's seccomp filter to allow BitBake's
user-namespace sandbox; it does not use a privileged container or host devices.

OE's public shared-state cache and hash-equivalence service are configured.
Unmatched dependencies are built locally. The first build includes a toolchain
and Go; subsequent builds retain downloads and shared-state results. Parallelism
is limited to two tasks/four compiler jobs on hosts with at least 12 GiB RAM.
Smaller builders use one task, with two compiler jobs for GCC's cross-compiler
because its generators use more memory than typical C compilation.
Network-only shared-state availability checks use sixteen concurrent requests.
Go recipes receive their own process and heap limits; those settings do not
change cache keys for unrelated C/C++ recipes.
Do not run two BitBake builds against this volume simultaneously.

The wrapper supports `-Action Build`, `-Action Test` and `-Action Export`.
Build and Test start remote jobs; use `tools/remote-build.ps1 -Action Status`
or `-Action Logs` to inspect them. Export copies public outputs into
`out/carbon/os`, keeping the earlier local artifacts intact.
An image being built is not a passed test: inspect `result-os-*.json` and the
associated serial transcripts for completed validation.

## Platform boot

1. ROMMON authenticates the complete `.bin` and kexecs its signed kernel.
2. PID 1 creates the minimal bootstrap mounts and starts the service graph.
3. The platform service prepares runtime/cgroup filesystems and makes the root read-only.
4. The datastore service finds existing Btrfs labelled `FRDATA` and mounts `/datastore/data0`.
5. Configuration and networking services validate and apply JSON startup settings.
6. The container engine starts after its dependencies are ready, with its data
   root on the datastore and its API restricted to a Unix socket.

No startup path formats a disk. Missing storage or invalid configuration leaves
Docker stopped and reports a diagnostic state. Datastore creation and RAID
policy are explicit operations in the [management CLI](CLI.md).

The image uses Docker/Moby, containerd and runc from the pinned
meta-virtualization recipes. Docker uses overlay2, the unified cgroup hierarchy,
seccomp and a persistent local log driver. The supervised SSH service exposes
the appliance CLI when explicitly enabled with local AAA. There is no graphical
console, getty or host Unix-shell endpoint; privileged diagnostics use the
explicit debug container.

The root bind mount is created before the runtime filesystems and made
read-only before datastore/configuration startup. This avoids duplicate proc/sys mounts while
leaving runtime and datastore mounts writable. The initramfs includes bootstrap
console/null device nodes; PID 1 later mounts devtmpfs. Iptables' userspace
extensions are installed explicitly because optional package recommendations
are disabled.

The supervisor sets Moby's `DOCKER_RAMDISK=1` because Linux cannot use
`pivot_root` from its initial rootfs. Moby passes `NoPivotRoot` to runc, which
uses its mount-move/chroot path. Namespace, cgroup, seccomp and capability
isolation remain enabled and are checked by the container acceptance probe.

## Initial startup-config schema

Path: `/datastore/data0/config/startup-config.json`.

```json
{
  "version": 1,
  "hostname": "ctr",
  "network": {
    "interface": "eth0",
    "address": "10.0.2.15/24",
    "gateway": "10.0.2.2",
    "dns": "10.0.2.3"
  }
}
```

`version` and `hostname` are required when a configuration file exists.
`network` is optional; if present, an interface is required. It may use DHCP,
a static IPv4 address/prefix, or no address. Gateway and DNS are optional static
IPv4 settings on the management interface. No configuration file means the
default hostname and no external interface address. Host networking here is
separate from ROMMON's recovery network configuration.

The management service extends this schema with container definitions,
datastores and isolation policy. See [CLI.md](CLI.md) for configuration modes,
immediate running configuration, explicit candidate transactions, persistence
and recovery.

## Appliance console and tests

`os/tests/qemu-ios-workflow.py` exercises the account, `login local`, hostname,
Ctrl-Z and `write memory` workflow without `commit`, then logs out and reboots.
It also checks Ctrl-D, rejected edits, unchanged Docker PID/start count, and
saved versus unsaved configuration. It supports KVM and Windows WHPX, requires
prepared launcher templates and their expected source-build identity, and writes
only to a new disposable output directory:

```powershell
$build = python tools/build-identity.py
python os/tests/qemu-ios-workflow.py --artifacts out/carbon/os --output out/ios-session-test --accelerator whpx --expected-source-build $build
```

The independent `qemu-aaa.py` suite covers console and SSH authentication,
privileges, session termination, concurrent edits, expiry, account revocation,
accounting, reboot persistence and physical password recovery.

The custom PID 1 supervises the [modular service graph](../service/README.md),
including the serial console as its own restartable process.
Normal boot reports appliance stages, then waits for RETURN and displays the
configured hostname prompt. Raw kernel and daemon output is kept in bounded
volatile logs; it appears only when requested. See [BOOT-UX.md](BOOT-UX.md) for
Cisco source references, restart policy, log limits and the full command list.

| Command | Action |
|---|---|
| `show platform` | Show datastore, image and container-service state |
| `show platform software service` | Show process state, PID and restart counters |
| `show logging kernel` | Inspect retained kernel diagnostics |
| `show logging container-engine` | Inspect Docker/containerd diagnostics |
| `request platform software service container-engine restart` | Restart the supervised service |
| `request platform diagnostics containers` | Run offline container checks |
| `show logging diagnostics` | Inspect the detailed test report |
| `reload` | Stop services, sync storage and reboot |

Operational and configuration commands use validated argument lists without
shell passthrough. A separate management service owns configuration and actions.

The offline test image contains a small static C program built by OE. It tests
PID isolation, dropped capabilities, seccomp, no-new-privileges, memory/PID
cgroup limits, a read-only container root and persistent bind-mount writes.
Separate invocations test bridge networking and a published TCP port.
The QEMU harness reboots the machine and checks the persisted counter again.
No registry access is needed.

After a successful build and export:

```powershell
.\os\run.ps1 -Fresh
```

The VM uses 4096 MiB, a disposable bootflash image and a separate Btrfs data
image. Changes persist in the interactive copies. `-Fresh` replaces those
copies with the tested fixtures. The harness also checks missing-datastore
and malformed-JSON startup cases.

The Windows launcher uses WHPX hardware acceleration with `-NoSecureBoot`:

```powershell
.\os\run.ps1 -NoSecureBoot -UpdateImage
```

Use `-Accelerator tcg` to explicitly select software emulation. Secure Boot
launches default to TCG because WHPX cannot provide the SMM required by our
Secure Boot firmware; an explicit `-Accelerator whpx` requires `-NoSecureBoot`.
WHPX uses the installed QEMU's `share/edk2-x86_64-code.fd` and
`share/edk2-i386-vars.fd`, with a separate persistent firmware variable store.
It shares the existing optional-firmware bootflash and datastore. An accelerator
failure is reported without silently falling back to software emulation.
The WHPX profile uses `qemu64`; `host` caused a firmware general-protection
exception on the tested QEMU build. This profile does not expose nested KVM
for Kata; use the Linux KVM test host for Kata execution.

With TCG, allow up to ten minutes for boot: after `Image signature verified.`, the kernel's
independent authentication/load operation can take several minutes without
console output. Its next message is `Booting authenticated system image...`.
With `-NoSecureBoot`, `% Firmware trust is unavailable. Image signatures remain
mandatory.` is an informational notice immediately before that operation,
not a boot refusal. Image signature verification is still enforced.

Linux acceptance uses `core2duo,vmx=off`, matching the image's `core2-64`
baseline and exercising operation without KVM. With Debian QEMU 7.2.22,
`max,vmx=off,svm=off` produced a glibc loader assertion during container launch;
that synthetic CPU profile is not qualified. The native host smoke test uses
`max` separately. A separate `kata` acceptance case runs on sascha with nested
KVM; only a passing result bound to the exported image qualifies guest execution.

To use a newly exported image while retaining an existing interactive datastore,
run `.\os\run.ps1 -UpdateImage`. This replaces only interactive bootflash
(including its ROMMON boot settings); the datastore and firmware variables
remain in place. When no interactive copies exist, the launcher initializes them.

After the Linux QEMU tests pass and their artifacts are exported, the native
host smoke test boots disposable snapshots and runs the container checks again:

```powershell
python .\os\tests\native-smoke.py
# Windows hardware acceleration, with firmware verification explicitly optional:
python .\os\tests\native-smoke.py --accelerator whpx --no-report
```

It records `result-os-native.json` and `qemu-os-native.log` alongside the images
(`native-whpx` for WHPX).
The exported datastore and interactive disks are not modified by this test.

## Remaining layers

Kata 4.3.1, Cloud Hypervisor and the Mariner guest are image-bundled. The host
includes KVM/vsock/tun and traffic-control prerequisites. Carbon exposes no
`/dev/kvm`; its TCG tests do not establish nested-KVM operation. Use
`tools/test-sascha.ps1` for the additional KVM scenarios, including guest
networking, published ports and native/Kata runtime overrides. The image-bound
validation report identifies which scenarios passed.

The [release custody workflow](../boot/docs/release-key-management.md) separates
public build trust from encrypted offline signing keys and stages key revocation.
Still pending: per-release image upgrade/rollback policy,
multiple datastores, storage erase/reprovision,
and qualification on physical hardware. The CLH VM factory uses the optional
blockfile storage backend; see [CLI setup and storage behavior](CLI.md).
Global Kata selection
is inherited, with explicit per-container opt-out or opt-in.
