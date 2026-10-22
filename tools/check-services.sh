#!/bin/bash
set -euo pipefail
cd /src
flags=(-Wall -Wextra -Werror -Iservice)
if [[ "${FCR_SANITIZE:-0}" == 1 ]]; then
    flags+=(-g -fsanitize=address,undefined -fno-omit-frame-pointer)
fi
editor=os/meta-fractal/recipes-core/fractal-init/files
gcc "${flags[@]}" "$editor/probe.c" -o /tmp/fractal-probe
gcc "${flags[@]}" "$editor/virtiofsd.c" -o /tmp/fractal-virtiofsd
gcc "${flags[@]}" "$editor/buildkit-runc.c" -o /tmp/fractal-buildkit-runc
gcc "${flags[@]}" -I"$editor" os/tests/test-line-editor.c "$editor/line-editor.c" -o /tmp/line-editor-test
/tmp/line-editor-test
gcc "${flags[@]}" os/tests/test-syslog.c "$editor/syslog.c" service/supervisor.c -o /tmp/syslog-test
/tmp/syslog-test
gcc "${flags[@]}" service/manager.c service/graph.c service/supervisor.c -o /tmp/fractal-init
gcc "${flags[@]}" -DFCR_ROMMON service/manager.c service/graph.c service/supervisor.c -o /tmp/rommon-init
gcc "${flags[@]}" os/meta-fractal/recipes-core/fractal-init/files/{platform,console,line-editor,syslog}.c \
    service/{supervisor,control,logging}.c -ljson-c -lblkid -lutil -o /tmp/platform
gcc "${flags[@]}" boot/tests/test-monitor.c service/{supervisor,control,logging}.c -lcrypto -o /tmp/monitor-test
/tmp/monitor-test
gcc "${flags[@]}" os/tests/test-supervisor.c service/supervisor.c -o /tmp/supervisor-test
/tmp/supervisor-test
gcc "${flags[@]}" service/test-graph.c service/graph.c service/supervisor.c -o /tmp/graph-test
/tmp/graph-test
gcc "${flags[@]}" service/test-control.c service/graph.c service/supervisor.c -o /tmp/control-test
/tmp/control-test
gcc "${flags[@]}" service/test-health.c service/graph.c service/supervisor.c -o /tmp/health-test
/tmp/health-test
