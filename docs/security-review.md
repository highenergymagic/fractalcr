# Security controls and assessment

This reference describes the security controls in FractalCR and the information
available for assessing a build. For vulnerability reporting, see
[Security policy](../SECURITY.md). FractalCR is a development preview; an
independent comprehensive security audit has not been completed.

## Administrative access

Privilege 15 grants host administration, including container host mounts and
the privileged debug container. Physical console and firmware access permit
password recovery. These forms of access are trusted administration; they are
not tenant isolation boundaries.

Local AAA protects console and SSH sessions. Authentication fails closed when
credentials or configuration cannot be read. Command authorization is checked
when an operation runs, including after an interactive confirmation. Account
changes revoke existing sessions. See [Authentication and management](../os/docs/aaa-and-management.md).

The local audit log is protected from unprivileged accounts but remains writable
by host root. Remote tamper-evident accounting and datastore encryption are not
provided.

## Boot and recovery

With Secure Boot enabled, firmware authenticates ROMMON. ROMMON authenticates
the system-image envelope and passes the verified payload through a sealed
memory descriptor to the kernel, which checks its PE signature. Optional Secure
Boot policy removes firmware authentication of ROMMON; system-image verification
remains enabled.

The image's signed security epoch must meet ROMMON's embedded minimum. The floor
is fixed at build time. Retiring an older ROMMON requires firmware-signer
revocation. See [Image signing](releases.md) and the
[image format](https://github.com/highenergymagic/fractalcr-rommon/blob/main/FORMAT.md).

## Build-input controls

| Input | Validation |
| --- | --- |
| Kernel and Buildroot archives | Pinned SHA-256; cached source files, modes, links, and inventory checked against the archive |
| OpenEmbedded layers | Exact Git revisions; modified checkouts and unexpected remote URLs rejected |
| Builder image and packages | Registry digest, dated Debian package snapshot, signed package metadata, and recorded parent image identity |
| Go dependencies | Checked-in vendor trees or reviewed vendor patches; offline module and toolchain selection |
| Rust dependencies | Pinned compiler, frozen lockfiles, and vendor archives used for offline builds |
| Development snapshots | Normalized text line endings, binary preservation, file-hash manifest, and rejection of links or signing-key material |
| Release source archives | Clean committed source and matching component revisions |

Changed inputs invalidate the associated cached outputs. The source synchronization
also removes obsolete recipe files and refreshes BitBake's input checksums.
[Dependency maintenance](dependencies.md) describes the update procedures.

## Runtime controls

Host kernels enable allocator hardening, hardened usercopy, fortification,
allocation initialization, and list hardening. Unprivileged BPF is disabled by
default in the system image. Kernel logs are restricted.

Management file access uses bounded reads and descriptor-based validation of
file type, ownership, permissions, and link count. Native API entry points
restrict executable creation. The supervisor accepts enumerated lifecycle
operations over a local socket and uses image-defined service executables.

Kata uses a separate guest kernel and filesystem. Guest seccomp and SELinux are
disabled in the current profile. Host runc tests do not cover those guest controls.
See [Native services](../os/docs/native-services.md) and
[Firmware qualification](../os/docs/cli-and-container-qualification.md).

## Package inventory and advisory reports

OpenEmbedded produces package SPDX documents and `system.oe-vex.json`, which
records source versions and recipe-level vulnerability dispositions. Kernel
backports are recorded in `boot/kernel/security/kernel-security.json` and
mirrored in the system kernel recipe.

The rootfs scanner inventories the host and nested Kata guest separately. For
stripped guest images, ELF package notes can identify the advisory feed, but
they do not provide a complete package database. Reports record that limitation.
The [management binary advisory review](../tools/security/go-findings.md) documents
the narrowly scoped Go findings handled by the source check.

To assess an image, collect its source revision, package inventories, applied
backports, scanner database identity, and signed image hashes. Evaluate reported
advisories against those inputs and the features included in the image. Build
and scan procedures are in [Building from source](building.md).

## Open assessment items

The [October 2026 custom-code review](custom-code-security-audit.md) records a
corrected console memory-safety defect, SSH teardown hardening, test results, and
the remaining review scope. It does not close the independent-review requirement.

For CVE-2026-31692, an isolated KVM test first reproduced unauthorized veth
peer creation in the VM host network namespace on the unpatched 6.12.112-cip33
image. The administrator-only debug service supplied test tools; the process
performing the operation ran as UID 65534 with all host capabilities dropped
and entered its own user and network namespaces. The signed
`6.12.112-cip33-security2-fractalcr` image built with the
[6.12 backport](../os/meta-fractal/recipes-kernel/linux/files/CVE-2026-31692-peer-netns.patch)
returned `Operation not permitted` for the same operation, created no host
interface, and still allowed a privileged control operation. This demonstrates
the kernel boundary and its repair; it does not establish reachability from a
default Docker container.

| Area | Status |
| --- | --- |
| Kernel peer network namespaces | The hash-pinned CVE-2026-31692 backport passed source, build, and KVM regression checks. Qualification of the distributed release images remains pending. |
| Guest inventory | Stripped Kata guest metadata limits package-level advisory coverage. |
| Imported binaries | Kata guest kernel/rootfs and BuildKit cross-CPU QEMU helpers are hash-pinned upstream bundles. Independent source reproduction is outstanding. |
| Updated runtime dependencies | Source-built VMM and runtime updates require qualification in the resulting firmware image. |
| Independent review | Comprehensive trust-boundary audit, sustained fuzzing, and complete advisory/license assessment remain outstanding. |
| Recovery and hardware | Power-loss-qualified A/B updates and target-machine hardware qualification are incomplete. |

These items are separate from successful compilation or individual unit tests.
[Release status](development-status.md) lists operational issues, including
the startup/reload regression. Release reports identify which image-level tests
completed for each published build.
