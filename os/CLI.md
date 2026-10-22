# Appliance CLI

The serial console talks to a separately supervised management service over a
root-only local socket. PID 1 continues supervising services while commands run.
The bootstrap recovery subset remains available if management is stopped;
configured or unreadable AAA cannot be bypassed through that fallback.

[Appliance management and AAA](docs/aaa-and-management.md) covers local login,
enable/disable, privilege levels, SSH VTY, session expiry, accounting, output
filters and physical-console password recovery. Each authenticated session has
its own configuration candidate; stale commits are rejected.

## Configuration

```
ctr# configure terminal
ctr(config)# hostname edge
ctr(config)# interface eth0
ctr(config-interface)# ip address 192.0.2.10/24
ctr(config-interface)# ip gateway 192.0.2.1
ctr(config-interface)# ip name-server 192.0.2.53
ctr(config-interface)# exit
ctr(config)# do show configuration diff
ctr(config)# commit
edge(config)# end
edge# write memory
```

Changes are staged until `commit`. `abort` discards the candidate. `end` returns
to operational mode; entering a new configuration session loads running state.
`do` executes operational commands from configuration modes. Keywords accept
unique abbreviations (`sh ver`). `sh?` lists matching words; `show ?` lists only
the next command level. Both retain the input line. Tab completes a unique
keyword; up/down recall history; Ctrl-U deletes back to the start of the line
and Ctrl-Z returns to operational mode. Ctrl-V inserts a literal question mark.
Quoted arguments have no shell expansion or substitution. `terminal length 0`
disables pagination; `show history` and `show terminal` inspect session settings.

Hostname, network and runtime changes stop the configuration service's consumers,
apply the new state, then wait for readiness. Failed apply attempts to restore
the previous state. Interrupted commits leave a RAM-backed rollback journal for
management restart. Reboot always reloads startup-config. Container definition
changes alone do not restart Docker or recreate containers.

`write memory` and `copy running-config startup-config` atomically replace
`/datastore/data0/config/startup-config.json` with file and directory fsync.
Running state in `/run/running-config.json` survives management/console restarts.
The network worker retains its applied snapshot until shutdown to remove the
old address before applying its replacement.

The version-1 schema accepts the earlier hostname/network files and adds
datastores, isolation and container definitions. Unknown fields, duplicate JSON
keys, symlinks, nonregular files and files over 64 KiB are rejected.
Currently one static IPv4 interface and one datastore, `data0`, are supported.
DHCP, IPv6 configuration and multiple management interfaces are not implemented.

## Containers

```
edge# request image pull alpine:3.22
edge# config
edge(config)# container example
edge(config-container)# image alpine:3.22
edge(config-container)# command sleep infinity
edge(config-container)# environment MESSAGE "hello world"
edge(config-container)# restart-policy unless-stopped
edge(config-container)# no container-isolation
edge(config-container)# commit
edge(config-container)# end
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
edge# config
edge(config)# system container-isolation install runtime kata-containers
edge(config)# system container-isolation enable
edge(config)# system container-isolation kata-containers default-runtime clh
edge(config)# system container-isolation kata-containers runtime clh default-memory 256M
edge(config)# system container-isolation kata-containers base-os Microsoft/CBL-Mariner
edge(config)# system container-isolation kata-containers apply-to all-containers
edge(config)# container native-exception
edge(config-container)# no container-isolation
edge(config-container)# commit
```

Install selects artifacts authenticated inside the OS image. Enable requires
working KVM and successful KVM VM creation. `apply-to all-containers` changes
Docker's default runtime. Per-container `no container-isolation` selects runc.
With `apply-to explicit`, `container-isolation kata-containers enable` selects
Kata for that container. `container-isolation inherit` restores inheritance.
Existing containers keep their creation-time runtime.
`show container-isolation detail` displays the Kata runtime's parsed profile.

VM factory and arbitrary guest OS imports are rejected. TCG acceptance checks
artifact availability and rejection without KVM; it cannot validate Kata guest
execution or the configured guest memory limit.
Sources: [Kata release](https://github.com/kata-containers/kata-containers/releases/tag/4.3.1),
[Docker runtimes](https://docs.docker.com/engine/daemon/alternative-runtimes/).

## Storage and recovery

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

The explicit format command uses the candidate disk list. Commit never formats.
The datastore service must be stopped. Formatting rejects mounted disks,
partitions, active holders and duplicate aliases. It does not use force: disks
containing filesystems or partition tables need a separate erase/reprovision
workflow, not implemented here. Format labels the filesystem `FRDATA` and
starts services. After readiness, commit and save.

Malformed startup-config leaves management available. Build a valid candidate,
commit and save it. `show logging configuration` shows the original parse error.

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
