#!/usr/bin/env python3
"""Exercise IOS running/startup and session semantics on disposable KVM/WHPX disks."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import queue
import re
import shutil
import subprocess
import threading
import time

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--artifacts', type=Path, required=True, help='Prepared launcher templates')
p.add_argument('--output', type=Path, required=True, help='New directory for disposable VM state')
p.add_argument('--accelerator', choices=['kvm', 'whpx'], required=True)
p.add_argument('--expected-source-build', required=True)
a = p.parse_args()
art, out = a.artifacts.resolve(), a.output.resolve()
out.mkdir(parents=True, exist_ok=False)
qemu = shutil.which('qemu-system-x86_64')
if not qemu:
    raise SystemExit('qemu-system-x86_64 is not on PATH')
whpx = a.accelerator == 'whpx'
firmware = Path(qemu).parent/'share/edk2-x86_64-code.fd' if whpx else art/'OVMF_CODE_4M.secboot.fd'
vars_template = Path(qemu).parent/'share/edk2-i386-vars.fd' if whpx else art/'OVMF_VARS-trusted.fd'
shutil.copyfile(vars_template, out/'vars.fd')
if os.name == 'nt':
    shutil.copyfile(art/'data-os-containers.img', out/'data.img')
else:
    subprocess.run(['cp', '--sparse=always', str(art/'data-os-containers.img'), str(out/'data.img')], check=True)
flash = art/('flash-os-optional-firmware.img' if whpx else 'flash-os-containers.img')
password = '__connect'
result = dict(case='ios-workflow-'+a.accelerator, passed=False, checks=[],
              system_sha256=hashlib.file_digest((art/'system.bin').open('rb'), 'sha256').hexdigest(),
              rommon_sha256=hashlib.file_digest((art/'rommon.efi').open('rb'), 'sha256').hexdigest(),
              source_build=a.expected_source_build)
log = (out/'serial.log').open('w', encoding='utf-8')
child = None
reader = None
messages = None
buffer = ''
hostname = 'ctr'

def check(message):
    result['checks'].append(message)
    print('PASS '+message, flush=True)

def spawn():
    global child, reader, messages, buffer
    args = [qemu, '-machine', 'q35,smm=off' if whpx else 'q35,smm=on',
            '-accel', a.accelerator, '-cpu', 'qemu64' if whpx else 'host,vmx=off,svm=off',
            '-m', '4096', '-smp', '2',
            *([] if whpx else ['-global', 'driver=cfi.pflash01,property=secure,value=on']),
            '-drive', f'if=pflash,format=raw,unit=0,readonly=on,file={firmware}',
            '-drive', f'if=pflash,format=raw,unit=1,file={out/"vars.fd"}',
            '-drive', f'if=none,id=flash,format=raw,snapshot=on,file={flash}',
            '-device', 'virtio-blk-pci,drive=flash,bootindex=0',
            '-drive', f'if=virtio,format=raw,file={out/"data.img"}',
            '-netdev', 'user,id=net0', '-device', 'virtio-net-pci,netdev=net0',
            '-display', 'none', '-serial', 'stdio', '-monitor', 'none', '-no-reboot']
    child = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                             creationflags=subprocess.CREATE_NO_WINDOW if os.name == 'nt' else 0)
    messages, buffer = queue.Queue(), ''
    process, stream = child, messages
    def read():
        while data := process.stdout.read1(4096): stream.put(data)
        stream.put(None)
    reader = threading.Thread(target=read, daemon=True)
    reader.start()
    boot = expect('Press RETURN to get started!', 300)
    assert 'Image signature verified.' in boot and '%FCR-5-SYSTEM_READY' in boot, boot
    send('\n')

def send(text, delay=None):
    for byte in text.encode():
        child.stdin.write(bytes([byte])); child.stdin.flush()
        time.sleep(delay if delay is not None else (.02 if whpx else .005))

def expect(pattern, timeout=90):
    global buffer
    end = time.monotonic()+timeout
    while True:
        match = re.search(pattern, buffer)
        if match:
            response, buffer = buffer[:match.end()], buffer[match.end():]
            return response
        if time.monotonic() >= end:
            raise AssertionError('Timeout expecting '+pattern+'; tail: '+buffer[-1800:])
        try: data = messages.get(timeout=min(1, max(.01, end-time.monotonic())))
        except queue.Empty: continue
        if data is None: raise AssertionError('Unexpected QEMU exit: '+buffer[-1800:])
        text = data.decode('utf-8', errors='replace')
        log.write(text); log.flush(); buffer += text

def prompt(mode=''):
    return expect(re.escape(hostname+('('+mode+')' if mode else '')+'# '))

def command(text, mode='', error=False):
    send(text+'\n'); response = prompt(mode)
    failures = re.findall(r'(?:^|[\r\n])% (.+)', response)
    if error: assert failures, 'expected command rejection: '+response
    else: assert not failures, 'command rejected: '+response
    return response

def login():
    expect('Username: '); send('test'); time.sleep(1.2); send('admin\r\n')
    expect('Password: '); send(password[:8]); time.sleep(1.2); send(password[8:]+'\r\n')
    response = prompt()
    assert password not in response, 'login password echoed'

def finish():
    child.wait(timeout=120); reader.join(timeout=2)
    assert child.returncode == 0, 'QEMU failed to shut down cleanly'

try:
    spawn()
    expect(re.escape('Enter initial configuration dialog? [yes/no]: ')); send('no\n'); prompt()
    assert 'startup-config is not present' in command('show startup-config')
    command('write memory')  # Seed startup so reconnect does not repeat first-use setup.
    command('terminal length 0')
    startup_usage = command('show startup-config | include ^Using')
    assert re.search(r'Using [1-9][0-9]* out of 262144 bytes', startup_usage), startup_usage
    check('Startup configuration reports missing state and actual bounded storage usage')
    command('__connect', error=True)
    command('__help show ', error=True)
    send('disable\n'); expect(re.escape('ctr> ')); send('enable\n'); prompt()
    check('Console disable/enable works without an enable secret')
    send('dir ?'); help=prompt()
    assert all(word in help for word in ['<filesystem:>', 'data0:', 'flash:', 'nvram:', '/recursive']), help
    expect('dir '); send('\x15'); prompt()
    send('dir fl?'); help=prompt(); assert 'flash:' in help
    expect('dir fl'); send('\x15'); prompt()
    assert 'Directory of' in command('dir /recursive /all data0:')
    # Session filesystem state must affect native operations, not host cwd.
    command('mkdir data0:session-test'); command('cd data0:session-test')
    assert 'data0:/session-test' in command('pwd')
    assert 'Directory of data0:/session-test' in command('dir')
    command('cd ..'); command('rmdir session-test'); command('cd bootflash:')
    check('Directory help enumerates mounted filesystem prefixes and preserves typed input')
    send('copy running-config startup-config\n'); expect(re.escape('Destination filename [startup-config]? ')); send('\n'); prompt()
    send('reload in 10 maintenance\n'); expect(re.escape('Proceed with reload? [confirm]')); send('\n')
    assert 'SHUTDOWN in 0:' in prompt()
    assert 'maintenance' in command('show reload')
    cancelled=command('reload cancel')
    assert '%Reload cancelled.' in cancelled and 'SHUTDOWN ABORTED' in cancelled
    assert 'No reload is scheduled.' in command('show reload')
    send('reload\r\n'); expect(re.escape('Proceed with reload? [confirm]'))
    time.sleep(1.2); send('\x03'); prompt()  # CRLF must not confirm the new dialog.
    command('show version')
    assert 'Time source is' in command('show clock detail')
    assert 'Number of lines which match regexp = 1' in command('show running-config | count hostname')
    numbered = command('show running-config linenum | include hostname')
    assert re.search(r'\n 2 : hostname '+re.escape(hostname)+r'\r?\n', numbered), numbered
    check('Configuration line numbers retain their values through output filtering')
    interfaces = command('show ip interface brief')
    port_match = re.search(r'\b((?:[A-Za-z]+Ethernet|Ethernet)[0-9/]+)\s', interfaces)
    assert port_match, interfaces
    port = port_match.group(1)
    command('conf t', 'config'); command('interface '+port, 'config-if')
    command('description conformance-port', 'config-if'); send('\x1a'); prompt()
    selected = command('show running-config interface '+port+' linenum')
    assert ' 2 : interface '+port in selected and 'description conformance-port' in selected
    assert 'hostname '+hostname not in selected and ' exit' not in selected
    command('conf t', 'config'); command('interface '+port, 'config-if')
    command('no description', 'config-if'); send('\x1a'); prompt()
    check('Selected interface configuration displays only that port with stable line numbering')
    assert 'Number of lines which match regexp = 1' in command('show version | count Source build:')
    assert 'Line' in command('show users') and 'Host(s)' in command('show users')
    assert '16 vty 15' in command('show users all wide')
    terminal=command('show terminal')
    assert all(s in terminal for s in ['Line 0,', 'Length: 0 lines', 'Timeouts:', 'Editing is enabled.', 'History is enabled, history size is 10.']), terminal
    assert 'Line 0,' in command('show line console 0')
    assert 'Line 16,' in command('show line vty 15')
    command('terminal history size 27'); command('terminal no history')
    assert 'History is disabled, history size is 27.' in command('show terminal')
    command('terminal history'); command('terminal no editing')
    send('show cl\x01ock\n'); prompt()  # Ctrl-A is disabled; text stays at the end.
    assert 'Editing is disabled.' in command('show terminal')
    command('terminal editing'); command('terminal no history size')
    command('show version'); command('terminal no editing')
    command('disable 7')
    assert 'show version' not in command('show history')
    assert 'Editing is disabled.' in command('show terminal')
    command('enable'); command('terminal editing')
    command('conf t', 'config'); command('file prompt noisy', 'config'); send('\x1a'); prompt()
    assert 'file prompt noisy' not in command('show history')
    command('conf t', 'config'); send('\x10'); prompt('config'); expect('file prompt noisy')
    send('\x15'); prompt('config'); command('no file prompt', 'config'); send('\x1a'); prompt()
    check('Terminal display uses actual settings; editing toggles work and EXEC/configuration histories stay separate')
    command('show version | redirect data0:version.txt')
    assert 'FractalCR Software' in command('more data0:version.txt')
    assert 'FractalCR Software' in command('more /ascii data0:version.txt | include FractalCR')
    assert '00000000:' in command('more /binary data0:version.txt')
    assert 'hostname '+hostname in command('more system:running-config | include hostname')
    assert 'Using ' in command('more nvram:startup-config')
    command('more data0:__missing_file__', error=True)
    check('Native more displays files, configuration aliases, ASCII/hex formats and filtered output')
    command('terminal length 4')
    send('more system:running-config\n'); expect(re.escape(' --More-- '))
    send('/^line console\r\n'); searched=expect(re.escape(' --More-- '))
    assert 'filtering...' in searched and 'line console 0' in searched and 'hostname '+hostname not in searched, searched
    send('q'); prompt()
    send('more system:running-config\n'); expect(re.escape(' --More-- '))
    send('+^line \n'); filtered=prompt()
    assert 'line console 0' in filtered and 'line vty' in filtered and ' exec-timeout' not in filtered, filtered
    for interrupt in ['\x1a', '\x1e']:
        send('more system:running-config\n'); expect(re.escape(' --More-- ')); send(interrupt); prompt()
    send('show history\n'); expect(re.escape(' --More-- '))
    send('/show history$\n'); history=prompt()
    assert 'filtering...' in history and 'show history' in history, history
    command('terminal length 0')
    assert 'FractalCR Software' in command('show version')
    check('Pager search and include filters work on native and API output; CRLF, quit, Ctrl-Z and Ctrl-^ return to a usable prompt')
    assert 'version.txt' in command('dir data0:version.txt')
    command('show clock | append data0:version.txt')
    assert 'Time source is' in command('show clock detail | tee data0:clock.txt')
    assert 'Time source is' in command('show clock detail | tee /append data0:clock.txt')
    command('delete /force data0:version.txt'); command('delete /force data0:clock.txt')
    assert 'No reload is scheduled.' in command('show reload | tee data0:reload.txt')
    assert 'reload.txt' in command('dir data0:reload.txt')
    command('delete /force data0:reload.txt')
    check('Native-console and management output redirect, append and tee through rooted file APIs')
    command('conf t', 'config'); command('clock timezone NST -3 30', 'config')
    send('clock ?'); clock_help = prompt('config')
    assert not re.search(r'\n\s+save\s', clock_help), clock_help
    expect('clock '); send('\x15'); prompt('config')
    command('clock save interval 24', 'config', error=True)
    check('Hardware-calendar platform omits the NVRAM-only clock save command')
    command('enable secret level 6 5 $1$abcd$2hDGYdE0UrwK2pqoiF6BB1', 'config')
    command('enable secret level 7 0 level-seven-test', 'config'); send('\x1a'); prompt()
    assert ' NST ' in command('show clock')
    assert ' NST ' in command('show calendar')
    command('clock update-calendar')
    command('clock read-calendar')
    command('conf t', 'config'); command('clock summer-time NDT recurring', 'config'); send('\x1a'); prompt()
    assert 'Summer time ends' in command('show clock detail')
    command('conf t', 'config'); command('no clock summer-time', 'config'); send('\x1a'); prompt()
    send('disable\n'); expect(re.escape(hostname+'> ')); send('enable 7\n'); expect('Password: '); send('level-seven-test\n'); prompt()
    assert 'Current privilege level is 7' in command('show privilege')
    command('enable'); send('disable\n'); expect(re.escape(hostname+'> '))
    send('enable 6\n'); expect('Password: '); send('cisco\n'); prompt()
    assert 'Current privilege level is 6' in command('show privilege')
    check('An imported type 5 enable secret authenticates at its configured privilege level')
    command('enable'); command('conf t','config'); command('no enable secret level 6','config'); command('no enable secret level 7','config'); command('no clock timezone','config'); send('\x1a'); prompt()
    command('clock set 18:00:00 1 January 2003')
    assert '2003' in command('show clock')
    command('clock update-calendar')
    assert '2003' in command('show calendar')
    command('clock read-calendar')
    send('reload at 19:00\n')
    response = expect(r'Save\? \[yes/no\]: |Proceed with reload\? \[confirm\]')
    assert 'The clock has not been set' not in response, response
    send('\x03'); prompt()
    command('conf t', 'config'); command('clock timezone PST -8', 'config')
    command('clock summer-time PDT recurring', 'config'); send('\x1a'); prompt()
    command('clock set 00:00:00 8 March 2026')
    send('reload in 180\n')
    response = expect(r'Save\? \[yes/no\]: |Proceed with reload\? \[confirm\]')
    assert 'Reload scheduled for 04:00:' in response and 'PDT Sun Mar 8 2026' in response, response
    send('\x03'); prompt()
    command('conf t', 'config'); command('no clock summer-time', 'config'); command('no clock timezone', 'config'); send('\x1a'); prompt()
    now = datetime.now(timezone.utc)
    command('clock set '+now.strftime('%H:%M:%S %d %B %Y'))
    command('clock update-calendar')
    check('Native calendar read/write works and manually set clocks can schedule reloads independently of their year')
    check('Timezone display and level-specific enable authentication work through the serial console')
    before_time_policy = command('show platform software service')
    time_engine = re.search(r'container-engine\s+running\s+(\d+)\s+(\d+)', before_time_policy)
    assert time_engine, before_time_policy
    command('conf t', 'config'); command('ntp update-calendar', 'config'); send('\x1a'); prompt()
    assert 'ntp update-calendar' in command('show running-config')
    command('conf t', 'config'); command('no ntp update-calendar', 'config'); send('\x1a'); prompt()
    assert 'ntp update-calendar' not in command('show running-config')
    command('conf t', 'config'); command('ntp update-calendar', 'config'); send('\x1a'); prompt()
    after_time_policy = command('show platform software service')
    assert re.search(r'container-engine\s+running\s+(\d+)\s+(\d+)', after_time_policy).groups() == time_engine.groups(), after_time_policy
    check('Calendar synchronization is opt-in and policy changes restart only the time service without restarting Docker')
    section=command('show running-config | section line')
    assert 'line console 0' in section and 'exec-timeout' in section and 'hostname ctr' not in section
    check('Copy prompts, scheduled reload, show reload, cancellation and Ctrl-C work through the serial console')
    policy_before=command('show platform software service')
    policy_engine=re.search(r'container-engine\s+running\s+(\d+)\s+(\d+)', policy_before)
    assert policy_engine, policy_before
    command('conf t', 'config'); command('file prompt quiet', 'config'); send('\x1a'); prompt()
    assert 'file prompt quiet' in command('show running-config')
    command('copy running-config data0:__cancel')
    command('copy running-config data0:__cancel')  # Quiet permits overwrite.
    send('copy data0: data0:__tick\n'); expect(re.escape('Source filename []? ')); send('__cancel\n'); prompt()
    assert '__tick' in command('dir data0:')
    command('conf t', 'config'); command('file prompt noisy', 'config'); send('\x1a'); prompt()
    send('copy data0:__tick data0:__end\n')
    expect(re.escape('Source filename [data0:/__tick]? ')); send('\n')
    expect(re.escape('Destination filename [__end]? ')); send('\n'); prompt()
    send('delete data0:__end\n'); expect(re.escape('Filename [data0:/__end]? ')); send('\n')
    expect(re.escape('Delete data0:/__end? [confirm]')); send('\x03'); prompt()
    assert '__end' in command('dir data0:__end')
    command('delete /force data0:__end')
    command('conf t', 'config'); command('no file prompt', 'config'); send('\x1a'); prompt()
    assert 'file prompt ' not in command('show running-config')
    command('delete /force data0:__tick'); command('delete /force data0:__cancel')
    policy_after=command('show platform software service')
    assert re.search(r'container-engine\s+running\s+(\d+)\s+(\d+)', policy_after).groups() == policy_engine.groups(), policy_after
    check('File prompt policies honor required information, overwrite, noisy confirmations and Ctrl-C; reserved-looking filenames remain literal')
    check('File prompt changes apply live without restarting Docker or platform configuration')
    for key in ['logout\n', 'exit\n', '\x04']:
        send(key); expect('Press RETURN to get started!')
        send('show version\n'); response = prompt()  # Text before RETURN must be ignored.
        assert 'FractalCR Software' not in response
    check('Bootstrap logout, exit and empty-line Ctrl-D end EXEC and wait for RETURN')
    command('conf t', 'config')
    hostname='h'*63
    send('hostn '+hostname+'\n'); prompt('config')
    check('A valid 63-character hostname is shown by the real console without truncation or fallback')
    send('hostn ctr-test\n'); hostname='ctr-test'; prompt('config')
    send('\x1a'); prompt()
    command('terminal length 0'); command('terminal width 512')
    before = command('show platform software service')
    identity = re.search(r'container-engine\s+running\s+(\d+)\s+(\d+)', before)
    assert identity, before
    command('conf t', 'config')
    # Reproduce the reported parser/help failure through the actual C console.
    command('user scratch priv 15 passw 0 secret', 'config')
    for bad in ['conf t', 'linve vty 0', 'line vty', 'line vty 16', 'line vty 0 16']:
        response=command(bad, 'config', error=True)
        if bad=='line vty': assert 'Incomplete command.' in response
    send('line vty ?'); help=prompt('config')
    assert '<0-15>' in help and 'virtual terminal line number' in help and 'cleartext' not in help
    expect('line vty '); send('0 ?'); help=prompt('config')
    assert '<cr>' in help and '<0-15>' in help and 'cleartext' not in help
    expect('line vty 0 '); send('4\n'); prompt('config-line')
    command('exec-timeout 2 30', 'config-line')
    command('line vty 5 15', 'config-line')
    command('exec-timeout 3', 'config-line')
    command('line vty 3', 'config-line')
    command('exec-timeout 4', 'config-line')
    command('line vty 8 4', 'config-line', error=True)
    command('exit', 'config')
    command('security passwords min-length 12', 'config')
    command('user scratch password 0 short', 'config', error=True)
    command('no security passwords min-length', 'config')
    command('no user scratch', 'config')
    command('container completion-test', 'config-container')
    send('no security p\t'); prompt('config-container'); expect('no security passwords ')
    send('\x15'); prompt('config-container')
    command('no container completion-test', 'config')
    check('Tab completion follows a shared parent prefix without changing mode; executing the parent command then leaves the submode')
    check('Reported VTY help/partial-input transcript, single/range selection, incomplete errors and explicit password policy work at the real console')
    command('user testadmin priv 15 pass 0 '+password, 'config')
    command('line con 0', 'config-line')
    command('login local', 'config-line')
    send('\x1a'); prompt()
    assert '[OK]' in command('wr me')
    command('conf t', 'config')
    send('hostn thething\n'); hostname='thething'; prompt('config')
    command('hostn bad_name', 'config', error=True)
    command('no user testadmin', 'config', error=True)
    send('\x1a'); prompt()
    assert '[OK]' in command('wr me')
    after = command('show platform software service')
    assert re.search(r'container-engine\s+running\s+(\d+)\s+(\d+)', after).groups() == identity.groups()
    running = command('sh run')
    assert 'thething' in running and 'testadmin' in running and password not in running
    check('Exact no-commit account/login/hostname/Ctrl-Z/write-memory sequence applies immediately without restarting Docker')
    check('Invalid hostname and last-administrator removal are rejected without altering running configuration')
    # Exercise the linked catalog through the real line editor, not only Go
    # parser tests. The help token must preserve the entire pending command.
    send('request compose up web det?'); help=prompt()
    assert 'detach' in help and '--detach' not in help, help
    expect('request compose up web det'); send('\x15'); prompt()
    send('request container-engine service create ?'); help=prompt()
    assert 'replicas' in help and '--replicas' not in help, help
    expect('request container-engine service create '); send('\x15'); prompt()
    check('Linked Compose and Swarm command help works through the serial editor, including interspersed options')
    send('request container-engine run volume ?'); help=prompt()
    assert 'volume' in help.lower() or 'bind mount' in help.lower(), help
    expect('request container-engine run volume '); send('\x15'); prompt()
    for incomplete in ['request container-engine run', 'request container-engine exec app', 'request container-engine cp app:/file']:
        assert 'Incomplete command.' in command(incomplete, error=True)
    command('request container-engine run -z image', error=True)
    command('request container-engine run --rm alpine', error=True)
    native_help=command('request container-engine run help')
    assert 'memory' in native_help and '--memory' not in native_help
    check('Required operands and native keyword help agree with execution; Unix options are rejected')
    # Horizontal scrolling redraws the prompt while the line is being typed.
    # Wait for the command diagnostic before accepting the final prompt.
    # Keep redraw traffic below the 115200-baud UART bandwidth on both hosts.
    command('terminal width 80')
    send('\\'*2046+'x\n', delay=.02)
    long_error=expect(r'% Invalid input[^\r\n]*')+prompt()
    assert 'Invalid input' in long_error and 'unavailable' not in long_error.lower(), long_error
    assert 'Platform kernel:' in command('show version')
    command('terminal width 512')
    check('A maximum-length escaped editor line reaches management and leaves the session usable')
    engine=command('request container-engine info format "{{.ServerVersion}}"')
    assert re.search(r'\d+\.\d+\.\d+',engine),engine
    assert 'v5.6.1' in command('request compose version')
    assert 'v0.38.1' in command('request buildx version')
    check('The packaged workload service reaches the Engine API and runs the linked Compose and Buildx clients')
    for key in ['logout\n', 'exit\n', '\x04']:
        send(key); login()
    check('Logout, exit and Ctrl-D require local authentication after login local')
    check('Authentication retains partial usernames and passwords across heartbeat intervals')
    check('CRLF login submissions remain single events across username/password/EXEC transitions')
    check('A configured password beginning with the former control prefix authenticates literally')
    command('conf t', 'config'); command('enable secret 0 __tick', 'config'); send('\x1a'); prompt()
    send('disable\n'); expect(re.escape(hostname+'> ')); send('enable\n'); expect('Password: ')
    send('__tick\n'); prompt()
    command('conf t', 'config'); command('no enable secret', 'config'); send('\x1a'); prompt()
    check('Literal former connect/tick events work as login and enable passwords without resetting the session')
    command('conf t', 'config'); command('line con 0', 'config-line')
    send('\x04'); prompt('config'); send('\x04'); prompt()
    check('Empty-line Ctrl-D leaves each configuration context; Ctrl-Z returns directly to EXEC')
    command('terminal length 0'); command('terminal width 512')
    version = command('sh ver')
    assert 'Source build: '+a.expected_source_build in version
    interfaces=command('show ip interface brief')
    physical=re.search(r'(?m)^([A-Za-z]+Ethernet[0-9]+|Ethernet[0-9]+)\s+',interfaces)
    assert physical, interfaces
    detail = command('show interfaces '+physical.group(1))
    assert physical.group(1)+' is ' in detail, detail
    assert 'line protocol is ' in detail and 'packets input' in detail, detail
    assert physical.group(1) in command('show ip interface '+physical.group(1)+' brief')
    assert 'Protocol Description' in command('show interfaces '+physical.group(1)+' description')
    check('Interface selectors and descriptions reach native status APIs through the console')
    command('conf t','config')
    command('interface '+physical.group(1),'config-if')
    command('role container','config-if'); command('exit','config')
    command('interface po1','config-if')
    command('role container','config-if'); command('member '+physical.group(1),'config-if')
    command('exit','config'); command('interface vl100','config-if')
    command('role container','config-if'); command('parent po1','config-if')
    command('ip address 192.0.2.100 255.255.255.0','config-if'); send('\x1a'); prompt()
    interfaces=command('show ip interface brief')
    assert 'Port-channel1' in interfaces and re.search(r'Vlan100\s+192\.0\.2\.100',interfaces),interfaces
    command('conf t', 'config'); command('file prompt quiet', 'config'); send('\x1a'); prompt()
    command('write memory')
    check('Packaged kernel creates a native port-channel and VLAN and applies its address from configuration mode')
    command('conf t', 'config')
    send('hostn unsaved\n'); hostname='unsaved'; prompt('config')
    send('\x1a'); prompt(); send('reload\n')
    expect(re.escape('System configuration has been modified. Save? [yes/no]: ')); send('no\n')
    expect(re.escape('Proceed with reload? [confirm]')); send('\n'); finish()
    hostname='thething'; spawn(); login()
    command('terminal length 0'); command('terminal width 512')
    startup = command('show startup-config')
    assert 'thething' in startup and 'testadmin' in startup and password not in startup and 'unsaved' not in startup
    assert 'ip address 192.0.2.100 255.255.255.0' in startup
    assert 'file prompt quiet' in startup
    assert 'ntp update-calendar' in startup
    command('copy running-config data0:prompt-persistence')
    command('delete data0:prompt-persistence')
    check('Saved file prompt policy survives a full reboot and controls native copy/delete without confirmations')
    interfaces=command('show ip interface brief')
    assert 'Port-channel1' in interfaces and re.search(r'Vlan100\s+192\.0\.2\.100',interfaces),interfaces
    check('Full reboot loads saved hostname and hashed local account, requires login, and discards unsaved running edits')
    send('request platform diagnostics containers\n')
    expect('%FCR-5-DIAGNOSTIC_PASS', 180); prompt()
    check('Container diagnostics still pass after configuration, session changes and reboot')
    result['passed']=True
except Exception as error:
    result['error']=str(error)
    raise
finally:
    if child and child.poll() is None:
        child.terminate()
        try: child.wait(timeout=15)
        except subprocess.TimeoutExpired: child.kill(); child.wait()
    if reader: reader.join(timeout=2)
    log.close()
    (out/'result.json').write_text(json.dumps(result, indent=2)+'\n')
