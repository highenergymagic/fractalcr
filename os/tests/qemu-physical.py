#!/usr/bin/env python3
"""Single-disk EFI boot, serial setup, DHCP, NIC roles, Docker networks, NTP.

Run with a loopback NTP fixture listening on the QEMU host UDP 123. This tests
emulated device paths; it does not claim qualification of a physical motherboard.
"""
import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import socket
import subprocess
import time
import pexpect
from ios_prompts import reload_appliance

p=argparse.ArgumentParser()
p.add_argument('--artifacts',type=Path,required=True)
p.add_argument('--transport',choices=['usb','nvme','ahci'],default='usb')
p.add_argument('--policy',choices=['required','optional'],default='optional')
a=p.parse_args()
src=Path(__file__).resolve().parents[2]
art=a.artifacts.resolve()
name='physical-'+a.transport+'-'+a.policy
image=art/(name+'.img')
variables=art/(name+'.vars.fd')
result={'case':name,'passed':False,'checks':[], 'system_sha256':hashlib.sha256((art/'system.bin').read_bytes()).hexdigest(),'rommon_sha256':hashlib.sha256((art/'rommon.efi').read_bytes()).hexdigest()}
subprocess.run(['python3',str(src/'os/tools/make-appliance.py'),'--efi',str(art/'rommon.efi'),'--system',str(art/'system.bin'),'--output',str(image),'--size-mib','4096','--firmware-policy',a.policy],check=True)
subprocess.run(['sfdisk','--verify',str(image)],check=True)
shutil.copy(art/('OVMF_VARS-trusted.fd' if a.policy=='required' else 'OVMF_VARS-unenrolled.fd'),variables)
log=(art/(name+'.log')).open('w')
child=None
password='physical-test-password'
def free_port():
    with socket.socket() as s:
        s.bind(('127.0.0.1',0))
        return s.getsockname()[1]
management_port,container_port=free_port(),free_port()
def check(s):
    result['checks'].append(s); print('PASS '+s,flush=True)
def boot():
    args=['-machine','q35,smm=on','-accel','kvm','-cpu','host','-m','4096','-smp','2',
          '-global','driver=cfi.pflash01,property=secure,value=on',
          '-drive',f'if=pflash,format=raw,unit=0,readonly=on,file={art / "OVMF_CODE_4M.secboot.fd"}',
          '-drive',f'if=pflash,format=raw,unit=1,file={variables}',
          '-drive',f'if=none,id=system,format=raw,file={image}',
          '-netdev',f'user,id=management,hostfwd=tcp:127.0.0.1:{management_port}-:22','-device','virtio-net-pci,netdev=management',
          '-netdev',f'user,id=container,net=192.0.2.0/24,hostfwd=tcp:127.0.0.1:{container_port}-:22','-device','e1000e,netdev=container',
          '-display','none','-serial','stdio','-monitor','none','-no-reboot']
    if a.transport=='usb': args+=['-device','qemu-xhci','-device','usb-storage,drive=system,bootindex=0']
    elif a.transport=='nvme': args+=['-device','nvme,drive=system,serial=FCRQUAL,bootindex=0']
    else: args+=['-device','ide-hd,drive=system,bus=ide.0,bootindex=0']
    c=pexpect.spawn('qemu-system-x86_64',args,encoding='utf-8',timeout=240)
    c.logfile_read=log
    c.expect('Image signature verified.'); c.expect('Press RETURN to get started!'); c.sendline('')
    return c
def answer(prompt,value):
    child.expect_exact(prompt); child.sendline(value)
def command(s,prompt='steel# ',timeout=120,allow_error=False):
    child.sendline(s); child.expect(re.escape(s[-24:])+r'\r+\n',timeout=timeout)
    child.expect_exact(prompt,timeout=timeout)
    output=child.before
    if not allow_error:
        assert not re.search(r'^% ',output,re.M),output
    return output
def login():
    answer('Username: ','admin'); answer('Password: ',password); child.expect_exact('steel# ')
    command('terminal length 0')
try:
    child=boot()
    answer('Enter initial configuration dialog? [yes/no]: ','yes')
    answer('Host name [ctr]: ','steel')
    answer('Management interface: ','eth0')
    answer('IP address/prefix or dhcp [dhcp]: ','dhcp')
    answer('NTP server (address or hostname; blank disables): ','10.0.2.2')
    answer('Administrator username [admin]: ','admin')
    answer('Administrator password: ',password)
    answer('Apply and save this configuration? [yes/no]: ','yes')
    child.expect_exact('Configuration applied and saved. Authenticate to continue.')
    login()
    check('single '+a.transport+' disk booted through EFI/ROMMON and saved serial setup with local AAA')
    for _ in range(30):
        output=command('show ip interface brief')
        if re.search(r'\s10\.0\.2\.15\s',output): break
        time.sleep(1)
    else: raise AssertionError('DHCP did not acquire a lease')
    check('DHCP acquired management address without a preseeded configuration')
    command('conf t', 'steel(config)# ')
    command('interface eth1','steel(config-if)# ')
    for s in ['role container','ip address dhcp']: command(s,'steel(config-if)# ')
    command('exit','steel(config)# ')
    command('network tenant','steel(config-network)# ')
    for s in ['driver bridge','subnet 198.51.100.0/24']: command(s,'steel(config-network)# ')
    command('exit','steel(config)# ')
    command('network external','steel(config-network)# ')
    for s in ['driver macvlan','parent eth1','subnet 192.0.2.0/24','gateway 192.0.2.2']: command(s,'steel(config-network)# ')
    command('commit','steel(config-network)# '); command('end')
    command('write memory')
    for _ in range(30):
        output=command('show ip interface brief')
        if re.search(r'\s192\.0\.2\.15\s',output): break
        time.sleep(1)
    else: raise AssertionError('second NIC did not acquire its independent lease')
    routes=command('show ip route')
    assert re.search(r'0\.0\.0\.0/0 .*via 10\.0\.2\.2, \S+Ethernet\d+|0\.0\.0\.0/0 .*via 10\.0\.2\.2, Ethernet\d+',routes),routes
    assert not re.search(r'0\.0\.0\.0/0 .*via 192\.0\.2\.2',routes),routes
    check('container-role DHCP NIC has its own address without replacing management routing')
    with socket.create_connection(('127.0.0.1',management_port),timeout=5) as s:
        assert s.recv(256).startswith(b'SSH-2.0-'), 'SSH missing on management NIC'
    try:
        with socket.create_connection(('127.0.0.1',container_port),timeout=3) as s:
            assert not s.recv(256).startswith(b'SSH-2.0-'), 'SSH exposed on container NIC'
    except (ConnectionError,socket.timeout):
        pass
    check('SSH is reachable through management and unavailable through the container NIC')
    command('request network tenant create'); command('request network external create')
    assert 'macvlan' in command('show network external detail')
    command('request container-engine import /usr/share/fractal/probe.tar fcr/probe:latest')
    assert 'CONTAINER_BRIDGE_OK' in command('request container-engine run rm enable network tenant arguments fcr/probe:latest /probe network')
    command('request network external remove')
    check('appliance CLI creates bridge and physical-parent macvlan Docker networks')
    for _ in range(45):
        output=command('show ntp status')
        if 'Clock is synchronized' in output: break
        time.sleep(2)
    else: raise AssertionError('chrony did not synchronize: '+output)
    check('supervised chrony synchronized to an actual NTP response source')
    assert 'Time source is NTP' in command('show clock detail')
    before_time = command('show platform software service')
    engine_identity = re.search(r'container-engine\s+running\s+(\d+)\s+(\d+)', before_time)
    assert engine_identity, before_time
    for policy in ['ntp update-calendar', 'no ntp update-calendar']:
        command('conf t', 'steel(config)# '); command(policy, 'steel(config)# '); command('end')
        for _ in range(45):
            output = command('show ntp status')
            if 'Clock is synchronized' in output: break
            time.sleep(2)
        else: raise AssertionError('NTP did not recover after calendar policy change: '+output)
        assert 'Time source is NTP' in command('show clock detail')
        after_time = command('show platform software service')
        assert re.search(r'container-engine\s+running\s+(\d+)\s+(\d+)', after_time).groups() == engine_identity.groups(), after_time
    check('NTP synchronization remains visible with RTC updates enabled or disabled and time policy changes preserve Docker')
    for cycle in range(5):
        command('request platform software service networking restart')
        deadline = time.monotonic() + 30
        while True:
            interfaces = command('show ip interface brief')
            if re.search(r'\s10\.0\.2\.15\s', interfaces): break
            assert time.monotonic() < deadline, 'DHCP did not recover: ' + interfaces
            time.sleep(1)
        # Keep the short restart-to-reload interval: waiting for Docker first
        # would conceal the interrupted startup failure this exercises.
        reload_appliance(child); child.expect(pexpect.EOF,timeout=120); child.close()
        child=boot(); login()
        assert 'tenant' in command('show networks')
        assert command('show running-config').count('ip address dhcp') >= 2
        assert 'CONTAINER_BRIDGE_OK' in command('request container-engine run rm enable network tenant arguments fcr/probe:latest /probe network')
        check(f'network service restart and cold boot cycle {cycle+1} preserved setup, credentials, DHCP and usable Docker network')
    result['passed']=True
except Exception as error:
    result['error']=str(error)
    if child is not None and child.isalive():
        try:
            for diagnostic in ['show platform software service','show logging container-engine','show logging datastore','show ip interface brief']:
                command(diagnostic,timeout=20,allow_error=True)
        except Exception:
            pass
    raise
finally:
    if child is not None and child.isalive(): child.terminate(force=True)
    log.close()
    (art/('result-'+name+'.json')).write_text(json.dumps(result,indent=2)+'\n')
