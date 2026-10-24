#!/usr/bin/env python3
"""Read-only protocol validation; never sends a valid format request."""
import os,pathlib,socket,struct,subprocess,sys,time
path=pathlib.Path('/run/fractal-storage.sock')
if os.geteuid()!=0:raise SystemExit('Run this isolated service test as root')
if path.exists():raise SystemExit('Refusing to replace an existing storage service socket')
child=subprocess.Popen([str(pathlib.Path(sys.argv[1]).resolve())])
try:
    for _ in range(50):
        if path.exists():break
        if child.poll() is not None:raise RuntimeError('storage service failed to start')
        time.sleep(.02)
    assert path.exists() and path.stat().st_mode & 0o777 == 0o600
    def check(message):
        with socket.socket(socket.AF_UNIX,socket.SOCK_SEQPACKET) as channel:
            channel.settimeout(3);channel.connect(str(path));channel.send(message)
            assert struct.unpack('<III',channel.recv(100))==(0x46535431,1,1)
    check(b'bad')
    check(struct.pack('<IIIII',0x46535431,1,2,0,0)+bytes(16*64))
    check(struct.pack('<IIIII',0x46535431,1,2,1,1)+b'sda'+bytes(16*64-3))
    check(struct.pack('<IIIII',0x46535431,1,2,0,1)+b'../sda'+bytes(16*64-6))
    check(struct.pack('<IIIII',0x46535431,1,2,0,1)+b'x'*(16*64))
    check(bytes(2048))
    print('STORAGE_PROTOCOL_INVALID_REQUESTS_REJECTED')
finally:
    child.terminate();child.wait(timeout=5)
    assert not path.exists()
