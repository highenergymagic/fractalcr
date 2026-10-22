# Locked builds and reproducibility

Clone recursively as described in [architecture](architecture.md). Linux x86-64,
Docker, Git, Python 3.11+ and Bash are required. Allow at least 16 GiB RAM and
200 GiB disk for a comfortable OE build. Four compiler jobs are used on the
current builder. The Windows wrapper snapshots the same source to an SSH host.

```sh
# Linux with ordinary Docker group access:
FCR_DOCKER_DIRECT=1 bash tools/build.sh all All
# Linux with passwordless doas:
bash tools/build.sh all All
# Repeat appliance/rootfs/kernel generation and compare bytes:
bash tools/build.sh all Reproduce
```

```powershell
# Existing SSH build host; Docker Desktop is not involved:
.\tools\remote-build.ps1 -BuildHost carbon
.\tools\remote-build.ps1 -Action Status
.\tools\remote-build.ps1 -Action Export
```

The build never resolves a newer kernel or moving OE branch. Archive versions
and SHA-256 values live in `boot/sources.json`; the OE kernel recipe must agree.
Upstream layers use exact Git revisions. Go builds use the checked-in vendor
tree without network module resolution. Source-fetch caches reject dirty OE
repositories and unexpected remote URLs. Archive updates invalidate extracted
source and incompatible compiler outputs. Existing unmarked archive caches are
rebuilt once when adopting the new cache scheme.

ROMMON builds its musl compiler/libc through Buildroot rather than importing the
old Bootlin toolchain. This makes Buildroot's maintained libc security patches
part of the image. Toolchain configuration and security override changes
invalidate incompatible rootfs/compiler/kernel outputs; downloads and signing
keys survive that invalidation. OpenSSL 3.5.9 is pinned separately because the
Buildroot release predates that security update.
Extracted source is checked against its authenticated archive before reuse,
including unexpected files. Buildroot package downloads live outside the source
tree and survive source upgrades; old in-source download caches migrate once.

The builder base is pinned by registry digest. Debian package resolution uses
the October 20, 2026 snapshot with signed APT metadata. The minimal base lacks a
CA bundle, so the first CA-package installation uses HTTP with APT signature and
package-hash verification; subsequent requests use HTTPS. Snapshot expiration
checking is disabled intentionally, but signature checking remains mandatory.
The firmware Python wheel is hash-locked; its dependencies come from the same
APT snapshot, and `pip check` verifies compatibility. The OS builder receives
the just-built boot builder's immutable local image ID.

`tools/build-inputs.py` records source hashes, public trust fingerprints, installed
builder package versions and builder identity. `tools/reproduce.sh` forces two
appliance/rootfs/kernel regenerations and compares output bytes. This retains
dependency compiler caches: it is not a clean-room rebuild of the entire
toolchain. Fixed builder inputs also do not imply byte-identical Docker image
IDs: package scripts, layer metadata and image creation timestamps can vary.

For stronger release evidence, run on two independent fresh builders, retain
source archives and downloads, compare unsigned outputs, and investigate every
difference. Compose, Buildx, BuildKit, CNI helpers and the Kata Go runtime now
compile from pinned source using the Go 1.26.10 toolchain. Reviewed dependency
patches and deterministic vendor archives are checked into the OE layer; module
downloads and automatic toolchain selection are disabled during compilation.
Cloud Hypervisor and virtiofsd also build from pinned source with frozen Cargo
lockfiles, vendor archives and the checksum-pinned Rust 1.99.1 build toolchain.
Cargo cannot access registries or update dependencies during the build. The
selected Linux dependency trees are included with the image for review.
Kata's guest kernel/rootfs and BuildKit's cross-CPU QEMU helpers still enter
through hash-pinned release bundles. Their independent
source reproduction remains a release gate. The OE public sstate mirror is another trusted build input. A
source-only audit build must disable that mirror and use a fresh sstate cache.

The Windows development snapshot normalizes text line endings, preserves binary
inputs and records each file's SHA-256 in `workspace-manifest.json`. It rejects
links and private signing material. This snapshot includes working-tree edits;
release archives use `tools/source-archive.py` and require clean committed source.

OE emits package SPDX documents with source identities, kernel configuration
and package options. Per-file source enumeration is disabled; complete source
archives and download hashes are retained separately.
It also retains OE's source-version/CPE inventory and recipe-level CVE patch
dispositions, including the embedded kernel, as `system.oe-vex.json`.
An actual image can additionally be scanned using `tools/security/scan-rootfs.sh`
with arguments `rootfs.cpio.gz NEW-AUDIT-DIRECTORY system.oe-vex.json`
inside a disposable Linux audit container. It inventories the host and nested
Kata guest separately, pins the scanner downloads, records the vulnerability
database identity and explicitly reports missing package metadata. Where the
Kata guest lacks `os-release`, consistent embedded ELF package notes identify
the Azure Linux 3 advisory feed; the selected feed and evidence are recorded.
Source-version findings must be reconciled against OE's recorded backports;
the scanner does not automatically suppress them. A zero-match
report for a stripped guest is not approval to release it.

Do not publish builder volumes, whole artifact directories, interactive disk
images or firmware variable stores. Development keys live in build volumes.
See [release policy](releases.md) for the public artifact allowlist.
