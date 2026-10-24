# CLI behavior and container-stack qualification

Research and implementation: 2026-10-21. This is an independently implemented
container appliance. It does not contain Cisco firmware or implement the IOS
routing command set. Production readiness requires evidence beyond a working
command prompt; the acceptance tests below are part of that evidence.

## Native keyword CLI and CLH factory qualification (2026-10-24 UTC)

Source `9c93c2ca7e3fa8838b19d7232a9c3a65404cc4981f70bb8ac8287e6ba9bd16ea`
produced system SHA-256
`70669572fed30baa008835a386e6dfe5559407dadaac5d91cd6b3d975310e744`.
Twelve matching-image acceptance suites passed: IOS workflows on KVM and WHPX,
AAA, CLI editing/help, container persistence/diagnostics, Compose/Buildx/BuildKit,
blank-disk initialization, damaged-configuration recovery, USB setup, ordinary
Kata, CLH factory, and three-node Swarm. The Swarm harness now submits native
keyword commands, including `detach enable` and `force enable`. It verifies
encrypted traffic captures, ingress, secrets/configs, drain, leader loss, quorum
loss/recovery, update/rollback and complete cluster removal.

Go tests, race tests and vet passed on carbon. Parser fuzzing completed 48,679
executions without failure. The OE build checks that both embedded command
catalogs match the linked clients. Console acceptance rejects Unix option syntax
and checks native keyword help, operands and literal container arguments.

The factory suite uses Docker's blockfile snapshotter and the supervised
container-storage and VM-factory services. It verifies independent container
roots, persistent writes across container/service restart and full reboot,
refreshed guest clocks and randomness, network/port forwarding, and explicit
runtime overrides. Application bind mounts and volumes use fresh Kata VMs with
virtio-fs; shared-volume writes are tested. Live archive/copy and commit operations
on block-backed Kata containers are rejected by the linked API client.

Diagnostics now use one bounded cleanup budget, retry transient Engine failures,
and retain the exact owned container ID. Unit tests exercise error classification
and deadlines. VM acceptance verifies that every owned probe is removed, both
before and after reboot while another acceptance VM is active.

A subsequent reproducibility check found that recording physical sparse extents
made the compressed seed depend on host allocation layout. The packer now records
fixed nonzero pages and pins the filesystem epoch. Sparse and fully allocated
fixtures produce identical archives. Two runs with OE's pinned e2fsprogs produce
seed SHA-256 `bd3ff7c23c11ad51d1a504c1fc94db19fa03e845014312e59d280be8cb014614`.
The seed retains exactly the same logical filesystem bytes as the qualified seed.

The follow-on source identity is
`7254114c849594360a3f73ac0bbec6d11d12931ce51f588ae50ed7571b774588`.
Its signed system image SHA-256 is
`bd597b2d2f047013f445f3db90ecbb2f7703567380907e5bc77cb226f01b286e`.
A comparison of all 1,410 initramfs entries found only two changes: the build
identity and the seed archive encoding. Executables, configuration defaults,
permissions and filesystem seed contents are unchanged. Final-image acceptance
passed the full CLH factory scenario, ordinary container/diagnostic persistence,
and the WHPX IOS workflow. These tests and the regression reference are recorded
separately in the qualification
manifest; results are never assigned to another image hash. Full-image
bit-for-bit reproduction and physical motherboard qualification are separate
from this seed reproducibility and VM acceptance evidence.

## Expanded API CLI qualification (2026-10-23)

The management CLI embeds catalogs generated from the linked Engine, Compose,
Buildx and BuildKit command objects. CI and the OE recipe regenerate and compare
them. The Engine frontend catalog currently contains 252 visible descendants;
the BuildKit catalog contains 29, including framework help commands. These are command counts, not counts of unique
features or a claim that every external provider has been qualified.

The CLI expansion passed `go test -race ./...` and `go vet ./...` on carbon.
Final parser fuzzing in this iteration completed 47,597 executions in 30 seconds.
Focused tests cover interspersed options versus opaque container argv, contextual
help, abbreviations, configuration replay, persistence and privilege boundaries.
Native API qualification against carbon's Engine exercised container resources,
health checks, multiple networks and lifecycle. Linked client tests exercised
Compose lifecycle and root, image and builder build aliases with subprocess
execution denied in the frontend.

On sascha, a fresh isolated network namespace passed native MTU, IPv4/IPv6,
static routes, bond creation, VLAN addressing and reverse-order cleanup. A
separate disposable Btrfs filesystem passed read-only snapshot creation and
listing, preservation, deletion, scrub, balance and persisted maintenance results.
Neither test modifies the host's management network or a real datastore disk.

The OE image build subsequently exposed a host-dependent setup test: it selected
a container veth from `net.Interfaces` even though setup requires physical ports.
Setup now receives its physical inventory through the dialog instance, and the
unit test supplies a fixed inventory and checks rejection of an unlisted VLAN.
The focused regression and the OE management test suite pass with this fix.

The source revision `799f36a71ed4b30669eb218992a53ad8505ac788e096c24f622328c75f26abe3`
produced system image SHA-256
`3a4cea68391256f72168d4d1b3f359beafaa01b16347b0e5caf4edbb3c9b22cb`.
Its AAA suite passed local and SSH authentication, concurrent configuration,
session revocation, idle expiry, accounting, reboot persistence and ROMMON
password recovery. Its workload suite passed builds with executed RUN steps,
Compose deployment, standalone BuildKit, appliance file paths, context
export/import, Buildx history/link output, single-node Swarm and cancellation.
The expanded IOS workflow passed on KVM and WHPX, including maximum-length
escaped input, configuration and authentication persistence, and native VLAN/bond
creation. Single-disk USB setup passed management/container DHCP separation,
management-only SSH, bridge/macvlan networks, actual NTP synchronization and three
network-restart/cold-boot cycles. Each result records the exact system and ROMMON
hashes. The container-persistence, explicit blank-disk RAID1 initialization, and
damaged-configuration recovery regressions also passed. Nine matching-image
results gate launcher promotion, together with artifact hashes and a comparison
of runtime source files against the immutable build archive.

One concurrent regression run completed its container checks but exceeded the
diagnostics worker's five-second cleanup deadline. Its failure transcript is
retained separately; it is not counted as a passing result. This exposes load
sensitivity in the former diagnostic timeout. The bounded cleanup change and
matching-image regressions above address this failure.

Three-node Swarm and nested Kata passed on the immediately preceding candidate
`64c8553e2ea16b42fb4257c2f150eac53311854de9cc8832293e47abb98a5b21`
(system SHA-256 `fe2e0105b4aaebe02fb32ba807b126b0063d97ed48f6cad9b00b4e3bd0b27c09`).
Swarm covered encrypted overlay traffic, ingress, secrets/configs, manager loss,
quorum restoration, update/rollback and cluster removal. Kata covered networking,
runtime overrides and fresh wall clocks across a launch delay. These results
are not relabeled as tests of the final image. The subsequent source changes
adjusted management request framing and inherited BuildKit options. Factory
qualification is a separate `kata-factory` scenario: delayed snapshot restores,
guest clock refresh and RNG output, root filesystem persistence and independence,
shared-volume writes, and a reboot that reconstructs the template while retaining
container state. Results must match the exact candidate image hashes.

## Cisco CLI contract

Cisco's [IOS XE CLI basics](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/configuration/xe-16-12/fundamentals-xe-16-12-book/cf-cli-basics.html)
distinguishes word help (`sh?`) from next-token help (`show ?`). Both preserve
the current input. Help lists only the next grammar level; `<cr>` denotes a
complete command. Keywords accept unique abbreviations and case-insensitive
matching. Arguments retain their original case. An exact keyword takes
precedence over longer keywords with the same prefix.

The implementation uses one grammar for execution, context help and completion.
The serial editor implements immediate `?`, Tab, cursor and word movement,
history with draft restoration, literal `?` via Ctrl-V, deletion and kill-ring
recall, Ctrl-C cancellation and Ctrl-Z return to EXEC. Empty-line Ctrl-D
leaves a configuration context or logs out from EXEC; with text present it
deletes at the cursor. Syntax diagnostics
distinguish ambiguous, incomplete and invalid commands. Configuration modes
use `do` for EXEC operations. `terminal length 0` disables the pager; the default
is 24 lines. Return advances one line, Space a page and q/Ctrl-C exits output.
Recovery parses the local subset using the same immutable management binary
when management or storage is unavailable and AAA is absent. Configured or
unreadable AAA fails closed; ROMMON 0x2142 supplies explicit physical recovery.

Normal `configure terminal` commands immediately update running configuration.
`write memory` persists startup configuration as JSON. `configure candidate`
provides an optional transaction for changes that must apply together. Complete
IOS routing/platform parity is outside scope. [Appliance management parity](aaa-and-management.md)
includes local AAA, privileges, console/SSH VTY, timeouts, output filters and the
shared editor/help grammar; its separate VM tests establish these behaviors.

## Mode and line grammar regression (2026-10-23)

FRRouting's [CLI developer guide](https://docs.frrouting.org/projects/dev-guide/en/latest/cli.html)
and [VTY shell implementation](https://github.com/FRRouting/frr/blob/master/vtysh/vtysh.c)
describe separate command graphs for each mode and parent-mode fallback. Its
[matcher](https://github.com/FRRouting/frr/blob/master/lib/command_match.c) also
distinguishes token types, complete matches, and incomplete input.
FractalCR now resolves a submode's commands first, then tries global configuration
only if the command is invalid there. Executing a parent command leaves the
submode; help and completion never change mode. This prevents a global `network`
selector from masking a container's `network` setting. EXEC commands still
require `do`; `conf t` is not a configuration-mode command.

Cisco's [line command reference](https://www.cisco.com/E-Learning/bulk/public/tac/cim/cib/using_cisco_ios_software/cmdrefs/line.htm)
defines a first line and optional last line. `line vty 0 4`, `line vty 5 15`,
and `line vty 3` now select real independent policies. The prompt is
`(config-line)#`. SSH allocates the first available enabled line, exports that
identity to management, and enforces that line's timeout and transport policy.
`show users` identifies the allocated VTY. Disabling one line terminates its
connection without disconnecting other lines. Existing configurations retain
their global VTY defaults until a line is overridden.

Numeric arguments are range checked by the same parser used for help traversal.
Help descriptions for repeated tokens belong to their command paths: a console
line number cannot inherit password-encoding help. An incomplete command does
not execute or change mode. The serial acceptance test reproduces the reported
transcript, including entering a range after `line vty ?` without retyping the
preserved input. The SSH suite tests independent timeout and transport behavior.

Cisco's [password minimum-length command](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/security/s1/sec-s1-cr-book/sec-cr-s1.html)
is an explicit policy. `security passwords min-length N` now enforces this for
new passwords; `no security passwords min-length` removes it. The old hidden
12-character minimum is gone. Empty passwords remain invalid, the maximum is
256 bytes, and accepted passwords are always stored as salted PBKDF2 verifiers.
Changing the policy does not invalidate existing accounts.

## Interrupted-build recovery

The appliance kernel task always publishes the newly built bundled image.
It does not depend on backing up an existing unbundled kernel: interruption
between the generic kernel class's backup and restore steps previously left a
successful later build able to deploy an older `.initramfs` image.

Before signing, `os/tools/verify-bundled-image.py` checks the source identity in
the root filesystem and verifies that the complete current cpio archive occurs
inside the decompressed kernel payload. Comparing the complete archive also
covers the management and console executables. A stale kernel cannot pass by
merely containing the expected identity string. Run the guard's regression tests
with `python os/tests/test-bundled-image.py`.

The VEX summary index is reset at BuildStarted as well as upstream's normal
BuildCompleted cleanup, so an interrupted build cannot cause duplicate package
entries in the next vulnerability report. Individual recipe reports are retained.

## Native container tools

The complete installed Docker client is available through
`request container-engine <arguments>`. Examples:

```
request container-engine compose -p app up -d
request container-engine buildx build load enable tag app:latest data0:
request container-engine swarm init advertise-addr 192.0.2.10
request container-engine exec -it app /bin/sh
request container-engine service ls
```

The prefix follows appliance abbreviation rules. Native arguments retain Docker
syntax, including flags; they are passed directly as an argv array, never to a
host shell. Use native `--help` for these arguments. Interactive clients get a
PTY, and streaming output is not truncated by the management RPC response limit.
The working directory is `/datastore/data0/projects`; client state is under
`/datastore/data0/client`. Projects and build contexts must be placed there
before use (Docker `cp` is available). Native Docker changes are Docker-managed
state, not automatically imported into the appliance running configuration.

Pinned components: Docker Engine 29.3.0, Compose 5.6.1, Buildx 0.38.1 and
BuildKit 0.34.1. The additional tools use SHA-256-pinned official upstream release
binaries. This reproduces appliance assembly; it is not a claim of independently
rebuilding those upstream binaries from source. See the [Docker CLI reference](https://docs.docker.com/reference/cli/docker/).

Standalone BuildKit is a disabled-by-default supervised service:

```
request platform software service buildkit start
request container-build debug workers
```

Its persistent state is `/datastore/data0/buildkit`; its socket is local only.
The OCI worker retains process isolation. A fixed-program runc adapter supplies
`--no-pivot`, which is required by the embedded initramfs root. Upstream's
[worker constructor](https://github.com/moby/buildkit/blob/v0.34.1/worker/runc/runc.go)
does not expose its executor's NoPivot option through buildkitd configuration.
The adapter is not `--oci-worker-no-process-sandbox`.

Docker default-runtime policy applies to Docker-created containers, including
Compose containers unless they override their runtime. BuildKit uses its own
OCI worker; changing Docker's default runtime does not put its build steps into
Kata guests. That isolation policy needs separate integration and qualification.

Swarm adds built-in VXLAN, IPVS and IPsec kernel support, consistent with
[Docker's overlay networking](https://docs.docker.com/engine/network/drivers/overlay/)
and [ingress routing mesh](https://docs.docker.com/engine/swarm/ingress/).
The [three-node qualification](swarm-qualification.md) checks cross-host
encrypted traffic, secrets/configs, manager failure and quorum recovery.
Production-scale performance remains outside that suite's scope.

Kata's virtiofsd launcher creates an attached, read-only root view in a private
mount namespace, carries the runtime mount trees into it, then executes the
upstream daemon with its namespace sandbox still enabled. This accommodates
the initramfs root without selecting `--sandbox=none` or `--sandbox=chroot`.
Runtime diagnostics go through the supervised local syslog receiver.
The current profile disables guest seccomp and guest SELinux. The native runc
seccomp test does not qualify those guest-side controls. Kata VM isolation and
guest security-policy support must be evaluated separately.
The profile sets `create_container_timeout = 60` explicitly: the pinned
[Kata 4.2 agent client](https://github.com/kata-containers/kata-containers/blob/4.2.0/src/runtime/virtcontainers/kata_agent.go)
uses that value directly as the container-creation deadline.

The pinned Kata runtime rescans Docker network namespaces asynchronously after
starting the guest. A workload must tolerate a short interval before its
interface becomes available. The networking acceptance probe waits at most ten
seconds, then requires a guest bridge address and a working published HTTP port.
See Kata's [Docker 28/29 networking investigation](https://github.com/kata-containers/kata-containers/issues/9340)
and [post-start rescan implementation](https://github.com/kata-containers/kata-containers/pull/12754).

The host kernel uses performance optimization (`-O2`). During qualification,
the pinned OE GCC 15.3.0 compiler at `-Os` omitted both parent-pointer assignments
from the TCX bundle initializer in `net/sched/sch_ingress.c`. Attaching Kata's
traffic-control filter then faulted in `tcx_miniq_inc` at address `0x628` and
blocked network cleanup. Recompiling that same translation unit with either
`-O2` or `-fno-tree-dse` retained the assignments. The kernel recipe requires
`CONFIG_CC_OPTIMIZE_FOR_PERFORMANCE=y`; VM acceptance also rejects kernel
Oops, BUG, panic, soft-lockup and RCU-stall records. This is a finding for this
specific compiler/kernel combination, not a claim about every size-optimized
kernel.

## Boot compression and wording

The kernel and embedded userland are already inside a gzip-compressed kernel
payload, which self-decompresses during boot.
Recompressing the previous 194,911,064-byte system image with gzip level 6
produced 193,255,833 bytes: 0.849% smaller. A new outer zip format would add
another extraction step and recovery format without a meaningful size benefit.
The expanded CLI/container-stack image (335,342,424 bytes, SHA-256
`fe6f21c79fff7e478b68fec2e442c825ab1bd51bd181e3486d2b6d86f5512a33`)
compressed to 333,303,548 bytes with gzip level 6: 0.608% saved, taking 9.63
seconds on the development workstation. This measurement predates the kernel
optimization change; it is not a compression benchmark for subsequent images.
The monitor's progress hashes track bytes copied into the authenticated sealed
image. Completion is printed only after signature verification and sealing.
Its label is `Loading and verifying image`, not a fabricated decompression
percentage. ISR4331-specific boot evidence remains in
`boot/docs/isr4331-research.md`; older ISR self-decompression transcripts are
not treated as proof of the 4331 boot sequence.

## Acceptance and limits

`os/tests/qemu-system.py` records image hashes, accelerator, serial transcripts
and checks for each scenario. It accepts `--artifacts PATH --accel kvm` on sascha.
Core cases cover persistence, service recovery, malformed configuration,
missing storage, explicit RAID1 initialization and optional firmware Secure
Boot. Additional cases exercise the actual serial editor, client builds,
Compose, standalone BuildKit, Swarm ingress and nested Kata.
The TCG boot allowance is ten minutes because authenticating this embedded
image inside the emulated kernel takes several minutes. This changes only the
boot wait; command, service-readiness and container-runtime deadlines remain
bounded separately. KVM uses the shorter four-minute boot allowance.

Passing tests qualify those specific behaviors and image hashes. They do not
establish all Docker storage/network plugins, rootless operation, GPU/CDI,
cross-platform builds, all Compose/Buildx flags or long-duration fault tolerance.
Those remain explicit production acceptance work. Dedicated AAA and three-node
Swarm harnesses record their own checks and exact image hashes.
Docker swap-limit detection now uses a dedicated engine cgroup, after enabling
the root memory controller. Runtime tests require `.SwapLimit=true` and verify
a container's 32 MiB memory limit and 16 MiB additional swap limit directly in
cgroup v2. This validates accounting/enforcement configuration without requiring
the host to have a swap device.
The pinned [Docker detector](https://github.com/moby/moby/blob/docker-v29.3.0/pkg/sysinfo/cgroup2_linux.go)
checks `memory.swap.max` in dockerd's own cgroup; the root cgroup lacks that
file. The engine wrapper now enters `/sys/fs/cgroup/fractal-engine` before exec.
The three-node suite requires bounded manager demotion and worker removal,
followed by dissolving the final single-manager swarm. Forced removal from a
multi-manager cluster timed out in a stress run and remains unqualified.
Private development signing keys must be replaced using the implemented
[release-key custody and rotation process](../../boot/docs/release-key-management.md)
before distribution as a production appliance.
