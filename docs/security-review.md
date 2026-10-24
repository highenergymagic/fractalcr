# Security review and remediation

For the current development checkpoint, see [development status](development-status.md).
The dated review and build notes below are historical evidence, not a statement
that those jobs are still running or that later images inherit their results.

Date: October 22, 2026 UTC. Status: targeted first-party review and automated
checks, **not an independent or exhaustive security audit**. Existing VM
qualification describes the previously exported image hashes; this review does
not recertify those images after changing build infrastructure.

## Development checkpoint — October 22

This commit preserves the remediation work on private `main`; it does not
approve a release or replace the previously exported interactive images.
All 24 Linux tooling tests pass. Both refreshed Rust VMMs compile from frozen
source inputs and pass OE packaging QA. The two kernel backport bodies reproduce
exactly from their checksum-pinned upstream updates with the recorded exclusions.
Earlier Go race/vet, bounded fuzzing, C sanitizer and signing tests are described
below with their scope limitations.

At this checkpoint carbon is building the OS from `candidate-v25` and running
ROMMON reproduction from `candidate-v24`. The offline image scan waits for the
OS build to succeed. Logs are under `~/FractalCR-security/`:

- `image-v25.log` and `image-v25-exit-code`
- `boot-reproduce-v24.log` (completion status: `~/FractalCR-build/exit-code`)
- `image-v25-audit.log` and `image-v25-audit-exit-code`

Packaging/signing/export of the OS, final image advisory disposition, both-image
reproducibility and the new Sascha VM qualification remain pending. No automatic
promotion is queued. The network-namespace advisory remains open for review;
the proposed isolated runtime probe was not run and is not included in this
checkpoint. Source review notes are retained in
`out/security-remediation/kernel-source-review-notes.json`. Do not treat the
absence of that test as evidence that the finding is fixed or exploitable.

## Findings and disposition

| ID | Finding | Impact / status |
| --- | --- | --- |
| FCR-001 | Boot fetch could replace the archive but keep compiling previously extracted kernel/Buildroot source | High supply-chain correctness risk. Fixed with hash-marked extraction and dependent output invalidation; pin-change regression test. |
| FCR-002 | OE fetch accepted modified files when HEAD already matched its pin | High reproducibility/integrity risk on a polluted cache. Fixed by rejecting dirty checkouts and changed origins. Regression tests cover modified and untracked files and origin substitution. |
| FCR-003 | Layer copying retained deleted recipes in an incremental workspace | Medium build consistency risk. Fixed by deleting stale destination files before supplying generated first-party sources. |
| FCR-004 | Digest-pinned Debian base still installed packages from moving repositories; OS inherited local latest parent | Medium reproducibility risk. Snapshot repositories, hash-locked wheel, immutable local parent ID and builder identity recording added. Fixed inputs do not establish byte-identical OCI layers. |
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

The Go module-level GO-2026-5932 finding concerns unmaintained x/crypto/openpgp.
That package is neither imported nor included in this project's vendor tree.
It is distinct from the SSH package used here. Sources:
[GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809),
[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932).

## Review coverage

The additional kernel CPE scan returned 423 advisory matches. A first comparison
against CIP's security tracker identified 127 with a recorded 6.12 fix no later
than the pinned base; the rest require source/configuration review, not blanket
suppression. Some relevant fixes, including TCP CVE-2026-64024, are recorded in
6.12.114 after the current CIP release. Shared ROMMON/OS backport inputs now cover
the 6.12.113 and 6.12.114 stable updates, excluding version-label changes and
the conflicting Renesas ARM irqchip changes outside this x86-64 target. One
Renesas DRM update already present in CIP is also excluded; DRM is disabled.
The original and derived patch hashes and exclusions are recorded in
`boot/kernel/security/kernel-security.json`. This remains a modified CIP
6.12.112-cip33 build, not an official newer CIP release. Patch application,
rebuilt images and VM tests must pass before these updates are qualified.
Sources: [CIP security tracker](https://gitlab.com/cip-project/cip-kernel/cip-kernel-sec),
[Linux stable updates](https://cdn.kernel.org/pub/linux/kernel/v6.x/incr/).

Read the ROMMON verifier and recovery parser, supervisor control socket and
bootstrap, management socket/session handling, local AAA and SSH server,
configuration persistence, subprocess dispatch, signing vault tooling, source
fetching and builder assembly. Confirmed the verifier passes a sealed read-only
descriptor to kexec rather than reopening flash; inputs have size/type bounds;
Unix control endpoints require root peer credentials; SSH exposes a bounded PTY
service rather than forwarding or arbitrary shell execution. These observations
are review evidence, not proof that no defects remain.

Automated evidence is retained locally in `out/github-preparation`: Gitleaks
8.30.1 scanned the publishable source snapshot with no findings; govulncheck
1.9.0 analyzed Linux management sources with the image's Go 1.26.9 toolchain and
reported no reachable vulnerable symbols. It retained package/module findings
above. The Windows toolchain scan is separate and is not image evidence.
Source-fetch regression tests cover stale pins, corruption, traversal, dirty
checkouts and origin mismatch. GitHub runs management unit/race tests, vet,
vendor consistency and vulnerability reachability checks, plus trusted-main C
and source-fetch tests. Scanner databases and runner packages change over time;
record the report date and tool versions when evaluating results.

Remediation evidence is retained in `out/security-remediation`. Linux source
tests, C service tests under ASAN/UBSAN, Go race tests and vet passed. Separate
180-second CLI and configuration fuzz runs completed 303,026 and 2,459,044
executions respectively without a failure. These runs are useful bounded
checks, not sustained independent fuzzing.

The v2 C verifier also passed 512 deterministic signed-envelope mutations under
ASAN/UBSAN, plus oversized-file and FIFO rejection tests. The offline signer and
OVMF tests passed a raised epoch floor, refusal to lower that floor, migration
from legacy trust profiles, and staged firmware key revocation. The retired
ROMMON was rejected after revocation. These signing tests used disposable keys
and the preserved pre-toolchain-migration firmware fixture; they do not qualify
the newly rebuilt firmware's driver or recovery behavior.

The baseline image scan identified 394 module-level host advisory matches.
Counts are not distinct exploitable vulnerabilities: several packages have OE
backports or are libraries whose vulnerable server entry points are not shipped.
The baseline Kata guest returned zero matches while lacking a distribution ID
and package database. That is an explicit coverage failure, not a clean bill of
health. Selecting the correct Azure Linux feed exposed 21 matches in that old
guest. The pinned Kata 4.3.1 guest reduced those to two libuv matches. Its image
hash is `ece394c1cd8c4285811eda6352c434fdd57916b5d6cb573e5ff6d169369199ff`.
The report and exact ELF dependency inventory are retained under
`out/security-remediation` and the build host's `kata-4.3.1-audit` directory.

For that specific guest, CVE-2024-24806 is fixed in its detected libuv 1.48.0;
the embedded package note omits the RPM release suffix, causing comparison
against `1.48.0-1.azl3` to misclassify it. See the
[upstream advisory](https://github.com/libuv/libuv/security/advisories/GHSA-f74f-cvh7-c6q6).
CVE-2024-22017 concerns Node.js dropping privileges after creating libuv's
io_uring state. Node.js is absent. The guest's ELF dependency closure reaches
libuv only through BIND utility programs, none setuid/setgid; Kata agent,
systemd, chronyd, D-Bus and network services do not link it. This is a
default-service reachability disposition, not a claim that the library cannot
be misused by another program. See the
[Node.js advisory](https://nodejs.org/en/blog/vulnerability/february-2024-security-releases).
Neither match is globally suppressed. Guest package completeness, Rust/VMM
dependencies and source reproduction still require separate evidence.
New reports must be tied to new image hashes and reviewed individually.

The Kata 4.3.1 agent also contains rustls-webpki 0.103.15. Its selected dependency
tree reaches that crate through jsonschema's HTTP resolver, used by the CDI
library. The reviewed agent calls CDI's built-in-schema validator: its schema
and definitions are compiled into the library, all references resolve locally,
and the container's document is a validation instance rather than a schema.
The source hashes, references and selected dependency-tree hash are retained in
`out/security-remediation/kata-agent-cdi-evidence.json`. This supports a limited
reachability disposition for that CDI path. It does not patch webpki, prove all
binary paths unreachable, or authorize suppressing the advisory in other images.

The rebuilt ROMMON with the internal musl toolchain passed all 13 boot/recovery
scenarios. Its signed EFI SHA-256 is
`a41cacd4ce7c781d5e5a8ccae1b5c3c74940ef62ada6680a5fe4efbd20187a70`.
The subsequent forced two-pass rebuild is still under qualification. OS
reproduction now explicitly rebuilds the Kata runtime and both source-built
VMM packages in addition to the appliance packages, rootfs and kernel; dependency
toolchain caches remain retained.

## Work required before a production security claim

Perform independent review of all trust-boundary code and sustained parser/SSH/
image fuzzing with sanitizers; threat-model compromised workloads and malicious
storage; inventory every image package, firmware blob and guest component;
generate SPDX and license/source materials; scan actual shipped binaries and
triage advisories with reachability evidence. Evaluate kernel configuration,
driver attack surface, container escape mitigations and privileged native CLI
semantics. Qualify clean builders, physical hardware, power-loss recovery,
anti-rollback, update transport and key-compromise recovery. Maintain an issue
for each open gate and bind evidence to exact artifact hashes.

The persistent audit log is protected from unprivileged users but is writable by
host root; it is not tamper-evident remote accounting. Datastore contents are not
encrypted by this design. Physical recovery can bypass startup AAA. Optional
Secure Boot cannot authenticate ROMMON. These are explicit trust boundaries,
not guarantees provided by the current platform.
