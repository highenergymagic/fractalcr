# Security assessment and release requirements

This document records targeted review findings and defines security acceptance
for FractalCR firmware. An independent comprehensive audit remains outstanding.
Use the advisory metadata and qualification reports supplied with the selected
release; findings must be evaluated against its actual package versions and hashes.

## Findings and disposition

The findings below were recorded during the October 2026 source review.
Disposition records describe those changes; evaluate the selected release
against its supplied inventories and current advisories before deployment.

| ID | Finding | Impact / status |
| --- | --- | --- |
| FCR-001 | Boot fetch could replace the archive but keep compiling previously extracted kernel/Buildroot source | High supply-chain correctness risk. Fixed with hash-marked extraction and dependent output invalidation; pin-change regression test. |
| FCR-002 | OE fetch accepted modified files when HEAD already matched its pin | High reproducibility/integrity risk on a polluted cache. Fixed by rejecting dirty checkouts and changed origins. Regression tests cover modified and untracked files and origin substitution. |
| FCR-003 | Layer copying retained deleted recipes in an incremental workspace | Medium build consistency risk. Fixed by deleting stale destination files before supplying generated first-party sources. |
| FCR-004 | Digest-pinned Debian base still installed packages from moving repositories; OS inherited local latest parent | Medium reproducibility risk. Snapshot repositories, hash-locked wheel, verified content-derived parent tag and builder identity recording added. Fixed inputs do not establish byte-identical OCI layers. |
| FCR-005 | Buildroot 2025.02.12 and Kata 4.2.0 are behind supported upstream releases | Updated source inputs to Buildroot 2025.02.18 and Kata 4.3.1. Buildroot archive checked against its maintainer's signed checksum. New image qualification is required before promotion. |
| FCR-006 | Go 1.26.9 includes GO-2026-6809; fixed in 1.26.10 | OE compiler, management CI and source-built container tools updated to 1.26.10. Compiler and first-party race tests passed. Scan each resulting executable; an imported package is not by itself a reachable vulnerable call. |
| FCR-007 | Incomplete image SBOM, CVE disposition and source reproduction of upstream binary bundles | Still a release gate. OE package SPDX/source identities and separate host/guest scanners added. Compose, Buildx, BuildKit, CNI and Kata's Go runtime build from source. Source recipes for Cloud Hypervisor and virtiofsd are under qualification. Kata guest and QEMU helper bundles remain imported. Stripped guest package metadata limits OS advisory completeness. |
| FCR-008 | Image format v1 accepts older correctly signed releases | V2 authenticates a 64-bit security epoch. ROMMON enforces an embedded minimum; v1, tampered epochs and lower epochs fail verifier tests. This is a fixed trust-generation floor, not an automatically advancing monotonic counter. Strong retirement also requires revoking old ROMMON firmware signatures. |
| FCR-009 | SSH host-key checks and private audit/vault inputs could encounter substituted or special files | Descriptor-based no-follow, nonblocking opens with same-descriptor owner/mode/type/link-count and size checks. Tests reject symlinks, FIFOs, shared inodes and exposed private inputs. |
| FCR-010 | Vendored runtime dependencies lag security updates; some imported Go tools use older compilers | Runtime pins and reviewed vendor patches updated; Docker 29.9.2, containerd 2.2.10 and runc 1.5.2 selected. New binaries still require image-level advisory disposition and VM qualification. |
| FCR-011 | Vendor patch export omitted deleted files and upstream-ignored new files | Fixed patch generation against HEAD with forced intent-to-add and explicit delete/add pairs. GNU patch application to pristine source reproduced all five refreshed trees byte-for-byte. Regression test covers deletion, rename and ignored files. |
| FCR-012 | Kernel allocator, copy and list hardening options were disabled | Enabled allocator randomization/hardening, hardened usercopy, fortification, allocation initialization, list hardening and restricted kernel logs. OS defaults unprivileged BPF off. Both builds assert required resolved options. |
| FCR-013 | Windows development archives preserved CRLF shell scripts and lacked a precise input manifest | Deterministic snapshot tool normalizes text, preserves binaries, rejects private keys/links and records file hashes. Remote build and VM-test wrappers use it; release archives still require clean Git revisions. |
| FCR-014 | Deterministic snapshot mtimes could hide changed files from rsync and BitBake's persistent checksum cache | Content-checking source synchronization, explicit OE input timestamp invalidation and rebuilding the small local ROMMON package prevent stale source reuse. Compiler and content-addressed sstate caches remain available. |
| FCR-015 | Buildroot's latest release still selects OpenSSL 3.5.8, predating September security fixes | Pin ROMMON OpenSSL to 3.5.9 through the external-tree override and upstream archive hash. Track this maintenance line in dependency discovery. ROMMON uses RSA/SHA-256 verification, but shipping obsolete unused crypto code is unnecessary. |
| FCR-016 | The external 2024 Bootlin toolchain imports musl 1.2.5 and bypasses current Buildroot libc fixes | Switch ROMMON to Buildroot's source-built musl 1.2.6 with its maintained security backports. Invalidate compiler/rootfs/kernel outputs when toolchain configuration or override inputs change. This requires a new toolchain and complete ROMMON rebuild. |
| FCR-017 | An archive hash marker did not detect subsequent edits to extracted source | Compare cached source bytes, modes, links and path inventory to the authenticated archive before reuse. Modified and untracked source regression tests added. This detects polluted caches; it does not make a compromised build host trustworthy. |
| FCR-018 | Stripped guest distro metadata caused the advisory scanner to return misleading zero matches | Select the Azure Linux 3 feed only when consistent ELF package notes identify it, retaining the supporting evidence. Distinguish binary package notes from a real package database. Add an independent OE source-CPE scan with recorded backport dispositions. |
| FCR-019 | Imported VMM binaries contain Rust dependencies with newer advisories | Cloud Hypervisor 54.0 includes rustls 0.23.45 and webpki 0.103.15; virtiofsd 1.14.0 includes older time/logger dependencies. Add source builds with rustls 0.23.46, webpki 0.103.16 and refreshed virtiofsd dependencies, including replacing its atty-dependent logger. Frozen inputs and selected Linux feature trees are recorded. Image compilation and VM qualification remain required. |


## Review procedure

1. Record the integration revision, component pins, source manifests, builder
   identity, and signed image hashes.
2. Review the image verifier, recovery input handling, supervisor control,
   management authorization, SSH, configuration persistence, and signing tools.
3. Run source-fetch regression tests, Go race/vet checks, parser fuzzing, native
   sanitizer tests, negative image-signature tests, and image acceptance suites.
4. Generate the host SBOM and inventory the Kata guest and runtime components
   separately. A guest without a package database requires an explicit inventory;
   zero scanner matches do not establish complete coverage.
5. Review every advisory against the shipped version, applied backports, selected
   build features, and reachable code. Retain unresolved findings in release notes.
6. Verify source/license materials, clean-build reproduction, hardware recovery,
   and firmware-key revocation before production acceptance.

## Dependency evidence

Kernel backports and exclusions are recorded in
`boot/kernel/security/kernel-security.json` and mirrored in the OE recipe.
Verify derived patches with `tools/security/verify-kernel-backports.py`; retain
the base CIP identity in the image metadata. Review the
[CIP security tracker](https://gitlab.com/cip-project/cip-kernel/cip-kernel-sec)
when updating kernel inputs.

For Go management findings, see [Advisory dispositions](../tools/security/go-findings.md).
For all dependency updates, use [Dependency maintenance](dependencies.md). Review
Kata guest libraries and Rust VMM dependencies independently of the host Go scan.
An exception for one package, symbol, or image hash does not apply to another image.

## Trust boundaries and open acceptance items

Privilege 15 and physical recovery are administrative trust boundaries. The
persistent audit log is protected from unprivileged accounts but remains writable
by host root. Datastore encryption and tamper-evident remote accounting are not
provided. Optional Secure Boot leaves ROMMON unauthenticated by firmware, while
system-image signatures remain mandatory.

Independent trust-boundary review, sustained fuzzing, complete advisory and license
disposition, fresh-builder reproduction, and physical hardware coverage remain
release acceptance items. CVE-2026-31692, concerning peer network-namespace
capability checks, remains unresolved for the selected kernel. The 6.12
implementation uses a different peer-creation path from the upstream fix;
version comparison alone does not establish its disposition. No runtime
qualification is recorded for that finding. Refer to
[Release status and restrictions](development-status.md) for the startup/reload
regression and to [Release policy](releases.md) for update and rollback restrictions.
