# Development checkpoint — October 24, 2026

FractalCR is a development preview of a serial-managed container appliance from
Fractal Microsystems. This checkpoint saves the implementation on `main`; it
does not publish a 1.0 release, certify IOS compatibility, or promote a new image.

## What is available

The project includes a signed ROM Monitor, an immutable OpenEmbedded system
image, a native service supervisor, persistent configuration, local AAA and SSH,
and appliance commands for Docker, Compose, Buildx/BuildKit, Swarm and Kata.
The CLI follows IOS-style configuration, contextual help, abbreviated commands,
line editing, file operations and management displays. Its reference baseline
is ISR 4331 IOS XE 17.12 documentation. FractalCR is an independent project;
it is not Cisco software or an assertion of complete IOS compatibility.

Start with [architecture](architecture.md), [build instructions](building.md),
[the CLI reference](../os/CLI.md), [AAA](../os/docs/aaa-and-management.md), and
[USB/VPS image provisioning](../os/docs/releases.md). Clone recursively:

```sh
git clone --recurse-submodules https://github.com/highenergymagic/fractalcr.git
```

GitHub's automatic source ZIP does not include submodules. Build instructions support a Linux Docker host; the
maintainer's host names are examples, not services available to contributors.

## Tested source and images

- Source checkpoint `00f3c2359ca35edf7da7069c03e660c2374c9a3e` passed the
  application Go race tests and vet on Linux. Native supervisor and dependency
  graph tests, including sanitizer checks, passed. Its ROMMON build completed;
  the rebuilt ROMMON has not completed boot qualification.
- The latest local launcher image remains iteration 19, system SHA-256
  `de8ec345832836828e95dfda5cfb4757fae9045843ee1ef8d2375bebd5a0450c`.
  Its recorded management and USB suites passed 75 checks. These tests are
  bounded evidence, not a production certification.
- Iteration 20, system SHA-256
  `a2ea9aaab404aa0f69759811aa3d7bb35c669e6845c7826dc0e6cae9554827f6`,
  passed its serial, SSH/AAA and no-RTC clock suites with the corrected test
  harness. Its USB cold-boot suite failed. It was **not promoted**.
- Native configuration-file import/export and calendar-authority changes are
  tested at source level but still await a new system image and live qualification.
  The iteration 21 build stopped at the disk-space guard; it produced no candidate.

Detailed local reports are retained under `out/ios-conformance/`. They are not
included in the source repository or presented as downloadable release assets.
Future image releases must publish sanitized, hash-bound qualification reports.

## Known blocker and remaining work

Reloading while Docker is initializing, after a networking restart, can leave a
conflicting default bridge record and prevent the engine from starting at the
next boot. Iteration 20 reproduced this. Earlier images have not been cleared
of this scenario. The supervisor now defers engine termination until readiness
or its original startup deadline, retaining dependencies and bounded cleanup.
That fix passed native tests but **has not yet passed the five-cycle image test**.
Do not use this checkpoint for workloads whose availability depends on that fix.

The [conformance audit](ios-management-conformance.md) retains the outstanding
workflow and output differences, including network file transfers, additional
AAA/terminal/NTP commands and platform-specific output details. No physical ISR
transcript was used as a byte-for-byte oracle. There is no claim of zero gaps.

The [security review](security-review.md) and [release policy](releases.md) retain
the security, SBOM/license, reproduction and hardware acceptance gates. No
independent comprehensive security audit has been completed. Images use
development signing; production signing is an operator-managed workflow.

## Resume from this checkpoint

1. Qualify the new ROMMON, build the system image from the pinned source and
   preserve its exact source and artifact identities.
2. Run management qualification and the five-cycle USB cold-boot regression.
   Investigate failures before replacing any launcher image.
3. Continue the documented conformance and release gates as separately bounded
   work. Do not treat a successful source build as image qualification.

No new firmware build, release or image promotion is queued by this checkpoint.
Source publication is separate from firmware qualification. See the
[publication review](publication-review.md) for the history scan and reporting
setup used to make the repositories public as a development preview.
