# Release status and restrictions

FractalCR is available as a development preview. Use the
[releases page](https://github.com/highenergymagic/fractalcr/releases) to select
published images and their qualification reports. Source availability does not
imply that a corresponding firmware release has completed qualification.

## Platform support

The target is x86-64 UEFI with a serial console at 115200 baud, 8N1. Ordinary
containers run without virtualization extensions. Kata requires KVM; virtualized
installations also require nested virtualization. Physical hardware support must
be verified on the intended motherboard, storage controller, and network adapter.

## Known startup and reload issue

Reloading during Docker initialization after a networking restart can leave a
conflicting default bridge record and prevent the engine from starting on the
next boot. The supervisor defers termination until readiness or the original
startup deadline and retains required services during shutdown.

The correction has passed native tests; repeated cold-boot image qualification
is still required. Until a release report covers that scenario, wait for the
container engine to become ready before reloading. If the engine fails, collect
`show platform software service` and `show logging container-engine`. Preserve
the datastore; deleting its network database is not a documented recovery step.

## Management restrictions

- Local AAA and SSH are supported. RADIUS, TACACS+, CLI views, and the complete
  Cisco IOS command set are not implemented.
- The system CLI supports local file operations. Use ROMMON for TFTP recovery;
  system-mode network copy protocols are not implemented.
- Scheduled reloads survive management-service restarts but not a full reboot.
- Startup configuration uses the appliance JSON model. Configuration-file import
  validates the complete file before applying it.
- The platform supports one datastore, `data0`. Increasing a deployment disk's
  size does not automatically expand the partition or filesystem.

See [Management command compatibility](ios-management-conformance.md) for detailed
command and output differences, and the [CLI reference](../os/CLI.md) for supported
syntax.

## Security and update restrictions

Development releases use development keys. Operators requiring their own trust
identity must follow the [signing procedure](releases.md). Privilege 15 and
physical recovery access are trusted administrative access. Datastore encryption,
tamper-evident remote accounting, and power-loss-qualified A/B updates are not
provided. Independent comprehensive security review remains outstanding.

## Qualification records

Release reports identify tested images by hash and list the completed checks.
ROMMON tests use a signed boot witness; system-image tests exercise the appliance
OS. Neither test class substitutes for the other. VM qualification does not
establish physical hardware compatibility or production-scale performance.

See [Firmware qualification](../os/docs/cli-and-container-qualification.md),
[Security review](security-review.md), and [Image deployment](../os/docs/releases.md).
