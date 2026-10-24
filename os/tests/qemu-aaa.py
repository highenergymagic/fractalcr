#!/usr/bin/env python3
"""Serial and SSH AAA/CLI acceptance against the actual immutable appliance."""
import argparse, hashlib, json, re, shutil, socket, subprocess, sys, time
from pathlib import Path
import pexpect
from ios_prompts import reload_appliance
SRC=Path(__file__).resolve().parents[2]
sys.path.insert(0,str(SRC/'boot/tests'))
from qemu_process import stop_qemu
p=argparse.ArgumentParser();p.add_argument('--artifacts',type=Path,required=True);p.add_argument('--output',type=Path,required=True)
a=p.parse_args();art=a.artifacts.resolve();out=a.output.resolve();out.mkdir(parents=True,exist_ok=False)
result=dict(case='aaa',passed=False,accel='kvm',checks=[],system_sha256=hashlib.sha256((art/'system.bin').read_bytes()).hexdigest(),rommon_sha256=hashlib.sha256((art/'rommon.efi').read_bytes()).hexdigest())
with socket.socket() as s:s.bind(('127.0.0.1',0));port=s.getsockname()[1]
flash=out/'flash.img';data=out/'data.img';variables=out/'vars.fd'
subprocess.run(['python3',str(SRC/'boot/tools/make-flash.py'),'--efi',str(art/'rommon.efi'),'--system',str(art/'system.bin'),'--output',str(flash),'--size-mib','1024','--no-tampered-fixture'],check=True,stdout=subprocess.DEVNULL)
subprocess.run(['python3',str(SRC/'os/tools/make-data.py'),'--output',str(data)],check=True,stdout=subprocess.DEVNULL)
shutil.copyfile(art/'OVMF_VARS-trusted.fd',variables)
log=(out/'serial.log').open('w');child=None;clients=[]
def check(text):result['checks'].append(text);print('PASS '+text,flush=True)
def spawn(recovery=False):
    c=pexpect.spawn('qemu-system-x86_64',['-machine','q35,smm=on','-accel','kvm','-cpu','host,vmx=off,svm=off','-m','4096','-smp','2',
        '-global','driver=cfi.pflash01,property=secure,value=on','-drive',f'if=pflash,format=raw,unit=0,readonly=on,file={art/"OVMF_CODE_4M.secboot.fd"}',
        '-drive',f'if=pflash,format=raw,unit=1,file={variables}','-drive',f'if=virtio,format=raw,file={flash}','-drive',f'if=virtio,format=raw,file={data}',
        '-netdev',f'user,id=net0,hostfwd=tcp:127.0.0.1:{port}-:22','-device','virtio-net-pci,netdev=net0',
        '-display','none','-chardev','stdio,id=serial0,signal=off','-serial','chardev:serial0','-monitor','none','-no-reboot'],encoding='utf-8',timeout=180)
    c.logfile_read=log
    if recovery:
        c.expect_exact('Autoboot in 5 seconds');c.sendcontrol('c');c.expect(r'rommon \d+ > ')
        c.sendline('confreg 0x2142');c.expect(r'rommon \d+ > ');c.sendline('boot')
    c.expect_exact('%FCR-5-SYSTEM_READY');c.expect_exact('Press RETURN to get started!');c.sendline('');return c
def prompt(c,config=False,user=False):c.expect(r'ctr(?:-test)?(?:\(config[^)]*\))?'+('> ' if user else '# '),timeout=90)
def command(c,text,user=False):
    c.sendline(text);prompt(c,user=user);return c.before
def login(c,user,password):
    c.sendline(user);c.expect_exact('Password: ');c.sendline(password);prompt(c,user=user=='reader')
    assert password not in c.before,'password echoed'
def ssh_login(user,password):
    # Line policy is reconciled by the SSH service once per second. Wait for
    # its listener after re-enabling VTY transport; never retry authentication.
    deadline=time.monotonic()+15
    while True:
        c=pexpect.spawn('ssh',['-tt','-p',str(port),'-o','StrictHostKeyChecking=accept-new','-o',f'UserKnownHostsFile={out/"known_hosts"}',
                              '-o','PubkeyAuthentication=no','-o','PreferredAuthentications=password',f'{user}@127.0.0.1'],encoding='utf-8',timeout=30)
        c.logfile_read=(out/f'ssh-{user}-{len(clients)}.log').open('w');clients.append(c)
        try:
            c.expect('password:')
            break
        except pexpect.EOF:
            diagnostic=c.before
            c.close()
            if time.monotonic() >= deadline or not any(s in diagnostic for s in ['Connection reset','Connection refused','Connection closed']): raise
            time.sleep(.25)
    c.sendline(password);c.expect_exact('Press RETURN to get started!');c.sendline('');prompt(c,user=user=='reader')
    command(c,'terminal width 512',user=user=='reader');command(c,'terminal length 0',user=user=='reader')
    return c
ADMIN='Qualification-admin-410!';READER='Qualification-reader-410!';ENABLE='Qualification-enable-410!'
try:
    child=spawn();prompt(child);command(child,'terminal length 0');command(child,'terminal width 512')
    for line in ['conf t',f'user freya priv 15 password 0 {ADMIN}',f'user admin priv 15 password 0 {ADMIN}',f'username reader privilege 1 secret 0 {READER}',f'enable secret 0 {ENABLE}',
                 'aaa new-model','aaa authentication login default local','aaa authorization exec default local','aaa accounting exec default start-stop local',
                 'ip ssh version 2','line console 0','login local','exec-timeout 10 0','exit','line vty 0 15','login local','transport input ssh','exec-timeout 10 0','exit']:
        response=command(child,line)
        assert not re.search(r'% (?:Invalid|Incomplete|Ambiguous|Error)',response),response
    child.send('?');prompt(child)
    assert all(word in child.before for word in ['username','interface','network','ntp','container','storage','system','aaa']),child.before
    command(child,'end');child.sendline('logout');child.expect_exact('Username: ');login(child,'freya',ADMIN)
    assert 'privilege level is 15' in command(child,'sh priv')
    child.sendline('logout');child.expect_exact('Username: ');login(child,'admin',ADMIN)
    command(child,'terminal length 0');command(child,'write memory')
    assert ADMIN not in command(child,'show history')
    running=command(child,'show running-config')
    assert ADMIN not in running and 'enable secret 9 $9$' in running,running
    check('abbreviated username/password 0 and secret 0 configure local AAA; bootstrap privilege ends at logout; serial password hidden; startup configuration saved')
    child.sendline('logout');child.expect_exact('Username: ');login(child,'reader',READER)
    assert 'privilege level is 1' in command(child,'sh priv',user=True)
    for text in ['conf t','request container-engine run anything','request platform debug service-container enter','show running-config']:
        response=command(child,text,user=True);assert 'Invalid input' in response or 'Authorization failed' in response,response
    child.send('sh?');child.expect_exact('show');prompt(child,user=True);child.sendcontrol('u');prompt(child,user=True)
    child.sendline('show version | include ^Platform');prompt(child,user=True)
    assert 'Platform kernel:' in child.before and 'FractalCR Software' not in child.before,child.before
    child.sendline('enable');child.expect_exact('Password: ');time.sleep(2);child.sendline(ENABLE);prompt(child)
    assert 'privilege level is 15' in command(child,'show privilege')
    command(child,'disable',user=True)
    check('user EXEC authorization, inline help, output filter, enable password and disable work over serial')
    admin=ssh_login('admin',ADMIN);reader=ssh_login('reader',READER)
    assert 'privilege level is 1' in command(reader,'show privilege',user=True)
    for line in ['conf t','no enable secret','end']: command(admin,line)
    assert 'No password set' in command(reader,'enable',user=True)
    assert 'privilege level is 1' in command(reader,'show privilege',user=True)
    command(child,'enable');assert 'privilege level is 15' in command(child,'show privilege')
    command(child,'disable',user=True)
    for line in ['conf t',f'enable secret 0 {ENABLE}','end']: command(admin,line)
    check('Removing the enable secret permits console enable but denies remote privilege elevation')
    command(admin,'configure candidate');command(admin,'hostname staged-elsewhere')
    assert 'Platform kernel:' in command(reader,'show version',user=True)
    command(admin,'abort')
    check('SSH and serial share the CLI semantics; simultaneous sessions retain independent candidates and privileges')
    for line in ['conf t', f'username operator privilege 5 secret {READER}',
                 'privilege exec level 5 configure terminal', 'privilege configure level 5 hostname',
                 'file privilege 5', 'privilege exec level 5 delete', 'privilege exec all level 5 copy',
                 'privilege exec level 5 show running-config',
                 'privilege exec level 5 request container-engine events', 'end']:
        command(admin, line)
    operator = ssh_login('operator', READER)
    for line in ['conf t', 'hostname ctr', 'hostname ctr-test', 'end']: command(operator, line)
    command(operator, 'conf t')
    assert 'Invalid input' in command(operator, f'username forbidden privilege 15 secret {READER}')
    command(operator, 'end')
    for line in ['conf t', 'no file privilege', 'end']: command(admin, line)
    denied = command(operator, 'show running-config')
    assert 'Invalid input' in denied or 'Authorization failed' in denied, denied
    for line in ['conf t', 'file privilege 5', 'end']: command(admin, line)
    visible = command(operator, 'show running-config')
    assert 'hostname ctr-test' in visible and 'username ' not in visible and 'enable secret' not in visible, visible
    assert 'Invalid input' in command(operator, 'show running-config view full')
    for line in ['conf t', 'privilege exec level 5 show running-config view full', 'end']: command(admin, line)
    assert 'username admin' in command(operator, 'show running-config view full')
    assert 'username admin' not in command(operator, 'show running-config')
    check('Running-config requires file access, omits configuration above the user level, and exposes the full configuration only with a separate view-full grant')
    assert 'Invalid input' in command(operator, 'request container-engine debug enable container rm argument forbidden')
    check('Level-five command delegation permits hostname edits and one native operation, while denying account creation and sibling operations')
    command(admin, 'show version | redirect data0:privilege-test.txt')
    operator.sendline('delete data0:privilege-test.txt'); operator.expect_exact('Delete data0:privilege-test.txt? [confirm]')
    for line in ['conf t', 'privilege exec reset delete', 'end']: command(admin, line)
    operator.sendline(''); prompt(operator)
    assert 'Authorization failed.' in operator.before
    assert 'privilege-test.txt' in command(admin, 'dir data0:')
    operator.sendline('copy running-config startup-config'); operator.expect_exact('Destination filename [startup-config]? ')
    for line in ['conf t', 'privilege exec all reset copy', 'end']: command(admin, line)
    operator.sendline(''); prompt(operator)
    assert 'Authorization failed.' in operator.before
    check('Changing command privileges revokes pending delete and configuration-copy confirmations before mutation')
    # Replay the last minute so creating the fixture immediately after the
    # command echo cannot race the native client's event subscription.
    operator.sendline('request container-engine events since 1m'); operator.expect_exact('request container-engine events since 1m')
    command(admin, 'request container-engine network create argument privilege-fixture')
    operator.expect('network create', timeout=15)
    for line in ['conf t', 'privilege exec reset request container-engine events', 'end']: command(admin, line)
    operator.expect_exact('% Command authorization revoked.', timeout=15); prompt(operator)
    check('A running native Engine event stream terminates when its command grant is revoked')
    command(admin, 'request container-engine network rm argument privilege-fixture')
    for line in ['conf t', 'privilege exec reset configure terminal', 'privilege configure reset hostname',
                 'privilege exec all reset show running-config', 'no file privilege',
                 'no username operator', 'end']: command(admin, line)
    operator.expect(pexpect.EOF, timeout=10)
    command(admin, 'delete /force data0:privilege-test.txt')
    admin_peer=ssh_login('admin',ADMIN)
    command(admin_peer,'configure candidate')
    for line in ['conf t','line console 0','exec-timeout 11 0','exit','commit','end']:command(admin,line)
    assert 'changed' in command(admin_peer,'commit').lower()
    command(admin_peer,'abort')
    check('a stale candidate cannot overwrite another session\'s committed configuration')
    command(admin,'conf t'); command(admin_peer,'conf t')
    for line in ['line console 0','exec-timeout 12 0','exit','end']: command(admin,line)
    for line in ['line vty 0 15','exec-timeout 13 0','exit','end']: command(admin_peer,line)
    running=command(admin,'show running-config json')
    assert '"console_timeout_seconds": 720' in running and '"timeout_seconds": 780' in running,running
    check('simultaneous ordinary configuration sessions preserve each other\'s immediate edits without commit')
    for key in ['logout\n','exit\n','\x04']:
        peer=ssh_login('admin',ADMIN); peer.send(key); peer.expect(pexpect.EOF,timeout=15)
    check('SSH logout, EXEC exit and empty-line Ctrl-D close the transport')
    # Different VTY policies must affect the selected sessions, not all SSH.
    # All existing clients have finite timeouts here; close them to make slots deterministic.
    for old in [admin, reader, admin_peer]: old.close(force=True)
    time.sleep(2)
    child.sendline('enable');child.expect_exact('Password: ');child.sendline(ENABLE);prompt(child)
    for line in ['conf t','line vty 0 15','transport input none','line vty 0 1','transport input ssh','exec-timeout 10','end']:
        assert '% ' not in command(child,line)
    first=ssh_login('admin',ADMIN);second=ssh_login('reader',READER)
    users=command(first,'show users')
    assert 'vty 0' in users and 'vty 1' in users,users
    for line in ['conf t','line vty 1','exec-timeout 0 3','end']: assert '% ' not in command(child,line)
    second.expect(pexpect.EOF,timeout=10)
    assert 'Platform kernel:' in command(first,'show version')
    for line in ['conf t','line vty 1','exec-timeout 10','end']: assert '% ' not in command(child,line)
    second=ssh_login('reader',READER)
    for line in ['conf t','line vty 1','transport input none','end']: assert '% ' not in command(child,line)
    second.expect(pexpect.EOF,timeout=10)
    assert 'Platform kernel:' in command(first,'show version')
    for line in ['conf t','line vty 0 15','transport input ssh','exec-timeout 10','end']: assert '% ' not in command(child,line)
    admin=first;reader=ssh_login('reader',READER)
    # Return serial to the reader identity expected by the later revocation check.
    child.sendline('logout');child.expect_exact('Username: ');login(child,'reader',READER)
    check('Single VTY timeout and transport changes close only the selected SSH line; other sessions remain usable')

    # SSH exec and subsystems are denied even for a privilege 15 account.
    denied=pexpect.spawn('ssh',['-p',str(port),'-o',f'UserKnownHostsFile={out/"known_hosts"}','-o','PubkeyAuthentication=no',f'admin@127.0.0.1','id'],encoding='utf-8',timeout=30)
    denied.expect('password:');denied.sendline(ADMIN);denied.expect(pexpect.EOF);denied.close();assert denied.exitstatus!=0
    check('SSH arbitrary exec requests cannot open a Unix shell')
    for line in ['conf t','line vty 0 15','exec-timeout 0 5','exit','commit','end']:command(admin,line)
    reader.expect(pexpect.EOF,timeout=15)
    check('idle VTY sessions expire and close their SSH transport')
    admin=ssh_login('admin',ADMIN)
    for line in ['conf t','line vty 0 15','exec-timeout 10 0','exit','commit','end']:command(admin,line)
    reader=ssh_login('reader',READER)
    for line in ['conf t',f'username reader privilege 2 secret {READER}','commit','end']:command(admin,line)
    reader.expect(pexpect.EOF,timeout=10)
    for line in ['conf t',f'username reader privilege 1 secret {READER}','commit','end']:command(admin,line)
    reader=ssh_login('reader',READER)
    check('changing an account\'s secret or assigned privilege revokes its active session')
    for line in ['conf t','no username reader','commit','end']:command(admin,line)
    reader.expect(pexpect.EOF,timeout=10)
    check('deleting a local account revokes its existing VTY session')
    # The serial reader was revoked by the earlier account change as well.
    # Consume that challenge before testing a separate management restart.
    child.expect_exact('Username: ');login(child,'admin',ADMIN)
    command(admin,'terminal width 512')
    for line in ['conf t',f'username streamer privilege 15 secret {READER}','commit','end']:command(admin,line)
    streamer=ssh_login('streamer',READER)
    streamer.sendline('request container-engine events');time.sleep(2)
    for line in ['conf t','no username streamer','commit','end']:command(admin,line)
    streamer.expect(pexpect.EOF,timeout=10)
    check('account revocation also terminates a VTY occupied by a streaming native client')
    before_lines = command(admin, 'show line')
    before_uses = {int(m[0]): int(m[1]) for m in re.findall(r'(?m)^\s*\*?\s*(\d+)\s+(?:CTY|VTY)\s+.*?\s+-\s+-\s+-\s+-\s+-\s+(\d+)\s+', before_lines)}
    assert before_uses and max(before_uses.values()) > 1, before_lines
    admin.sendline('request platform software service management restart')
    admin.expect(pexpect.EOF,timeout=30)
    if child.expect_exact(['Username: ','Press RETURN to get started!'],timeout=30)==1:
        child.sendline('');child.expect_exact('Username: ')
    login(child,'admin',ADMIN)
    admin=ssh_login('admin',ADMIN)
    check('management service restart requires console reauthentication and restores authenticated SSH')
    command(admin,'terminal length 0');command(admin,'terminal width 512')
    after_lines = command(admin, 'show line')
    after_uses = {int(m[0]): int(m[1]) for m in re.findall(r'(?m)^\s*\*?\s*(\d+)\s+(?:CTY|VTY)\s+.*?\s+-\s+-\s+-\s+-\s+-\s+(\d+)\s+', after_lines)}
    assert after_uses.keys() == before_uses.keys() and all(after_uses[n] >= count for n, count in before_uses.items()), after_lines
    check('Line usage counters survive management restart and continue counting authenticated sessions')
    accounting=command(admin,'show logging aaa')
    assert 'authentication' in accounting or '"event":"login"' in accounting
    assert '"transport":"ssh"' in accounting and '"event":"command"' in accounting
    assert all(secret not in accounting for secret in [ADMIN,READER,ENABLE])
    check('persistent AAA accounting records identities and command classes without plaintext secrets')
    command(admin,'write memory')
    reload_appliance(admin);child.expect(pexpect.EOF,timeout=180);stop_qemu(child)
    child=spawn();child.expect_exact('Username: ');login(child,'admin',ADMIN)
    check('AAA configuration and SSH host identity persist across a complete reboot')
    admin2=ssh_login('admin',ADMIN);assert 'privilege level is 15' in command(admin2,'show privilege')
    reload_appliance(admin2);child.expect(pexpect.EOF,timeout=180);stop_qemu(child)
    child=spawn(recovery=True);prompt(child)
    for line in ['terminal length 0','terminal width 512','configure candidate','do copy startup-config candidate-config',f'username admin privilege 15 secret {ADMIN}-recovered']:
        command(child,line)
    command(child,'commit');command(child,'end');child.sendline('logout');child.expect_exact('Username: ');login(child,'admin',ADMIN+'-recovered')
    command(child,'write memory');reload_appliance(child);child.expect(pexpect.EOF,timeout=180);stop_qemu(child)
    child=spawn();child.expect_exact('Username: ');login(child,'admin',ADMIN+'-recovered')
    admin3=ssh_login('admin',ADMIN+'-recovered');assert 'privilege level is 15' in command(admin3,'show privilege')
    check('physical ROMMON recovery restores a forgotten administrator secret, preserves configuration and returns to authenticated normal boot')
    result['passed']=True
except Exception as error:result['error']=str(error);raise
finally:
    for c in clients:
        try:c.close(force=True)
        finally:c.logfile_read.close()
    stop_qemu(child);log.close();(out/'result-os-aaa.json').write_text(json.dumps(result,indent=2)+'\n')
