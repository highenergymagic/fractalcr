# Management command conformance

FractalCR uses IOS XE 17.12 documentation for ISR 4000 management conventions.
This reference maps implemented behavior to its checks and lists differences.
The command references also include other Cisco platforms; their platform-specific
options do not imply support in FractalCR. Hardware identity and measurements
describe the actual appliance.

## References

Consulted 2026-10-24:

- [Configuration Fundamentals: D through E](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/D_through_E.html): directory operations, privilege transitions, command editing, and `do`.
- [Configuration Fundamentals: R through setup](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/R_through_setup.html): reload scheduling and rename.
- [Configuration Fundamentals: C commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/C_commands.html): copy operands and interactive filename selection.
- [Configuration Fundamentals: F through K](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/F_through_K.html): `file prompt` confirmation policies and required information.
- [Configuration Fundamentals: L through mode](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/L_through_mode.html): persistent terminal page length and its reset form.
- [IOS XE 17 configuration partition examples](https://www.cisco.com/c/en/us/td/docs/routers/ios/config/17-x/syst-mgmt/b-system-management/m_cm-partition-0.pdf): canonical `line con` output and single-line versus VTY-range display.
- [Configuration Fundamentals: show through show fm summary](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/show_through_show_fm_summary.html): filesystem types, aliases, flags and default-device marker.
- [Configuration Fundamentals: show protocols through showmon](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/show_protocols_through_showmon.html): scheduled reload status, configuration display, version information.
- [Configuration Fundamentals: terminal commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/slave_auto-sync_config_through_terminal-type.html): history, screen geometry, and terminal monitoring.
- [Configuration Fundamentals: monitor through Q](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/monitor_event-trace_through_Q.html): `pwd`, `more`, file display formats, configuration aliases and output filters.
- [Show-command filtering](https://www.cisco.com/c/en/us/td/docs/ios/fundamentals/command/reference/cf_book/cf_s1.html): section include/exclude behavior.
- [Cisco IOS XE advisory command example](https://sec.cloudapps.cisco.com/security/center/content/CiscoSecurityAdvisory/cisco-sa-webui-cmdij-FzZAeXAy): the displayed label for `show ... | count ...`.
- [show clock](https://www.cisco.com/E-Learning/bulk/public/tac/cim/cib/using_cisco_ios_software/cmdrefs/show_clock.htm): synchronization markers and detail output.
- [Security command reference: show through show users](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/security/s1/sec-s1-cr-book/sec-cr-s5.html): active/inactive lines and wide user names.
- [ISR 4000 console and SSH handling](https://www.cisco.com/c/en/us/td/docs/routers/access/isr4400/software/configuration/xe-17/isr4400-sw-config-xe-17/console_port_telnet_and_ssh_handling.html): line numbering and status displays.
- [Terminal services command reference](https://www.cisco.com/c/en/us/td/docs/ios/termserv/command/reference/tsv_book/tsv_s1.html): terminal detail fields.
- [Security: enable commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/security/d1/sec-d1-cr-book/sec-cr-e1.html): level-specific enable authentication.
- [Security: privilege commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/security/m1/sec-m1-cr-book/ppp_accounting_through_quit.html): command levels, implicit parents, `all`, submodes and reset behavior.
- [Running configuration at lower privilege levels](https://www.cisco.com/c/en/us/support/docs/routers/asr-1000-series-aggregation-services-routers/212149-Configure-IOS-XE-to-display-full-show-ru.html): configuration visibility and the separately authorized `view full` variant.
- [Configuration Fundamentals: write commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/fundamentals/command/cf_command_ref/test_cable-diagnostics_through_xmodem.html): `write terminal` as a running-configuration display alias.
- [Protecting secrets](https://www.cisco.com/c/en/us/about/trust-center/resilient-infrastructure/protecting-secrets.html): type 8 and 9 password verifiers.
- [IOS XE 17.12 secret migration](https://www.cisco.com/content/en/us/td/docs/switches/lan/catalyst9500/software/release/17-12/configuration_guide/sec/b_1712_sec_9500_cg.pdf): type 5 imports and platform-dependent conversion to convoluted type 9. This Catalyst reference does not establish ISR conversion behavior.
- [Basic System Management: A through M](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/bsm/command/bsm-cr-book/bsm-cr-a1.html): software/hardware clocks, timezone offsets and summer-time transitions.
- [Basic System Management: N through T](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/bsm/command/bsm-cr-book/m_bsm-cr-n1.html): opt-in periodic NTP calendar updates.
- [Interface and Hardware commands](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/interface/command/ir-cr-book/ir-s4.html): selected-interface status and description displays.
- [IP Addressing Services command reference](https://www.cisco.com/c/en/us/td/docs/ios-xml/ios/ipaddr/command/ipaddr-cr-book.pdf): primary and secondary IPv4 addresses and subnet masks.
- [ISR 4000 IOS XE 17 software installation](https://www.cisco.com/c/en/us/td/docs/routers/access/isr4400/software/configuration/xe-17/isr4400-sw-config-xe-17/installing_the_software.html): platform boot and filesystem context.

## Implementation and test coverage

This table is a contributor reference. Named Go tests are in `internal/cli`
unless another package or path is shown. Console and SSH image tests are in
`os/tests/qemu-ios-workflow.py` and `os/tests/qemu-aaa.py`; the management
qualification tool runs them against a signed image. A pending image result
means the source check exists but image-level verification is incomplete.

| Area | Behavior | Verification |
| --- | --- | --- |
| Context help | Word-boundary filesystem placeholder and mounted prefixes; partial filesystem help preserves the typed prefix; unique Tab completion | `TestIOSFilesystemHelpAndReloadGrammar`; `qemu-ios-workflow.py` |
| Privilege | Console enable without a configured secret; remote sessions require a secret; explicit levels; disable never raises privilege; removable enable secret; cancel an enable challenge; configuration commands reach the configuration transaction | `TestIOSEnableConsoleAndVTY`; `TestIOSEnableDispatchDoesNotConsumeConfiguration`; serial/SSH AAA workflow |
| Command privileges | `privilege mode [all] {level level \| reset} command-string` and `no` reset; parent-keyword assignments; descendant/submode assignment; privilege-filtered configuration help and execution; deterministic saved-policy replay | `TestCommandPrivilegeDelegationAndReplay`; `TestCommandPrivilegeParentsResetAndZero`; `TestPrivilegeAllIncludesSubmodesAndRevokesLive` |
| Privilege configuration output | Retain the documented `all` form, including implicit EXEC parents; preserve narrower exceptions and defaults after partial resets; compact node groups in bounded startup storage | `TestPrivilegeAllRetainsDocumentedConfigurationForm`; `TestPrivilegeGroupsMatchNodeAssignmentsAcrossEdits`; `TestPrivilegeNestedGroupsAndPartialReset`; configuration capacity/load tests |
| Command revocation | File, reload and output confirmations recheck current command policy; native client streams recheck the parsed operation, including after global options, without renewing idle deadlines | `TestDelegatedFileConfirmationRevokesOnPolicyChange`; `TestDelegatedCopySaveAndRevocation`; `TestReloadAndOutputRecheckCommandPolicy`; `TestNativeStreamGrantRevocationAndIdleDeadline`; expanded SSH qualification |
| Delegated configuration output | Ordinary `show running-config` and `write terminal` show commands editable at the session level; `view full` requires a separate grant; read-only filters inherit the display command's level | `TestDelegatedRunningConfigurationVisibility`; expanded SSH qualification |
| File access levels | `file privilege 0..15`, default 15; command permission and content-access permission are separate, including running/startup configuration displays and `write terminal`; changing either revokes pending file mutations | `TestFilePrivilegeIsIndependentFromCommandGrant`; `TestCopySaveRejectsDirtyTransactionAndRevokedFileAccess`; SSH file-access revocation check |
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
| Clock configuration | Both manual date orders, software/hardware clock APIs, `show calendar`, timezone offsets, recurring/dated summer-time rules, southern-hemisphere transitions, detail output | `TestClockEXECReachesNativeOperations` verifies EXEC/`do` dispatch and authorization; clock API tests with injected clocks; transition boundary tests; serial display/configuration checks |
| Reload notices | Independent notice delivery per authenticated session; schedule/cancel/minute warning; warning state survives management restart without replay | `TestReloadNotificationsAndRestart`; serial schedule/cancel notices |
| Local-time scheduling | Target-date summer-time offset, repeated-hour selection, rejection of skipped local times, timezone label retained after management restart, clock-source validation independent of calendar year | `TestReloadAcrossSummerTime`; `TestReloadClockLabelSurvivesRestart`; `TestReloadRequiresClockSourceNotRecentYear` |
| Calendar device | Built-in x86 CMOS RTC driver, build-time driver requirements, native read/write operations, sanitized hardware failures | Kernel recipe assertions; native clock tests; serial calendar read/write qualification |
| Directory time | Configured timezone and the summer-time offset at each file's timestamp, for file, directory and all-filesystem listings | `TestDirectoryUsesConfiguredClockAtFileTimestamp` |
| Calendar policy | `ntp update-calendar`, `no` and `default` forms; disabled by default; NTP synchronization independent of RTC update policy; time configuration restarts only time-sync and rolls back on failure | Time-service and CLI policy tests; serial policy/save/reboot qualification |
| Calendar EXEC privilege | `clock read-calendar` and `clock update-calendar` available at user EXEC; arbitrary date/time setting remains privileged | `TestClockEXECReachesNativeOperations` |
| NVRAM clock fallback | `clock save interval 8..24` and its documented numbered `no` form; unavailable with a hardware calendar; persist only NTP update timestamps, enforce intervals across service restarts, restore before networking once per boot without rewinding a newer clock | `TestClockSaveIntervalGrammarAndPersistence`; `TestTrackingReferenceRequiresNetworkTime`; `TestSavedClockIntervalAndAuthority`; `TestSavedClockRestoresOnceBeforeNetworking`; `TestSavedClockDevicePolicyAndFailures`; three-cold-boot no-RTC KVM qualification in the release gate |
| Startup display | Documented missing-file response and `Using ... out of ... bytes` header; byte count comes from the same validated saved-file read; capacity is the enforced 256 KiB configuration slot; sanitized read failures | `TestStartupConfigurationOutputAndFailures`; serial qualification |
| Numbered configuration | `show running-config linenum`, numbered configuration lines below the unnumbered size header, output filtering after numbering, delegated visibility and file-access gates | `TestRunningConfigurationLineNumbers`; documented Configuration Fundamentals sample; serial qualification |
| Legacy secret import | Type 5 user and per-level enable secret import, native MD5-crypt verification, bounded canonical encoding and constant-time comparison; plaintext configuration continues to generate type 9 | `TestType5IndependentVectors` using independent OpenSSL vectors; `TestIOSSecretImportAndAlgorithms`; console/SSH authentication qualification |
| Line usage | Authenticated uses counted per physical/virtual line; retained across management restart; reset on a full machine boot | `TestLineCountersSurviveManagementRestart`; console/SSH restart qualification |
| Interface selection | Selected interface detail, description, and IP brief display; configured descriptions and observed link state; administrative shutdown forces protocol down | Interface display/dispatch tests; serial qualification |
| File display | Native `more`, configuration aliases, contextual filesystem help, format selection, output filters, privilege checks, rooted regular-file reads, bounded decompression and terminal control handling | `TestMoreConfigurationAliasesAndPolicy`; `TestMoreNativeConfinementAndWorkingDirectory`; `TestFormatsAndTerminalSafety`; `TestCompressedIntegrityAndLimits`; serial qualification |
| Interface configuration display | `show running-config interface` with joined or separated type/index and optional `linenum`; selected-port output, delegated visibility and file-access gates; `(config-if)#` prompt | `TestInterfaceRunningConfiguration`; configuration replay tests; serial qualification |
| Interactive pager | `/` searches remaining output, `+` includes matching lines, `-` excludes them; first filter remains active while later searches are allowed; complete lines span API/log chunks; EOF and Ctrl-C/Z/^ stop output; exact-width lines do not acquire a spurious row | `os/tests/test-pager.c` with ASan/UBSan; native service compiler checks; serial qualification |
| Persistent line terminals | Console/VTY `history`, `history size`, `editing`, `length`, and `no`/`default` forms; independent VTY ranges; line history-size changes do not reenable disabled history; new sessions inherit saved defaults while unrelated edits preserve session overrides; canonical `line con`, single-VTY display and omitted default timeout | `TestLineTerminalPersistenceAndDefaults`; `TestLineTerminalValidationAndClone`; `TestLineTerminalPolicyAndSessionOverrides`; `TestLineTerminalDelegatedDisplay`; new-image serial qualification pending |
| Word editing | Escape-D cuts the next word into the delete buffer; Escape-C/U/L capitalize, uppercase and lowercase the remaining word; upper/lowercase key variants and leading spaces are handled | `os/tests/test-line-editor.c` with ASan/UBSan; new-image serial qualification pending |
| Configuration files | CLI-text running/startup exports, local-file merge into running configuration, validated startup replacement, legacy JSON import, original-policy authorization throughout parsing, rooted bounded regular-file reads | `TestConfigurationImportIsStagedAndRoundTrips`; `TestConfigurationImportUsesOriginalAuthorization`; `TestConfigurationCopiesExportCLIAndImportObjects`; `TestConfigurationImportFileConfinement`; new-image serial qualification pending |
| Calendar authority | `clock calendar-valid`, `no` and `default`; persistent hardware-calendar authority, NTP precedence, no promotion of a stopped NVRAM timestamp; command hidden/rejected without an RTC | `TestCalendarAuthorityConfiguration`; `TestCalendarAuthoritySourceSelection`; new-image serial qualification pending. This implements clock authority/display policy; NTP server distribution is still outstanding. |
| Reload during engine initialization | Defer engine termination until API readiness, retaining providers; keep the original startup deadline and bounded forced cleanup; preserve Engine state | Native supervisor and dependency-graph fault tests; five-cycle image qualification pending for the startup/reload issue. See [Release status](development-status.md). |

Test names identify source checks. Release qualification reports identify the
exact tested image. Comparison against a physical ISR 4331 transcript has not
been completed; byte-for-byte output equivalence is not established. A device
comparison must identify the exact IOS XE 17.12 maintenance release.

## Restrictions and output differences

| Area | Restriction or difference |
| --- | --- |
| Reload verification | `/noverify`, pre-reload `/verify`, and `warm` are rejected. ROMMON authenticates every system image; only cold reload is supported. |
| File deletion | Deletion immediately unlinks a file. There are no legacy flash deleted-file records for `/all` to display. |
| Network file access | System CLI copy and output redirection use local volumes. Network copy protocols are unavailable; ROMMON provides TFTP recovery separately. |
| Output limits | Redirection accepts at most 1 MiB per response; append accepts an existing file up to 64 MiB. `more` limits both reads and decompression to 256 KiB. |
| Configuration import | CLI-text exports and legacy JSON imports are supported. Imports validate the complete input before applying it atomically. Durable startup storage is JSON. Invalid input does not produce IOS-style partial application. |
| File display and pager | ASCII, hex/text, gzip/zlib, and EBCDIC code page 037 are supported. Other compression formats and remote URLs are unavailable. Exact ISR binary-dump spacing, invalid-pattern diagnostics, and active-filter replacement diagnostics are unverified. |
| Authentication | Local AAA is supported. RADIUS, TACACS+, CLI views, password types 6/14, and automatic conversion of imported type 5 secrets to type 14 are unavailable. |
| Command privileges | Parser modes and native workload operations support delegation. Per-option grants inside workload clients are unavailable. Exact ISR formatting after partial privilege-group resets requires device comparison. |
| Reload persistence and output | Scheduled reloads survive management restart, but not a machine reboot. Warning cadence and reload-history output are not fully matched to ISR output. |
| Terminal and interface displays | `show users summary`, additional terminal settings, and additional interface counter/display variants remain incomplete. See the CLI reference for available commands. |
| Time services | NTP client synchronization and optional hardware-calendar updates are supported. The complete IOS NTP command family, including serving authoritative calendar time to clients, is unavailable. Restored NVRAM time displays `*` and `Time source is NVRAM`; exact ISR equivalence is unverified. |
| Daylight-saving transitions | Reload scheduling arithmetic is tested. The reference does not define repeated-hour selection or skipped-hour diagnostics; their exact output requires a device transcript. |
| Appliance extensions | Workload, datastore-array, Kata, Compose, BuildKit, and Swarm commands are FractalCR extensions. |

For supported command procedures, see the [CLI reference](../os/CLI.md) and
[Local AAA](../os/docs/aaa-and-management.md).

## Running the checks

Run Go and C compilation on a Linux development host. With the
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
prepares disposable launcher disks and runs the serial, console/SSH AAA and
no-RTC clock-restoration suites sequentially on KVM. Its result binds the console
reports to the signed image hash and the clock report to the exact kernel and
source identity. The development-release workflow publishes this result as
`management-qualification.json`. These tests require KVM. Test disks and private SSH material are not
included in release assets or failure diagnostics.
