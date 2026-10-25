# Firmware qualification

This guide defines acceptance testing for FractalCR system images. Use the
[CLI reference](../CLI.md) to operate an appliance and
[Release status and restrictions](../../docs/development-status.md) to review
known issues before deployment.

## Prerequisites

- A recursive integration checkout matching the candidate image sources.
- A Linux x86-64 test host with QEMU, OVMF, KVM access, and the dependencies in
  the supplied builder images.
- Matching signed system and ROMMON artifacts, public trust, and build manifests.
- Empty output directories and sufficient space for disposable VM disks.
- Nested virtualization for Kata tests; approximately 12 GiB VM memory for the
  three-node Swarm test.

Keep production datastores and networks separate from acceptance fixtures.

## Required coverage

| Area | Checks |
| --- | --- |
| Console | Modes, abbreviation, contextual help, editing, pagination, interruption, and reboot persistence |
| AAA and SSH | Local authentication, command privileges, concurrent sessions, revocation, timeout, accounting, and physical recovery |
| Storage and setup | Blank disks, single-disk provisioning, configuration validation, persistence, and damaged-configuration recovery |
| Networking | Management/container separation, DHCP, routes, VLANs, bonds, Docker networks, and management-only SSH |
| Workloads | Container lifecycle, builds, Compose, Buildx, BuildKit, persistent data, diagnostics, and cleanup |
| Time | NTP synchronization, hardware-calendar policy, and saved-time restoration without an RTC |
| Kata | Runtime selection, network readiness, published ports, guest filesystem persistence, and isolation |
| VM factory | Delayed restore refreshes wall clock and randomness; independent roots; persistence after reboot |
| Swarm | Quorum, encrypted overlay, ingress, secrets/configs, failure recovery, rollback, and decommissioning |
| Recovery | USB/TFTP transport, signature rejection, ordered boot, cancellation, and saved monitor settings |

## Run management qualification

Run from the integration checkout on the KVM host:

```sh
python3 tools/qualify-management.py --artifacts /path/to/signed-artifacts --output /path/to/new-test-directory
```

The tool prepares disposable launcher disks and runs serial management, AAA/SSH,
and no-RTC clock tests. The result binds console reports to the system and ROMMON
hashes and clock results to the kernel and source identity. KVM is required.

## Additional image tests

The `os/tests/` harnesses cover container persistence, physical-media emulation,
Kata, VM factories, and Swarm. Select the scenario and required resources using
each script's `--help`. See [Three-node Swarm qualification](swarm-qualification.md)
for its test procedure. ROMMON uses its separate boot/recovery test suite.

The release workflow also boots the distributed USB images and checks the
single-disk hypervisor image with NoCloud provisioning and SSH. Physical-machine
qualification must record the motherboard, firmware settings, storage, adapters,
console connection, and image hashes.

## Interpret results

Require `passed: true`, all expected checks, and hashes matching the artifacts
to be distributed. Preserve failed results separately. Do not reuse an earlier
image's report for rebuilt or newly signed firmware.

A completed source test does not establish image behavior. Likewise, successful
VM tests do not establish hardware compatibility, sustained throughput, or
power-loss resilience. Test report directories may contain private test keys and
populated disks; publish only the explicit release-artifact allowlist.

## Runtime restrictions

Docker runtime policy applies to Docker-created containers, including Compose
workloads without an explicit override. Standalone BuildKit uses its own OCI
worker and requires separate isolation qualification.

Kata factory tests must launch a new workload after a measured delay and compare
its wall clock with the host. Restored guests must not retain the template's
creation time. Application bind mounts and volumes use fresh Kata VMs with
virtio-fs. Live archive/copy and commit operations on block-backed Kata containers
are rejected by the linked client.

Guest seccomp and SELinux are disabled in the current Kata profile. Host runc
seccomp results do not cover guest controls. Review guest and host dependencies
separately using the [security review procedure](../../docs/security-review.md).
