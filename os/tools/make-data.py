#!/usr/bin/env python3
"""Populate a disposable Btrfs data-disk file. Never opens a physical device."""
import argparse
import json
from pathlib import Path
import subprocess
import tempfile

p = argparse.ArgumentParser()
p.add_argument("--output", type=Path, required=True)
p.add_argument("--invalid-config", action="store_true")
p.add_argument("--size-mib", type=int, default=4096)
p.add_argument("--tool-fixtures", action="store_true")
p.add_argument("--config", type=Path, help="Explicit startup configuration for a disposable VM")
a = p.parse_args()
if not 512 <= a.size_mib <= 1048576:
    p.error("--size-mib must be between 512 and 1048576")
if a.output.exists() and not a.output.is_file():
    p.error("output must be a regular disk-image file")
with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)
    (root / "config").mkdir()
    (root / "selftest").mkdir()
    # mkfs --rootdir preserves the invoking user's ownership. The witness
    # drops every capability, including DAC_OVERRIDE, so its fixture must be
    # writable even when this disposable image is created without root.
    (root / "selftest").chmod(0o777)
    if a.tool_fixtures:
        (root / "projects").mkdir()
        (root / "projects/Dockerfile").write_text('FROM scratch\nCOPY probe /probe\nRUN ["/probe", "network"]\nENTRYPOINT ["/probe"]\nCMD ["serve"]\n')
        (root / "projects/compose.yaml").write_text('services:\n  probe:\n    image: fcr/probe:latest\n    command: ["/probe", "serve"]\n    ports: ["18080:8080"]\n')
        (root / "files").mkdir()
        for name in ("Dockerfile", "compose.yaml"):
            (root / "files" / name).write_bytes((root / "projects" / name).read_bytes())
    config = {"version": 1, "hostname": "ctr-test",
              "network": {"interface": "eth0", "address": "10.0.2.15/24",
                          "gateway": "10.0.2.2", "dns": "10.0.2.3"}}
    if a.config:
        config = json.loads(a.config.read_text())
    (root / "config/startup-config.json").write_text(
        "{broken JSON\n" if a.invalid_config else json.dumps(config) + "\n")
    with a.output.open("wb") as f:
        f.truncate(a.size_mib * 1024 * 1024)
    subprocess.run(["mkfs.btrfs", "-f", "-L", "FRDATA", "--rootdir", str(root),
                    str(a.output)], check=True)
