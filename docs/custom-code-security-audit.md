# Custom code security review

Review date: October 25, 2026.

This review examined the custom firmware trust boundaries and exercised their
parsers and service interfaces. The baseline was integration revision `30de98c`,
ROMMON revision `b5b1c6b`, supervisor revision `3013ac3`, and platform layer
revision `5d04785`. The corrections described below follow that baseline.

The review found a reproducible memory-safety defect in the system console and
an unbounded SSH output-drain wait. Both have been corrected. No image-signature
bypass was identified in the ROMMON paths examined. That result does not prove
the absence of vulnerabilities.

## Findings

### SEC-001: Invalid memory-copy length during kill-ring replacement

**Severity:** High. **Status:** Corrected.

The system console line editor removed the previous yank before checking whether
its replacement would fit. A rejected replacement left the saved yank range
inconsistent with the shortened line. Repeating Escape-Y could then underflow
the length supplied to `memmove`.

The defect was reproduced under AddressSanitizer, which reported
`negative-size-param` in `erase`, called by `yank`. The input consists of two
different-length kill-ring entries, a nearly full command line, and repeated
Escape-Y. An authenticated SSH user can access this editor regardless of command
privilege. The physical console also uses it. The console process runs with host
privileges; the demonstrated result is a memory-safety failure, not a demonstrated
arbitrary-code-execution exploit. ROMMON uses a different input implementation.

The editor now checks replacement capacity before modifying either the line or
the kill-ring selection. A regression test verifies that repeated rejected
replacements preserve the complete editor state. A libFuzzer target checks line
length, cursor position, string termination, and saved yank ranges.

### SEC-002: Unbounded SSH output drain after console exit

**Severity:** Medium. **Status:** Corrected.

After the console child exited, the SSH service waited indefinitely for its output
writer. A client that stopped consuming output could leave that writer blocked
and retain a VTY slot. The normal console idle timer cannot terminate a process
that has already exited.

The service now allows five seconds to drain pending output, then closes the
transport. Tests cover a blocked transport writer and normal completed output.
This test does not emulate every SSH client or network condition.

### Verification-test defects

**Status:** Corrected.

The recovery-network test still used an obsolete `boot_image` function signature
and did not compile against the current interface. It has been updated and run.
The verifier's negative tests also accepted any nonzero exit status, potentially
mistaking a crash for successful rejection. Every verification invocation now
rejects signal termination and sanitizer diagnostics.

## Coverage and results

| Component | Review focus | Validation performed |
| --- | --- | --- |
| ROMMON image verifier | Envelope boundaries, exact-length reads, signing domain, epoch floor, sealed payload, kernel handoff | Positive and negative verification cases; 512 deterministic envelope mutations under ASan/UBSan |
| ROMMON recovery | Device inventory, environment parsing, path grammar, TFTP peer/block handling, bounded transfers | Configuration tests under ASan/UBSan; TFTP empty, exact-block, duplicate, error and block-rollover cases; loopback ICMP |
| ROMMON input validators | Image references and setting values | 9,658,973 libFuzzer executions in a 61-second run under sanitizers |
| System console | Line editing, completion boundaries, management replies, pager | Regression tests, ASan/UBSan, and 1,241,224 editor fuzz executions in a 61-second run |
| Supervisor | Root-only control socket, packet sizes, readiness and dependency lifecycle | Sanitized graph and malformed/idle control-client tests |
| Management | Authentication, authorization checks, session revocation, file confinement, provisioning, network configuration | Go package tests and configuration fuzzing: 1,949,019 executions in 60 seconds |
| Native workload adapters | Linked clients, local file translation, Git publication, SSH host verification, process-execution restriction | Go package tests and `go vet` |
| Build and signing tools | Archive traversal and links, source-cache verification, private-key input, image signing | Source inspection and 35 build-tool tests |

GCC's static analyzer completed without diagnostics for ROMMON, the supervisor,
and the platform C sources reviewed, including the console and platform entry
points. Analyzer output is supporting evidence, not a proof of memory safety.
The Btrfs-linked storage adapter was inspected separately; its upstream formatter
was not included in this static-analysis result.

All root-module `internal`, `cmd`, and `pkg` Go tests passed. The Kata factory
service is built in its upstream runtime module and is not covered by that
root-module test result.

## Trust boundaries

* ROMMON authenticates the system envelope before handoff. The verified payload
  is sealed against modification, and kernel loading independently checks the PE
  signature. Host verifier tests do not exercise the firmware or kernel's own
  signature implementation; boot qualification remains necessary.
* Recovery environment checksums detect damage, not malicious modification.
  Environment values cannot replace the embedded image verification key.
* Management and supervisor IPC are local privileged interfaces. Root-only socket
  permissions and peer credentials protect them; they are not container-facing
  public APIs.
* Privilege 15 and explicitly delegated workload administration confer extensive
  host authority. Debug containers and host bind mounts are not a sandbox for an
  untrusted administrator.
* Ordinary file operations use descriptor-rooted access. Workload path translation
  has a different purpose and is not itself a filesystem sandbox.
* NoCloud provisioning accepts bounded local configuration documents with explicit
  fields and key-only users. It does not execute provisioning scripts.

## Remaining assessment work

This is a source-focused security review, not an independent certification or an
exhaustive line-by-line audit of every custom implementation and dependency.
The following work remains:

* Sustained, corpus-seeded fuzzing of environment files, recovery packet sequences,
  management requests, and the complete console/SSH integration. The short fuzz
  runs above are smoke tests; the ROMMON fuzz target covers value validators, not
  every recovery state machine.
* Adversarial testing of service restart races, SSH flow control, resource
  exhaustion, storage-device replacement, and power loss in complete firmware.
* Independent review of the custom Kata patches, factory snapshot lifecycle,
  Btrfs adapter integration, and imported runtime code. Successful host tests do
  not establish guest isolation.
* Review of the optional debug image's mutable registry tag and stronger isolation
  of pre-authentication SSH connections from the configured VTY allocation.
* Hardware firmware, kernel filesystem drivers, cryptographic libraries, and
  third-party dependency auditing. In particular, the unresolved kernel and
  guest-security items in [Security review](security-review.md) remain open.

## Reproducing host checks

On a Linux development host with GCC, Clang, OpenSSL development headers, and
libFuzzer support, run:

```sh
bash tools/security/test-custom-c.sh
go test ./internal/... ./cmd/... ./pkg/...
go vet ./internal/... ./cmd/... ./pkg/...
go test ./internal/config -run='^$' -fuzz=FuzzDecodeRoundTrip -fuzztime=60s -parallel=2
python3 -m unittest discover -s tools/tests
```

`FCR_FUZZ_SECONDS` controls each C fuzz run. The verifier test additionally needs
the boot builder's `/work` and `/src` mounts and disposable development keys:

```sh
FCR_SANITIZE=1 python3 boot/tests/test-verifier.py
```

Use disposable test keys for host verification tests. Image-level qualification
must use the resulting firmware and its matching trust material.
