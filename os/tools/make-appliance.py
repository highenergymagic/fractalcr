#!/usr/bin/env python3
"""Create a new GPT image file for USB/SATA/NVMe. Never open a block device.

An ESP contains the removable-media EFI loader and optional authenticated OS.
Only --with-datastore adds a Btrfs partition for single-disk hypervisors. No credentials,
startup configuration, containers, or host identities are copied into the image.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import struct
import subprocess
import tempfile
import uuid
import zlib

SECTOR = 512
MIB = 1024 * 1024
ESP = uuid.UUID('c12a7328-f81f-11d2-ba4b-00a0c93ec93b')
LINUX = uuid.UUID('0fc63daf-8483-4772-8e79-3d69d8477de4')

def entry(kind, first, last, name):
    return struct.pack('<16s16sQQQ72s', kind.bytes_le, uuid.uuid4().bytes_le,
                       first, last, 0, name.encode('utf-16le'))

def header(current, backup, sectors, entries_lba, disk_id, crc):
    data = bytearray(struct.pack('<8sIIIIQQQQ16sQIII', b'EFI PART', 0x10000,
                    92, 0, 0, current, backup, 34, sectors-34,
                    disk_id.bytes_le, entries_lba, 128, 128, crc))
    struct.pack_into('<I', data, 16, zlib.crc32(data))
    return bytes(data).ljust(SECTOR, b'\0')

def sparse_copy(src, out):
    with src.open('rb') as stream:
        while block := stream.read(MIB):
            if block.count(0) == len(block):
                out.seek(len(block), 1)
            else:
                out.write(block)

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--efi', type=Path, required=True)
    p.add_argument('--system', type=Path, help='Omit for loader-only recovery media')
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--size-mib', type=int, default=2048)
    p.add_argument('--with-datastore', action='store_true', help='Add data0 for single-disk hypervisor deployment')
    p.add_argument('--firmware-policy', choices=['required', 'optional'], default='required')
    a = p.parse_args()
    if not 2048 <= a.size_mib <= 1048576:
        p.error('size must be between 2048 and 1048576 MiB')
    for source in (a.efi, a.system):
        if source and (not source.is_file() or source.is_symlink()):
            p.error('inputs must be regular files')
    if a.system and a.system.stat().st_size > 900*MIB:
        p.error('system image exceeds ESP capacity')
    # Exclusive creation also refuses existing devices, links and disk images.
    fd = os.open(a.output, os.O_CREAT | os.O_EXCL | os.O_RDWR, 0o600)
    with os.fdopen(fd, 'w+b') as out, tempfile.TemporaryDirectory(dir=a.output.parent) as temporary:
        root = Path(temporary)
        esp, data = root/'esp.img', root/'data.img'
        sectors = a.size_mib * 2048
        esp_first, esp_last = 2048, 1025*2048-1
        data_first, data_last = esp_last+1, (sectors-34)//2048*2048-1
        with esp.open('wb') as f: f.truncate((esp_last-esp_first+1)*SECTOR)
        subprocess.run(['mkfs.vfat', '-F', '32', '-n', 'FRFLASH', str(esp)], check=True)
        subprocess.run(['mmd','-i',str(esp),'::/EFI','::/EFI/BOOT'],check=True)
        subprocess.run(['mcopy','-i',str(esp),str(a.efi),'::/EFI/BOOT/BOOTX64.EFI'],check=True)
        if a.system:
            subprocess.run(['mcopy','-i',str(esp),str(a.system),'::/system.bin'],check=True)
        body = ('BOOT=bootflash:system.bin\nCONFREG=0x2102\nIP_ADDRESS=\n'
                'IP_SUBNET_MASK=\nDEFAULT_GATEWAY=\nTFTP_SERVER=\nTFTP_FILE=\n'
                'ETHER_PORT=0\nSECURE_BOOT='+a.firmware_policy+'\n').encode()
        env = root/'rommon.env1'
        env.write_bytes(b'FCR-ENV-1 1 '+hashlib.sha256(body).hexdigest().encode()+b'\n'+body)
        subprocess.run(['mcopy','-i',str(esp),str(env),'::/rommon.env1'],check=True)
        entries = entry(ESP,esp_first,esp_last,'FRFLASH')
        if a.with_datastore:
            with data.open('wb') as f: f.truncate((data_last-data_first+1)*SECTOR)
            subprocess.run(['mkfs.btrfs','-L','FRDATA','-d','single','-m','dup',str(data)],check=True)
            entries += entry(LINUX,data_first,data_last,'FRDATA')
        entries = entries.ljust(128*128,b'\0')
        crc, identity = zlib.crc32(entries), uuid.uuid4()
        mbr=bytearray(SECTOR)
        mbr[446:462]=struct.pack('<B3sB3sII',0,b'\0\2\0',0xee,b'\xff\xff\xff',1,min(sectors-1,0xffffffff))
        mbr[510:512]=b'\x55\xaa'
        out.write(mbr)
        out.write(header(1,sectors-1,sectors,2,identity,crc)); out.write(entries)
        out.seek(esp_first*SECTOR); sparse_copy(esp,out)
        if a.with_datastore:
            out.seek(data_first*SECTOR); sparse_copy(data,out)
        out.seek((sectors-33)*SECTOR); out.write(entries)
        out.write(header(sectors-1,1,sectors,sectors-33,identity,crc))
        out.truncate(sectors*SECTOR); out.flush(); os.fsync(out.fileno())
    manifest = {'format':'GPT raw disk', 'size_bytes':sectors*SECTOR,
                'firmware_policy':a.firmware_policy,'console':'ttyS0,115200n8',
                'data':'empty Btrfs single; no startup-config' if a.with_datastore else 'external disks; no datastore on boot media',
                'inputs':{p.name:hashlib.file_digest(p.open('rb'),'sha256').hexdigest()
                          for p in (a.efi,a.system) if p}}
    a.output.with_suffix(a.output.suffix+'.json').write_text(json.dumps(manifest,indent=2)+'\n')
    print(a.output)

if __name__ == '__main__':
    main()
