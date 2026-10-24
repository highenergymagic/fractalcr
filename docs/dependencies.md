# Dependency maintenance

Updates are explicit source changes. Discovery runs independently of builds:

```sh
python3 tools/dependencies.py discover --output out/dependency-report.json
python3 tools/dependencies.py update-archive linux --version 6.12.N-cipM --sha256 REVIEWED_SHA256
python3 tools/dependencies.py update-archive buildroot --version 2025.02.N --sha256 REVIEWED_SHA256
python3 tools/dependencies.py vendor-go
```

The discovery command checks the current CIP 6.12 line, Buildroot 2025.02 LTS,
OE Wrynose/BitBake 2.18 branches, ROMMON OpenSSL 3.5, Go 1.26, Docker 29, containerd 2.2, runc 1.5
and the upstream container-tool releases. It
produces a report; it does not install discovered software. The archive updater
downloads and hashes the actual archive before changing pins, refuses
downgrades, and updates both kernel consumers together. A failed download leaves
the pins unchanged. Review both component diffs before committing their gitlinks.

Buildroot updates also verify the upstream clearsigned checksum document against
the maintainer key and fingerprint pinned in `boot/upstream-trust`. The key's
initial provenance is documented there; it is not fetched and trusted afresh
for every update.

Discovery reads the CIP checksum listing over HTTPS. **It does not verify the
listing's PGP signature or the developer's release signature.** Before accepting
a release, verify its developer signature using an independently authenticated
maintainer key. Never import and trust a key solely because the same server
offered it next to the download. Kernel.org distinguishes its checksum autosigner
from developer signatures in its [verification guide](https://kernel.org/signature.html).

For OE updates, change the relevant revisions in `os/sources.json` to the reviewed
branch heads in the report; all layers must remain on the compatible series.
For Go updates, edit `go.mod` through a reviewed dependency PR, run `vendor-go`,
then commit `go.mod`, `go.sum` and `vendor` together. Dependabot opens Go and
Actions updates. Component gitlinks are updated explicitly after integration
review. Automatic component-pin updates are not configured.
CI checks that vendoring matches the module files.
For upstream Go applications, `tools/security/refresh-vendor.py` operates on a
clean checkout at an explicitly reviewed full commit. Supply exact module
versions and the approved Go executable. It verifies modules against the Go
checksum database, generates the vendor tree, and exports a frozen patch plus
hash manifest. For projects without checked-in vendoring, use `--vendor-archive`.
The generated patch includes deleted and ignored vendor files; its regression
test covers both. Review and apply the patch to a pristine upstream tree before
building. Never run this network-dependent refresh inside an image build.
For Rust VMM dependencies, retain the upstream commit, refreshed Cargo.lock,
offline vendor archive and the manifest from `tools/security/freeze-cargo.py`.
Review OSV results against the selected Linux feature tree; a whole-workspace
lockfile includes packages that may not be built. The exact compiler is a pinned
upstream binary build input and is not independently source-bootstrapped here.

The CIP kernel additionally carries stable security backports recorded in
`boot/kernel/security/kernel-security.json`, mirrored in the OE kernel recipe.
Verify the derived patch bodies against the downloaded upstream `.xz` updates
with `tools/security/verify-kernel-backports.py`. The base version remains visible;
these are distribution backports, not an official newer CIP release. A kernel
pin update must review or retire this patch series and update both copies. The
build rejects a mismatched base or hash. ROMMON applies updates to a fresh copy
of archive-verified source, preserving the original cache for validation.
Kata updates also need guest kernel/image filename review in the OE recipe;
blind version replacement would be unsafe. Builder snapshot changes are explicit
updates to `boot/debian.sources`, followed by builder and image qualification.

The scheduled GitHub discovery workflow publishes a downloadable report. It has
no signing credentials, cannot write component repos and does not auto-merge.
This lets updates arrive automatically for review without making the meaning of
a source revision change every time it is built.

Before promoting a dependency change: review release/security notes and source
provenance, run unit and negative signature tests, rebuild both images, compare
reproducibility results, then run boot/recovery, AAA, Docker/Kata and Swarm cases
affected by the change. Record both fixed CVEs and outstanding findings. An
update available is not itself proof of a vulnerability, and a clean scanner
result is not proof of security.
