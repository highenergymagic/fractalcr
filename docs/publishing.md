# Publishing development releases

The **Release firmware images** GitHub Actions workflow builds the pinned source,
tests the firmware, packages installation media, and publishes a development-signed
prerelease. It runs on manual dispatch and version tags. Ordinary branch pushes
do not trigger firmware builds.

## Prerequisites

- Repository access to dispatch workflows and push release tags.
- Committed integration source with matching ROMMON, supervisor, and OE-layer
  revisions. Cross-component changes are committed in the component repositories
  before their gitlinks are updated in the integration repository.
- A Linux x86-64 runner with Docker, Python 3, at least 16 GiB RAM, sufficient
  build storage, and KVM. See [Build requirements](building.md).

The default runner is GitHub-hosted `ubuntu-24.04`. The repository variable
`FIRMWARE_BUILD_RUNNER` selects another runner label. The workflow generates
development keys inside its build volumes and uses public component checkouts.

## Publish a development snapshot

1. Open the repository's **Actions** tab and select **Release firmware images**.
2. Select **Run workflow** and choose the branch containing the desired source.
3. Follow the build and qualification jobs. After they pass, the workflow publishes
   `dev-YYYYMMDD-RUN-ATTEMPT` for the selected commit.
4. Open the release and check its asset list and qualification reports.

A manual dispatch publishes a prerelease when successful. A failed build leaves
diagnostic artifacts in the Actions run and does not create a release.

## Publish a versioned release

The example below uses version `1.0.0`.

1. Set the integration repository's `VERSION` file to `1.0.0`.
2. Regenerate the supervisor version header from the integration checkout:

   ```sh
   python3 tools/version-metadata.py --output service/release.h
   ```

3. Commit and push the supervisor header, then commit its updated gitlink and
   `VERSION` in the integration repository.
4. Run **Release firmware images** on that branch and review the resulting
   installation images and test reports.
5. Tag the selected integration commit `v1.0.0` and push the tag. The tag-triggered
   workflow validates the version, rebuilds the firmware, and publishes a
   development-signed prerelease after qualification.

## Build results

The workflow checks ROMMON recovery, system startup, NoCloud provisioning,
console and SSH management, and both USB boot policies. Its
`management-qualification.json` report identifies the tested system image.
Physical hardware compatibility is tracked separately in
[Release status](development-status.md).

The downloadable assets include USB and qcow2 images, public trust files,
checksums, source, manifests, package metadata, and qualification reports.
Build volumes and interactive test disks contain private keys or generated
machine state and are excluded from the release package. Each workflow build
creates a new signing identity, including a version-tag rebuild of an already
tested source revision.

See [Software releases and image signing](releases.md) for installation and
trust behavior, and [Firmware qualification](../os/docs/cli-and-container-qualification.md)
for the test procedures.
