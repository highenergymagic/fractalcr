#!/usr/bin/env python3
"""Boot loader-only GPT USB media, with no system image or startup config."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import pexpect

p=argparse.ArgumentParser()
p.add_argument('--artifacts',type=Path,required=True)
a=p.parse_args(); art=a.artifacts.resolve(); src=Path(__file__).resolve().parents[2]
disk=art/'loader-only.img'; variables=art/'loader-only-vars.fd'
subprocess.run(['python3',str(src/'os/tools/make-appliance.py'),'--efi',str(art/'rommon.efi'),'--output',str(disk),'--size-mib','2048','--firmware-policy','optional'],check=True)
subprocess.run(['sfdisk','--verify',str(disk)],check=True)
shutil.copy(art/'OVMF_VARS-unenrolled.fd',variables)
result={'case':'loader-only','passed':False,'rommon_sha256':hashlib.sha256((art/'rommon.efi').read_bytes()).hexdigest()}
log=(art/'loader-only.log').open('w'); child=None
try:
    child=pexpect.spawn('qemu-system-x86_64',['-machine','q35,smm=on','-accel','kvm','-m','512',
        '-global','driver=cfi.pflash01,property=secure,value=on',
        '-drive',f'if=pflash,format=raw,unit=0,readonly=on,file={art / "OVMF_CODE_4M.secboot.fd"}',
        '-drive',f'if=pflash,format=raw,unit=1,file={variables}',
        '-drive',f'if=none,id=usb,format=raw,file={disk}','-device','qemu-xhci',
        '-device','usb-storage,drive=usb,bootindex=0','-display','none','-serial','stdio','-monitor','none','-no-reboot'],encoding='utf-8',timeout=90)
    child.logfile_read=log
    child.expect(r'rommon \d+ > ')
    child.sendline('set'); child.expect(r'rommon \d+ > ')
    assert 'SECURE_BOOT=optional' in child.before,child.before
    child.sendline('dir bootflash:'); child.expect(r'rommon \d+ > ')
    assert 'rommon.env1' in child.before and 'system.bin' not in child.before,child.before
    child.sendline('show version'); child.expect(r'rommon \d+ > ')
    assert 'mandatory' in child.before,child.before
    result['passed']=True
    print('PASS loader-only GPT USB enters ROMMON recovery without a system image',flush=True)
finally:
    if child is not None and child.isalive(): child.terminate(force=True)
    log.close(); (art/'result-loader-only.json').write_text(json.dumps(result,indent=2)+'\n')
