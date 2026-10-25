# Management binary advisory review

`tools/security/check-go-findings.py` evaluates the management binary's
`govulncheck` report. It retains every finding in the JSON artifact and fails on
reachable symbols except the exact, source-hash-checked cases below. Changes to
the advisory ID, module version, package, symbol or source require a new review.
This does not exempt Docker Engine or the rest of the Docker client module.

Reviewed 2026-10-24:

* [GO-2026-4887 / upstream advisory](https://github.com/moby/moby/security/advisories/GHSA-x744-4wpc-v9h2)
  concerns the daemon forwarding oversized request bodies to authorization plugins.
* [GO-2026-4883 / upstream advisory](https://github.com/moby/moby/security/advisories/GHSA-pxq6-2prw-chj9)
  concerns the daemon comparing requested and approved plugin privileges.

Both Go reports are unreviewed module-wide reports without vulnerable-symbol
precision. The management trace reaches only `pkg/namesgenerator`'s `init` and
`GetRandomName` through Buildx. That package consists of static adjective/name
lists and random string selection; it contains neither daemon operation.
Those two symbols at `github.com/docker/docker v28.5.2+incompatible` are not
affected. The source hash in the check pins the reviewed implementation.

The separately built Docker daemon is pinned to 29.9.2 in the OE layer. Upstream
lists 29.3.1 as the fixed Engine version for both advisories. Engine/rootfs scans
remain separate from this management-binary review.
