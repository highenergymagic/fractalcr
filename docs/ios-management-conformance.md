# IOS management conformance

The reference baseline is an ISR 4331 running **IOS XE 17.12**, as selected by
the project owner. The ISR 4000 IOS XE 17 guide and Cisco's consolidated
command references supply the documented behavior. Cisco's consolidated
references also contain commands for other chassis, storage implementations,
and releases. Those entries are not evidence that every listed option is an
ISR 4331 feature. Hardware identity and measurements must describe the actual
FractalCR machine, not a fictional Cisco router.

## References

Consulted 2026-10-24:

- [Configuration Fundamentals: D through E](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/D_through_E.html): directory operations, privilege transitions, command editing, and `do`.
- [Configuration Fundamentals: R through setup](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/R_through_setup.html): reload scheduling and rename.
- [Configuration Fundamentals: C commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/C_commands.html): copy operands and interactive filename selection.
- [Configuration Fundamentals: F through K](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/F_through_K.html): `file prompt` confirmation policies and required information.
- [Configuration Fundamentals: show through show fm summary](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/show_through_show_fm_summary.html): filesystem types, aliases, flags and default-device marker.
- [Configuration Fundamentals: show protocols through showmon](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/show_protocols_through_showmon.html): scheduled reload status, configuration display, version information.
- [Configuration Fundamentals: terminal commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/slave_auto-sync_config_through_terminal-type.html): history, screen geometry, and terminal monitoring.
- [Configuration Fundamentals: monitor through Q](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/monitor_event-trace_through_Q.html): `pwd` and filesystem context.
- [Show-command filtering](https://www.cisco.com/c/en/us/td/docs/ios/fundamentals/command/reference/cf_book/cf_s1.html): section include/exclude behavior.
- [Cisco IOS XE advisory command example](https://sec.cloudapps.cisco.com/security/center/content/CiscoSecurityAdvisory/cisco-sa-webui-cmdij-FzZAeXAy): the displayed label for `show ... | count ...`.
- [show clock](https://www.cisco.com/E-Learning/bulk/public/tac/cim/cib/using_cisco_ios_software/cmdrefs/show_clock.htm): synchronization markers and detail output.
- [Security command reference: show through show users](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/security/s1/sec-s1-cr-book/sec-cr-s5.html): active/inactive lines and wide user names.
- [ISR 4000 console and SSH handling](https://www.cisco.com/c/en/us/td/docs/routers/access/isr4400/software/configuration/xe-17/isr4400-sw-config-xe-17/console_port_telnet_and_ssh_handling.html): line numbering and status displays.
- [Terminal services command reference](https://www.cisco.com/c/en/us/td/docs/ios/termserv/command/reference/tsv_book/tsv_s1.html): terminal detail fields.
- [Security: enable commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/security/d1/sec-d1-cr-book/sec-cr-e1.html): level-specific enable authentication.
- [Protecting secrets](https://www.cisco.com/c/en/us/about/trust-center/resilient-infrastructure/protecting-secrets.html): type 8 and 9 password verifiers.
- [Basic System Management: A through M](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/bsm/command/bsm-cr-book/bsm-cr-a1.html): software/hardware clocks, timezone offsets and summer-time transitions.
- [IP Addressing Services command reference](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/ipaddr/command/ipaddr-cr-book.pdf): primary and secondary IPv4 addresses and subnet masks.
- [ISR 4000 IOS XE 17 software installation](https://www.cisco.com/c/en/us/td/docs/routers/access/isr4400/software/configuration/xe-17/isr4400-sw-config-xe-17/installing_the_software.html): platform boot and filesystem context.

## Implemented and exercised in this change

| Area | Behavior | Evidence |
| --- | --- | --- |
| Context help | Word-boundary filesystem placeholder and mounted prefixes; partial filesystem help preserves the typed prefix; unique Tab completion | `TestIOSFilesystemHelpAndReloadGrammar`; `qemu-ios-workflow.py` |
| Privilege | Console enable without a configured secret; remote sessions require a secret; explicit levels; disable never raises privilege; removable enable secret; cancel an enable challenge; configuration commands reach the configuration transaction | `TestIOSEnableConsoleAndVTY`; `TestIOSEnableDispatchDoesNotConsumeConfiguration`; serial/SSH AAA workflow |
| Directory | `/all`, `/recursive`, both orders, `all-filesystems`, individual files, IOS prefix wildcard matching, timestamp offsets, capacity footer, output filters | `TestIOSDirectoryOptionsAndConfinement` |
| Filesystem display | Default bootflash marker, `flash:` alias, logical flash/NVRAM types, actual capacity and access flags | `TestIOSDirectoryOptionsAndConfinement` |
| Current directory | `cd`, `pwd`, relative file operands, volume changes, parent traversal confined to the volume, separate session defaults | `TestIOSWorkingDirectoryIsolation`; serial workflow |
| Delete | `/force`, `/recursive`, combinations and options before/after the URL; rooted filesystem operations; protected volume roots | Filesystem confinement and recursive-delete tests |
| Copy | Source filename prompt when omitted; destination default; explicit overwrite confirmation; cancellation and privilege recheck before mutation | `TestIOSCopyPromptsAndOverwrite`; serial configuration-copy workflow |
| File prompting | Persistent `alert`, `noisy`, `quiet`, `no` and `default` forms; quiet still requests missing source information; native copy/delete/rename/move/mkdir/rmdir confirmation handling; `/force` overrides delete prompting; policy changes do not restart platform services | `TestIOSFilePromptConfiguration`; `TestIOSFilePromptPolicy`; `TestFilePromptApplyDoesNotRestartServices`; serial workflow |
| Literal dialog input | Editor actions are separate JSON fields, never reserved password/filename strings; real Ctrl-C cancels without interpreting typed `__cancel` as an event | `TestManagementLiteralAuthenticationInput`; `TestManagementEventFraming`; `TestCopyLiteralControlFilenames`; serial workflow |
| Line endings | CR, LF and fragmented CRLF submit once across authentication/dialog state transitions | Native editor test; serial CRLF username/password workflow |
| Reload | Unsaved-configuration question, confirmation, Ctrl-C, immediate and reasoned reload, `in` minutes/hours, `at` time/date, both date orders, 24-day boundary, `cancel`, `show reload` | Reload parser/dialog tests; serial reload and reboot workflow |
| Reload lifetime | Timer independent of terminal connection; state survives management service restart; cancellation prevents firing; supervisor failure reported | `TestIOSReloadSaveConfirmCancelAndRestart` |
| Configuration display/save | Actual rendered configuration byte count; write acknowledgement; running/startup separation | CLI and serial persistence tests |
| Terminal | History enable/disable/default size; monitor on/off; filename and reload prompts preserve session display/history settings; heartbeat preserves partially typed usernames and passwords | Native C checks; serial workflow pauses across heartbeat intervals |
| IPv4 | Address plus dotted subnet mask, secondary addresses, corresponding removal, contiguous-mask validation, dotted-mask configuration display | `TestIOSAddressMasksAndSaveOutput` |
| Filtering | Section selection with implicit/explicit include and exclude; IOS count-result label in Go and native-console filtering | `TestIOSSectionFilters`; filter tests; serial section/count checks |
| Line timeout | `no exec-timeout` disables the timer; `default exec-timeout` restores ten minutes | `TestIOSLineTimeoutReset` |
| Session and clock display | Active-line marker, line/user/idle/peer columns; clock detail and synchronization marker | Serial workflow; native clock/session APIs |
| User and line displays | `show users all`, `wide`, and `all wide`; inactive VTY lines; terminal geometry, baud, history, editing, monitor state, timeout, activation time, and line-specific selection | Terminal/session unit tests; serial workflow |
| Terminal histories | Independent EXEC/configuration buffers; editing toggle; bounded history eviction when size changes; privilege downgrade/logout reset both buffers | Native C regression tests; serial workflow |
| Secret encodings | Type 8/9 import and generation, explicit SHA256/scrypt selection, type 7 password import rehashed to type 9, and independent enable secrets for levels 1 through 15 | Independent published hash vectors; configuration and authentication tests; serial level-7 authentication |
| Output files | `redirect`, `append`, `tee`, and `tee /append`; native-console and management output share rooted atomic file writes; unprivileged writes and protected destinations rejected | `TestOutputRedirectionAtomicAppendAndConfinement`; serial workflow, including `show reload` |
| Clock configuration | Both manual date orders, software/hardware clock APIs, `show calendar`, timezone offsets, recurring/dated summer-time rules, southern-hemisphere transitions, detail output | Clock API tests with injected clocks; transition boundary tests; serial display/configuration checks |
| Reload notices | Independent notice delivery per authenticated session; schedule/cancel/minute warning; warning state survives management restart without replay | `TestReloadNotificationsAndRestart`; serial schedule/cancel notices |
| Local-time scheduling | Target-date summer-time offset, repeated-hour selection, rejection of skipped local times, timezone label retained after management restart, clock-source validation independent of calendar year | `TestReloadAcrossSummerTime`; `TestReloadClockLabelSurvivesRestart`; `TestReloadRequiresClockSourceNotRecentYear` |
| Calendar device | Built-in x86 CMOS RTC driver, build-time driver requirements, native read/write operations, sanitized hardware failures | Kernel recipe assertions; native clock tests; serial calendar read/write qualification |
| Directory time | Configured timezone and the summer-time offset at each file's timestamp, for file, directory and all-filesystem listings | `TestDirectoryUsesConfiguredClockAtFileTimestamp` |

Test names identify executable evidence, not a declaration of complete IOS
compatibility. The VM report identifies the exact tested system image hash.
These checks compare documented behavior with FractalCR tests. They do not run
the same command transcript against a licensed ISR 4331 IOS XE instance, and
therefore do not establish byte-for-byte prompt/output equivalence. Differences
between Cisco platforms and releases mean that a future device transcript must
identify its precise 17.12 maintenance release before serving as that oracle.

## Platform boundaries and outstanding differences

This is **not yet an exhaustive, byte-for-byte IOS conformance certification**.
The following differences must remain visible in engineering documentation;
they must not be hidden behind accepted no-op keywords:

- ROMMON image signatures remain mandatory. `/noverify` is unavailable.
  Pre-reload `/verify` and `warm` are also rejected; verification happens in
  ROMMON, and the platform currently implements cold reload only.
- The storage uses immediate unlink, not the deleted-file records of legacy
  linear flash. `/all` has no additional deleted entries on these filesystems.
- Network copy protocols remain unimplemented. Output redirection currently
  supports local writable volumes, with a 1 MiB response limit and a 64 MiB
  existing-file limit for append. Copy's durable configuration representation
  is the appliance JSON model.
- Local AAA is not all IOS AAA: CLI views, arbitrary per-command privilege
  assignment, additional Cisco password encodings (including type 5/6/14),
  and additional line/login settings need separate implementation and
  conformance work.
  External RADIUS/TACACS+ services remain outside the previously agreed
  [local AAA scope](../os/docs/aaa-and-management.md); they are not implemented.
- Reload warning cadence and reload-history output have not been fully matched.
  Scheduled reloads are not retained across a complete machine reboot.
- Remaining `show users` variants, interface counters/variants, NTP command
  options, and terminal editor/history edge cases still need a
  command-by-command comparison. Existing support is not proof of parity.
  Concrete gaps include `show users summary`; persistent line-use counters;
  additional line/terminal settings and their corresponding display fields;
  and the complete
  clock command family (`clock calendar-valid`, periodic calendar saving).
  Generic clock and NTP
  support does not imply support for every IOS clock/NTP command.
  The references do not specify the choice between repeated local times or
  the diagnostic for a skipped local time: those two reload outputs remain
  pending a device transcript even though transition arithmetic is tested.
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

`tools/qualify-management.py --artifacts <signed-artifacts> --output <new-directory>`
prepares disposable launcher disks and runs both the serial and console/SSH AAA
suites sequentially on KVM. Its result binds both reports to the signed image
hash. The development-release workflow requires this gate and publishes its
report as `management-qualification.json`; it fails rather than skipping the
checks when its runner lacks KVM. Test disks and private SSH material are not
included in release assets or failure diagnostics.
