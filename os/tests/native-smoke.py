#!/usr/bin/env python3
"""Exercise exported OS artifacts using native QEMU and disposable snapshots."""
import hashlib
import argparse
import json
import os
from pathlib import Path
import queue
import shutil
import subprocess
import threading
import time
from quiet_console import assert_quiet_boot

parser = argparse.ArgumentParser()
parser.add_argument("--artifacts", type=Path,
                    default=Path(__file__).resolve().parents[2] / "out" / "carbon" / "os")
parser.add_argument("--no-report", action="store_true",
                    help="record this result without aggregating other scenarios")
parser.add_argument("--accelerator", choices=("tcg", "whpx"), default="tcg")
options = parser.parse_args()
art = options.artifacts.resolve()
qemu = shutil.which("qemu-system-x86_64")
if not qemu:
    raise SystemExit("qemu-system-x86_64 is not on PATH")
whpx = options.accelerator == "whpx"
case = "native-whpx" if whpx else "native"
variables = art / f"OVMF_VARS-os-{case}.fd"
firmware = Path(qemu).parent / "share" / "edk2-x86_64-code.fd" if whpx else art / "OVMF_CODE_4M.secboot.fd"
vars_template = Path(qemu).parent / "share" / "edk2-i386-vars.fd" if whpx else art / "OVMF_VARS-trusted.fd"
flash = "flash-os-optional-firmware.img" if whpx else "flash-os-containers.img"
shutil.copyfile(vars_template, variables)
args = [
    qemu, "-machine", "q35,smm=off" if whpx else "q35,smm=on",
    "-accel", options.accelerator,
    "-cpu", "qemu64" if whpx else "max", "-m", "4096",
    *([] if whpx else ["-global", "driver=cfi.pflash01,property=secure,value=on"]),
    "-drive", f"if=pflash,format=raw,unit=0,readonly=on,file={firmware}",
    "-drive", f"if=pflash,format=raw,unit=1,file={variables}",
    "-drive", f"if=none,id=flash,format=raw,snapshot=on,file={art / flash}",
    "-device", "virtio-blk-pci,drive=flash,bootindex=0",
    "-drive", f"if=virtio,format=raw,snapshot=on,file={art / 'data-os-containers.img'}",
    "-netdev", "user,id=net0", "-device", "virtio-net-pci,netdev=net0",
    "-display", "none", "-serial", "stdio", "-monitor", "none", "-no-reboot",
]
result = {
    "case": "native-os-whpx" if whpx else "native-os", "passed": False, "qemu": qemu,
    "accelerator": options.accelerator,
    "rommon_sha256": hashlib.sha256((art / "rommon.efi").read_bytes()).hexdigest(),
    "system_sha256": hashlib.sha256((art / "system.bin").read_bytes()).hexdigest(),
}
child = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                         stderr=subprocess.STDOUT,
                         creationflags=subprocess.CREATE_NO_WINDOW if os.name == "nt" else 0)
messages = queue.Queue()

def read():
    while data := child.stdout.read1(4096):
        messages.put(data)
    messages.put(None)

reader = threading.Thread(target=read, daemon=True)
reader.start()
def send(data):
    # Windows QEMU's stdio UART can overrun when a pipe feeds an entire line
    # while the guest echoes it. Pace the serial transport, not guest execution.
    for byte in data:
        child.stdin.write(bytes([byte]))
        child.stdin.flush()
        time.sleep(0.02)

text = ""
stage = "boot"
transcript = (art / f"qemu-os-{case}.log").open("w", encoding="utf-8")
try:
    end = time.monotonic() + 360
    while time.monotonic() < end:
        try:
            data = messages.get(timeout=1)
        except queue.Empty:
            continue
        if data is None:
            raise RuntimeError("QEMU exited before container validation completed")
        chunk = data.decode("utf-8", errors="replace")
        text += chunk
        transcript.write(chunk)
        transcript.flush()
        if "!!!! X64 Exception" in text:
            raise RuntimeError("UEFI firmware exception; see serial transcript")
        if stage == "boot" and "Press RETURN to get started!" in text:
            assert_quiet_boot(text)
            assert "%FCR-5-SYSTEM_READY" in text, "OS startup failed"
            assert "Image signature verified." in text
            if whpx:
                assert "Firmware trust is unavailable. Image signatures remain mandatory." in text
            else:
                assert "Secure Boot: enabled" in text
            send(b"\n")
            stage = "login"; text = ""
        elif stage == "login" and "ctr-test# " in text:
            send(b"terminal length 0\n")
            stage = "terminal"; text = ""
        elif stage == "terminal" and "ctr-test# " in text:
            send(b"request platform diagnostics containers\n")
            stage = "test"; text = ""
        elif stage == "test" and "%FCR-3-DIAGNOSTIC_FAILED" in text:
            send(b"show logging diagnostics\n")
            stage = "failed"; text = ""
        elif stage == "failed" and "ctr-test# " in text:
            raise RuntimeError(f"Native container test failed; see qemu-os-{case}.log")
        elif stage == "test" and "%FCR-5-DIAGNOSTIC_PASS" in text and "ctr-test# " in text:
            assert_quiet_boot(text)
            send(b"show logging diagnostics\n")
            stage = "evidence"; text = ""
        elif stage == "evidence" and "CONTAINER_PORT_PUBLISH_OK" in text:
            for marker in ("HOST_ROOT_READONLY_OK", "CONTAINER_SWAP_LIMIT_OK", "CONTAINER_ISOLATION_OK",
                           "PERSISTENT_COUNTER=3", "CONTAINER_BRIDGE_OK",
                           "CONTAINER_PORT_PUBLISH_OK"):
                assert marker in text, "Missing validation: " + marker
            result["passed"] = True
            break
    assert result["passed"], f"Native boot/test timed out; see qemu-os-{case}.log"
finally:
    child.terminate()
    try:
        child.wait(timeout=10)
    except subprocess.TimeoutExpired:
        child.kill()
        child.wait()
    reader.join(timeout=2)
    transcript.close()
    (art / f"result-os-{case}.json").write_text(json.dumps(result, indent=2) + "\n")
print(json.dumps(result))
if not options.no_report and not whpx:
    subprocess.run([os.sys.executable, str(Path(__file__).resolve().parents[1] / "tools" / "report.py"),
                    "--artifacts", str(art), "--native"], check=True)
