# Native appliance services

Runtime management must use kernel interfaces, linked library APIs, or typed
service requests. Formatting human-readable output belongs at the CLI boundary.
Do not introduce command-output parsing or executable utility adapters.
Starting a supervised daemon or an explicit container workload is a service
lifecycle operation. Backend implementations, including Docker and BuildKit,
may invoke their own internal helpers. Do not fork or rewrite upstream daemon
code solely to remove those calls. The appliance's user-facing operations must
use APIs rather than utility execution and command-output parsing.

## Implemented in the current development sources

- ROMMON recovery uses IPv4 interface/route ioctls, UDP TFTP and ICMP sockets.
  It has no executable helper calls. Buildroot disables BusyBox; incremental
  image assembly removes the previous BusyBox binary and applet links.
- TFTP enforces its transfer size/deadline, pins the server transfer endpoint,
  handles duplicate blocks and block-number rollover, and acknowledges final
  retransmissions. Received images still require the existing authentication.
- Platform startup activates loopback through an ioctl and registers Btrfs
  members through BTRFS_IOC_SCAN_DEV.
- The configuration service calls Prepare directly. BuildKit readiness uses
  its gRPC worker inventory instead of buildctl.
- DHCP uses typed packets and leases in the networking service. Discovery,
  unicast renewal, broadcast rebinding, expiry and cancellation replace the
  external udhcpc process and its environment-variable hook.
- Debug-container ownership and deletion use typed Engine records.
- Engine inventory and normal container/network operations use HTTP API data;
  networking uses netlink; time monitoring uses Chrony's binary local protocol.
- Compose calls linked Buildx Bake, including environment-backed build secrets.
  The build alias uses the linked Buildx implementation without a plugin process.
- Compose Git checkouts and Buildx Git inventory use go-git. Local repository
  transport uses its linked object server. Checkout publication is atomic and
  refuses replacement; failed fetches never enter the Compose cache.
- BuildKit's docker-container transport uses Engine exec/attach APIs. HTTP-upgrade
  buffered bytes, stdout/stderr framing, half-close and client cleanup are
  preserved without starting a host Docker client.
- Docker and BuildKit SSH endpoints use native SSH direct-streamlocal channels,
  with known-host checking and private-key/agent authentication. They do not
  launch ssh or request a remote shell/dial-stdio command. The remote account
  must have socket access and stream-local forwarding enabled. Specify an
  explicit URL socket path for rootless or non-default daemon installations.
- Datastore discovery uses libblkid and formatting uses a service linked with
  the pinned Btrfs formatter. Its root-only SOCK_SEQPACKET protocol carries fixed
  request/reply objects; no blkid or mkfs executable is invoked. Existing media
  are never force-formatted. Formatter output stays in the private service log.
- Diagnostics use Engine objects and HTTP requests, replacing the self-test
  shell script. Requests are cancelled on service shutdown and containers are
  deleted through the Engine API.
- The workload frontend, storage formatter and API-only management entry points
  deny execve/execveat, including x32 syscall forms. The management binary permits
  child lifecycles only in its SSH, time-sync and BuildKit service entry points.
- Supervisors use explicit execv/execl paths, with no PATH lookup or implicit
  command-interpreter fallback.

## Backend helpers and remaining CLI integration

- BuildKit's daemon-side Git helper and Docker Swarm/overlay's iptables helpers
  are permitted internal implementation details. Preserve their dependencies.
- Advanced workload dependencies contain credential-helper, SSH proxy/jump,
  provider/plugin and model-runner executable paths. Linking the upstream CLI
  does not itself remove those paths. They need native adapters and coverage.
  An exec denial is a safeguard, not a completed implementation of that feature.
- The interactive workload frontend still uses an appliance-owned process
  entry point. A streaming service protocol must preserve terminal ownership,
  authorization, cancellation and backpressure when replacing it.
- Keep backend helper dependencies in the system image without exposing a host
  shell or a general-purpose utility execution command through the appliance CLI.

BusyBox remains available to backend implementations;
its presence does not add a CLI entry point. ROMMON's native recovery remains
independent of BusyBox.

## Validation

The following describes historical implementation tests on a Linux build host. The management/CLI/config suites
pass with the race detector. Native ROMMON tests pass for empty and exact-block
transfers, duplicates, error responses, final-ACK retransmissions and 16-bit
block rollover, plus loopback ICMP. ROMMON compiles with its musl toolchain;
platform and manager compile with warnings treated as errors. DHCP discovery
and renewal pass a real UDP fixture test without changing host interfaces.
Native Git and Engine stream integration tests pass with executable creation
blocked. Compose lifecycle, Buildx builds, Compose Bake and BuildKit-only secret
mounts passed against the build host's Engine. Storage tests format only disposable
regular-file fixtures, reject overwrite and malformed protocol messages, and
verify the execution guard. Real block-device provisioning still needs VM tests.
The native container transport reached a real BuildKit v0.34.1 worker under the
exec guard. Native SSH tests verify stream-local-only requests, half-close,
known-host mismatch rejection, private-key permissions and cancellation cleanup.

These are source-level and protocol checks, not a qualification of every image.
See [current status](../../docs/development-status.md) and the
[image qualification record](cli-and-container-qualification.md) for image-bound
evidence. Internal candidate labels are not release versions.
