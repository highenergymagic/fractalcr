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
without a privilege 15 account or remove its last administrator. Secrets use
PBKDF2-HMAC-SHA256 with 600,000 iterations and a fresh 128-bit salt. They are
not Cisco type 7/8/9 encodings. Both `username NAME privilege LEVEL password 0 TEXT`
and `username NAME privilege LEVEL secret 0 TEXT` accept plaintext input that is
immediately hashed. Keywords accept unique prefixes, including
`user freya priv 15 password 0 TEXT`. The `0` is optional. Omitting `privilege`
retains an existing account's level or uses level 1 for a new account;
`username NAME privilege LEVEL` changes an existing user's level without
replacing its password. Encoded Cisco password imports are rejected. The input
may contain 1..256 characters. `security passwords min-length 12` enforces an explicit minimum for newly configured passwords. Password prompts do not echo or
enter command history. Configuration commands are omitted from history.

Privilege 0 provides session control, enable and help. Levels 1 through 14
provide explicitly allowed read-only views and terminal settings; level 15
provides configuration and operational control. Native Docker/BuildKit commands,
workload logs, configuration files, service control and debug entry require
level 15. `enable` authenticates the configured enable secret; `disable` returns
to level 1 (remaining at level 0 if already there) and clears terminal history. Help and completion use the same
authorization rules as execution. SSH sessions have the same editor and pager
as the physical console, with independent identities and privileges. Normal
configuration commands use the latest running state, preserving other sessions'
edits. Explicit candidate transactions reject stale commits.

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
