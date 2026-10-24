#!/usr/bin/env python3
"""Exercise NVRAM clock restoration without an RTC on disposable KVM disks.

This is a direct-kernel component test, not a firmware authentication test.
The signed kernel's embedded userland is unchanged; a test-only command line
disables rtc-cmos initialization. Production ROMMON does not expose this option.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
from pathlib import Path
import re
import socket
import struct
import subprocess
import sys
import threading
import time
import pexpect

root = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(root / 'boot/tests'))
from qemu_process import finish_qemu, stop_qemu

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--kernel', type=Path, required=True)
p.add_argument('--output', type=Path, required=True)
p.add_argument('--expected-source-build', required=True)
a = p.parse_args()
kernel, out = a.kernel.resolve(), a.output.resolve()
out.mkdir(parents=True, exist_ok=False)
result = dict(case='clock-save-no-rtc', passed=False, accel='kvm',
              boot='direct kernel; not firmware authentication qualification',
              kernel_sha256=hashlib.file_digest(kernel.open('rb'), 'sha256').hexdigest(),
              source_build=a.expected_source_build, checks=[])
child = None
log = (out / 'serial.log').open('w')
ntp = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
stop = threading.Event()
worker = None

def stamp(value):
    value += 2208988800
    return struct.pack('!II', int(value) & 0xffffffff, int(value % 1 * 2**32))

def serve_time():
    while not stop.is_set():
        try:
            data, peer = ntp.recvfrom(512)
        except socket.timeout:
            continue
        if len(data) >= 48 and data[0] & 7 == 3:
            now = time.time() + 3600  # ahead of the next cold boot's KVM clock
            reply = struct.pack('!BBBbII4s', 0x24, 2, 6, -20, 0, 655, b'TEST')
            ntp.sendto(reply + stamp(now-1) + data[40:48] + stamp(now) + stamp(now), peer)

def check(message):
    result['checks'].append(message)
    print('PASS ' + message, flush=True)

def boot():
    args = ['-machine', 'q35', '-accel', 'kvm', '-cpu', 'host', '-m', '4096', '-smp', '2',
            '-kernel', str(kernel), '-append',
            'console=ttyS0,115200 rdinit=/init quiet loglevel=0 panic=-1 initcall_blacklist=cmos_init',
            '-drive', f'if=virtio,format=raw,file={out / "data.img"}',
            '-netdev', 'user,id=management', '-device', 'virtio-net-pci,netdev=management',
            '-display', 'none', '-serial', 'stdio', '-monitor', 'none', '-no-reboot']
    c = pexpect.spawn('qemu-system-x86_64', args, encoding='utf-8', timeout=180)
    c.logfile_read = log
    c.expect_exact('Press RETURN to get started!')
    c.sendline('')
    c.expect_exact('clocktest# ')
    return c

def command(text, mode='', allow_error=False):
    child.sendline(text)
    child.expect(re.escape(text[-24:]) + r'\r+\n')
    child.expect_exact('clocktest' + ('(config)' if mode else '') + '# ')
    output = child.before.replace('\r', '')
    if not allow_error:
        assert not re.search(r'^% ', output, re.M), output
    return output

try:
    # Fail if another service owns UDP 123; never replace it.
    ntp.bind(('127.0.0.1', 123))
    ntp.settimeout(.5)
    worker = threading.Thread(target=serve_time)
    worker.start()
    config = dict(version=1, hostname='clocktest',
                  network=dict(interface='eth0', address='10.0.2.15/24', gateway='10.0.2.2', dns='10.0.2.3'),
                  ntp=dict(servers=['10.0.2.2']), clock=dict(save_interval_hours=8))
    (out / 'startup.json').write_text(json.dumps(config) + '\n')
    subprocess.run([sys.executable, str(root / 'os/tools/make-data.py'), '--output', str(out / 'data.img'),
                    '--config', str(out / 'startup.json')], check=True, stdout=subprocess.DEVNULL)
    child = boot()
    command('terminal length 0')
    assert a.expected_source_build in command('show version')
    assert '% ' in command('show calendar', allow_error=True)
    command('conf t', 'config')
    command('clock save interval 8', 'config')
    command('end')
    check('No-RTC boot accepts the platform-specific clock save command')
    for _ in range(60):
        record_output = command('more nvram:saved-clock.json', allow_error=True)
        try:
            record = json.loads(record_output)
            saved = datetime.fromisoformat(record['time'].replace('Z', '+00:00'))
            if saved.timestamp() > time.time() + 3000:
                break
        except (ValueError, KeyError):
            pass
        time.sleep(2)
    else:
        raise AssertionError('NTP timestamp was not persisted: ' + record_output)
    assert 'Time source is NTP' in command('show clock detail')
    check('Native time service saves an actual synchronized NTP timestamp')
    command('conf t', 'config')
    command('no ntp server 10.0.2.2', 'config')
    command('end')
    command('write memory')
    stop.set()
    worker.join(timeout=2)
    assert not worker.is_alive()
    command('clock set 00:00:00 1 January 2003')
    time.sleep(65)  # cross the real persistence worker's next polling interval
    assert json.loads(command('more nvram:saved-clock.json')) == record
    check('A manually set clock does not overwrite the network-derived timestamp')
    child.sendline('reload')
    child.expect_exact('Proceed with reload? [confirm]')
    child.sendline('')
    child.expect(pexpect.EOF, timeout=60)
    stop_qemu(child)
    child = boot()
    command('terminal length 0')
    output = command('show clock detail')
    assert 'Time source is NVRAM' in output and output.startswith('*'), output
    now_line = output.splitlines()[0][1:]
    restored = datetime.strptime(now_line, '%H:%M:%S.%f UTC %a %b %d %Y').replace(tzinfo=timezone.utc)
    assert 0 <= (restored - saved).total_seconds() < 180, (restored, saved)
    assert 'clock save interval 8' in command('show startup-config')
    check('Cold boot restores saved time before networking and marks it non-authoritative')
    result['passed'] = True
except Exception as error:
    result['error'] = str(error)
    raise
finally:
    stop.set()
    if worker is not None:
        worker.join(timeout=2)
    ntp.close()
    finish_qemu(child, result, out / 'result-clock-save.json', log)
