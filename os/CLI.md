# Appliance CLI

This reference describes the current source tree. Start with [getting started](../docs/getting-started.md)
for first boot, and check [development status](../docs/development-status.md) for
changes not yet qualified in an image. Examples use illustrative addresses and
interface names: select actual ports with `show interfaces`.

The serial console talks to a separately supervised management service over a
root-only local socket. PID 1 continues supervising services while commands run.
The bootstrap recovery subset remains available if management is stopped;
configured or unreadable AAA cannot be bypassed through that fallback.

[Appliance management and AAA](docs/aaa-and-management.md) covers local login,
enable/disable, privilege levels, SSH VTY, session expiry, accounting, output
filters and physical-console password recovery. Normal configuration edits
running state immediately. Explicit candidate
transactions have independent staging; stale commits are rejected.

The [IOS management conformance record](../docs/ios-management-conformance.md)
links the Cisco command references and records tested behavior and remaining
differences.

## Configuration

An empty datastore starts the serial setup dialog. `setup` runs it again from
privileged physical-console EXEC. It collects a hostname, management NIC,
DHCP/static addressing, an optional NTP server, and a local administrator.
The password is hidden and stored as a salted verifier. Review the summary
before applying and saving. `abort` leaves the dialog; disk initialization,
when explicitly requested, is a separate irreversible operation.

Prepared single-disk media already has an empty Btrfs datastore. With separate
blank data storage, setup lists disks and requires `INITIALIZE` for the selected
whole disk. Existing filesystems, partition tables, mounted disks and disks in
use are refused. Setup never silently erases an existing installation.

```
ctr# configure terminal
ctr(config)# hostname edge
edge(config)# interface GigabitEthernet0
edge(config-if)# ip address 192.0.2.10 255.255.255.0
edge(config-if)# ip gateway 192.0.2.1
edge(config-if)# ip name-server 192.0.2.53
edge(config-if)# exit
edge(config)# end
edge# write memory
```

Each accepted command in `configure terminal` applies to running configuration
immediately. `end` and Ctrl-Z return to EXEC without discarding changes;
`write memory` saves running configuration for the next boot. Failed edits are
rolled back and failed saves return an error. Hostname and account edits do not
restart workloads.

Use `configure candidate` when several dependent changes must apply together.
In that explicit mode, `commit` applies the transaction and `abort` discards it.
Leaving or saving with uncommitted changes is refused. In ordinary configuration
mode, `abort` exits without undoing accepted commands.
`do` executes operational commands from configuration modes. Keywords accept
unique abbreviations (`sh ver`). `sh?` lists matching words; `show ?` lists only
the next command level. Both retain the input line. Tab completes a unique
keyword; up/down recall history; Ctrl-U deletes back to the start of the line
and Ctrl-Z returns to operational mode. On an empty line Ctrl-D leaves the
current configuration context, or logs out from EXEC. With text present it
deletes at the cursor. `logout` and EXEC `exit` close the session; the console
returns to login, or waits for RETURN if local login is not configured.
Ctrl-V inserts a literal question mark.
Quoted arguments have no shell expansion or substitution. `terminal length 0`
disables pagination; `show history` and `show terminal` inspect session settings.
Under `line console 0` or a `line vty` range, `length`, `history`,
`history size` and `editing` set persistent defaults, with `no` and `default`
forms. Changing line history size does not reenable disabled history. EXEC
`terminal` overrides last for that session; a new login uses the saved line
settings. `show line vty 4` also reports the configured defaults of an idle line.

Local accounts accept `user admin priv 15 password 0 TEXT` and
`username admin privilege 15 secret 0 TEXT`. Both hash the plaintext input;
passwords may contain 1..256 characters. Use `security passwords min-length 12` to enforce a minimum on newly configured passwords. Use `username NAME ?` or
`username NAME privilege 15 ?` to see the available account settings.
`show version` includes the source-build identifier of the running image.

`show running-config linenum` adds line numbers. Use
`show running-config interface GigabitEthernet0` to display one port, or append
`linenum` to number that selection. Interface descriptions take the remaining
literal text: quotes and backslashes are part of the description.

Without an enable secret, the physical console can return from `disable` to
`enable` without a password. SSH requires a configured enable secret to raise
privilege. `enable LEVEL` and `disable LEVEL` select a privilege level; disable
cannot raise it. `no enable secret` removes the configured secret.

`dir ?` lists mounted filesystem prefixes alongside the filename placeholder.
`dir fl?` lists matching prefixes and retains the partial input. Use `cd data0:`
and `pwd` to select and inspect the default directory for this session. Relative
filenames, `cd ..`, `/recursive`, `/all`, `all-filesystems`, and filename prefix
wildcards are supported. `copy` prompts for omitted source names, destination
names and overwrite confirmation. `delete /force` suppresses confirmation;
`/recursive` includes directory contents. Operations use named appliance volumes.

`more data0:filename` displays a local file. `more system:running-config` and
`more nvram:startup-config` display configuration in CLI form. Use `/ascii` for
text, `/binary` for a hex/text dump, `/compressed` for gzip/zlib data, or
`/ebcdic` for code page 037. For example,
`more /ascii data0:notes | include uplink` filters the displayed lines.
File and decompressed input are limited to 256 KiB per display; larger input
returns an error rather than silently truncating it.

`file prompt alert` is the default confirmation policy. `file prompt noisy`
confirms file-operation parameters; `file prompt quiet` suppresses confirmation,
including overwrite confirmation, while still requesting missing information.
`no file prompt` and `default file prompt` restore the default. Like other global
settings, save the policy with `write memory` to retain it after reload. Ctrl-C
cancels a pending file dialog.

`reload` asks whether to save modified running configuration, then asks for
confirmation. Return confirms; Ctrl-C cancels. `reload in 10 maintenance` schedules
a reload ten minutes ahead; `reload at 23:30` uses the next occurrence of that
time. An explicit date can follow the time in either day/month or month/day
order. `show reload` displays the pending request and `reload cancel` cancels it.
Schedules survive a management-service restart and expire on machine reboot.
Image authentication remains mandatory during every boot.

Network and runtime changes stop the configuration service's consumers,
apply the new state, then wait for readiness. Failed apply attempts to restore
the previous state. Interrupted commits leave a RAM-backed rollback journal for
management restart. Reboot always reloads startup-config. Container definition
changes alone do not restart Docker or recreate containers.

Configuration exports (`copy running-config data0:backup` and
`copy startup-config data0:backup`) contain CLI configuration text. Copying a
local configuration file to `running-config` merges it into the current
configuration; copying to `startup-config` saves it for the next boot. Imports
are validated in memory before applying or saving, and reject operational
commands. Failed validation leaves configuration unchanged. Existing JSON
backups are also accepted. Import files are limited to 256 KiB and every command
is checked against the caller's original privilege policy.

`write memory` and `copy running-config startup-config` atomically replace
`/datastore/data0/config/startup-config.json` with file and directory fsync.
Running state in `/run/running-config.json` survives management/console restarts.
The network worker retains its applied snapshot until shutdown to remove the
old address before applying its replacement.

## Interfaces, Docker networks and time

```
ctr# configure terminal
ctr(config)# interface GigabitEthernet0
ctr(config-if)# role management
ctr(config-if)# ip address dhcp
ctr(config-if)# exit
ctr(config)# interface GigabitEthernet1
ctr(config-if)# role container
ctr(config-if)# no ip address
ctr(config-if)# exit
ctr(config)# network external
ctr(config-network)# driver macvlan
ctr(config-network)# parent GigabitEthernet1
ctr(config-network)# subnet 192.0.2.0/24
ctr(config-network)# gateway 192.0.2.1
ctr(config-network)# exit
ctr(config)# ntp server time.example.net
ctr(config)# end
ctr# request network external create
ctr# show networks
ctr# show network external detail
ctr# show ntp status
ctr# show ntp associations
ctr# write memory
```

Select one management interface. SSH binds to its IPv4 address **and device**.
Container-role interfaces can use static IPv4, DHCP, or no host address for a
macvlan/ipvlan parent. Only management supplies the host default route and DNS;
DHCP on a container NIC does not replace them. `shutdown`/`no shutdown` controls
the link. Switching to DHCP clears static address/gateway/DNS. `show interfaces`
and `show ip route` report actual runtime state, including acquired leases.

Docker network drivers are `bridge`, `overlay`, `macvlan` and `ipvlan`.
`internal` and `attachable` are optional. Overlay creation requires an initialized
Swarm. Network definitions are staged inventory: explicit `request network NAME
create` and `remove` control their lifecycle; applying a definition never destroys an existing
network. `request network NAME connect CONTAINER` and `disconnect CONTAINER`
manage membership. Within container configuration, `network NAME` selects the
network used by `request container NAME create`. Native Engine network options
remain available through `request container-engine network ...`.

The `networking` service owns DHCP clients and configured addresses. The
`time-sync` service runs chronyd with reduced privileges, a syscall filter and
local-only command access. No NTP server or remote command port is opened.
`show clock` uses the configured timezone (UTC by default); `show clock detail`
includes the time source. `show ntp status` reports network synchronization
separately from service readiness. `no ntp server NAME` removes a source. Replace
`time.example.net` in the example with a reachable NTP server.

Startup configuration is stored as validated JSON. Unknown fields, duplicate
keys, symlinks, nonregular files and files over 256 KiB are rejected.
Configure one management interface and additional container interfaces. Interfaces
support DHCP or static IPv4, secondary IPv4 addresses, IPv6, MTU, and static routes.
The appliance currently uses one datastore, `data0`.
Only one interface can have the management role. DHCPv4 and static IPv6
configuration are supported; DHCPv6 is not implemented.

## Containers

```
edge# request image pull alpine:3.22
edge# configure terminal
edge(config)# container example
edge(config-container)# image alpine:3.22
edge(config-container)# command sleep infinity
edge(config-container)# environment MESSAGE "hello world"
edge(config-container)# restart-policy unless-stopped
edge(config-container)# no container-isolation
edge(config-container)# end
edge# write memory
edge# request container example create
edge# request container example start
edge# show container example detail
edge# show container example logs
edge# request container example stop
edge# request container example remove
```

Commands become executable argument lists, never shell strings.
`publish 8080:80/tcp` adds a port mapping. `no publish`, `no command` and
`no environment NAME` remove those settings. `no container NAME` removes only
the definition. Create fails if the name is already occupied; remove requires
the container to be stopped.

`volume NAME /TARGET [read-only]` mounts a Docker named volume stored in the
datastore; `no volume` clears mounts. `show volumes` and
`request volume NAME create|remove` manage these volumes explicitly.
Container settings include `memory 32M`, `pids-limit 32`, `read-only`,
`security no-new-privileges` and `capability drop all`.

## Kata

The image includes a source-built Kata 4.3.1 Go shim, checksum-pinned Cloud Hypervisor,
virtiofsd, the upstream guest kernel and CBL-Mariner 3.0 guest. Unused VMMs and
guest images are excluded. The host remains CIP SLTS. Upstream dependency
versions remain in `/opt/kata/versions.yaml`.

```
edge# show container-isolation capabilities
edge# configure terminal
edge(config)# system container-isolation install runtime kata-containers
edge(config)# system container-isolation enable
edge(config)# system container-isolation kata-containers default-runtime clh
edge(config)# system container-isolation kata-containers runtime clh default-memory 256M
edge(config)# system container-isolation kata-containers base-os Microsoft/CBL-Mariner
edge(config)# system container-isolation kata-containers apply-to all-containers
edge(config)# container native-exception
edge(config-container)# no container-isolation
edge(config-container)# end
edge# write memory
```

Install selects artifacts authenticated inside the OS image. Enable requires
working KVM and successful KVM VM creation. `apply-to all-containers` changes
Docker's default runtime. Per-container `no container-isolation` selects runc.
With `apply-to explicit`, `container-isolation kata-containers enable` selects
Kata for that container. `container-isolation inherit` restores inheritance.
Existing containers keep their creation-time runtime.
`show container-isolation detail` displays the Kata runtime's parsed profile.

The CLH factory uses block-backed container roots. Select it on an empty engine:

```
ctr(config)# system container-engine
ctr(config-engine)# storage backend blockfile
ctr(config-engine)# exit
ctr(config)# system container-isolation install runtime kata-containers
ctr(config)# system container-isolation enable
ctr(config)# system container-isolation kata-containers apply-to all-containers
ctr(config)# system container-isolation kata-containers runtime clh vm-factory enable
ctr(config)# end
ctr# request system container-isolation kata-containers vm-factory create
ctr# show container-isolation detail
ctr# write memory
```

Export/remove existing containers, images and build cache before changing storage
backends. Block snapshots have an 8 GiB filesystem capacity and consume datastore
space as blocks are written; Btrfs reflinks share unchanged extents. Template RAM
is separate from the memory of each running guest. The supervised `vm-factory`
service rebuilds its template after reboot or configuration changes. Repeating
the create request returns the existing ready factory.

Eligible containers restore from the template with refreshed wall time and RNG
state. Containers with application bind mounts or volumes use fresh Kata VMs
with virtio-fs so shared writes retain their normal behavior. Stop a block-backed
Kata container before copying, exporting or committing its root filesystem.
The API client enforces this for direct Engine and Compose operations.
`overlay2` remains the default storage backend. Arbitrary guest OS imports remain
unsupported; the authenticated system image supplies the guest kernel and OS.

TCG acceptance checks
artifact availability and rejection without KVM; it cannot validate Kata guest
execution or the configured guest memory limit.
Sources: [Kata release](https://github.com/kata-containers/kata-containers/releases/tag/4.3.1),
[Docker runtimes](https://docs.docker.com/engine/daemon/alternative-runtimes/).

## Storage and recovery

Online datastore maintenance uses kernel APIs:

```
ctr# request storage datastore data0 scrub start
ctr# show storage datastore data0 scrub
ctr# request storage datastore data0 scrub cancel
ctr# request storage datastore data0 balance start
ctr# show storage datastore data0 balance
ctr# request storage datastore data0 balance cancel
ctr# request storage datastore data0 snapshot create before-change
ctr# show storage datastore data0 snapshots
ctr# request storage datastore data0 snapshot delete before-change confirm before-change
```

Scrub and balance return control to the console while management tracks the
operation. Results are saved on the datastore. Snapshots are read-only filesystem
snapshots; they do not coordinate application-level transactions inside containers.
These commands do not launch `btrfs` or other filesystem utilities.

```
ctr# show storage disks
ctr# config
ctr(config)# storage datastore name data0
ctr(config-storage-datastore)# file-system format btrfs
ctr(config-storage-datastore)# preset raid1
ctr(config-storage-datastore)# disk vdb
ctr(config-storage-datastore)# disk add vdc
ctr(config-storage-datastore)# exit
ctr(config)# do request storage datastore data0 format confirm data0
```

The explicit format command uses the configured disk list (the candidate list
inside an explicit transaction). Configuration edits never format disks.
Incomplete disk definitions can be saved; formatting requires the full disk set.
The datastore service must be stopped. Formatting rejects mounted disks,
partitions, active holders and duplicate aliases. It does not use force: disks
containing filesystems or partition tables need a separate erase/reprovision
workflow, not implemented here. Format labels the filesystem `FRDATA` and
starts services. After readiness, save with `write memory`.

Malformed startup configuration fails closed for authentication. Follow
[physical recovery](docs/aaa-and-management.md#physical-console-password-recovery)
to create and save a replacement. A damaged file does not grant unauthenticated
management access.

## Debug service container

`request platform debug service-container enable` fetches `debian:trixie-slim`
if absent, then starts a separately supervised privileged runc container with
host PID/network access and host root mounted at `/host`.
`request platform debug service-container enter` attaches Bash; `exit` returns.
`request platform debug service-container disable` removes this debug container.
Disable waits for the worker to stop and reconciles any owned container left by
a worker crash. Cleanup failures are reported; an unrelated container using the
reserved name is never removed. The worker has a bounded cleanup grace period
before the supervisor stops its providers.
Its filesystem is ephemeral. The Debian tag is a runtime download, not part of
the reproducible OS image. Guest VM debugging is not implemented.

## Workload command discovery

`request container-engine` uses the linked Engine, Compose, and Buildx clients.
`request container-build` uses the linked BuildKit client. Their command catalogs
are generated from those clients, embedded in management, and checked for drift
in CI and the image build. Help and completion do not launch a workload process.

Direct families include `request compose`, `request buildx`, `request buildkit`,
`request swarm`, `request service`, `request stack`, `request node`,
`request secret`, `request registry`, and `request context`.

```
ctr# request compose ?
ctr# request buildx build ?
ctr# request service create replicas ?
ctr# request container-engine client-context lab container inspect application
ctr# request compose file data0:compose.yaml up detach enable
ctr# request container-engine run rm enable arguments alpine echo --help
```

`?` lists only the immediate children or settings at the current position. Partial
keywords accept unambiguous abbreviations; Tab completes one unique match.
Argument values retain their spelling and boundaries. Incomplete command groups
and missing setting values are rejected without leaving the current mode. Container
command arguments are passed as values, not interpreted by a host shell.
Settings use keyword/value pairs; booleans take `enable` or `disable`. Unix
options such as `--detach` and `-d` are rejected. `argument VALUE` supplies one
literal operand and `arguments VALUE...` supplies all remaining operands,
including container command arguments that happen to match appliance keywords.
`help` describes the current operation without invoking a client help process.
Global Engine connection settings precede the command family. Saved contexts are
honored. `build`, `image build`, and `builder build` use linked Buildx, including
its builder selection setting.

File operands accept the same `data0:` namespace as `dir`, `copy`, `move`, and
`delete`. For example, `request compose file data0:compose.yaml up detach enable` and
`request buildx build file data0:Dockerfile data0:` read files from the appliance
file volume. Build output/cache paths, BuildKit local contexts, and bind-mount
sources also accept this namespace. Container argv, environment values, and
container-side paths are preserved verbatim.
Browser actions print a link for the operator to open on their workstation.

This catalog describes the linked client surface. Runtime prerequisites still
apply: manager-only Swarm operations require a manager, devices must exist, and
plugin/provider implementations must be installed and supported. Catalog
coverage is not a claim that every external provider has been qualified.

## Extended persistent configuration

Container mode includes CPU allocation (`cpus`, `cpu-shares`, `cpu-set`), memory
reservation and swap, shared memory (`memory-shared`), process limits, user,
guest hostname, entrypoint, working directory, labels, logging, health checks,
capabilities, namespace modes, security options, devices/GPUs, DNS, host mappings,
sysctls, tmpfs and bind mounts, and multiple network attachments. Settings have
`no` forms and are represented in running/startup configuration. Creating a
container uses the same API object builder as offline configuration export.
Editing a definition does not silently recreate an existing container.

```
ctr(config)# container application
ctr(config-container)# user 1000:1000
ctr(config-container)# guest-hostname application
ctr(config-container)# cpus 1.5
ctr(config-container)# memory 256M
ctr(config-container)# memory-reservation 128M
ctr(config-container)# memory-shared 32M
ctr(config-container)# logging driver local
ctr(config-container)# logging option max-size 10m
ctr(config-container)# healthcheck command /app/check
ctr(config-container)# healthcheck interval 30s
ctr(config-container)# network attach front alias application
ctr(config-container)# network attach back
```

Named volume definitions support driver selection, driver options, and labels:
`volume NAME` enters volume mode; `request volume NAME create` creates the defined
volume through the Engine API. Docker network mode includes driver options,
labels, IPv6, IPAM drivers/options, multiple allocation pools, gateways,
allocation ranges, and reserved addresses. For example:

```
ctr(config-network)# ipv6 enable
ctr(config-network)# ipam pool 10.80.0.0/24 gateway 10.80.0.1
ctr(config-network)# ipam pool 10.80.0.0/24 range 10.80.0.128/25
ctr(config-network)# ipam pool 10.80.0.0/24 reserve router 10.80.0.2
ctr(config-network)# ipam pool fd80::/64 gateway fd80::1
```

`system container-engine` enters engine configuration mode. It includes registry
mirrors and insecure registries, default address pools, logging, live restore,
transfer concurrency, shutdown timeout, bridge addressing, IPv6, MTU, DNS, and
engine labels. These settings render into the managed daemon configuration;
platform configuration application retains its existing rollback behavior.

Physical interface configuration supports descriptions, MTU, secondary IPv4
addresses, IPv6 addresses, and an IPv6 management gateway. Static routes use
`ip route PREFIX INTERFACE GATEWAY metric NUMBER` or its `ipv6 route` equivalent.
Use `direct` for an on-link route. `no ip route PREFIX INTERFACE` removes a route.
`show ipv6 interface brief` and `show ipv6 route` read kernel state directly.

Virtual interfaces use `interface Vlan100` and `interface Port-channel1`.
`Vl100` and `Po1` abbreviations are accepted. A VLAN takes a `parent INTERFACE`;
a port-channel takes `member INTERFACE` and `channel-mode MODE`. Supported modes
are active-backup (default), 802.3ad, balance-xor, balance-rr, broadcast,
balance-tlb, and balance-alb. Configure physical members as container interfaces
without addresses, then configure the port-channel and its VLANs. Members inherit
the port-channel MTU. VLAN MTU cannot exceed its parent. Creation, membership,
addressing, and teardown use native netlink operations. The system kernel must
include bonding and 802.1Q support; the kernel configuration enables both.
