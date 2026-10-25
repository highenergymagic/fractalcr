# Boot sequence and service management

The appliance uses a serial console at 115200 baud, 8 data bits, no parity, and
1 stop bit. Connect to the first UART before powering on.

## Boot sequence

1. UEFI loads ROMMON. With Secure Boot enabled, firmware verifies its EFI signature.
2. ROMMON discovers recovery devices, loads its saved environment, and displays
   platform identity and image policy.
3. The five-second autoboot countdown accepts Ctrl-C or BREAK to enter the monitor.
4. ROMMON loads and authenticates the selected image. Kernel authentication and
   loading are silent until `Booting authenticated system image...`.
5. The system supervisor starts platform, datastore, configuration, networking,
   and enabled workload services in dependency order.
6. The console displays readiness or degraded status, followed by
   `Press RETURN to get started!`. Press RETURN to enter setup or login.

For recovery after a failed handoff, see the [ROM Monitor guide](https://github.com/highenergymagic/fractalcr-rommon/blob/main/README.md).

## Console operation

The signed ROMMON supplies `quiet loglevel=0` to the authenticated system
kernel. Its own signed command line uses the same policy. PID 1 emits bootstrap
status, then a supervised console owns interactive serial output. Helper and
daemon output is captured separately. The kernel ring remains available for diagnostics.

Startup messages identify the completed stage or failed service. Kernel device
enumeration and daemon output are retained in diagnostic logs. Firmware output
before ROMMON is controlled by the motherboard or VM firmware. Fatal kernel
panics occur outside service supervision.

The console uses the command grammar for abbreviation, contextual help and Tab
completion, with cursor/word editing, history, a kill ring and pagination.
It remains nonblocking while an operator types. The physical console permits
initial provisioning; [local AAA](docs/aaa-and-management.md) then supplies
authentication, privileges, accounting and SSH VTY sessions. Damaged credentials
fail closed until explicit physical ROMMON configuration recovery.

## Service behavior

The shared [service implementation](https://github.com/highenergymagic/fractalcr-supervisor/blob/main/README.md) is compiled into both
ROMMON and the system image. It implements stopped, starting, running, stopping,
backoff, failed and completed-job states. Platform, logging, datastore,
configuration, networking, local syslog, container engine, BuildKit, SSH, diagnostics
and console are separate
supervised processes. Their dependency graph gates startup and orders shutdown.

Readiness uses the Docker API, with a 60-second startup deadline. Three failed
one-second health probes trigger recovery. Unexpected exits use exponential
backoff; four failures without 60 seconds of sustained health latch the service
failed until an explicit start. Most services receive TERM for their process
group, followed by KILL after a ten-second grace period. Docker receives TERM
on its leader so it can shut down containerd, with a 60-second stop deadline.
A stop requested during Docker initialization waits for readiness or the original
startup deadline. See [Release status](../docs/development-status.md) for the
startup/reload issue and its qualification status. Other long-running jobs have service-specific deadlines. PID 1 reaps adopted children. This is process-group ownership;
processes deliberately escaping that group are not a security boundary.

Logs live in volatile `/run/log`, with a 256 KiB active file and one rotated
file per captured stream. A reboot clears these diagnostic files. Kernel,
platform helpers, the container engine and supervisor events are separate.
Container diagnostics are a supervised, time-bounded one-shot job with rotated output.
Use `show logging` to inspect captured output. Remote syslog and persistent
service journaling are not provided.

The local `syslog` service receives Unix datagrams on `/dev/log` into the bounded
`/run/log/syslog.log`. It is a prerequisite for the container engine because
Kata's shim requires that socket. The separate `logging` service collects the
kernel ring. Optional standalone `buildkit` depends on datastore and networking,
reports readiness only after its worker API responds, and is initially disabled.
Native Docker clients run behind a PTY owned by the console, so interactive
exec/attach and Ctrl-C work while PID 1 continues supervising services.

## Verify startup

At a privileged EXEC prompt, inspect system identity and service state:

```text
show version
show platform
show platform software service
```

Enabled services must report readiness before dependent operations can succeed.
For a stopped or failed service, inspect its log and the state of its providers:

```text
show logging
show logging kernel
show logging platform
show logging container-engine
```

Logs are volatile. Collect relevant output before restarting or reloading.

## Control a service

Use `start`, `stop`, or `restart` with the service name. Restarting a provider
also stops its dependent services and starts enabled consumers after the provider
is ready. Restarting the container engine can interrupt workloads.

```text
request platform software service container-engine restart
show platform software service
show logging container-engine
```

A failed service remains latched after its retry budget is exhausted. Correct
the underlying cause before issuing an explicit start or restart. Missing storage
or invalid configuration must be repaired before their consumers can start.

## Run container diagnostics

```text
request platform diagnostics containers
show logging diagnostics
```

The diagnostics job has a bounded runtime and captured output. For persistent
failures, use [Troubleshooting](../docs/troubleshooting.md).

## Qualification coverage

Source tests exercise crash loops, health/readiness failures, resistant
descendants, and log rotation. VM tests cover dependency restarts, console
responsiveness, container operations, reboot persistence, and diagnostic capture.
See [Firmware qualification](docs/cli-and-container-qualification.md) for procedures.
