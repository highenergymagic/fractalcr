# Public development-preview review

Reviewed October 24, 2026. This review concerns publishing source, not approving
firmware for production use. The [development status](development-status.md),
[conformance audit](ios-management-conformance.md) and [release gates](releases.md)
remain applicable.

Before publication, all four GitHub repositories were mirrored on the Linux
build host and scanned with Gitleaks 8.30.1 across all fetched refs. The scanner
download matched its release checksum. Initial histories contained 51 integration,
7 ROMMON, 3 supervisor and 13 OE-layer commits. No secrets were detected. A second
pass enabled archive and encoded-content inspection to depth three. It flagged
2,410 matches inside the OE layer's dependency archives: Cargo/Go checksums and
public upstream source/test fixtures, including test private keys. Every affected
non-checksum crate file was byte-compared with its published crate, whose archive
hash matched the pinned Cargo checksum (eight crates; 111 affected files including
checksum files and patches). No unresolved credential findings remained. These
public test fixtures are not FractalCR signing or SSH host keys. GitHub issue,
issue-comment, pull-request review-comment and release metadata were also scanned.
Scanner results are evidence of these checks, not proof that no private material
can exist. Reports are retained outside the published repositories.

Publication changes add security policies to every component, link GitHub's
private vulnerability-report form, remove private checkout credentials from CI,
and document the development-preview status. Component checkouts remain pinned
to exact commits. No release images or private build outputs are published by
this change; GitHub source ZIP downloads still omit submodules.

Private vulnerability reporting is enabled separately on each repository when
its visibility changes. The three obsolete read-only integration deploy keys
and corresponding Actions secrets are retired after public checkout works.
There are no self-hosted runners registered on the integration repository.
Signing keys, build caches and interactive datastore images remain on private
development machines.

Source publication does not start a firmware build, create a release tag, replace
the local launcher image, or close the outstanding startup/reload regression.
GitHub's normal source checks continue to run on pushes to `main`.
