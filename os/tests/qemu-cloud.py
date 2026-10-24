#!/usr/bin/env python3
"""Qualify the distributed single-disk qcow2, NoCloud keys and persisted identity."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import socket
import subprocess
import time
import pexpect

p = argparse.ArgumentParser()
p.add_argument('--artifacts', type=Path, required=True)
p.add_argument('--image', type=Path, required=True)
p.add_argument('--output', type=Path, required=True)
p.add_argument('--accel', choices=['tcg','kvm'], default='kvm')
a = p.parse_args()
art, image, out = a.artifacts.resolve(), a.image.resolve(), a.output.resolve()
out.mkdir(parents=True, exist_ok=False)
result = {'passed':False, 'checks':[], 'image_sha256':hashlib.file_digest(image.open('rb'),'sha256').hexdigest(),
          'system_sha256':hashlib.file_digest((art/'system.bin').open('rb'),'sha256').hexdigest(),
          'rommon_sha256':hashlib.file_digest((art/'rommon.efi').open('rb'),'sha256').hexdigest()}
run = lambda *args: subprocess.run(list(map(str,args)),check=True,stdout=subprocess.DEVNULL)
info = json.loads(subprocess.check_output(['qemu-img','info','--output=json',str(image)]))
assert info['format']=='qcow2' and 'backing-filename' not in info
run('qemu-img','create','-f','qcow2','-F','qcow2','-b',image,out/'disk.qcow2')
shutil.copy(art/'OVMF_VARS-unenrolled.fd',out/'vars.fd')
for name in ('key','other-key'):
    run('ssh-keygen','-q','-t','ed25519','-N','','-f',out/name)
def seed(key, hostname):
    (out/'meta-data').write_text(f'instance-id: qualification\nlocal-hostname: {hostname}\n')
    public=(out/(key+'.pub')).read_text().strip()
    (out/'user-data').write_text(f'#cloud-config\nssh_pwauth: false\nusers:\n  - name: admin\n    lock_passwd: true\n    ssh_authorized_keys: ["{public}"]\n')
    run('genisoimage','-quiet','-output',out/'seed.iso','-volid','CIDATA','-joliet','-rock',out/'meta-data',out/'user-data')
seed('key','vps-test')
with socket.socket() as sock:
    sock.bind(('127.0.0.1',0)); port=sock.getsockname()[1]
vm=None
log=(out/'console.log').open('w')
def boot():
    args=['-machine','q35,smm=on','-accel',a.accel,'-cpu','host' if a.accel=='kvm' else 'core2duo','-m','4096','-smp','2',
          '-global','driver=cfi.pflash01,property=secure,value=on',
          '-drive',f'if=pflash,format=raw,unit=0,readonly=on,file={art / "OVMF_CODE_4M.secboot.fd"}',
          '-drive',f'if=pflash,format=raw,unit=1,file={out / "vars.fd"}',
          '-drive',f'if=virtio,format=qcow2,file={out / "disk.qcow2"}',
          '-drive',f'if=ide,media=cdrom,readonly=on,file={out / "seed.iso"}',
          '-netdev',f'user,id=management,hostfwd=tcp:127.0.0.1:{port}-:22',
          '-device','virtio-net-pci,netdev=management','-display','none','-serial','stdio','-monitor','none','-no-reboot']
    child=pexpect.spawn('qemu-system-x86_64',args,encoding='utf-8',timeout=600 if a.accel=='tcg' else 300)
    child.logfile_read=log
    try:
        child.expect_exact('System initialization complete.')
    except Exception:
        child.terminate(force=True)
        raise
    return child
def ssh_args(key='key'):
    return ['-o','BatchMode=yes','-o','IdentitiesOnly=yes','-o','ConnectTimeout=5',
            '-o','StrictHostKeyChecking=accept-new','-o',f'UserKnownHostsFile={out / "known_hosts"}',
            '-i',str(out/key),'-p',str(port),'admin@127.0.0.1']
def login():
    for attempt in range(20):
        child=pexpect.spawn('ssh',['-tt']+ssh_args(),encoding='utf-8',timeout=10)
        try:
            child.expect_exact('vps-test# '); return child
        except (pexpect.EOF,pexpect.TIMEOUT):
            child.close(force=True)
            if attempt==19: raise
            time.sleep(2)
def check(text):
    result['checks'].append(text); print('PASS '+text,flush=True)
try:
    vm=boot(); session=login()
    session.sendline('show version'); session.expect_exact('vps-test# ')
    version=session.before
    assert 'Fractal Microsystems' in version and 'https://www.fractalmicro.org/techsupport' in version,version
    assert 'ROM: FractalCR System Bootstrap, Version 1.' in version,version
    assert 'System image file is "bootflash:system.bin"' in version,version
    assert 'vps-test uptime is' in version and 'Source build:' in version,version
    (out/'show-version.txt').write_text(version)
    session.sendline('logout');session.expect(pexpect.EOF);session.close()
    check('standalone qcow2 boots; NoCloud CD-ROM provisions DHCP and SSH key-only privilege 15 CLI')
    check('show version reports branding, actual ROMMON version, boot reference, uptime and build identity')
    wrong=subprocess.run(['ssh']+ssh_args('other-key')+['anything'],capture_output=True,timeout=15)
    assert wrong.returncode!=0 and b'Permission denied' in wrong.stderr,wrong.stderr
    execute=subprocess.run(['ssh']+ssh_args()+['uname -a'],capture_output=True,timeout=15)
    assert execute.returncode!=0 and b'Linux' not in execute.stdout,execute.stdout
    check('unprovisioned key and SSH Unix command execution are refused')
    known=(out/'known_hosts').read_bytes()
    vm.terminate(force=True); vm.close(); vm=None
    seed('other-key','must-not-reprovision')
    vm=boot();session=login();session.sendline('logout');session.expect(pexpect.EOF);session.close()
    assert (out/'known_hosts').read_bytes()==known
    check('reboot preserves hostname, administrator key and SSH host identity despite replacement seed')
    result['passed']=True
finally:
    if vm is not None: vm.terminate(force=True);vm.close()
    log.close()
    (out/'result.json').write_text(json.dumps(result,indent=2)+'\n')
