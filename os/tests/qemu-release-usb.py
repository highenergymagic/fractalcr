#!/usr/bin/env python3
"""Boot the actual distributed compressed USB images, including firmware policy."""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
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
    # Exercise a server RAID virtual disk and paravirtual NIC as well as the
    # virtio baseline. The USB must never become a datastore candidate.
    for policy,adapter in (('required','virtio'),('optional','virtio'),
                           ('optional','megasas-gen2')):
        case = policy if adapter == 'virtio' else 'server-adapters'
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
        variables=out/(case+'.vars.fd')
        table=json.loads(subprocess.check_output(['sfdisk','--json',str(raw)]))['partitiontable']['partitions']
        assert len(table)==1 and table[0]['name']=='FRFLASH',table
        data=out/(case+'-data.img')
        with data.open('xb') as disk: disk.truncate(4*1024**3)
        shutil.copy(art/('OVMF_VARS-trusted.fd' if policy=='required' else 'OVMF_VARS-unenrolled.fd'),variables)
        args=['-machine','q35,smm=on','-accel',a.accel,'-cpu','host' if a.accel=='kvm' else 'core2duo','-m','4096','-smp','2',
              '-global','driver=cfi.pflash01,property=secure,value=on',
              '-drive',f'if=pflash,format=raw,unit=0,readonly=on,file={art / "OVMF_CODE_4M.secboot.fd"}',
              '-drive',f'if=pflash,format=raw,unit=1,file={variables}',
              '-drive',f'if=none,id=system,format=raw,file={raw}',
              '-device','qemu-xhci','-device','usb-storage,drive=system,bootindex=0',
              '-display','none','-serial','stdio','-monitor','none','-no-reboot']
        if adapter == 'virtio':
            args += ['-drive',f'if=virtio,format=raw,file={data}',
                     '-nic','user,model=virtio-net-pci']
        else:
            args += ['-drive',f'if=none,id=data,format=raw,file={data}',
                     '-device','megasas-gen2,id=scsi0',
                     '-device','scsi-hd,drive=data,bus=scsi0.0',
                     '-netdev','user,id=management',
                     '-device','vmxnet3,netdev=management']
        child=None
        with (out/(case+'.log')).open('w') as log:
            try:
                child=pexpect.spawn('qemu-system-x86_64',args,encoding='utf-8',timeout=600 if a.accel=='tcg' else 300)
                child.logfile_read=log
                child.expect_exact('Image signature verified.')
                child.expect_exact('Press RETURN to get started!');child.sendline('')
                child.expect_exact('Enter initial configuration dialog? [yes/no]: ')
                child.sendline('yes');child.expect_exact("Data disk (or 'abort'): ")
                if adapter == 'virtio':
                    expected_disk = 'vda'
                    assert re.search(r'(?m)^vda\r*$',child.before),child.before
                    rejected_disk = 'sda'
                else:
                    candidates = re.findall(r'(?m)^(sd[a-z])\r*$',child.before)
                    assert len(candidates) == 1,candidates
                    expected_disk = candidates[0]
                    rejected_disk = 'sdb' if expected_disk == 'sda' else 'sda'
                child.sendline(rejected_disk);child.expect_exact("Data disk (or 'abort'): ")
                assert 'select an unused whole disk from the list' in child.before,child.before
                child.sendline(expected_disk);child.expect_exact('Type INITIALIZE to format the selected unused disk: ')
                child.sendline('INITIALIZE');child.expect_exact('Host name [ctr]: ')
                child.sendline('ctr');child.expect_exact('Management interface: ')
                child.sendline('eth0');child.expect_exact('IP address/prefix or dhcp [dhcp]: ')
                child.sendline('dhcp');child.expect_exact('NTP server (address or hostname; blank disables): ')
                child.sendline('');child.expect_exact('Administrator username [admin]: ')
                child.sendline('admin');child.expect_exact('Administrator password: ')
                child.sendline('USB-qualification-592!');child.expect_exact('Apply and save this configuration? [yes/no]: ')
                child.sendline('yes');child.expect_exact('Username: ')
                child.sendline('admin');child.expect_exact('Password: ')
                child.sendline('USB-qualification-592!');child.expect_exact('ctr# ')
                def command(line):
                    child.sendline(line)
                    child.expect(re.escape(line) + r'\r+\n')
                    child.expect_exact('ctr# ')
                    return child.before
                command('terminal length 0')
                storage=command('show storage')
                assert 'Datastore data0 is online' in storage,storage
                version=command('show version')
                assert 'Fractal Microsystems' in version and 'bootflash:system.bin' in version,version
                if adapter != 'virtio':
                    assert '1 physical network interface' in version,version
                result['checks'].append(f'{case}: boot-only USB has one FRFLASH partition; setup rejects the USB and initializes data0 on a separate {adapter} disk; management NIC and administrator configuration are available')
                print('PASS '+result['checks'][-1],flush=True)
            finally:
                if child is not None: child.terminate(force=True);child.close()
        label=subprocess.check_output(['blkid','-p','-s','LABEL','-o','value',str(data)],text=True).strip()
        assert label=='FRDATA',label
        raw.unlink()
        data.unlink()
    result['passed']=True
finally:
    (out/'result.json').write_text(json.dumps(result,indent=2)+'\n')
