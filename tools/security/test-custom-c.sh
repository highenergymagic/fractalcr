#!/bin/bash
# Host-side sanitizer and bounded fuzz checks. Requires GCC, Clang and libcrypto.
set -euo pipefail
cd "$(dirname "$0")/../.."
seconds=${FCR_FUZZ_SECONDS:-60}
[[ "$seconds" =~ ^[1-9][0-9]*$ ]] || exit 2
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
platform=os/meta-fractal/recipes-core/fractal-init/files
flags=(-g -O1 -Wall -Wextra -Werror -fsanitize=address,undefined -fno-omit-frame-pointer)
gcc "${flags[@]}" -I"$platform" os/tests/test-line-editor.c "$platform/line-editor.c" -o "$work/editor"
"$work/editor"
gcc "${flags[@]}" -I"$platform" os/tests/test-pager.c "$platform/pager.c" -o "$work/pager"
"$work/pager"
gcc "${flags[@]}" -Iservice boot/tests/test-monitor.c service/control.c service/logging.c service/supervisor.c -lcrypto -o "$work/monitor"
"$work/monitor"
clang "${flags[@]}" -fsanitize=fuzzer -I"$platform" os/tests/fuzz-line-editor.c "$platform/line-editor.c" -o "$work/fuzz-editor"
clang "${flags[@]}" -fsanitize=fuzzer -Iservice boot/tests/fuzz-monitor.c service/control.c service/logging.c service/supervisor.c -lcrypto -o "$work/fuzz-monitor"
mkdir "$work/editor-corpus" "$work/monitor-corpus"
"$work/fuzz-editor" -max_total_time="$seconds" -max_len=8192 "$work/editor-corpus"
"$work/fuzz-monitor" -max_total_time="$seconds" -max_len=8191 "$work/monitor-corpus"
