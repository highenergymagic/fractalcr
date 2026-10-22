#!/bin/bash
set -euo pipefail
cd /work
editor=/src/os/meta-fractal/recipes-core/fractal-init/files
gcc -Wall -Wextra -Werror -I"$editor" /src/os/tests/test-line-editor.c "$editor/line-editor.c" -o /tmp/test-line-editor
/tmp/test-line-editor | tee artifacts/line-editor-tests.log
gcc -Wall -Wextra -Werror -I/src/service /src/os/tests/test-syslog.c "$editor/syslog.c" /src/service/supervisor.c -o /tmp/test-syslog
/tmp/test-syslog | tee artifacts/syslog-tests.log
gcc -Wall -Wextra -Werror -I/src/service \
    /src/os/tests/test-supervisor.c /src/service/supervisor.c \
    -o /tmp/test-supervisor
/tmp/test-supervisor | tee artifacts/supervisor-tests.log
gcc -Wall -Wextra -Werror -I/src/service /src/service/test-graph.c \
    /src/service/graph.c /src/service/supervisor.c -o /tmp/test-graph
/tmp/test-graph | tee artifacts/service-graph-tests.log
gcc -Wall -Wextra -Werror -I/src/service /src/service/test-control.c \
    /src/service/graph.c /src/service/supervisor.c -o /tmp/test-control
/tmp/test-control | tee artifacts/control-tests.log
gcc -Wall -Wextra -Werror -I/src/service /src/service/test-health.c \
    /src/service/graph.c /src/service/supervisor.c -o /tmp/test-health
/tmp/test-health | tee artifacts/health-tests.log
sbverify --cert artifacts/firmware.crt artifacts/rommon.efi
sbverify --cert artifacts/firmware.crt artifacts/system.efi
if [[ "${1:-}" == --unit-only ]]; then exit 0; fi
# Each VM owns separate firmware, flash, datastore, transcript and result files.
# Two TCG CPUs leave headroom on the four-core build host for runtime deadlines.
python3 - <<'PY'
from concurrent.futures import ThreadPoolExecutor
import subprocess

cases = ("containers", "no-datastore", "invalid-config", "optional-firmware", "blank-datastore")
def run(case):
    return subprocess.run(["python3", "/src/os/tests/qemu-system.py", "--case", case]).returncode
with ThreadPoolExecutor(max_workers=2) as pool:
    results = list(pool.map(run, cases))
if any(results):
    raise SystemExit("Failed VM scenarios: " + ", ".join(case for case, rc in zip(cases, results) if rc))
PY
python3 /src/os/tools/report.py
