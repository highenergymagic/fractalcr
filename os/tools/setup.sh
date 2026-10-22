#!/bin/bash
set -euo pipefail
cd /work
mkdir -p build/conf downloads sstate-cache artifacts
rsync -a --delete --exclude=.git --exclude=__pycache__ /src/os/meta-fractal/ meta-fractal/
rsync -a --exclude=.git --exclude=__pycache__ /src/service/ meta-fractal/recipes-core/fractal-init/files/
management_source=/work/meta-fractal/recipes-core/fractal-management/files/source
mkdir -p "$management_source"
rsync -a --delete /src/internal/ "$management_source/internal/"
rsync -a --delete /src/cmd/ "$management_source/cmd/"
cp /src/go.mod "$management_source/"
cp /src/go.sum "$management_source/"
rsync -a --delete /src/vendor/ "$management_source/vendor/"
ln -sfn /work/sources/bitbake /work/sources/openembedded-core/bitbake
cp /boot-work/build/keys/firmware.crt meta-fractal/recipes-kernel/linux/files/firmware.crt
cat > build/conf/bblayers.conf <<'EOF'
POKY_BBLAYERS_CONF_VERSION = "2"
BBPATH = "${TOPDIR}"
BBFILES ?= ""
BBLAYERS = " \
 /work/sources/openembedded-core/meta \
 /work/sources/meta-openembedded/meta-oe \
 /work/sources/meta-openembedded/meta-python \
 /work/sources/meta-openembedded/meta-networking \
 /work/sources/meta-openembedded/meta-filesystems \
 /work/sources/meta-virtualization \
 /work/meta-fractal \
"
EOF
cat > build/conf/local.conf <<'EOF'
MACHINE = "fractal-x86-64"
DISTRO = "fractal"
CONF_VERSION = "2"
DL_DIR = "/work/downloads"
SSTATE_DIR = "/work/sstate-cache"
BB_NUMBER_THREADS = "1"
PARALLEL_MAKE = "-j4"
PARALLEL_MAKE:pn-gcc-cross-x86_64 = "-j2"
BB_DISKMON_DIRS = "STOPTASKS,${TMPDIR},8G,100K STOPTASKS,${DL_DIR},8G,100K"
SSTATE_MIRRORS ?= "file://.* https://sstate.yoctoproject.org/all/PATH;downloadfilename=PATH"
BB_HASHSERVE = "auto"
BB_HASHSERVE_DB_DIR = "${SSTATE_DIR}"
BB_HASHSERVE_UPSTREAM = "wss://hashserv.yoctoproject.org/ws"
BB_SIGNATURE_HANDLER = "OEEquivHash"
BB_GENERATE_MIRROR_TARBALLS = "1"
BB_GIT_SHALLOW = "1"
BB_GENERATE_SHALLOW_TARBALLS = "1"
INITRAMFS_IMAGE = "fractal-system"
INITRAMFS_IMAGE_BUNDLE = "1"
INITRAMFS_MAXSIZE = "1572864"
INHERIT += "rm_work fractal-build-limits"
RM_WORK_EXCLUDE += "linux-fractal-cip"
EOF
# Carbon has four CPUs and 16 GiB RAM. Permit two tasks there, retaining the
# conservative settings for small builders. Do not export build limits globally:
# that would invalidate shared-state cache keys for unrelated recipes.
memory_kib=$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)
if (( memory_kib >= 12 * 1024 * 1024 )); then
    cat >> build/conf/local.conf <<'EOF'
BB_NUMBER_THREADS = "2"
PARALLEL_MAKE:pn-gcc-cross-x86_64 = "-j4"
EOF
fi
chown -R builder:builder /work/meta-fractal /work/build/conf
chown builder:builder /work /work/build /work/downloads /work/sstate-cache /work/artifacts
