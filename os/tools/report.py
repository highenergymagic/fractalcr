#!/usr/bin/env python3
"""Write a report only for results matching the current exported images."""
import argparse
import hashlib
import json
import re
from pathlib import Path

p = argparse.ArgumentParser()
p.add_argument("--artifacts", type=Path, default=Path("/work/artifacts"))
p.add_argument("--native", action="store_true")
p.add_argument("--whpx", action="store_true", help="require native Windows WHPX acceptance")
p.add_argument("--debug", action="store_true", help="include the registry-dependent Debian debug test")
p.add_argument("--extended", action="store_true", help="require CLI, full client stack and nested Kata scenarios")
p.add_argument("--tcg", action="store_true", help="require additional core TCG results under artifacts/tcg")
p.add_argument("--management", action="store_true", help="require serial/SSH AAA and physical password recovery")
p.add_argument("--swarm", action="store_true", help="require isolated three-node Swarm failure qualification")
p.add_argument("--physical", action="store_true", help="require single-disk USB, NVMe and AHCI setup/network/time qualification")
a = p.parse_args()
art = a.artifacts
hashes = {name: hashlib.sha256((art / name).read_bytes()).hexdigest()
          for name in ("rommon.efi", "system.bin")}
cases = ["containers", "no-datastore", "invalid-config", "optional-firmware", "blank-datastore"]
if a.native:
    cases.append("native")
if a.whpx:
    cases.append("native-whpx")
if a.debug:
    cases.append("debug-container")
if a.extended:
    cases += ["cli", "container-tools", "kata"]
if a.management:
    cases.append("aaa")
if a.swarm:
    cases.append("swarm-multinode")
lines = ["# Appliance OS validation", "", "All results below match these images.", "",
         "| Image | Bytes | SHA-256 |", "|---|---:|---|"]
for name, digest in hashes.items():
    lines.append(f"| `{name}` | {(art/name).stat().st_size:,} | `{digest}` |")
if (art / "reproducibility.json").exists():
    reproducibility = json.loads((art / "reproducibility.json").read_text())
    assert reproducibility["passed"], "Reproducibility check failed"
    assert set(reproducibility["artifacts"]) == {"system.bin", "system.efi"}, "Incomplete reproducibility artifacts"
    for name, digest in reproducibility["artifacts"].items():
        assert hashlib.sha256((art / name).read_bytes()).hexdigest() == digest, f"Stale reproducibility result: {name}"
    lines += ["", "Reproducibility: PASS (`reproducibility.json`).",
              reproducibility["scope"] + "."]
lines += ["", "| Scenario | Result |", "|---|---|"]
for case in cases:
    result = json.loads((art / f"result-os-{case}.json").read_text())
    assert result["passed"], f"Failed scenario: {case}"
    assert result["rommon_sha256"] == hashes["rommon.efi"], f"Stale ROMMON result: {case}"
    assert result["system_sha256"] == hashes["system.bin"], f"Stale OS result: {case}"
    if case == "swarm-multinode":
        assert result["esp_packets"] > 0 and len(result["overlay_captures"]) == 3, "Missing encrypted-overlay evidence"
        for name, expected in result["overlay_captures"].items():
            assert Path(name).name == name and name.startswith("overlay-only-"), "Invalid capture filename"
            actual = hashlib.sha256((art / "swarm-multinode" / name).read_bytes()).hexdigest()
            assert actual == expected, f"Stale encrypted-overlay capture: {name}"
    lines.append(f"| {case} ({result.get('accel',result.get('accelerator','tcg'))}) | PASS |")
    for check in result.get("checks", []):
        lines.append(f"| {check} | PASS |")
if a.physical:
    for case in ("physical-usb-optional", "physical-nvme-required", "physical-ahci-optional"):
        result = json.loads((art / f"result-{case}.json").read_text())
        assert result["case"] == case and result["passed"], f"Failed physical-media scenario: {case}"
        assert result["rommon_sha256"] == hashes["rommon.efi"] and result["system_sha256"] == hashes["system.bin"], f"Stale physical-media evidence: {case}"
        lines.append(f"| {case} (KVM) | PASS |")
        for check in result["checks"]:
            lines.append(f"| {check} | PASS |")
if a.tcg:
    for case in ("containers", "no-datastore", "invalid-config", "optional-firmware", "blank-datastore"):
        result = json.loads((art / "tcg" / f"result-os-{case}.json").read_text())
        assert result["passed"] and result["accel"] == "tcg", f"Failed TCG scenario: {case}"
        assert result["rommon_sha256"] == hashes["rommon.efi"], f"Stale TCG ROMMON result: {case}"
        assert result["system_sha256"] == hashes["system.bin"], f"Stale TCG OS result: {case}"
        lines.append(f"| Additional core regression: {case} (tcg) | PASS |")
assert (art / "supervisor-tests.log").read_text().count("PASS ") == 6
assert (art / "service-graph-tests.log").read_text().count("PASS ") == 6
assert (art / "control-tests.log").read_text().count("PASS ") == 3
assert (art / "health-tests.log").read_text().count("PASS ") == 3
assert (art / "line-editor-tests.log").read_text().count("PASS ") == 6
assert "PASS syslog delivery" in (art / "syslog-tests.log").read_text()
lines += ["", "Supervisor fault tests: PASS (`supervisor-tests.log`).",
          "Dependency lifecycle tests: PASS (`service-graph-tests.log`).",
          "Control transport tests: PASS (`control-tests.log`).",
          "Asynchronous Docker health tests: PASS (`health-tests.log`).",
          "Serial line editor tests: PASS (`line-editor-tests.log`).",
          "Syslog delivery/restart tests: PASS (`syslog-tests.log`).",
          "", "Normal startup is checked for kernel/daemon console leakage. Raw kernel",
          "diagnostics remain available on explicit request. Tests exercise service",
          "controls, partial console input, Docker isolation/networking, data across",
          "reboot, and failure of storage/configuration prerequisites.",
          "", "Scope: QEMU with the accelerator recorded per case, OVMF, firmware verification enabled or explicitly optional,",
          "4096 MiB RAM. Physical hardware is not validated.",
          "Kata guest execution is qualified only when the explicit kata case appears above.",
          "", "Evidence: `result-os-*.json` and `qemu-os-*.log`. Logs are deliberately raw",
          "only after explicit diagnostic commands in these transcripts."]
swap_logs = [(art / "qemu-os-containers.log", 2), (art / "qemu-os-optional-firmware.log", 1)]
if a.whpx:
    swap_logs.append((art / "qemu-os-native-whpx.log", 1))
if a.tcg:
    swap_logs += [(art / "tcg/qemu-os-containers.log", 2), (art / "tcg/qemu-os-optional-firmware.log", 1)]
for path, minimum in swap_logs:
    assert path.read_text(errors="replace").count("CONTAINER_SWAP_LIMIT_OK") >= minimum, f"Missing swap-limit probe evidence: {path}"
lines += ["", "Docker swap-limit detection and cgroup v2 limit probes: PASS, including reboot and optional firmware verification."]
if (art / "result-launcher-ssh-forward.json").exists():
    launcher = json.loads((art / "result-launcher-ssh-forward.json").read_text())
    assert launcher["passed"] and launcher["system_sha256"] == hashes["system.bin"], "Stale/failed Windows launcher test"
    assert launcher["host_address"] == "127.0.0.1" and launcher["guest_port"] == 22
    lines += ["", "Actual Windows launcher with optional localhost-only SSH forwarding: PASS (`result-launcher-ssh-forward.json`)."]
if (art / "source-snapshot.json").exists():
    snapshot = json.loads((art / "source-snapshot.json").read_text())
    assert hashlib.sha256((art / "build-source.tar.gz").read_bytes()).hexdigest() == snapshot["sha256"], "Stale source snapshot archive"
    lines += ["", "Exact build source: `build-source.tar.gz`; archive hash and builder identity: `source-snapshot.json`."]
(art / "validation.md").write_text("\n".join(lines) + "\n")
transcript = (art / "qemu-os-containers.log").read_bytes().decode("utf-8", errors="replace").replace("\r", "")
transcript = re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", transcript)
start = transcript.index("Initializing recovery devices...")
end = transcript.index("Press RETURN to get started!", start) + len("Press RETURN to get started!")
(art / "boot-console.txt").write_text(transcript[start:end] + "\n", encoding="utf-8")
print("Validated image hashes and wrote validation.md")
