#!/usr/bin/env python3
"""Boot the actual distributed compressed USB images, including firmware policy."""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import shutil
import pexpect

p=argparse.ArgumentParser()
p.add_argument('--artifacts',type=Path,required=True)
p.add_argument('--images',type=Path,required=True)
p.add_argument('--output',type=Path,required=True)
p.add_argument('--accel',choices=['tcg','kvm'],default='kvm')
a=p.parse_args()
art,images,out=a.artifacts.resolve(),a.images.resolve(),a.output.resolve()
out.mkdir(parents=True,exist_ok=False)
result={'passed':False,'images':{},'checks':[]}
try:
    for policy in ('required','optional'):
        matches=list(images.glob('usb-secureboot.img.gz' if policy=='required' else 'usb.img.gz'))
        assert len(matches)==1,matches
        compressed=matches[0]
        result['images'][compressed.name]=hashlib.file_digest(compressed.open('rb'),'sha256').hexdigest()
        raw=out/(policy+'.img')
        with gzip.open(compressed,'rb') as source,raw.open('xb') as target:
            while chunk:=source.read(1024*1024):
                if chunk.count(0)==len(chunk): target.seek(len(chunk),1)
                else: target.write(chunk)
            target.truncate()
        variables=out/(policy+'.vars.fd')
        shutil.copy(art/('OVMF_VARS-trusted.fd' if policy=='required' else 'OVMF_VARS-unenrolled.fd'),variables)
        args=['-machine','q35,smm=on','-accel',a.accel,'-cpu','host' if a.accel=='kvm' else 'core2duo','-m','4096','-smp','2',
              '-global','driver=cfi.pflash01,property=secure,value=on',
              '-drive',f'if=pflash,format=raw,unit=0,readonly=on,file={art / "OVMF_CODE_4M.secboot.fd"}',
              '-drive',f'if=pflash,format=raw,unit=1,file={variables}',
              '-drive',f'if=none,id=system,format=raw,file={raw}',
              '-device','qemu-xhci','-device','usb-storage,drive=system,bootindex=0',
              '-nic','none','-display','none','-serial','stdio','-monitor','none','-no-reboot']
        child=None
        with (out/(policy+'.log')).open('w') as log:
            try:
                child=pexpect.spawn('qemu-system-x86_64',args,encoding='utf-8',timeout=600 if a.accel=='tcg' else 300)
                child.logfile_read=log
                child.expect_exact('Image signature verified.')
                child.expect_exact('Press RETURN to get started!');child.sendline('')
                child.expect_exact('Enter initial configuration dialog? [yes/no]: ')
                child.sendline('no');child.expect_exact('ctr# ')
                child.sendline('show storage');child.expect_exact('ctr# ')
                assert 'Datastore data0 is online' in child.before,child.before
                child.sendline('show version');child.expect_exact('ctr# ')
                assert 'Fractal Microsystems' in child.before and 'bootflash:system.bin' in child.before,child.before
                result['checks'].append(f'{policy}: compressed release USB image boots with matching firmware policy, finds its datastore and offers serial setup')
                print('PASS '+result['checks'][-1],flush=True)
            finally:
                if child is not None: child.terminate(force=True);child.close()
        raw.unlink()
    result['passed']=True
finally:
    (out/'result.json').write_text(json.dumps(result,indent=2)+'\n')
