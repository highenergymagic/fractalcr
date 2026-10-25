#!/usr/bin/env python3
"""Boot the actual signed OE OS and exercise Docker and failure behavior."""
import argparse
import fcntl
import hashlib
import json
import re
from pathlib import Path
import shutil
import subprocess
import sys
import time
from quiet_console import assert_quiet_boot
import pexpect
from ios_prompts import reload_appliance
sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "boot" / "tests"))
from qemu_process import finish_qemu, stop_qemu

p = argparse.ArgumentParser()
p.add_argument("--case", choices=["containers", "no-datastore", "invalid-config", "optional-firmware", "blank-datastore", "debug-container", "cli", "container-tools", "kata", "kata-factory", "time-sync"], required=True)
p.add_argument("--artifacts", type=Path, default=Path("/work/artifacts"))
p.add_argument("--accel", choices=["tcg","kvm"], default="tcg")
a = p.parse_args()
art = a.artifacts.resolve()
src = Path(__file__).resolve().parents[2]
case = a.case
# Authenticating the expanded embedded image in the emulated kernel can take
# several minutes. Keep command/runtime deadlines separate from boot latency.
boot_timeout = 600 if a.accel == "tcg" else 240
# Hold this before creating fixtures: a second run must never truncate disks
# still attached to the first VM, even if QEMU later rejects its own file lock.
case_lock = (art / f".os-{case}.lock").open("a")
try:
    fcntl.flock(case_lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
except BlockingIOError:
    p.error(f"scenario {case} is already using {art}")
flash = art / f"flash-os-{case}.img"
disk = art / f"data-os-{case}.img"
mirror = art / f"data-os-{case}-mirror.img"
variables = art / f"OVMF_VARS-os-{case}.fd"
subprocess.run(["python3", str(src / "boot/tools/make-flash.py"), "--efi", str(art / "rommon.efi"),
                "--system", str(art / "system.bin"), "--output", str(flash), "--size-mib", "1024", "--no-tampered-fixture"], check=True)
if case == "blank-datastore":
    for target in (disk, mirror):
        with target.open("wb") as f:
            f.truncate(512 * 1024 * 1024)
elif case != "no-datastore":
    make = ["python3", str(src / "os/tools/make-data.py"), "--output", str(disk)]
    if case in ("container-tools","kata","kata-factory"): make += ["--size-mib","16384" if case=="kata-factory" else "8192","--tool-fixtures"]
    if case == "invalid-config":
        make += ["--invalid-config"]
    if case == "time-sync":
        startup=art/'time-sync-startup.json'
        startup.write_text(json.dumps({'version':1,'hostname':'ctr-test','network':{'interface':'eth0','address':'10.0.2.15/24','gateway':'10.0.2.2','dns':'10.0.2.3'},'ntp':{'servers':['10.0.2.2']}}))
        make += ['--config',str(startup)]
    subprocess.run(make, check=True)
shutil.copy(art / ("OVMF_VARS-unenrolled.fd" if case == "optional-firmware" else "OVMF_VARS-trusted.fd"), variables)
result = {
    "case": case, "passed": False, "checks": [], "accel": a.accel,
    "rommon_sha256": hashlib.sha256((art / "rommon.efi").read_bytes()).hexdigest(),
    "system_sha256": hashlib.sha256((art / "system.bin").read_bytes()).hexdigest(),
}
log = (art / f"qemu-os-{case}.log").open("w")
child = None
recover_startup = False

def spawn():
    args = [
        # Match the OS's core2-64 baseline, without VMX. The max model can
        # expose emulated SVM and cannot test the unavailable-KVM boundary.
        "-machine", "q35,smm=on", "-accel", a.accel, "-cpu",
        ("host" if case in ("kata","kata-factory") else "host,vmx=off,svm=off") if a.accel=="kvm" else "core2duo,vmx=off",
        "-m", "4096", "-smp", "2",
        "-global", "driver=cfi.pflash01,property=secure,value=on",
        "-drive", f"if=pflash,format=raw,unit=0,readonly=on,file={art / 'OVMF_CODE_4M.secboot.fd'}",
        "-drive", f"if=pflash,format=raw,unit=1,file={variables}",
        "-drive", f"if=none,id=flash,format=raw,file={flash}",
        "-device", "virtio-blk-pci,drive=flash,bootindex=0",
        "-netdev", "user,id=net0", "-device", "virtio-net-pci,netdev=net0",
        "-display", "none", "-chardev", "stdio,id=serial0,signal=off",
        "-serial", "chardev:serial0", "-monitor", "none", "-no-reboot",
    ]
    if case != "no-datastore":
        args += ["-drive", f"if=virtio,format=raw,file={disk}"]
    if case == "blank-datastore":
        args += ["-drive", f"if=virtio,format=raw,file={mirror}"]
    process = pexpect.spawn("qemu-system-x86_64", args, encoding="utf-8",
                            codec_errors="replace", timeout=240)
    process.logfile_read = log
    if case == "optional-firmware" or recover_startup:
        process.expect("Autoboot in 5 seconds")
        process.sendcontrol("c"); process.expect(r"rommon \d+ > ")
        process.sendline("confreg 0x2142" if recover_startup else "SECURE_BOOT=optional"); process.expect(r"rommon \d+ > ")
        if not recover_startup:
            process.sendline("sync"); process.expect(r"rommon \d+ > ")
        process.sendline("boot")
    process.expect("Image signature verified.")
    return process

def check(message):
    result["checks"].append(message)
    print("PASS " + case + ": " + message, flush=True)

def console_login():
    child.expect_exact("Press RETURN to get started!")
    log.flush()
    assert_quiet_boot((art / f"qemu-os-{case}.log").read_text())
    child.sendline("")
    if child.expect([r"ctr(?:-test)?# ", r"Enter initial configuration dialog\? \[yes/no\]: "]) == 1:
        child.sendline("no")
        child.expect(r"ctr(?:-test)?# ")
    child.sendline("terminal length 0")
    child.expect(r"ctr(?:-test)?# ")

def prompt():
    child.expect(r"ctr(?:-test)?# ")

def ready():
    state = child.expect(["%FCR-5-SYSTEM_READY", "%FCR-3-SYSTEM_DEGRADED", "Kernel panic"], timeout=boot_timeout)
    console_login()
    if state != 0:
        for command in ("show logging platform", "show logging container-engine"):
            child.sendline(command); prompt()
        raise AssertionError("OS did not reach ready state; see serial log")
    child.sendline("show platform")
    child.expect_exact("Datastore data0: online")
    child.expect_exact("System image: read-only")
    child.expect_exact("Container service: running")
    prompt()

def containers(counter):
    child.sendline("request platform diagnostics containers")
    state = child.expect(["%FCR-5-DIAGNOSTIC_PASS", "%FCR-3-DIAGNOSTIC_FAILED"])
    prompt()
    child.sendline("show logging diagnostics")
    if state != 0:
        prompt()
        raise AssertionError("Container diagnostics failed; see serial log")
    child.expect_exact("HOST_ROOT_READONLY_OK")
    child.expect_exact("CONTAINER_ISOLATION_OK")
    child.expect_exact(f"PERSISTENT_COUNTER={counter}")
    child.expect_exact("CONTAINER_BRIDGE_OK")
    child.expect_exact("CONTAINER_PORT_PUBLISH_OK")
    prompt()
    output=execute("request container-engine ps all enable filter label=com.fractalcr.diagnostics=true format '{{.ID}}'")
    assert not re.search(r"(?m)^[0-9a-f]{12,64}\r?$", output), output
    check("diagnostic completion removes every owned probe container")

def management():
    child.sendline("show platform software service")
    child.expect(r"platform\s+running")
    child.expect(r"logging\s+running")
    child.expect(r"datastore\s+running")
    child.expect(r"configuration\s+running")
    child.expect(r"networking\s+running")
    child.expect(r"container-engine\s+running")
    child.expect(r"console\s+running"); prompt()
    check("platform capabilities run as individually supervised processes")
    child.sendline("request platform software service container-engine restart")
    child.expect_exact("%FCR-5-SERVICE_RESTART")
    # A partial command must not block the supervisor's readiness/health loop.
    prompt()
    child.send("show plat")
    child.expect_exact("%FCR-5-SERVICE_READY")
    child.expect_exact("ctr-test# show plat")
    child.sendline("form")
    child.expect_exact("Container service: running"); prompt()
    child.sendline("request platform software service container-engine stop")
    child.expect_exact("%FCR-5-SERVICE_STOP"); prompt()
    time.sleep(2)
    child.sendline("show platform")
    child.expect_exact("Container service: stopped"); prompt()
    child.sendline("request platform software service container-engine start")
    child.expect_exact("%FCR-5-SERVICE_READY"); prompt()
    check("service restart/stop/start and partial-line console input preserve supervision")
    child.sendline("request platform software service logging restart")
    child.expect_exact("logging: Restart requested.")
    child.expect_exact("logging: Ready."); prompt()
    child.sendline("request platform software service datastore restart")
    child.expect_exact("datastore: Restart requested.")
    child.expect_exact("container-engine: Stopped.")
    child.expect_exact("networking: Stopped.")
    child.expect_exact("configuration: Stopped.")
    child.expect_exact("datastore: Stopped.")
    child.expect_exact("datastore: Ready.")
    child.expect_exact("configuration: Ready.")
    child.expect_exact("networking: Ready.")
    child.expect_exact("container-engine: Ready."); prompt()
    check("datastore restart drains and restores dependents in order")
    child.sendline("request platform software service platform restart")
    child.expect_exact("platform: Restart requested.")
    child.expect_exact("platform: Stopped.")
    child.expect_exact("platform: Ready.")
    child.expect_exact("container-engine: Ready."); prompt()
    check("platform restart preserves runtime mounts and restores the service graph")
    child.sendline("show platform software service")
    child.expect(r"container-engine\s+running\s+(\d+)\s+(\d+)")
    engine_identity = child.match.groups(); prompt()
    child.sendline("request platform software service console restart")
    child.expect_exact("Press RETURN to get started!")
    child.sendline(""); prompt()
    child.sendline("terminal length 0"); prompt()
    child.sendline("show platform")
    child.expect_exact("Container service: running"); prompt()
    child.sendline("show platform software service")
    child.expect(r"container-engine\s+running\s+(\d+)\s+(\d+)")
    assert child.match.groups() == engine_identity, "Console restart changed container-engine PID/start count"
    prompt()
    check("console restart reconnects without restarting the container engine")


def configuration_cli():
    def command(text, expected=None, mode=""):
        child.sendline(text)
        if expected:
            child.expect_exact(expected)
        child.expect_exact("ctr-test" + (f"(config{mode})" if mode != "exec" else "") + "# ")
    command("configure candidate", mode="")
    command("hostname temporary", mode="")
    command("do show candidate-config", 'hostname temporary', mode="")
    command("abort", "Candidate discarded.", mode="exec")
    command("configure candidate", mode="")
    command("interface eth0", mode="-if")
    command("ip address invalid", mode="-if")
    command("commit", "% address must be IPv4/prefix", mode="-if")
    command("ip address 10.0.2.16/24", mode="-if")
    command("commit", "Running configuration committed.", mode="-if")
    command("do show interfaces", "Internet address is 10.0.2.16", mode="-if")
    command("do show running-config", "ip address 10.0.2.16 255.255.255.0", mode="-if")
    command("ip address 10.0.2.15/24", mode="-if")
    command("commit", "Running configuration committed.", mode="-if")
    command("ip address 10.0.2.17/24", mode="-if")
    command("ip gateway 192.0.2.1", mode="-if")
    command("commit", "% apply failed; previous configuration restored:", mode="-if")
    command("abort", mode="exec")
    command("show interfaces", mode="exec")
    assert "Internet address is 10.0.2.15" in child.before and "Internet address is 10.0.2.17" not in child.before
    command("show running-config", "ip address 10.0.2.15 255.255.255.0", mode="exec")
    check("staging, abort, validation, live network apply and failed-apply rollback work")
    # All image/daemon information comes through the appliance command surface.
    command("show images", mode="exec")
    image_match = __import__("re").search(r"fractal/probe(?::|\s+)([0-9a-f]{64})", child.before)
    assert image_match, "offline image tag missing"
    tag = image_match.group(1)
    command("configure candidate", mode="")
    command("container cli-probe", mode="-container")
    command("image fractal/probe:" + tag, mode="-container")
    command("volume cli-state /state", mode="-container")
    command("memory 32M", mode="-container")
    command("pids-limit 32", mode="-container")
    command("read-only", mode="-container")
    command("security no-new-privileges", mode="-container")
    command("capability drop all", mode="-container")
    command('environment NOTE "literal $(not-a-shell)"', mode="-container")
    command("no container-isolation", mode="-container")
    command("commit", "Running configuration committed.", mode="-container")
    command("end", mode="exec")
    command("request container cli-probe create", mode="exec")
    command("request container cli-probe start", "cli-probe", mode="exec")
    time.sleep(2)
    command("show container cli-probe logs", "PERSISTENT_COUNTER=1", mode="exec")
    command("request container cli-probe remove", "cli-probe", mode="exec")
    command("write memory", "[OK]", mode="exec")
    command("request platform software service management restart", mode="exec")
    child.expect_exact("management: Ready."); prompt()
    command("terminal length 0", mode="exec")
    command("show running-config", 'container cli-probe', mode="exec")
    child.send("show runn\t")
    child.expect_exact("show running-config ")
    child.sendline(""); child.expect_exact('container cli-probe'); prompt()
    child.send("show versin\x1b[Do\n")
    child.expect_exact("FractalCR Software, Version"); prompt()
    child.send("\x1b[A\n")
    child.expect_exact("FractalCR Software, Version"); prompt()
    check("container create/start/logs/remove, persistence, management restart and tab completion work")
    command("show container-isolation capabilities", "Hardware virtualization: unavailable", mode="exec")
    command("configure candidate", mode="")
    command("system container-isolation install runtime kata-containers", mode="")
    command("commit", "Running configuration committed.", mode="")
    command("end", mode="exec")
    command("show container-isolation detail", "Kata Containers: installed", mode="exec")
    command("configure candidate", mode="")
    command("system container-isolation enable", mode="")
    command("commit", "% KVM is unavailable", mode="")
    command("abort", mode="exec")
    check("Kata installation is reported through native status and absent KVM rejects enable")


def execute(command, expected=None, timeout=90, expected_error=False):
    child.sendline(command)
    # Asynchronous service events redraw the current prompt. Synchronize on
    # the submitted command's echoed tail before accepting its response prompt.
    child.expect(re.escape(command[-24:])+r"\r+\n",timeout=timeout)
    child.expect_exact("ctr-test# ", timeout=timeout)
    output=child.before
    if expected: assert expected in output, (command, output)
    assert "% Client terminated" not in output, (command,output)
    if not expected_error:
        assert "% Client exited" not in output, (command,output)
    return output


def wait_service(name,state="running"):
    deadline=time.monotonic()+90
    while time.monotonic()<deadline:
        output=execute("show platform software service")
        if re.search(r"^"+re.escape(name)+r"\s+"+state+r"\b",output.replace("\r",""),re.M): return
        time.sleep(0.25)
    raise AssertionError("Service did not reach "+state+": "+name)


def cli_semantics():
    execute("sh ver","FractalCR Software, Version")
    execute("SH VER","FractalCR Software, Version")
    child.send("sh?")
    child.expect_exact("ctr-test# sh")
    assert "show" in child.before and "version" not in child.before, child.before
    child.sendline(" ver"); child.expect_exact("FractalCR Software, Version"); prompt()
    child.send("show ?")
    child.expect_exact("ctr-test# show ")
    assert "version" in child.before and "software" not in child.before and "Use ?" not in child.before, child.before
    child.sendline("ver"); child.expect_exact("FractalCR Software, Version"); prompt()
    child.send("show plat ?"); child.expect_exact("ctr-test# show plat ")
    assert "software" in child.before and "<cr>" in child.before and "service " not in child.before, child.before
    child.sendcontrol("c"); prompt()
    execute("sh c",'% Ambiguous command: "sh c"')
    execute("sh","% Incomplete command.")
    output=execute("sh nonexistent","% Invalid input detected at '^' marker.")
    assert "^\n" in output.replace("\r",""), output
    child.send("sh ve\t"); child.expect_exact("sh version ")
    child.sendline(""); child.expect_exact("FractalCR Software, Version"); prompt()
    child.send("show versin\x1b[Do\n"); child.expect_exact("FractalCR Software, Version"); prompt()
    child.send("\x1b[A\n"); child.expect_exact("FractalCR Software, Version"); prompt()
    child.sendline("conf t"); child.expect_exact("ctr-test(config)# ")
    child.sendline("do sh ver"); child.expect_exact("FractalCR Software, Version"); child.expect_exact("ctr-test(config)# ")
    child.send("container literal\x16?\n"); child.expect_exact("% invalid container name"); child.expect_exact("ctr-test(config)# ")
    child.sendcontrol("z"); prompt()
    check("abbreviations, immediate one-level help, preserved input, literal ?, cursor/history, diagnostics and do/Ctrl-Z")
    execute("terminal length 5")
    child.sendline("show logging kernel"); child.expect_exact(" --More-- ")
    child.send("\n"); child.expect_exact(" --More-- ")
    child.send("q"); prompt()
    execute("terminal length 0")
    check("pager advances one line on Return and exits on q without consuming the next command")
    execute("req plat soft serv management stop")
    wait_service("management","stopped")
    execute("sh ver","FractalCR Software, Version")
    child.send("sh?"); child.expect_exact("ctr-test# sh")
    assert "show" in child.before, child.before
    child.sendcontrol("c"); prompt()
    execute("req plat soft serv management start")
    wait_service("management")
    execute("sh run",'hostname ctr-test')
    check("local grammar and help remain usable while management is stopped, and service recovers")


def container_tools():
    prefix="request container-engine "
    execute(prefix+"version", "29.9.2")
    execute(prefix+"compose version", "5.6.1")
    execute(prefix+"buildx version", "0.38.1")
    execute("request container-build version enable", "0.34.1")
    execute(prefix+"import /usr/share/fractal/probe.tar fcr/probe:latest", "sha256:")
    execute(prefix+"create name seed arguments fcr/probe:latest /probe serve")
    execute(prefix+"cp seed:/probe data0:probe")
    execute(prefix+"rm seed", "seed")
    execute(prefix+"buildx build progress plain load enable tag fcr/built:latest file data0:Dockerfile data0:", "CONTAINER_BRIDGE_OK",180)
    execute(prefix+"run rm enable arguments fcr/built:latest network", "CONTAINER_BRIDGE_OK")
    check("pinned native clients work and Buildx builds/loads/runs an image with an executed Dockerfile RUN step")
    execute('request buildx history inspect')
    execute('request buildx history open', 'docker-desktop://')
    check('Buildx history inspection and browser-link output work without a host browser helper')
    execute('request context create cli-export docker host=unix:///run/docker.sock', 'cli-export')
    execute('request context export cli-export data0:qualification.context')
    execute('dir data0:', 'qualification.context')
    execute('request context import cli-import data0:qualification.context')
    execute("request context inspect cli-import format '{{.Endpoints.docker.Host}}'", 'unix:///run/docker.sock')
    execute('request context rm cli-import cli-export')
    check('Context export and import use the appliance file namespace')
    execute(prefix+"compose file data0:compose.yaml project-name qualification up detach enable", timeout=180)
    execute(prefix+"run rm enable network host arguments fcr/probe:latest /probe request", "CONTAINER_PORT_PUBLISH_OK")
    execute(prefix+"compose file data0:compose.yaml project-name qualification down volumes enable")
    check("Compose creates a project, bridge network and published service, then removes them")
    execute("request platform software service buildkit start")
    wait_service("buildkit")
    execute("request container-build debug workers addr unix:///run/buildkit/buildkitd.sock timeout 10", "linux/amd64")
    execute("request container-build build frontend dockerfile.v0 local context=data0: local dockerfile=data0: output type=oci,dest=data0:buildkit.oci.tar", "CONTAINER_BRIDGE_OK",180)
    execute("request platform software service buildkit restart")
    wait_service("buildkit")
    execute("request container-build debug workers", "linux/amd64")
    check("standalone BuildKit executes an isolated build, exports OCI and survives supervised restart")
    execute(prefix+"swarm init advertise-addr 10.0.2.15", "Swarm initialized")
    execute(prefix+"network create driver overlay attachable enable opt encrypted qualification")
    execute(prefix+"service create detach enable no-resolve-image enable name qualification network qualification publish published=18080,target=8080 arguments fcr/probe:latest /probe serve")
    for attempt in range(60):
        state=execute(prefix+"service ps no-trunc enable format '{{.CurrentState}} {{.Error}}' qualification")
        if "Running " in state: break
        assert "Rejected " not in state and "Failed " not in state, state
        time.sleep(1)
    else: raise AssertionError("Swarm service did not converge")
    execute(prefix+"run rm enable network host arguments fcr/probe:latest /probe request", "CONTAINER_PORT_PUBLISH_OK")
    execute(prefix+"service rm qualification")
    for _ in range(60):
        tasks=execute(prefix+"ps all enable filter label=com.docker.swarm.service.name=qualification format '{{.ID}}'")
        if not re.search(r'^[0-9a-f]{12,64}$',tasks.replace('\r',''),re.M): break
        time.sleep(2)
    else: raise AssertionError('removed service still has task containers')
    execute(prefix+"network rm qualification")
    execute(prefix+"swarm leave force enable", "Node left the swarm")
    check("single-node Swarm schedules a service on encrypted overlay configuration and serves traffic through ingress")
    child.sendline(prefix+"events")
    time.sleep(1); child.sendcontrol("c"); prompt()
    execute("sh ver","FractalCR Software, Version")
    check("Ctrl-C stops a streaming native client and returns to the appliance CLI")


def factory_persistence():
    global child
    samples=[]
    def witness(command, count):
        before=time.time()
        output=execute(command, "FACTORY_STATE="+str(count),180)
        after=time.time()
        wall=float(re.search(r"WALL_CLOCK=(\d+\.\d+)",output).group(1))
        assert before-5 <= wall <= after+5,(before,wall,after)
        samples.append(re.search(r"FACTORY_RANDOM=([0-9a-f]{64})",output).group(1))
    witness("request container-engine run name factory-persistent arguments fcr/probe:latest /probe factory-state",1)
    time.sleep(15)
    witness("request container-engine start attach enable factory-persistent",2)
    witness("request container-engine run rm enable arguments fcr/probe:latest /probe factory-state",1)
    assert len(set(samples))==3,samples
    check("CLH template restores refresh wall time and randomness; container root writes persist without leaking into a new container")
    child.sendline("request platform software service vm-factory restart")
    child.expect_exact("container-engine: Stopped.", timeout=120)
    child.expect_exact("vm-factory: Ready.", timeout=180)
    child.expect_exact("container-engine: Ready.", timeout=120); prompt()
    witness("request container-engine start attach enable factory-persistent",3)
    check("factory service restart drains Engine, rebuilds the template, and preserves container data")
    execute("request container-engine volume create factory-volume","factory-volume")
    volume_command="request container-engine run rm enable mount type=volume,src=factory-volume,dst=/state arguments fcr/probe:latest /probe factory-state /state/value"
    witness(volume_command,1)
    witness(volume_command,2)
    execute("request container-engine volume rm factory-volume","factory-volume")
    check("Kata containers with persistent volumes retain shared writes through the cold-start path")
    execute("write memory","[OK]")
    reload_appliance(child); child.expect(pexpect.EOF); stop_qemu(child)
    child=spawn(); ready()
    execute("request system container-isolation kata-containers vm-factory create","CLH VM factory ready")
    witness("request container-engine start attach enable factory-persistent",4)
    execute("request container-engine rm factory-persistent","factory-persistent")
    check("saved block storage and factory configuration rebuild the template after reboot and preserve container root data")


def kata_nested():
    execute("show container-isolation capabilities","Hardware virtualization: available")
    if case=="kata-factory":
        for command in ("conf t","system container-engine","storage backend blockfile","end"):
            child.sendline(command); child.expect(r"ctr-test(?:\(config(?:-engine)?\))?# ",timeout=180)
            assert "% " not in child.before,child.before
        execute("request container-engine info format '{{.Driver}}'","blockfile")
    execute("request container-engine import /usr/share/fractal/probe.tar fcr/probe:latest","sha256:")
    child.sendline("conf t"); child.expect_exact("ctr-test(config)# ")
    for command in ("system container-isolation install runtime kata-containers",
                    "system container-isolation enable",
                    "system container-isolation kata-containers apply-to all-containers",
                    "system container-isolation kata-containers runtime clh default-memory 256M",
                    "commit"):
        child.sendline(command); child.expect_exact("ctr-test(config)# ",timeout=120)
        assert "% " not in child.before, child.before
    child.sendline("end"); prompt()
    if case=="kata-factory":
        for command in ("conf t","system container-isolation kata-containers runtime clh vm-factory enable","end"):
            child.sendline(command); child.expect(r"ctr-test(?:\(config\))?# ",timeout=180)
            assert "% " not in child.before,child.before
        execute("request system container-isolation kata-containers vm-factory create","CLH VM factory ready")
        factory_persistence()
    execute("request container-engine info format '{{.DefaultRuntime}}'","kata")
    execute("request container-engine run rm enable arguments fcr/probe:latest /probe network-wait","CONTAINER_BRIDGE_OK",180)
    execute("request container-engine run rm enable runtime runc arguments fcr/probe:latest /probe network","CONTAINER_BRIDGE_OK")
    execute("request container-engine run detach enable name kata-web publish 18080:8080 arguments fcr/probe:latest /probe serve",timeout=180)
    if case == "kata-factory":
        execute("request container-engine cp kata-web:/factory-state /tmp/factory-copy", "stop container kata-web", expected_error=True)
        execute("request container-engine commit kata-web fcr/unsafe-live-copy", "stop container kata-web", expected_error=True)
        execute("show container-isolation detail", "VM factory")
        check("live block-backed Kata filesystem operations are rejected before reaching Engine archive or commit APIs")
    execute("request container-engine run rm enable runtime runc network host arguments fcr/probe:latest /probe request","CONTAINER_PORT_PUBLISH_OK")
    execute("request container-engine rm force enable kata-web","kata-web")
    check("Kata guest receives a bridge address and serves traffic through a published Docker port")
    execute("request container-engine run rm enable arguments fcr/probe:latest /probe identity","KERNEL=6.18.",180)
    execute("request container-engine run rm enable runtime runc arguments fcr/probe:latest /probe identity","KERNEL=6.12.")
    clocks = []
    for delay in (0, 15):
        time.sleep(delay)
        before = time.time()
        output = execute("request container-engine run rm enable arguments fcr/probe:latest /probe clock", "WALL_CLOCK=", 180)
        after = time.time()
        value = float(re.search(r"WALL_CLOCK=(\d+\.\d+)", output).group(1))
        assert before-5 <= value <= after+5, (before, value, after)
        clocks.append(value)
    assert clocks[1] - clocks[0] >= 10, clocks
    check("separately launched Kata guests report current wall time after a 15-second launch gap")
    child.sendline("conf t"); child.expect_exact("ctr-test(config)# ")
    for command in ("container native-override","image fcr/probe:latest","command /probe identity","no container-isolation","commit"):
        child.sendline(command); child.expect(r"ctr-test\(config(?:-container)?\)# ",timeout=120)
        assert "% " not in child.before, child.before
    child.sendline("end"); prompt()
    execute("request container native-override create")
    execute("request container native-override start")
    execute("show container native-override logs","KERNEL=6.12.")
    execute("request container native-override remove")
    child.sendline("conf t"); child.expect_exact("ctr-test(config)# ")
    for command in ("system container-isolation kata-containers apply-to explicit","container isolated",
                    "image fcr/probe:latest","command /probe identity","container-isolation kata-containers enable","commit"):
        child.sendline(command); child.expect(r"ctr-test\(config(?:-container)?\)# ",timeout=120)
        assert "% " not in child.before, child.before
    child.sendline("end"); prompt()
    execute("request container-engine info format '{{.DefaultRuntime}}'","runc")
    execute("request container isolated create")
    execute("request container isolated start",timeout=180)
    execute("show container isolated logs","KERNEL=6.18.")
    execute("request container isolated remove")
    check("nested KVM launches the 256M Kata/Cloud Hypervisor guest as Docker default and permits explicit runc override")
    check("guest/host kernel witnesses confirm per-container no container-isolation and explicit Kata selection")

def time_sync():
    for attempt in range(45):
        output=execute('show ntp status')
        if 'Clock is synchronized' in output: break
        time.sleep(2)
    else: raise AssertionError('NTP did not synchronize: '+output)
    time.sleep(20)
    execute('request platform software service time-sync restart')
    wait_service('time-sync')
    execute('request container-engine import /usr/share/fractal/probe.tar fcr/probe:latest','sha256:')
    execute('request container-engine run rm enable mount type=bind,src=/datastore/data0/chrony,dst=/state,readonly arguments fcr/probe:latest /probe ntp-state','NTP_PERSISTENT_STATE_OK')
    output=execute('show logging time-sync')
    assert 'Permission denied' not in output,output
    check('NTP synchronizes and preserves drift state across service restart on an older private-root datastore')


try:
    child = spawn()
    if case in ("cli","container-tools","kata","kata-factory","time-sync"):
        ready()
        {"cli":cli_semantics,"container-tools":container_tools,"kata":kata_nested,"kata-factory":kata_nested,"time-sync":time_sync}[case]()
    elif case == "debug-container":
        ready()
        child.sendline("request platform debug service-container enable")
        child.expect_exact("Privileged platform debug service enabled")
        child.expect_exact("debug-container: Ready."); prompt()
        child.sendline("request platform debug service-container enter")
        child.expect_exact("(platform debug container)")
        child.sendline("printf 'DEBUG_SHELL_OK\\n'; cat /etc/debian_version")
        child.expect_exact("DEBUG_SHELL_OK")
        child.expect(r"13\.[0-9]+")
        child.expect_exact("(platform debug container)")
        child.sendline("exit"); child.expect_exact("Returned to appliance console."); prompt()
        child.sendline("request platform debug service-container disable")
        child.expect_exact("Debug service disabled; container removed."); prompt()
        child.sendline("show containers"); prompt()
        assert "fractal-platform-debug" not in child.before, "debug container was not removed"
        check("Debian debug service enabled, shell entered/exited, and supervised container removed")
    elif case == "optional-firmware":
        ready()
        containers(1)
        check("signed OE OS and Docker work with firmware Secure Boot disabled and optional policy")
    elif case == "containers":
        ready()
        check("signed OE/glibc system booted; Btrfs mounted; root read-only; Docker API ready")
        management()
        containers(1)
        check("offline container passed PID/cgroup/seccomp/capability/read-only-root/bind-mount checks")
        check("container bridge and published TCP port worked")
        configuration_cli()
        reload_appliance(child)
        child.expect(pexpect.EOF)
        stop_qemu(child)
        child = spawn()
        ready()
        containers(2)
        child.sendline("show startup-config"); child.expect_exact('container cli-probe'); prompt()
        child.sendline("request container cli-probe create"); prompt()
        child.sendline("request container cli-probe start"); prompt()
        time.sleep(2)
        child.sendline("show container cli-probe logs"); child.expect_exact("PERSISTENT_COUNTER=2"); prompt()
        child.sendline("request container cli-probe remove"); prompt()
        check("Docker image and container-written counter survived a complete reboot")
    elif case == "blank-datastore":
        child.expect("%FCR-3-SYSTEM_DEGRADED", timeout=boot_timeout)
        console_login()
        child.sendline("conf t"); child.expect_exact("ctr(config)# ")
        child.sendline("user bootstrap-admin priv 15 password 0 Synthetic-bootstrap-592!"); child.expect_exact("ctr(config)# ")
        child.sendline("line con 0"); child.expect_exact("ctr(config-line)# ")
        child.sendline("login local")
        child.expect_exact("% initialize datastore data0 before enabling local login:")
        child.expect_exact("ctr(config-line)# ")
        child.sendcontrol("z"); prompt()
        child.sendline("wr me"); child.expect_exact("% datastore data0 is offline"); prompt()
        child.sendline("sh priv"); child.expect_exact("Current privilege level is 15"); prompt()
        check("AAA activation and save fail visibly without storage; bootstrap access remains available for initialization")
        for command in ("config", "storage datastore name data0", "preset raid1", "disk vdb", "disk add vdc", "exit"):
            child.sendline(command); child.expect(r"ctr\(config(?:-storage-datastore)?\)# ")
        child.sendline("do request storage datastore data0 format confirm data0")
        child.expect_exact("Datastore initialized.")
        child.expect_exact("container-engine: Ready.")
        child.expect_exact("ctr(config)# ")
        child.sendline("do show storage")
        child.expect(r"Data:\s+RAID1")
        child.expect_exact("ctr(config)# ")
        child.sendline("commit"); child.expect_exact("Running configuration committed."); child.expect_exact("ctr(config)# ")
        child.sendline("end"); prompt()
        child.sendline("write memory"); child.expect_exact("[OK]"); prompt()
        for command in ("conf t", "line con 0", "login local", "end"):
            child.sendline(command); child.expect(r"ctr(?:\(config(?:-line)?\))?# ")
        child.sendline("wr me"); child.expect_exact("[OK]"); prompt()
        child.sendline("logout"); child.expect_exact("Username: ")
        child.sendline("bootstrap-admin"); child.expect_exact("Password: ")
        child.sendline("Synthetic-bootstrap-592!"); prompt()
        child.sendline("terminal length 0"); prompt()
        check("after initialization, local login can be enabled, saved and authenticated without losing the bootstrap account")
        child.sendline("show platform"); child.expect_exact("Container service: running"); prompt()
        check("two blank disks initialized as RAID1 only by explicit format; services recover; configuration saved")
    elif case == "invalid-config":
        child.expect_exact("Configuration unavailable; authentication fails closed.", timeout=boot_timeout)
        check("malformed startup configuration cannot grant unauthenticated management access")
        stop_qemu(child)
        recover_startup = True
        child = spawn()
        child.expect_exact("Startup configuration ignored for physical-console recovery", timeout=boot_timeout)
        child.expect_exact("Press RETURN to get started!"); child.sendline(""); prompt()
        child.sendline("terminal length 0"); prompt()
        child.sendline("config"); child.expect_exact("ctr(config)# ")
        child.sendline("hostname ctr-test"); child.expect_exact("ctr-test(config)# ")
        child.sendline("commit"); child.expect_exact("Running configuration committed."); child.expect_exact("ctr-test(config)# ")
        child.sendline("end"); prompt()
        child.sendline("write memory"); child.expect_exact("[OK]"); prompt()
        reload_appliance(child); child.expect(pexpect.EOF); stop_qemu(child)
        recover_startup = False
        child = spawn(); ready()
        check("physical ROMMON 0x2142 recovery repairs damaged configuration and normal boot loads the repair")
    else:
        child.expect("%FCR-3-SYSTEM_DEGRADED", timeout=boot_timeout)
        console_login()
        child.sendline("show platform")
        child.expect_exact("Datastore data0: " + ("unavailable" if case == "no-datastore" else "online"))
        child.expect_exact("System image: read-only")
        child.expect_exact("Container service: stopped"); prompt()
        child.sendline("request platform software service container-engine start")
        child.expect_exact("% Service prerequisites are not satisfied."); prompt()
        check("failed prerequisites block container service without formatting storage")
    # Raw logs remain available on request, although normal boot was quiet.
    child.sendline("show logging kernel")
    child.expect_exact("Linux version"); prompt()
    assert not re.search(r"BUG:|Oops:|Kernel panic|soft lockup|rcu.*stall", child.before, re.I), "kernel fault in diagnostic log"
    check("quiet appliance boot retains kernel diagnostics on request")
    result["passed"] = True
except BaseException:
    if child is not None and child.isalive():
        try:
            child.sendcontrol("c")
            child.expect(r"ctr(?:-test)?# ",timeout=2)
            child.sendline("terminal length 0"); prompt()
            for service in ("kernel","container-engine","buildkit","syslog"):
                child.sendline("show logging "+service); prompt()
        except Exception:
            pass
    raise
finally:
    finish_qemu(child, result, art / f"result-os-{case}.json", log)
print(json.dumps(result))
