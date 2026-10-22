# Locked builds and reproducibility

Clone recursively as described in [architecture](architecture.md). Linux x86-64,
Docker, Git, Python 3.11+ and Bash are required. Allow at least 16 GiB RAM and
200 GiB disk for a comfortable OE build. Four compiler jobs are used on the
current builder. The Windows wrapper snapshots the same source to an SSH host.

```sh
# Linux with ordinary Docker group access:
FCR_DOCKER_DIRECT=1 bash tools/build.sh all All
# Linux with passwordless doas:
bash tools/build.sh all All
# Repeat appliance/rootfs/kernel generation and compare bytes:
bash tools/build.sh all Reproduce
```

```powershell
# Existing SSH build host; Docker Desktop is not involved:
.\tools\remote-build.ps1 -BuildHost carbon
.\tools\remote-build.ps1 -Action Status
.\tools\remote-build.ps1 -Action Export
```

The build never resolves a newer kernel or moving OE branch. Archive versions
and SHA-256 values live in `boot/sources.json`; the OE kernel recipe must agree.
Upstream layers use exact Git revisions. Go builds use the checked-in vendor
tree without network module resolution. Source-fetch caches reject dirty OE
repositories and unexpected remote URLs. Archive updates invalidate extracted
source and incompatible compiler outputs. Existing unmarked archive caches are
rebuilt once when adopting the new cache scheme.

The builder base is pinned by registry digest. Debian package resolution uses
the October 20, 2026 snapshot with signed APT metadata. The minimal base lacks a
CA bundle, so the first CA-package installation uses HTTP with APT signature and
package-hash verification; subsequent requests use HTTPS. Snapshot expiration
checking is disabled intentionally, but signature checking remains mandatory.
The firmware Python wheel is hash-locked; its dependencies come from the same
APT snapshot, and `pip check` verifies compatibility. The OS builder receives
the just-built boot builder's immutable local image ID.

`tools/build-inputs.py` records source hashes, public trust fingerprints, installed
builder package versions and builder identity. `tools/reproduce.sh` forces two
appliance/rootfs/kernel regenerations and compares output bytes. This retains
dependency compiler caches: it is not a clean-room rebuild of the entire
toolchain. Fixed builder inputs also do not imply byte-identical Docker image
IDs: package scripts, layer metadata and image creation timestamps can vary.

For stronger release evidence, run on two independent fresh builders, retain
source archives and downloads, compare unsigned outputs, and investigate every
difference. Upstream Kata, Compose, Buildx and BuildKit currently enter through
hash-pinned release binaries; the project does not yet reproduce those binaries
from source. The OE public sstate mirror is another trusted build input. A
source-only audit build must disable that mirror and use a fresh sstate cache.

Do not publish builder volumes, whole artifact directories, interactive disk
images or firmware variable stores. Development keys live in build volumes.
See [release policy](releases.md) for the public artifact allowlist.
