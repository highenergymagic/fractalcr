# IOS management conformance

The reference platform is an ISR 4000 running IOS XE. Cisco's consolidated
references also contain commands for other chassis, storage implementations,
and releases. Those entries are not evidence that every listed option is an
ISR 4331 feature. Hardware identity and measurements must describe the actual
FractalCR machine, not a fictional Cisco router.

## References

Consulted 2026-10-24:

- [Configuration Fundamentals: D through E](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/D_through_E.html): directory operations, privilege transitions, command editing, and `do`.
- [Configuration Fundamentals: R through setup](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/R_through_setup.html): reload scheduling and rename.
- [Configuration Fundamentals: C commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/C_commands.html): copy operands and interactive filename selection.
- [Configuration Fundamentals: show protocols through showmon](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/show_protocols_through_showmon.html): scheduled reload status, configuration display, version information.
- [Configuration Fundamentals: terminal commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/slave_auto-sync_config_through_terminal-type.html): history, screen geometry, and terminal monitoring.
- [IP Addressing Services command reference](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/ipaddr/command/ipaddr-cr-book.pdf): primary and secondary IPv4 addresses and subnet masks.
- [ISR 4000 IOS XE 17 software installation](https://www.cisco.com/c/en/us/td/docs/routers/access/isr4400/software/configuration/xe-17/isr4400-sw-config-xe-17/installing_the_software.html): platform boot and filesystem context.

## Implemented and exercised in this change

| Area | Behavior | Evidence |
| --- | --- | --- |
| Context help | Word-boundary filesystem placeholder and mounted prefixes; partial filesystem help preserves the typed prefix; unique Tab completion | `TestIOSFilesystemHelpAndReloadGrammar`; `qemu-ios-workflow.py` |
| Privilege | Console enable without a configured secret; remote sessions require a secret; explicit levels; disable never raises privilege; removable enable secret; cancel an enable challenge | `TestIOSEnableConsoleAndVTY`; existing AAA tests; serial workflow |
| Directory | `/all`, `/recursive`, both orders, `all-filesystems`, individual files, IOS prefix wildcard matching, timestamp offsets, capacity footer, output filters | `TestIOSDirectoryOptionsAndConfinement` |
| Current directory | `cd`, `pwd`, relative file operands, volume changes, parent traversal confined to the volume, separate session defaults | `TestIOSWorkingDirectoryIsolation`; serial workflow |
| Delete | `/force`, `/recursive`, combinations and options before/after the URL; rooted filesystem operations; protected volume roots | Filesystem confinement and recursive-delete tests |
| Copy | Source filename prompt when omitted; destination default; explicit overwrite confirmation; cancellation and privilege recheck before mutation | `TestIOSCopyPromptsAndOverwrite`; serial configuration-copy workflow |
| Reload | Unsaved-configuration question, confirmation, Ctrl-C, immediate and reasoned reload, `in` minutes/hours, `at` time/date, both date orders, 24-day boundary, `cancel`, `show reload` | Reload parser/dialog tests; serial reload and reboot workflow |
| Reload lifetime | Timer independent of terminal connection; state survives management service restart; cancellation prevents firing; supervisor failure reported | `TestIOSReloadSaveConfirmCancelAndRestart` |
| Configuration display/save | Actual rendered configuration byte count; write acknowledgement; running/startup separation | CLI and serial persistence tests |
| Terminal | History enable/disable/default size; monitor on/off; filename and reload prompts preserve session display/history settings | Native C compilation; serial workflow |
| IPv4 | Address plus dotted subnet mask, secondary addresses, corresponding removal, contiguous-mask validation, dotted-mask configuration display | `TestIOSAddressMasksAndSaveOutput` |
| Filtering | Section selection with implicit/explicit include and exclude; Go and native-console filtering | `TestIOSSectionFilters`; serial section-filter check |
| Line timeout | `no exec-timeout` disables the timer; `default exec-timeout` restores ten minutes | `TestIOSLineTimeoutReset` |
| Session and clock display | Active-line marker, line/user/idle/peer columns; clock detail and synchronization marker | Serial workflow; native clock/session APIs |

Test names identify executable evidence, not a declaration of complete IOS
compatibility. The VM report identifies the exact tested system image hash.

## Platform boundaries and outstanding differences

This is **not yet an exhaustive, byte-for-byte IOS conformance certification**.
The following differences must remain visible in engineering documentation;
they must not be hidden behind accepted no-op keywords:

- ROMMON image signatures remain mandatory. `/noverify` is unavailable.
  Pre-reload `/verify` and `warm` are also rejected; verification happens in
  ROMMON, and the platform currently implements cold reload only.
- The storage uses immediate unlink, not the deleted-file records of legacy
  linear flash. `/all` has no additional deleted entries on these filesystems.
- Network copy protocols, IOS `file prompt` policy, and the entire
  IOS output redirection/filter family have not been qualified by this change.
  Copy's durable configuration representation is the appliance JSON model.
- Local AAA is not all IOS AAA: CLI views, arbitrary per-command privilege
  assignment, level-specific enable secrets, legacy Cisco password encodings,
  RADIUS/TACACS+, and every line/login setting need separate implementation and
  conformance work.
- Reload warning broadcasts and reload-history output have not been matched.
  Scheduled reloads are not retained across a complete machine reboot.
- All `show users` variants, interface counters/variants, NTP command options, clock
  configuration, and all terminal editor/history edge cases still need a
  command-by-command comparison. Existing support is not proof of parity.
- FractalCR workload, storage-array, Kata, Compose, BuildKit and Swarm commands
  are appliance extensions, not IOS commands.

## Running the checks

Run Go and C compilation on `carbon`, not on the Windows workstation. With the
submodules checked out, use the application package roots (the OE layer contains
Go files compiled as part of the separate Kata upstream module):

```text
go test ./cmd/... ./internal/... ./pkg/...
go vet ./cmd/... ./internal/... ./pkg/...
```

The native service check is `tools/check-services.sh` inside the Linux builder.
The serial qualification is `os/tests/qemu-ios-workflow.py` with disposable
launcher disks on a KVM host. The script tests actual console prompts and a
complete reboot, not just direct Go calls.
