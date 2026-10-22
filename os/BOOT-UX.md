# Appliance boot and service supervision

## Reference behavior

Cisco's [ISR4331 password-recovery transcript](https://www.cisco.com/c/en/us/support/docs/ios-nx-os-software/ios-xe-16/217045-troubleshoot-password-recovery-in-cisco.html)
shows hardware initialization, bootstrap identity, platform memory and image
selection before entering the operating system. The [ISR4000 configuration
examples](https://www.cisco.com/c/en/us/td/docs/routers/access/isr4400/software/configuration/xe-17/isr4400-sw-config-xe-17/configuration_examples.html)
show verification, software identification and the RETURN gate before the
operator prompt. These family examples include other ISR4000 models and
development releases; they are not all exact ISR4331 production transcripts.

The [process-health chapter](https://www.cisco.com/c/en/us/td/docs/routers/access/isr4400/software/configuration/xe-17/isr4400-sw-config-xe-17/process_health_monitoring.html)
provides operational process inspection commands. The [system-message chapter](https://www.cisco.com/c/en/us/td/docs/routers/access/isr4400/software/configuration/xe-17/isr4400-sw-config-xe-17/system_messages.html)
describes messages intended for operators. FractalCR adopts the visible sequence
and structured facility/severity/event style, with its own names and wording.
It does not copy Cisco software or claim Cisco process-manager compatibility.

## Console contract

The signed ROMMON supplies `quiet loglevel=0` to the authenticated system
kernel. Its own signed command line uses the same policy. PID 1 emits bootstrap
status, then a supervised console owns interactive serial output. Helper and
daemon output is captured separately. The kernel ring remains available for diagnostics.

Normal startup reports actual stages: platform initialization, datastore,
configuration, service startup, readiness or degraded status, then the RETURN
gate and configured-hostname prompt. There are no fake hardware tests, progress
percentages, compiler banners, Linux device-enumeration messages, or machine
test markers in this path. OVMF/physical firmware output preceding ROMMON is
owned by system firmware. A fatal kernel panic is outside PID 1's supervision;
this console policy is not a panic interception mechanism.

The console uses the command grammar for abbreviation, contextual help and Tab
completion, with cursor/word editing, history, a kill ring and pagination.
It remains nonblocking while an operator types. The physical console permits
initial provisioning; [local AAA](docs/aaa-and-management.md) then supplies
authentication, privileges, accounting and SSH VTY sessions. Damaged credentials
fail closed until explicit physical ROMMON configuration recovery.

## Supervisor

The shared [service implementation](../service/README.md) is compiled into both
ROMMON and the system image. It implements stopped, starting, running, stopping,
backoff, failed and completed-job states. Platform, logging, datastore,
configuration, networking, local syslog, container engine, BuildKit, SSH, diagnostics
and console are separate
supervised processes. Their dependency graph gates startup and orders shutdown.

Readiness uses the Docker API, with a 60-second startup deadline. Three failed
one-second health probes trigger recovery. Unexpected exits use exponential
backoff; four failures without 60 seconds of sustained health latch the service
failed until an explicit start. Stop/restart sends TERM to the owned process group, then KILL
after ten seconds. PID 1 reaps adopted children. This is process-group ownership;
processes deliberately escaping that group are not a security boundary.

Logs live in volatile `/run/log`, with a 256 KiB active file and one rotated
file per captured stream. A reboot clears these diagnostic files. Kernel,
platform helpers, the container engine and supervisor events are separate.
Container diagnostics are a supervised, time-bounded one-shot job with rotated output.
Raw logs are shown only when explicitly requested. This is not remote syslog
or a persistent event journal.

The local `syslog` service receives Unix datagrams on `/dev/log` into the bounded
`/run/log/syslog.log`. It is a prerequisite for the container engine because
Kata's shim requires that socket. The separate `logging` service collects the
kernel ring. Optional standalone `buildkit` depends on datastore and networking,
reports readiness only after its worker API responds, and is initially disabled.
Native Docker clients run behind a PTY owned by the console, so interactive
exec/attach and Ctrl-C work while PID 1 continues supervising services.

Commands:

```
show version
show platform
show platform software service
show logging
show logging kernel
show logging platform
show logging container-engine
show logging diagnostics
request platform software service container-engine start
request platform software service container-engine stop
request platform software service container-engine restart
request platform diagnostics containers
reload
```

Unit fault tests run real child processes, including crash loops, health and
readiness failures, resistant descendants and a noisy log producer. QEMU tests
reject kernel/daemon leakage during normal startup, exercise service controls
with partially entered console input, repeat container checks and reboot
persistence, and request kernel diagnostics to confirm logs were retained.
