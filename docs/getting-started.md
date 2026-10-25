# Getting started

FractalCR currently targets evaluation and development. Read the
[known issues](development-status.md) first. This guide takes you from source to
a serial-managed VM. To install a published USB or hypervisor image, use
[Image deployment](../os/docs/releases.md).

## 1. Choose a target and build host

Use an x86-64 Linux Docker host to build. Plan for at least 16 GiB RAM and 200 GiB
free disk space. You need Git, Python 3.11 or newer and Bash. Building does not
require KVM, but full management qualification and Kata execution do.

For Windows VM use, install QEMU with its UEFI firmware files, put
`qemu-system-x86_64.exe` on PATH, and enable Windows Hypervisor Platform for WHPX.
The launcher allocates 4 GiB RAM. Its WHPX profile runs ordinary containers;
use a Linux KVM host for nested Kata tests. The console is serial-only.

Clone recursively as shown in the [project README](../README.md), then follow
[building](building.md). That guide covers local Linux and Windows-to-Linux SSH
builds. This source checkout does not include images. Check the
[releases page](https://github.com/highenergymagic/fractalcr/releases) for any
separately published prereleases; this guide does not assume one is available.

## 2. Prepare a blank VM

Build/test outputs can include populated acceptance-test disks. For a new
installation, generate blank launcher templates instead. On Linux, install
`mtools`, `dosfstools` and `btrfs-progs`, then run from the integration checkout:

```sh
python3 tools/prepare-launcher.py --artifacts /path/to/exported-os-artifacts --output /path/to/new-launcher-directory
```

The input directory must contain matching `system.bin`, `rommon.efi`,
`firmware.crt`, `OVMF_CODE_4M.secboot.fd`, `OVMF_VARS-trusted.fd` and
`OVMF_VARS-unenrolled.fd`. See [artifact export](building.md#export-build-artifacts).
The output directory must not already exist. It includes a blank 4 GiB Btrfs
datastore disk, bootflash templates, firmware files and `launcher-manifest.json`.
For Windows, copy the complete output directory to the workstation, for example
`C:\FractalCR-images\evaluation`.

From the integration checkout on Windows:

```powershell
.\os\run.ps1 -ArtifactDirectory C:\FractalCR-images\evaluation -NoSecureBoot -Accelerator whpx -SshPort 2222
```

Firmware Secure Boot is disabled for this WHPX configuration; system image
signatures remain mandatory. For Secure Boot evaluation, omit `-NoSecureBoot`
and select `-Accelerator tcg`. Software emulation can take several minutes to
boot. See [troubleshooting](troubleshooting.md) if the handoff appears silent.

### Linux KVM alternative

With QEMU installed and read/write access to `/dev/kvm`, use the same prepared
templates on Linux. Replace `/path/to/new-launcher-directory`. Create a new VM
directory and writable copies so the blank templates remain unchanged:

```sh
mkdir evaluation-vm
cp --reflink=auto --sparse=always /path/to/new-launcher-directory/flash-os-optional-firmware.img evaluation-vm/flash.img
cp --reflink=auto --sparse=always /path/to/new-launcher-directory/data-os-containers.img evaluation-vm/data.img
cp /path/to/new-launcher-directory/OVMF_CODE_4M.secboot.fd evaluation-vm/code.fd
cp /path/to/new-launcher-directory/OVMF_VARS-unenrolled.fd evaluation-vm/vars.fd
cd evaluation-vm
qemu-system-x86_64 -machine q35,smm=on -accel kvm -cpu host -smp 2 -m 4096 \
  -drive if=pflash,format=raw,readonly=on,file=code.fd \
  -drive if=pflash,format=raw,file=vars.fd \
  -drive if=virtio,format=raw,file=flash.img \
  -drive if=virtio,format=raw,file=data.img \
  -netdev user,id=net0,hostfwd=tcp:127.0.0.1:2222-:22 \
  -device virtio-net-pci,netdev=net0 -nographic
```

This uses optional firmware policy and unenrolled firmware variables; it still
verifies the system image. Ctrl-A X exits QEMU. Run the same QEMU command in
`evaluation-vm` to resume with persistent data. Return to the integration checkout
before running repository tools. Nested Kata additionally requires virtualization
extensions exposed to this guest; ordinary container evaluation does not.

For USB or VPS deployment, use [image deployment](../os/docs/releases.md).
The split VM disks above are not whole-disk USB images.

## 3. Complete serial setup

Press RETURN when requested. On an empty datastore, setup asks for a hostname,
management interface, DHCP or static addressing, optional NTP, and an
administrator account. Choose the management port from the discovered list;
interface names depend on the machine. Review and save the configuration.
Prepared media already contains a datastore and does not need formatting.

Use a unique password; there is no default login. `setup` restarts the dialog
from privileged physical-console EXEC. After setup:

```text
show version
show interfaces
show ip route
show platform software service
show running-config
write memory
```

Running configuration may contain sensitive values and password verifiers;
review it before sharing. If SSH is not enabled, follow the
[SSH procedure](../os/docs/aaa-and-management.md). With port forwarding from
step 2, use `ssh -p 2222 admin@127.0.0.1`, substituting your account name.
SSH opens the appliance CLI, not a shell or SFTP server.

## 4. Run a first container

This example needs working DNS and registry access. `alpine:3.22` is an example
image; choose an image suitable for your work.

```text
request image pull alpine:3.22
configure terminal
container example
image alpine:3.22
command sleep infinity
no container-isolation
end
write memory
request container example create
request container example start
show container example detail
```

Configuration defines the container; explicit requests create and start it.
Editing a definition does not recreate an existing container. To remove this
example, remove both its runtime object and saved definition:

```text
request container example stop
request container example remove
configure terminal
no container example
end
write memory
```

## 5. Learn the management model

`configure terminal` applies accepted edits immediately. `write memory` saves
them for boot; `end` and Ctrl-Z only leave configuration mode. `configure candidate`
is an optional transaction mode with `commit` and `abort`. Use `?` for next-token
help, `sh?` for keyword matches, and Tab for completion. See the
[CLI reference](../os/CLI.md) for networking, Compose, storage, time and Kata.

The launcher retains interactive disks. `-UpdateImage` replaces bootflash from
the selected templates while preserving the datastore; it also replaces ROMMON
settings on bootflash. **`-Fresh` resets interactive disks and loses their
configuration and container data.** Back up needed data before upgrades or resets.
Keep firmware, ROMMON and the system image from the same trust chain together;
changing build keys is not an ordinary OS-only update.
