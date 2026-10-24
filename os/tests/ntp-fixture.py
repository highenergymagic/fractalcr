#!/usr/bin/env python3
"""Loopback-only NTP qualification source. Fails rather than replacing a listener."""
import signal
import socket
import struct
import time

def stamp(t):
    seconds=t+2208988800
    return struct.pack('!II',int(seconds)&0xffffffff,int((seconds%1)*2**32))

sock=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
sock.bind(('127.0.0.1',123))
sock.settimeout(1)
running=True
def stop(*_):
    global running
    running=False
signal.signal(signal.SIGTERM,stop)
signal.signal(signal.SIGINT,stop)
print('NTP_FIXTURE_READY',flush=True)
while running:
    try: data,peer=sock.recvfrom(512)
    except socket.timeout: continue
    if len(data)<48 or data[0]&7!=3: continue
    received=time.time()
    packet=struct.pack('!BBBbII4s',0x24,2,6,-20,0,655,b'TEST')
    packet+=stamp(received-1)+data[40:48]+stamp(received)+stamp(time.time())
    sock.sendto(packet,peer)
sock.close()
