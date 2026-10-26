# Troubleshooting and recovery

Check [known issues](development-status.md) first. Commands below run at the
appliance prompt unless a host or ROMMON is specified.

| Symptom | What to check |
| --- | --- |
| Remote build connects to the wrong host | Pass `-BuildHost BUILD_HOST` to every remote build, status, and export invocation. The selected host is not remembered between invocations. |
| No output on a monitor | The console is serial-only. Use the first UART at 115200 baud, 8 data bits, no parity, 1 stop bit. |
| WHPX launch fails | Enable Windows Hypervisor Platform, check QEMU and its bundled firmware. WHPX requires `-NoSecureBoot`; use TCG for Secure Boot. |
| Boot pauses after image verification | Kernel authentication/loading is silent. TCG can take several minutes; the next normal message is `Booting authenticated system image...`. |
| `% Firmware trust is unavailable` | Expected with optional firmware policy and Secure Boot disabled. System signatures remain mandatory. |
| Signature verification fails | Use matching ROMMON, system image and trust files from the same signing identity. |
| Setup does not start | Startup configuration may already exist. Use blank prepared media for a new installation, or `setup` from a privileged physical console. |
| Setup lists no data disks | The USB is boot media and cannot be selected for `data0:`. Run `show storage disks` to see each disk's setup status. A separate writable non-USB disk may be initialized even if it has partitions, after an exact erase confirmation. Mounted or in-use disks are refused. A hardware RAID controller must expose a configured virtual disk; its individual member disks may not appear to the operating system. |
| `show version` reports no physical network interfaces | Check that the onboard or add-in NIC is enabled in firmware. Record the adapter model or PCI ID if a supported driver does not detect it. |
| SSH cannot connect | Check management address/route, AAA, credentials and VTY transport policy. With Windows user networking, request `-SshPort 2222` and connect to localhost port 2222. |
| Changes disappear after reboot | `end` does not save. Confirm `write memory` succeeds. Candidate changes also require `commit` first. |
| Changed definition does not change a container | Definitions and runtime objects have separate lifecycles. Plan explicit recreation after preserving required data. |
| Kata is unavailable | Check KVM and nested virtualization. The Windows WHPX profile does not expose nested KVM. |

## Inspect a service failure

```text
show platform software service
show logging configuration
show logging networking
show logging datastore
show logging container-engine
show storage datastore data0
```

Missing storage or invalid configuration keeps dependent services stopped.
Most service logs are volatile: collect relevant output before rebooting.
Remove secrets and identifying data before sharing diagnostics.

The [startup/reload blocker](development-status.md#known-startup-and-reload-issue)
can leave Docker unable to start because of a conflicting bridge record.
Repeated restarts are not a demonstrated fix; do not erase the datastore as a
default remedy. Preserve data and diagnostics. The fix awaits image qualification.

## Recover access or an image

Follow the complete [password-recovery procedure](../os/docs/aaa-and-management.md#physical-console-password-recovery)
for a forgotten administrator password. ROMMON `confreg 0x2142` temporarily
ignores startup configuration, not image authentication. Restore normal boot
policy after recovery.

For a missing or damaged image, use [USB recovery](https://github.com/highenergymagic/fractalcr-rommon/blob/main/README.md#usb-recovery)
or [TFTP recovery](https://github.com/highenergymagic/fractalcr-rommon/blob/main/README.md#network-recovery). Copy a matching signed image
to a new filename and retain a known-good alternative. ROMMON does not format
unrelated datastore disks. There is no power-loss-qualified A/B updater.

## Report a problem

For ordinary bugs, include `show version`, exact commands, expected/observed
behavior, hardware or QEMU accelerator, and sanitized service output in a
[GitHub issue](https://github.com/highenergymagic/fractalcr/issues).
Use [private reporting](../SECURITY.md) for security findings. Do not attach
datastore images, firmware variable stores or signing keys.
