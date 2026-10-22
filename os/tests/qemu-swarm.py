#!/usr/bin/env python3
"""Three independent appliance kernels, isolated L2, real Swarm failure tests."""
import argparse
import hashlib
import json
import re
import shutil
import socket
import struct
import subprocess
import sys
import time
from pathlib import Path
import pexpect

SRC = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SRC / 'boot/tests'))
from qemu_process import stop_qemu

p = argparse.ArgumentParser()
p.add_argument('--artifacts', type=Path, required=True)
p.add_argument('--output', type=Path, required=True)
a = p.parse_args()
art, out = a.artifacts.resolve(), a.output.resolve()
out.mkdir(parents=True, exist_ok=False)
result = dict(case='swarm-multinode', passed=False, checks=[], nodes=3, accel='kvm',
              system_sha256=hashlib.sha256((art/'system.bin').read_bytes()).hexdigest(),
              rommon_sha256=hashlib.sha256((art/'rommon.efi').read_bytes()).hexdigest())
with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as port_socket:
    port_socket.bind(('127.0.0.1',0)); port=port_socket.getsockname()[1]
flash=out/'bootflash.img'
subprocess.run(['python3',str(SRC/'boot/tools/make-flash.py'),'--efi',str(art/'rommon.efi'),
                '--system',str(art/'system.bin'),'--output',str(flash),'--size-mib','1024','--no-tampered-fixture'],check=True)

def check(message):
    result['checks'].append(message); print('PASS '+message,flush=True)

class Node:
    def __init__(self,index):
        self.index=index;self.name=f'swarm{index}';self.ip=f'10.77.42.{10+index}'
        self.log=(out/f'{self.name}.log').open('w');self.child=None
        config=out/f'{self.name}.json'
        config.write_text(json.dumps(dict(version=1,hostname=self.name,network=dict(interface='eth0',address=self.ip+'/24'))))
        self.disk=out/f'{self.name}.img';self.variables=out/f'{self.name}.fd'
        subprocess.run(['python3',str(SRC/'os/tools/make-data.py'),'--output',str(self.disk),'--config',str(config)],check=True,stdout=subprocess.DEVNULL)
        shutil.copyfile(art/'OVMF_VARS-trusted.fd',self.variables)
    def start(self):
        self.child=pexpect.spawn('qemu-system-x86_64',[
            '-machine','q35,smm=on','-accel','kvm','-cpu','host,vmx=off,svm=off','-m','4096','-smp','1',
            '-global','driver=cfi.pflash01,property=secure,value=on',
            '-drive',f'if=pflash,format=raw,unit=0,readonly=on,file={art/"OVMF_CODE_4M.secboot.fd"}',
            '-drive',f'if=pflash,format=raw,unit=1,file={self.variables}',
            '-drive',f'if=virtio,format=raw,readonly=on,file={flash}',
            '-drive',f'if=virtio,format=raw,file={self.disk}',
            '-netdev',f'socket,id=lan,mcast=230.77.42.1:{port},localaddr=127.0.0.1',
            '-device',f'virtio-net-pci,netdev=lan,mac=52:54:00:77:42:{self.index:02x}',
            '-object',f'filter-dump,id=capture,netdev=lan,file={out/(self.name+".pcap")}',
            '-display','none','-chardev','stdio,id=serial0,signal=off','-serial','chardev:serial0','-monitor','none','-no-reboot'],
            encoding='utf-8',codec_errors='replace',timeout=240)
        self.child.logfile_read=self.log
    def ready(self):
        state=self.child.expect(['%FCR-5-SYSTEM_READY','%FCR-3-SYSTEM_DEGRADED'],timeout=300)
        assert state==0,f'{self.name} degraded'
        self.child.expect_exact('Press RETURN to get started!');self.child.sendline('');self.prompt()
        self.command('terminal length 0')
        self.command('terminal width 512')
    def prompt(self): self.child.expect_exact(self.name+'# ')
    def command(self,text,timeout=120,allow_error=False):
        self.child.sendline(text);self.child.expect_exact(self.name+'# ',timeout=timeout)
        output=self.child.before
        if not allow_error:
            assert not re.search(r'% (?:Client exited|Invalid input|Incomplete command|Authorization failed)',output),output
        return output
    def docker(self,text,**kw):return self.command('request container-engine '+text,**kw)
    def stop(self):stop_qemu(self.child);self.child=None

def wait_for(test,message,timeout=120):
    end=time.monotonic()+timeout
    while time.monotonic()<end:
        if test():return
        time.sleep(2)
    raise AssertionError(message)

nodes=[]
try:
    for i in range(1,4):nodes.append(Node(i))
    for n in nodes:n.start()
    for n in nodes:
        n.ready()
        n.docker('import /usr/share/fractal/probe.tar fcr/probe:latest')
    check('three signed appliance VMs boot on an isolated loopback multicast LAN')
    manager=nodes[0]
    manager.docker(f'swarm init --advertise-addr {manager.ip}')
    token=re.search(r'SWMTKN-[A-Za-z0-9-]+',manager.docker('swarm join-token -q manager')).group(0)
    for n in nodes[1:]:n.docker(f'swarm join --token {token} --advertise-addr {n.ip} {manager.ip}:2377')
    def healthy():
        output=manager.docker("node ls --format '{{.Hostname}} {{.Status}} {{.ManagerStatus}}'",allow_error=True)
        return all(re.search(n.name+r'\s+Ready\s+(Leader|Reachable)',output) for n in nodes)
    wait_for(healthy,'manager quorum did not form')
    check('three-manager Raft quorum formed with mutually authenticated node membership')
    manager.docker('secret create qualification-secret /etc/fractal/release')
    manager.docker('config create qualification-config /etc/fractal/release')
    manager.docker('service create --detach --no-resolve-image --name material-witness --mode global-job --secret source=qualification-secret,target=qualification-secret,mode=0400 --config source=qualification-config,target=/qualification-config,mode=0444 fcr/probe:latest /probe swarm-material')
    def material_delivered():
        tasks=manager.docker("service ps --format '{{.Node}} {{.CurrentState}}' material-witness")
        return all(re.search(n.name+r'\s+Complete',tasks) for n in nodes)
    wait_for(material_delivered,'secret/config global jobs did not complete on every node')
    assert manager.docker('service logs --raw material-witness').count('SWARM_SECRET_CONFIG_OK')==3
    manager.docker('service rm material-witness')
    manager.docker('secret rm qualification-secret');manager.docker('config rm qualification-config')
    check('global jobs receive Swarm secrets and configs on every node with the requested read-only permissions')
    manager.docker('network create --driver overlay --attachable --opt encrypted qualified')
    manager.docker('service create --detach --no-resolve-image --name witness --network qualified --constraint node.hostname==swarm2 --publish published=18080,target=8080 fcr/probe:latest /probe serve')
    wait_for(lambda:'Running' in manager.docker("service ps --filter desired-state=running --format '{{.CurrentState}}' witness"),'service did not start')
    assert 'CONTAINER_PORT_PUBLISH_OK' in nodes[0].docker('run --rm --network qualified fcr/probe:latest /probe request witness 8080')
    check('cross-node service discovery and HTTP succeeded over the encrypted overlay')
    # Record encrypted traffic before exercising the intentionally plaintext ingress port.
    time.sleep(1)
    esp=0
    result['overlay_captures']={}
    for n in nodes:
        capture=(out/(n.name+'.pcap')).read_bytes();offset=24
        saved=out/('overlay-only-'+n.name+'.pcap')
        saved.write_bytes(capture)
        result['overlay_captures'][saved.name]=hashlib.sha256(capture).hexdigest()
        endian='<' if capture[:4]==b'\xd4\xc3\xb2\xa1' else '>'
        while offset+16<=len(capture):
            length=struct.unpack_from(endian+'I',capture,offset+8)[0];offset+=16
            frame=capture[offset:offset+length];offset+=length
            if len(frame)>=34 and frame[12:14]==b'\x08\x00' and frame[23]==50:esp+=1
            assert b'FCR_NETWORK_OK' not in frame,'overlay application payload appeared in plaintext on the VM LAN'
    assert esp>0,'no ESP traffic observed'
    result['esp_packets']=esp
    check('packet captures contain IPsec ESP and no plaintext application response during the overlay test')
    for n in nodes:
        assert 'CONTAINER_PORT_PUBLISH_OK' in n.docker(f'run --rm --network host fcr/probe:latest /probe request {n.ip} 18080')
    check('routing mesh serves the published port through every node, including nodes without a task')
    manager.docker('service update --detach --constraint-rm node.hostname==swarm2 --replicas 3 witness')
    wait_for(lambda:'3/3' in manager.docker("service ls --format '{{.Name}} {{.Replicas}}'"),'replicas failed to scale')
    manager.docker('node update --availability drain swarm2')
    def drained():
        tasks=manager.docker("service ps --filter desired-state=running --format '{{.Node}} {{.CurrentState}}' witness")
        return len(re.findall(r'swarm[13]\s+Running',tasks))==3 and not re.search(r'swarm2\s+Running',tasks)
    wait_for(drained,'drain failed to move all three running tasks off swarm2')
    manager.docker('node update --availability active swarm2')
    check('scaling and drain reschedule running tasks onto available nodes')
    listing=manager.docker("node ls --format '{{.Hostname}} {{.ManagerStatus}}'")
    leader=next(n for n in nodes if re.search(n.name+r'\s+Leader',listing))
    leader.stop();manager=next(n for n in nodes if n is not leader)
    wait_for(lambda:'Leader' in manager.docker("node ls --format '{{.ManagerStatus}}'",allow_error=True),'no leader elected')
    manager.docker('service scale --detach witness=2')
    def failover_tasks():
        tasks=manager.docker("service ps --filter desired-state=running --format '{{.Node}} {{.CurrentState}}' witness")
        return sum(len(re.findall(n.name+r'\s+Running',tasks)) for n in nodes if n is not leader)==2 and not re.search(leader.name+r'\s+Running',tasks)
    # service ls includes last-reported Running states of orphaned tasks on a
    # powered-off host. Assert current desired placements and live traffic.
    wait_for(failover_tasks,'leader loss blocked replacement scheduling')
    for n in nodes:
        if n is not leader:
            assert 'CONTAINER_PORT_PUBLISH_OK' in n.docker(f'run --rm --network host fcr/probe:latest /probe request {n.ip} 18080')
    check('abrupt leader VM loss preserves quorum and allows a new scheduling change')
    leader.start();leader.ready()
    wait_for(healthy,'restarted manager did not rejoin')
    check('manager restarts from its persistent datastore and rejoins the quorum')
    # The service version is changed and then rolled back through Raft.
    manager.docker('service update --detach --env-add QUALIFIED=1 witness')
    wait_for(lambda:re.search(r'"State":\s*"completed"',manager.docker('service inspect witness')),'update did not converge')
    assert 'QUALIFIED=1' in manager.docker("service inspect --format '{{json .Spec.TaskTemplate.ContainerSpec.Env}}' witness")
    manager.docker('service rollback --detach witness')
    wait_for(lambda:re.search(r'"State":\s*"rollback_completed"',manager.docker('service inspect witness')),'rollback did not converge')
    assert 'QUALIFIED=1' not in manager.docker("service inspect --format '{{json .Spec.TaskTemplate.ContainerSpec.Env}}' witness")
    check('service update and rollback converge across the cluster')
    manager.docker(f'service create --detach --no-resolve-image --name quorum-witness --constraint node.hostname=={manager.name} --publish published=18081,target=8080 fcr/probe:latest /probe serve')
    wait_for(lambda:'Running' in manager.docker("service ps --filter desired-state=running --format '{{.CurrentState}}' quorum-witness"),'quorum witness did not start')
    peers=[n for n in nodes if n is not manager]
    for n in peers:n.stop()
    time.sleep(8)
    denied=manager.docker('service scale --detach quorum-witness=2',timeout=120,allow_error=True)
    assert re.search(r'leader|quorum|deadline|context|unavailable',denied,re.I),denied
    assert 'CONTAINER_PORT_PUBLISH_OK' in manager.docker(f'run --rm --network host fcr/probe:latest /probe request {manager.ip} 18081')
    check('loss of manager quorum refuses scheduling changes while an existing task continues serving traffic')
    for n in peers:n.start()
    for n in peers:n.ready()
    wait_for(healthy,'quorum did not recover after two managers restarted')
    manager.docker('service scale --detach quorum-witness=2')
    wait_for(lambda:'2/2' in manager.docker("service ls --filter name=quorum-witness --format '{{.Replicas}}'"),'restored quorum cannot schedule')
    manager.docker('service rm quorum-witness')
    check('returning managers restore quorum and scheduling without forced cluster reconstruction')
    manager.docker('service rm witness');time.sleep(5)
    manager.docker('network rm qualified')
    # Keep each forced leave bounded, with peers still running during its request.
    for n in nodes:n.docker('swarm leave --force',timeout=120)
    check('all three engines leave the cluster without exceeding their shutdown deadline')
    result['passed']=True
except Exception as error:
    result['error']=str(error)
    for n in nodes:
        if n.child is not None and n.child.isalive():
            try:
                for text in ['service ps --no-trunc witness','node ls']:
                    n.docker(text,timeout=20,allow_error=True)
            except Exception:pass
    raise
finally:
    for n in nodes:
        try:n.stop()
        finally:n.log.close()
    (out/'result-os-swarm-multinode.json').write_text(json.dumps(result,indent=2)+'\n')
