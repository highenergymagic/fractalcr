# Security review: initial GitHub preparation

Date: October 22, 2026 UTC. Status: targeted first-party review and automated
checks, **not an independent or exhaustive security audit**. Existing VM
qualification describes the previously exported image hashes; this review does
not recertify those images after changing build infrastructure.

## Findings and disposition

| ID | Finding | Impact / status |
| --- | --- | --- |
| FCR-001 | Boot fetch could replace the archive but keep compiling previously extracted kernel/Buildroot source | High supply-chain correctness risk. Fixed with hash-marked extraction and dependent output invalidation; pin-change regression test. |
| FCR-002 | OE fetch accepted modified files when HEAD already matched its pin | High reproducibility/integrity risk on a polluted cache. Fixed by rejecting dirty checkouts and changed origins. Regression tests cover modified and untracked files and origin substitution. |
| FCR-003 | Layer copying retained deleted recipes in an incremental workspace | Medium build consistency risk. Fixed by deleting stale destination files before supplying generated first-party sources. |
| FCR-004 | Digest-pinned Debian base still installed packages from moving repositories; OS inherited local latest parent | Medium reproducibility risk. Snapshot repositories, hash-locked wheel, immutable local parent ID and builder identity recording added. Fixed inputs do not establish byte-identical OCI layers. |
| FCR-005 | Buildroot 2025.02.12 is behind discovered LTS 2025.02.18; Kata 4.2.0 is behind 4.3.1 | Open dependency-maintenance work. Version lag alone is not a CVE determination. Review upstream changes, update inputs and requalify before promotion. |
| FCR-006 | Go 1.26.9 includes GO-2026-6809; fixed in 1.26.10 | Open toolchain update. Linux source analysis found the affected x509 package imported but no reachable affected Certificate.Verify call in this management program. Other Go executables in the appliance require independent scanning. |
| FCR-007 | No complete image SBOM, CVE disposition or source reproduction of upstream binary bundles | Open release gate. Package manifest and pinned recipes are useful inventories but insufficient for Kata's nested guest and bundled libraries. |
| FCR-008 | Image format v1 accepts older correctly signed releases | Open anti-rollback design gate. Trust-key rotation/revocation exists; per-release monotonic policy does not. |

The Go module-level GO-2026-5932 finding concerns unmaintained x/crypto/openpgp.
That package is neither imported nor included in this project's vendor tree.
It is distinct from the SSH package used here. Sources:
[GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809),
[GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932).

## Review coverage

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
