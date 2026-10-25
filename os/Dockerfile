ARG BOOT_BUILDER
FROM ${BOOT_BUILDER}
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    chrpath diffstat g++ gcc locales socat texinfo zstd lz4 bzip2 \
    python3-jinja2 python3-subunit python3-pip python3-websockets pylint iputils-ping \
    btrfs-progs util-linux fdisk qemu-utils genisoimage openssh-client libjson-c-dev libblkid-dev && rm -rf /var/lib/apt/lists/* \
    && localedef -i en_US -f UTF-8 en_US.UTF-8 \
    && useradd -m -u 1000 -s /bin/bash builder
ENV LANG=en_US.UTF-8 LC_ALL=en_US.UTF-8
WORKDIR /work
