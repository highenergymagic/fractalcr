# Appliance management and local AAA

The scope is IOS-style appliance management: the shared CLI editor and help
grammar, local authentication, EXEC authorization, accounting, console and
SSH sessions, enable/disable, timeouts and output filters. Routing protocols,
Cisco binary compatibility and external TACACS+/RADIUS services are not part
of this scope. Normal configuration commands update running state immediately; `write memory`
persists the JSON startup configuration. `configure candidate` explicitly stages
a transaction when needed.

The initial physical console permits bootstrap configuration. SSH has no
listener until local AAA and SSH are enabled. Configure an administrator first:

```
configure terminal
security passwords min-length 12
username admin privilege 15 secret <choose-a-secret-of-at-least-12-characters>
username operator privilege 1 secret <choose-a-different-secret>
enable secret <choose-an-enable-secret>
aaa new-model
aaa authentication login default local
aaa authorization exec default local
aaa accounting exec default start-stop local
aaa accounting commands 15 default start-stop local
ip ssh version 2
line console 0
login local
exec-timeout 10 0
exit
line vty 0 15
login local
transport input ssh
exec-timeout 10 0
exit
end
write memory
logout
```

Activating AAA preserves the current physical bootstrap session until logout or
its configured idle timeout. New sessions require local login immediately.
Configuration cannot enable AAA
without a privilege 15 account or remove its last administrator. New secrets use
IOS type 9 scrypt verifiers with fresh salts. `algorithm-type sha256 secret`
selects IOS type 8 PBKDF2-SHA256; `algorithm-type scrypt secret` selects type 9.
Existing FractalCR PBKDF2 verifiers remain readable. Both `username NAME privilege LEVEL password 0 TEXT`
and `username NAME privilege LEVEL secret 0 TEXT` accept plaintext input that is
immediately hashed. Keywords accept unique prefixes, including
`user freya priv 15 password 0 TEXT`. The `0` is optional. Omitting `privilege`
retains an existing account's level or uses level 1 for a new account;
`username NAME privilege LEVEL` changes an existing user's level without
replacing its password. `secret 8` and `secret 9` import validated encoded
verifiers. `password 7` imports a reversible IOS password and immediately hashes
it as type 9; the reversible representation is not retained. Other Cisco secret
encodings remain unsupported. The input
may contain 1..256 characters. `security passwords min-length 12` enforces an explicit minimum for newly configured passwords. Password prompts do not echo or
enter command history. EXEC and configuration modes have separate history buffers;
configuration history includes commands as entered, including secret-setting
commands. Logout and privilege downgrade clear both buffers.

By default, privilege 0 provides session control, enable and help. Levels 1 through 14
provide explicitly allowed read-only views and terminal settings; level 15
provides configuration and operational control. Native Docker/BuildKit commands,
workload logs, configuration files, service control and debug entry require
level 15. `enable` defaults to level 15. `enable LEVEL` authenticates the secret
configured by `enable secret level LEVEL ...`; each level has a separate
verifier. An unset secret allows elevation at the physical console and denies
remote elevation. `disable` returns
to level 1 (remaining at level 0 if already there) and clears terminal history. Help and completion use the same
authorization rules as execution. SSH sessions have the same editor and pager
as the physical console, with independent identities and privileges. Normal
configuration commands use the latest running state, preserving other sessions'
edits. Explicit candidate transactions reject stale commits.

Command privileges can delegate individual operations and configuration modes:

```
configure terminal
privilege exec level 5 configure terminal
privilege configure level 5 hostname
privilege exec level 5 request container-engine events
username operator privilege 5
end
write memory
```

That account can change the hostname and observe Engine events. Account
management and other Engine operations retain their default level. A command
assignment also sets its parent keywords; set a parent explicitly afterward
when it needs a different level. `all` assigns descendants and associated
configuration submodes, for example `privilege configure all level 5 interface`.
Use `privilege ... reset COMMAND` or `no privilege ... level LEVEL COMMAND`
to remove an override; add `all` to reset descendants as well. The level in
the `no` form does not need to match the previous assignment.

Running configuration preserves `all` groups and narrower command exceptions.
Partial resets preserve unrelated permissions and retain default parent levels
when needed for safe CLI replay. Startup storage groups equal node assignments
and has a 256 KiB limit; both loading and saving enforce that limit.

Help, completion and execution use the current running policy. Pending file
and reload dialogs recheck it before acting. Running native clients check their
operation grant once per second and stop on revocation or loss of management;
these checks do not extend the session's idle timeout. Saving with `write memory`
persists the policy across a reboot.

At a delegated privilege level, `show running-config` and `write terminal`
display only configuration commands the user can modify. Grant
`show running-config view full` separately to allow a complete view without
granting configuration changes. JSON configuration views remain separate full
exports with their own command permissions.

File content access, running/startup configuration displays, and `write terminal`
also require the configured `file privilege` level, which defaults to 15.
For example, delegating `copy` to level 5 requires both
`privilege exec all level 5 copy` and `file privilege 5`. Reset file access with
`no file privilege`. Directory metadata remains available in user EXEC.

Three failed local attempts lock that account for 30 seconds. SSH additionally
limits authentication attempts and concurrent connections. Zero `exec-timeout`
disables idle expiry; otherwise timeout is expressed as minutes and seconds.
Deleting a user, replacing its secret or changing its assigned privilege revokes
existing sessions. SSH accepts interactive PTY sessions only: arbitrary exec,
SFTP, environment injection and forwarding requests are rejected. The persistent
Ed25519 SSH host key is generated on the datastore with mode 0600. There is no
default SSH credential and no Telnet listener.

For the Windows launcher using QEMU user networking, add `-SshPort 2222` to
forward localhost TCP 2222 to guest TCP 22. After enabling AAA and SSH above,
connect with `ssh -p 2222 admin@127.0.0.1`. The forwarding listener binds only
to localhost and is omitted unless requested. It assumes the launcher's default
QEMU management network and guest SSH port 22.

Accounting is always active when AAA is enabled. Events are JSON lines in
`/datastore/data0/log/aaa.jsonl`, with a 4 MiB active file and one rotated file.
Login and administrative command execution fail closed if accounting cannot be
written. The log records identity, transport, event and command class, omitting
arguments that may contain passwords, tokens or workload secrets. It is a local
operational audit trail, not an externally anchored tamper-proof record.

Authentication also fails closed when configuration cannot be read. The local
fallback parser does not grant privileged access when AAA is configured or its
configuration is unreadable. Restarting management does not bypass AAA.

For physical-console password recovery, interrupt ROMMON autoboot, enter
`confreg 0x2142`, then `boot`. Without `sync`, this affects only the current
monitor session. ROMMON still verifies both image signatures; it supplies only
the fixed `fcr.ignore_startup=1` recovery flag. The OS leaves startup-config in
place, starts with bootstrap console access and SSH disabled, and displays a
recovery warning. Enter `configure candidate`, then `do copy startup-config candidate-config`,
replace the forgotten user's secret, then `commit`, `end` and `logout`. Log in
with the replacement secret and `write memory`. Saving in recovery preserves
the original file as `startup-config.json.pre-recovery` without overwriting an
existing backup. For malformed JSON, build a replacement candidate instead of
copying the damaged file. Reboot normally; if 0x2142 was saved with `sync`, first
restore `confreg 0x2102` and `sync` in ROMMON. Protect physical-console and
bootflash access: recovery intentionally authorizes their operator to reset AAA.

Output filters use POSIX extended regular expressions and accept unique keyword
abbreviations through the normal grammar:

```
show version | include ^Platform
show platform software service | exclude stopped
show logging | begin SERVICE_READY
show platform software service | count running
```

`show version | ?` lists only filter keywords, and `show version ?` includes
`|` and `<cr>`. This is output filtering, not Unix shell piping.

The VM acceptance harness is `os/tests/qemu-aaa.py`. It validates serial and SSH
behavior, simultaneous sessions, privilege denial, filtering, timeout, account
revocation, persistence and SSH exec rejection against the actual image.
