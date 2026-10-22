# Signing and release policy

An HSM is optional. FractalCR already supports an encrypted software key vault,
public-only builders, offline signing of a reviewed hash, and UEFI key rotation.
See the [key-management procedure](../boot/docs/release-key-management.md).
Keep production private keys and their passphrases off GitHub and build hosts.

GrapheneOS's documented workflow separates building from signing, uses encrypted
release keys, and pins source revisions in release manifests. That is a useful
pattern to adopt; it does not establish which hardware GrapheneOS uses internally.
See its [build and signing guide](https://grapheneos.org/build).

Development images may be published as GitHub **prereleases** with an explicit
development-signing label. Our development keys are randomly generated private
keys, not publicly known test keys. They are retained on the builder, so a builder
compromise also compromises that development identity. Do not present them as
offline-custodied production releases or distribute their private keys.

Every release must identify the integration commit, three component commits,
dependency pins, builder package inventory, public-key fingerprints, unsigned
payload hashes, signed artifact hashes, exact qualification evidence and known
limitations. Signed source tags and an independently published verification key
are needed before claiming authenticated source releases; an ordinary GitHub tag
alone is not a cryptographic release-signing policy.

Publish only reviewed ROMMON/system images, public trust files, checksums,
receipts, sanitized test summaries, complete recursive source archives, license
notices and source-offer material required by included dependencies. Never upload
private keys, encrypted vaults, datastore disks, QEMU variable stores, shell
histories or complete unreviewed logs. GitHub's source zip omits submodules; use
`python3 tools/source-archive.py --output out/fractalcr-source.tar.gz` instead.

Release images must be the exact hashes tested. A newly rebuilt image does not
inherit an older image's qualification report. No workflow automatically signs
or publishes appliance images. Initial GitHub repositories contain source only.

Outstanding release gates include independent review, a complete image SBOM and
dependency vulnerability disposition, source/license compliance for upstream
binary bundles, fresh-builder reproduction, and hardware coverage. Format v1
does not enforce per-release anti-rollback. Key revocation is not a substitute
for a monotonic release-version policy. A/B power-loss-safe installation is also
not yet implemented. These limits must accompany any development prerelease.
