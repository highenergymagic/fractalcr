#!/usr/bin/env python3
"""Exercise IOS running/startup and session semantics on disposable KVM/WHPX disks."""
import argparse
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
password = 'Synthetic-ios-login-592!'
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
    expect('Username: '); send('test'); time.sleep(1.2); send('admin\n')
    expect('Password: '); send(password[:8]); time.sleep(1.2); send(password[8:]+'\n')
    response = prompt()
    assert password not in response, 'login password echoed'

def finish():
    child.wait(timeout=120); reader.join(timeout=2)
    assert child.returncode == 0, 'QEMU failed to shut down cleanly'

try:
    spawn()
    expect(re.escape('Enter initial configuration dialog? [yes/no]: ')); send('no\n'); prompt()
    command('write memory')  # Seed startup so reconnect does not repeat first-use setup.
    command('terminal length 0')
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
    send('reload in 10 maintenance\n'); expect(re.escape('Proceed with reload? [confirm]')); send('\n'); prompt()
    assert 'maintenance' in command('show reload')
    assert '%Reload cancelled.' in command('reload cancel')
    assert 'No reload is scheduled.' in command('show reload')
    send('reload\n'); expect(re.escape('Proceed with reload? [confirm]')); send('\x03'); prompt()
    command('show version')
    assert 'Time source is' in command('show clock detail')
    assert 'Line' in command('show users') and 'Host(s)' in command('show users')
    section=command('show running-config | section line')
    assert 'line console 0' in section and 'exec-timeout' in section and 'hostname ctr' not in section
    check('Copy prompts, scheduled reload, show reload, cancellation and Ctrl-C work through the serial console')
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
    command('conf t', 'config'); command('line con 0', 'config-line')
    send('\x04'); prompt('config'); send('\x04'); prompt()
    check('Empty-line Ctrl-D leaves each configuration context; Ctrl-Z returns directly to EXEC')
    command('terminal length 0'); command('terminal width 512')
    version = command('sh ver')
    assert 'Source build: '+a.expected_source_build in version
    interfaces=command('show ip interface brief')
    physical=re.search(r'(?m)^([A-Za-z]+Ethernet[0-9]+|Ethernet[0-9]+)\s+',interfaces)
    assert physical, interfaces
    command('conf t','config')
    command('interface '+physical.group(1),'config-interface')
    command('role container','config-interface'); command('exit','config')
    command('interface po1','config-interface')
    command('role container','config-interface'); command('member '+physical.group(1),'config-interface')
    command('exit','config'); command('interface vl100','config-interface')
    command('role container','config-interface'); command('parent po1','config-interface')
    command('ip address 192.0.2.100 255.255.255.0','config-interface'); send('\x1a'); prompt()
    interfaces=command('show ip interface brief')
    assert 'Port-channel1' in interfaces and re.search(r'Vlan100\s+192\.0\.2\.100',interfaces),interfaces
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
