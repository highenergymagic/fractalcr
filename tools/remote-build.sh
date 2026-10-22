#!/bin/bash
# Executed on the Linux build host from an immutable source snapshot.
set -euo pipefail
stage=${1:?boot, os, or all}
action=${2:?Build, Test, or All}
signing=${3:-development}
trust_id=${4:-development}
volume_prefix=fractalcr
case "$signing" in
    development) ;;
    external)
        [[ "$trust_id" =~ ^[a-f0-9]{16}$ && "$action" == Build && "$stage" != all ]] || exit 2
        volume_prefix="fractalcr-release-$trust_id"
        ;;
    *) exit 2 ;;
esac
case "$stage:$action" in
    boot:Build|boot:Test|boot:All|os:Build|os:Test|os:All|all:All|boot:Reproduce|os:Reproduce|all:Reproduce) ;;
    *) echo "Invalid build stage/action" >&2; exit 2 ;;
esac
src=$(cd "$(dirname "$0")/.." && pwd)
state="$HOME/FractalCR-build"
mkdir -p "$state"
exec 9>"$state/build.lock"
flock -n 9 || { echo "A FractalCR build is already running." >&2; exit 1; }
finish() {
    result=$?
    printf '%s\n' "$result" > "$state/exit-code"
    date -u +%FT%TZ > "$state/finished"
}
trap finish EXIT
rm -f "$state/exit-code" "$state/finished"
printf '%s\n' "$src" > "$state/source"
date -u +%FT%TZ > "$state/started"
# Direct Docker access is the portable default when FCR_DOCKER_DIRECT=1.
docker() {
    if [[ "${FCR_DOCKER_DIRECT:-0}" == 1 ]]; then command docker "$@";
    else doas -n docker "$@"; fi
}
docker info --format 'Build host: {{.OSType}}/{{.Architecture}}, {{.NCPU}} CPUs, {{.MemTotal}} bytes RAM'
run_stage() {
    local target=$1 mode=$2
    local image="fractalcr-${target}-builder"
    local -a mounts=(--mount "type=volume,source=${volume_prefix}-${target}-work,target=/work"
                     --mount "type=bind,source=$src,target=/src,readonly")
    mounts+=(-e "FCR_SIGNING_MODE=$signing")
    if [[ "$signing" == external ]]; then mounts+=(-e FCR_TRUST_DIR=/src/release-trust); fi
    if [[ "$target" == os ]]; then
        mounts+=(--mount "type=volume,source=${volume_prefix}-boot-work,target=/boot-work,readonly")
    fi
    if [[ "$mode" == Reproduce ]]; then
        # Reuse the exact builder image whose immutable ID is recorded below.
        local builder_id
        builder_id=$(docker image inspect --format '{{.Id}}' "$image")
        docker run --rm --name "fractalcr-${target}-reproduce" \
            --security-opt seccomp=unconfined "${mounts[@]}" -e JOBS=4 \
            -e "FCR_BUILDER_ID=$builder_id" "$builder_id" bash /src/tools/reproduce.sh "$target"
        mode=Test
    fi
    if [[ "$mode" == Build || "$mode" == All ]]; then
        local -a build_args=()
        if [[ "$target" == os ]]; then
            # Rebuild the pinned parent recipe first; never inherit an arbitrary
            # local 'latest' image left behind by an unrelated source checkout.
            docker build -t fractalcr-boot-builder -f "$src/boot/Dockerfile" "$src"
            build_args+=(--build-arg "BOOT_BUILDER=$(docker image inspect --format '{{.Id}}' fractalcr-boot-builder)")
        fi
        docker build "${build_args[@]}" -t "$image" -f "$src/$target/Dockerfile" "$src"
        local builder_id
        builder_id=$(docker image inspect --format '{{.Id}}' "$image")
        docker run --rm --name "fractalcr-${target}-build" \
            --security-opt seccomp=unconfined "${mounts[@]}" \
            -e JOBS=4 -e "FCR_BUILDER_ID=$builder_id" "$builder_id" bash "/src/$target/tools/build.sh"
    fi
    if [[ "$mode" == Test || "$mode" == All ]]; then
        docker run --rm --name "fractalcr-${target}-test" "${mounts[@]}" \
            "$image" bash "/src/$target/tools/test.sh"
    fi
}
if [[ "$stage" == all ]]; then
    run_stage boot "$action"
    run_stage os "$action"
else
    run_stage "$stage" "$action"
fi
echo FRACTALCR_REMOTE_JOB_PASSED
