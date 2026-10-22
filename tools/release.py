#!/usr/bin/env python3
"""Offline encrypted-key custody, public-only build trust, signing and verification.

Private keys never belong in source archives or builder volumes. PE signing runs
on a Linux signing station; its decrypted key is passed through a sealed memfd.
"""
import argparse
import datetime as dt
import fcntl
import getpass
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import struct
import subprocess
import tempfile
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa
from cryptography.x509.oid import NameOID

DOMAIN=b'FractalCR system image v2\n'
MAGIC=b'FCRBIN02'
ROOT=Path(__file__).resolve().parents[1]
def digest(b):return hashlib.sha256(b).hexdigest()
def public_id(key):return digest(key.public_bytes(serialization.Encoding.DER,serialization.PublicFormat.SubjectPublicKeyInfo))
def write(path,data,mode=0o600):
    path=Path(path);path.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,mode)
    with os.fdopen(fd,'wb') as f:f.write(data);f.flush();os.fsync(f.fileno())

def private_file(path, maximum=65536):
    # Validate the descriptor we read, and never wait for a FIFO or follow a
    # substituted symlink. Vault input is small and must have a single owner.
    fd=os.open(path,os.O_RDONLY|os.O_CLOEXEC|os.O_NOFOLLOW|os.O_NONBLOCK)
    with os.fdopen(fd,'rb') as f:
        info=os.fstat(f.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode&0o077 or info.st_uid!=os.geteuid() or info.st_nlink!=1 or info.st_size>maximum:
            raise ValueError('vault input must be an owned, private regular file within its size limit')
        value=f.read(maximum+1)
        if len(value)>maximum:raise ValueError('vault input exceeds size limit')
        return value
def password(args):
    if args.password_file:
        value=private_file(args.password_file,4096).rstrip(b'\r\n')
    else:value=getpass.getpass('Offline vault passphrase: ').encode()
    if len(value)<20:raise ValueError('vault passphrase must contain at least 20 bytes')
    return value
def trust(directory,allow_legacy=False):
    directory=Path(directory)
    for path in directory.iterdir():
        if path.name not in ('firmware.crt','release.pub','trust.json'):raise ValueError('unexpected file in public trust bundle')
        if not path.is_file() or path.is_symlink():raise ValueError('trust bundle must contain only regular public files')
        if b'PRIVATE KEY' in path.read_bytes():raise ValueError('private key found in public trust bundle')
    profile=json.loads((directory/'trust.json').read_text())
    cert=x509.load_pem_x509_certificate((directory/'firmware.crt').read_bytes())
    key=serialization.load_pem_public_key((directory/'release.pub').read_bytes())
    if not isinstance(key,rsa.RSAPublicKey) or key.key_size!=2048:raise ValueError('release format v2 requires RSA-2048')
    if profile.get('schema')!=1 or profile.get('firmware_sha256')!=digest(cert.public_bytes(serialization.Encoding.DER)) or profile.get('release_sha256')!=public_id(key):raise ValueError('trust fingerprint mismatch')
    floor=profile.get('minimum_security_epoch')
    legacy=allow_legacy and profile.get('format')==1 and floor is None
    if not legacy and (profile.get('format')!=2 or type(floor) is not int or not 1<=floor<2**64):raise ValueError('trust profile must specify a v2 security epoch floor; rotate legacy trust profiles')
    if cert.not_valid_after.replace(tzinfo=dt.timezone.utc)<=dt.datetime.now(dt.timezone.utc):raise ValueError('firmware certificate expired')
    return profile,cert,key
def pe_digest(data):
    # Authenticode excludes checksum, certificate directory and certificate table.
    if len(data)<256 or data[:2]!=b'MZ':raise ValueError('not a PE image')
    pe=struct.unpack_from('<I',data,0x3c)[0]
    if pe+24+160>len(data) or data[pe:pe+4]!=b'PE\0\0':raise ValueError('invalid PE header')
    opt=pe+24;magic=struct.unpack_from('<H',data,opt)[0]
    security=opt+(112 if magic==0x20b else 96 if magic==0x10b else 0)+32
    if magic not in (0x20b,0x10b):raise ValueError('unknown PE format')
    offset,length=struct.unpack_from('<II',data,security)
    if bool(offset)!=bool(length) or (offset and (offset+length!=len(data) or offset<security+8)):raise ValueError('noncanonical PE certificate table')
    end=offset or len(data)
    # sbsign adds zero alignment before the certificate table. Those bytes are
    # part of the signed content and must be normalized for the unsigned input.
    alignment=b'\0'*((-len(data))%8) if not offset else b''
    return digest(data[:opt+64]+data[opt+68:security]+data[security+8:end]+alignment)
def audit(vault,record):
    path=vault/'signing-audit.jsonl'
    fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_APPEND|os.O_NOFOLLOW|os.O_NONBLOCK,0o600)
    with os.fdopen(fd,'a') as f:
        info=os.fstat(f.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode&0o077 or info.st_uid!=os.geteuid() or info.st_nlink!=1:raise ValueError('signing audit must be an owned, private regular file')
        fcntl.flock(f,fcntl.LOCK_EX);record['time']=dt.datetime.now(dt.timezone.utc).isoformat();f.write(json.dumps(record,sort_keys=True)+'\n');f.flush();os.fsync(f.fileno())
def initialize(args):
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.-]{0,62}',args.name):raise ValueError('invalid release identity')
    if not 1<=args.minimum_security_epoch<2**64:raise ValueError('invalid minimum security epoch')
    secret=password(args);args.vault.mkdir(mode=0o700,parents=True,exist_ok=False)
    firmware=rsa.generate_private_key(public_exponent=65537,key_size=3072)
    release=rsa.generate_private_key(public_exponent=65537,key_size=2048)
    now=dt.datetime.now(dt.timezone.utc).replace(microsecond=0)
    name=x509.Name([x509.NameAttribute(NameOID.COMMON_NAME,'FractalCR Release '+args.name)])
    cert=x509.CertificateBuilder().subject_name(name).issuer_name(name).public_key(firmware.public_key()).serial_number(x509.random_serial_number()).not_valid_before(now).not_valid_after(now+dt.timedelta(days=1825)).add_extension(x509.BasicConstraints(ca=False,path_length=None),critical=True).sign(firmware,hashes.SHA256())
    for label,key in [('firmware',firmware),('release',release)]:
        write(args.vault/(label+'.encrypted.pem'),key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.BestAvailableEncryption(secret)))
    public=args.vault/'public';public.mkdir(mode=0o700)
    write(public/'firmware.crt',cert.public_bytes(serialization.Encoding.PEM),0o644)
    write(public/'release.pub',release.public_key().public_bytes(serialization.Encoding.PEM,serialization.PublicFormat.SubjectPublicKeyInfo),0o644)
    profile=dict(schema=1,name=args.name,firmware_sha256=digest(cert.public_bytes(serialization.Encoding.DER)),release_sha256=public_id(release.public_key()),format=2,minimum_security_epoch=args.minimum_security_epoch)
    write(public/'trust.json',(json.dumps(profile,indent=2)+'\n').encode(),0o644)
    audit(args.vault,dict(event='initialize',**profile));print(json.dumps(profile))
def inspect_vault(args):
    profile,cert,pub=trust(args.vault/'public');secret=password(args)
    for name,expected in [('firmware',cert.public_key()),('release',pub)]:
        path=args.vault/(name+'.encrypted.pem')
        key=serialization.load_pem_private_key(private_file(path),secret)
        if public_id(key.public_key())!=public_id(expected):raise ValueError('vault key does not match public trust')
    print(json.dumps(dict(verified=True,trust=profile)))
def prepare(args):
    profile,_,_=trust(args.public)
    args.destination.mkdir(parents=True,exist_ok=True)
    if list(args.destination.glob('*.key')) or list(args.destination.glob('*.encrypted.pem')):raise ValueError('production build refuses a directory containing private keys; use a fresh build volume')
    for name in ('firmware.crt','release.pub','trust.json'):
        target=args.destination/name
        data=(args.public/name).read_bytes()
        if target.exists():
            if target.read_bytes()!=data:raise ValueError('trust cannot change inside an existing build directory')
        else:write(target,data,0o644)
    pem_text=(args.public/'release.pub').read_text()
    header=f'/* Production public trust only. */\n#define FCR_MIN_SECURITY_EPOCH {profile["minimum_security_epoch"]}ULL\nstatic const char release_key_pem[] =\n'+''.join(json.dumps(line+'\n')+'\n' for line in pem_text.splitlines())+';\n'
    target=args.destination/'release-key.h'
    if target.exists():
        if target.read_text()!=header:raise ValueError('embedded trust header differs')
    else:write(target,header.encode(),0o644)
    print(json.dumps(profile))
def sign(args):
    profile,cert,pub=trust(args.vault/'public');secret=password(args)
    epoch=args.security_epoch if args.security_epoch is not None else profile['minimum_security_epoch']
    if not profile['minimum_security_epoch']<=epoch<2**64:raise ValueError('image security epoch is below trust policy or outside uint64')
    firmware=serialization.load_pem_private_key(private_file(args.vault/'firmware.encrypted.pem'),secret)
    release=serialization.load_pem_private_key(private_file(args.vault/'release.encrypted.pem'),secret)
    if public_id(firmware.public_key())!=public_id(cert.public_key()) or public_id(release.public_key())!=public_id(pub):raise ValueError('vault keys do not match public trust')
    source=args.input.read_bytes();original=pe_digest(source)
    if digest(source)!=args.expected_sha256:raise ValueError('input does not match the reviewed SHA-256')
    if args.output.exists():raise ValueError('refusing to overwrite a signed artifact')
    key_bytes=firmware.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption())
    fd=os.memfd_create('fractal-offline-signing-key',os.MFD_CLOEXEC|os.MFD_ALLOW_SEALING)
    try:
        os.write(fd,key_bytes);os.lseek(fd,0,os.SEEK_SET)
        fcntl.fcntl(fd,fcntl.F_ADD_SEALS,fcntl.F_SEAL_WRITE|fcntl.F_SEAL_SHRINK|fcntl.F_SEAL_GROW|fcntl.F_SEAL_SEAL)
        with tempfile.TemporaryDirectory() as tmp:
            signed=Path(tmp)/'signed.efi'
            unsigned=Path(tmp)/'unsigned.efi';unsigned.write_bytes(source)
            subprocess.run(['bash',str(ROOT/'boot/tools/sign-pe.sh'),'--key',f'/proc/self/fd/{fd}','--cert',str(args.vault/'public/firmware.crt'),'--output',str(signed),str(unsigned)],check=True,pass_fds=(fd,))
            payload=signed.read_bytes()
            if pe_digest(payload)!=original:raise ValueError('PE signer changed the authenticated payload')
            subprocess.run(['sbverify','--cert',str(args.vault/'public/firmware.crt'),str(signed)],check=True)
            if args.kind=='system':
                payload+=epoch.to_bytes(8,'big')
                payload+=release.sign(DOMAIN+payload,padding.PKCS1v15(),hashes.SHA256())+MAGIC
            write(args.output,payload,0o644)
            record=dict(event='sign',kind=args.kind,input_sha256=digest(source),output_sha256=digest(payload),trust=profile)
            if args.kind=='system':record['security_epoch']=epoch
            audit(args.vault,record);write(Path(str(args.output)+'.receipt.json'),(json.dumps(record,indent=2)+'\n').encode(),0o644)
            print(json.dumps(record))
    finally:os.close(fd)
def verify(args):
    profile,_,pub=trust(args.public);payload=args.input.read_bytes()
    if args.kind=='system':
        if len(payload)<=272 or payload[-8:]!=MAGIC:raise ValueError('invalid v2 system envelope')
        pub.verify(payload[-264:-8],DOMAIN+payload[:-264],padding.PKCS1v15(),hashes.SHA256())
        if int.from_bytes(payload[-272:-264],'big')<profile['minimum_security_epoch']:raise ValueError('image security epoch is below trust policy')
        payload=payload[:-272]
    if pe_digest(payload)!=pe_digest(args.unsigned.read_bytes()):raise ValueError('signed artifact differs from the reviewed unsigned build')
    with tempfile.TemporaryDirectory() as tmp:
        path=Path(tmp)/'image.efi';path.write_bytes(payload)
        subprocess.run(['sbverify','--cert',str(args.public/'firmware.crt'),str(path)],check=True)
    print(json.dumps(dict(verified=True,sha256=digest(args.input.read_bytes()),trust=profile)))
def rotation(args):
    old,cert,_=trust(args.vault/'public',allow_legacy=True);new,_,_=trust(args.new_public)
    if old['firmware_sha256']==new['firmware_sha256'] or old['release_sha256']==new['release_sha256']:raise ValueError('rotation requires new firmware and release keys')
    if new['minimum_security_epoch']<old.get('minimum_security_epoch',0):raise ValueError('rotation must not lower the security epoch floor')
    secret=password(args)
    key=serialization.load_pem_private_key(private_file(args.vault/'firmware.encrypted.pem'),secret)
    if public_id(key.public_key())!=public_id(cert.public_key()):raise ValueError('old vault key mismatch')
    args.output.mkdir(parents=True,exist_ok=False,mode=0o700)
    fd=os.memfd_create('fractal-rotation-key',os.MFD_CLOEXEC|os.MFD_ALLOW_SEALING)
    try:
        os.write(fd,key.private_bytes(serialization.Encoding.PEM,serialization.PrivateFormat.PKCS8,serialization.NoEncryption()))
        fcntl.fcntl(fd,fcntl.F_ADD_SEALS,fcntl.F_SEAL_WRITE|fcntl.F_SEAL_SHRINK|fcntl.F_SEAL_GROW|fcntl.F_SEAL_SEAL)
        for label,public in [('old',args.vault/'public'),('new',args.new_public)]:
            subprocess.run(['cert-to-efi-sig-list','-g','748e6cc5-19b4-486b-9e22-4e06d330890a',str(public/'firmware.crt'),str(args.output/(label+'.esl'))],check=True)
        # The dedicated appliance profile enrolls its firmware certificate as
        # PK and KEK as well as db. Updates are authorized by that current owner.
        updates=[('01-db-add-new','db','new',True),('02-KEK-add-new','KEK','new',True),
                 ('03-dbx-revoke-old','dbx','old',True),('04-KEK-retire-old','KEK','new',False),('05-PK-replace','PK','new',False)]
        base_time=dt.datetime.now(dt.timezone.utc).replace(microsecond=0)
        for index,(name,variable,which,append) in enumerate(updates):
            stamp=(base_time+dt.timedelta(seconds=index)).strftime('%Y-%m-%d %H:%M:%S')
            command=['sign-efi-sig-list']+(['-a'] if append else [])+['-t',stamp,'-k',f'/proc/self/fd/{fd}','-c',str(args.vault/'public/firmware.crt'),variable,str(args.output/(which+'.esl')),str(args.output/(name+'.auth'))]
            subprocess.run(command,check=True,pass_fds=(fd,),env={**os.environ,'LC_ALL':'C','TZ':'UTC'})
            actual=(args.output/(name+'.auth')).read_bytes()
            if struct.unpack_from('<H',actual)[0]!=base_time.year:raise ValueError('efitools timestamp parser is incompatible; update efitools before generating rotation packages')
        plan=dict(schema=1,old=old,new=new,phase1=['01-db-add-new.auth','02-KEK-add-new.auth'],
                  gate='Install and boot the new signed ROMMON and system; verify their identities before phase2.',
                  phase2=['03-dbx-revoke-old.auth','04-KEK-retire-old.auth','05-PK-replace.auth'],
                  warning='Phase2 revokes every image signed by the old firmware key. Preserve recovery media signed by the new keys.',
                  files={f.name:digest(f.read_bytes()) for f in args.output.iterdir()})
        write(args.output/'rotation.json',(json.dumps(plan,indent=2)+'\n').encode(),0o644)
        audit(args.vault,dict(event='rotation-package',old=old,new=new,files=plan['files']));print(json.dumps(plan))
    finally:os.close(fd)
def main():
    parser=argparse.ArgumentParser();sub=parser.add_subparsers(dest='command',required=True)
    q=sub.add_parser('init');q.add_argument('--vault',type=Path,required=True);q.add_argument('--name',required=True);q.add_argument('--password-file',type=Path);q.add_argument('--minimum-security-epoch',type=int,default=1);q.set_defaults(action=initialize)
    q=sub.add_parser('verify-vault');q.add_argument('--vault',type=Path,required=True);q.add_argument('--password-file',type=Path);q.set_defaults(action=inspect_vault)
    q=sub.add_parser('prepare-trust');q.add_argument('--public',type=Path,required=True);q.add_argument('--destination',type=Path,required=True);q.set_defaults(action=prepare)
    q=sub.add_parser('sign');q.add_argument('--vault',type=Path,required=True);q.add_argument('--password-file',type=Path);q.add_argument('--input',type=Path,required=True);q.add_argument('--expected-sha256',required=True);q.add_argument('--output',type=Path,required=True);q.add_argument('--kind',choices=['rommon','system'],required=True);q.add_argument('--security-epoch',type=int);q.set_defaults(action=sign)
    q=sub.add_parser('verify');q.add_argument('--public',type=Path,required=True);q.add_argument('--unsigned',type=Path,required=True);q.add_argument('--input',type=Path,required=True);q.add_argument('--kind',choices=['rommon','system'],required=True);q.set_defaults(action=verify)
    q=sub.add_parser('rotation');q.add_argument('--vault',type=Path,required=True);q.add_argument('--new-public',type=Path,required=True);q.add_argument('--password-file',type=Path);q.add_argument('--output',type=Path,required=True);q.set_defaults(action=rotation)
    args=parser.parse_args();args.action(args)
if __name__=='__main__':main()
