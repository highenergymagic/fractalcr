# FractalCR

Fractal Microsystems — Copyright (c) 2025–2026.
[Technical support](https://www.fractalmicro.org/techsupport).

A serial-managed container appliance: “Cisco IOS, but for containers.”

The private integration repository pins ROMMON, supervisor and OE layer
repositories at exact commits. Clone with `--recurse-submodules`; GitHub's source
zip does not include those components. Start with [repository architecture](docs/architecture.md),
[portable builds](docs/building.md), [dependency maintenance](docs/dependencies.md),
[release policy](docs/releases.md), and the [security review](docs/security-review.md).
See [USB/VPS image releases and initial SSH provisioning](os/docs/releases.md).
Current images are development-signed. This is not yet an independently audited
production release.

Development starts at the boot chain:

1. Buildroot ROM Monitor, signed EFI image, embedded verification keys.
2. Signed CIP kernel with embedded initramfs, verified and booted by ROMMON.
3. OE-built appliance OS: read-only root, persistent datastore, networking,
   Docker and first-class Kata support.
4. Appliance management CLI, JSON startup configuration, and a debug container.

Builds run in Docker on `carbon` over SSH, using passwordless `doas`.
Docker Desktop is not required. Start the boot → OS build and acceptance tests:

```powershell
.\tools\remote-build.ps1
.\tools\remote-build.ps1 -Action Status
.\tools\remote-build.ps1 -Action Logs
# Once finished:
.\tools\remote-build.ps1 -Action Export
```

The remote job survives SSH disconnects. It uses a separate source snapshot,
retains compiler/download caches in named volumes, and rejects overlapping
builds. `Exit code: 0` means the requested build/test sequence completed.
Exports live in `out/carbon/boot` and `out/carbon/os`, preserving the earlier
Docker Desktop artifacts. Development signing keys stay in the remote boot
volume; a fresh volume generates a new, internally consistent development
trust chain. Do not mix its firmware variables, ROMMON, or OS with older keys.

**The boot milestone is built and tested in Docker and native Windows QEMU:**
signed ROMMON verifies and boots a signed CIP kernel with an embedded initramfs,
and the negative signature/Secure Boot tests pass.
The witness is not the container host OS. The Go management service under
`cmd/fractal-management/` and `internal/` is built into the OE image.
`cmd/ctr` remains an offline configuration compiler.

See [boot/README.md](boot/README.md) for the build and QEMU test procedure and
[boot/FORMAT.md](boot/FORMAT.md) for the image format and trust chain.

The ROMMON recovery implementation adds numbered serial prompts, saved boot
variables, ordered autoboot, USB mass storage and Ethernet/TFTP recovery.
[ISR4331 research](boot/docs/isr4331-research.md) records the source evidence
and the recovery behavior implemented here.

**The underlying container OS now boots and passes Linux and Windows QEMU tests:**
a pinned OpenEmbedded build supplies the CIP kernel, glibc,
Docker/containerd/runc, and a small platform supervisor. Tests passed for
container isolation, bridge networking and port publishing, image/data
persistence across reboot, and diagnostic startup with missing storage or
malformed configuration. Reports bind results to image hashes in
`out/carbon/os/result-os-*.json`.

See [os/README.md](os/README.md) for the tested platform and startup schema.
After exporting, run `.\os\run.ps1 -NoSecureBoot -UpdateImage` for its serial
appliance console using Windows WHPX while retaining the interactive datastore.
Omit `-NoSecureBoot` to use the Secure Boot firmware under TCG.
The custom supervisor and operational console are described in
[os/BOOT-UX.md](os/BOOT-UX.md). [The CLI reference](os/CLI.md) covers live
configuration, containers, explicit storage initialization, Kata and debugging.

Secure Boot is required by default. ROMMON also supports
`SECURE_BOOT=optional`, saved with `sync`; outer image and inner kernel signatures
remain mandatory. Use `-NoSecureBoot` with either VM launcher to select separate
firmware/bootflash fixtures. Without firmware verification, authenticity of
ROMMON itself is not established.

ROMMON and the OS now share the [service supervisor and dependency graph](service/README.md).
That document covers service boundaries, restart ordering, degraded recovery,
and the forced two-build reproducibility check for both image stages.

The [CLI and container qualification contract](os/docs/cli-and-container-qualification.md)
records Cisco behavior, native Docker/Compose/Buildx/BuildKit access, test cases
and remaining production acceptance work. KVM and nested Kata tests run on
`freyja@sascha`, using disposable disk images under `/machines/fractalcr`.

[Appliance management and AAA](os/docs/aaa-and-management.md) documents local
users, privileges, SSH VTY, accounting, filters and physical password recovery.
[Three-node Swarm qualification](os/docs/swarm-qualification.md) covers encrypted
networking, secrets/configs, scheduling, manager failure and quorum recovery.
[Release key management](boot/docs/release-key-management.md) covers encrypted
offline custody, public-only builds, reviewed-image signing, and staged UEFI
key rotation and revocation.
