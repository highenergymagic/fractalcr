# Security policy

This project is under development. It has functional qualification and targeted
security tests; it has not passed an independent comprehensive security audit.
There are no production-supported release versions yet.

While repositories are private, report findings through a private repository
issue available to the owner and collaborators. Do not publish credentials,
private keys, customer data or weaponized reproductions. Before making these
repositories public, establish a private vulnerability-reporting channel and
publish its details here.

See [the security review](docs/security-review.md), [trust boundaries](docs/architecture.md)
and [release policy](docs/releases.md). Keep proof-of-concept artifacts in
disposable VMs. Physical access and privilege 15 are trusted administration;
workload escape, unauthenticated SSH access, lower-privilege escalation and
unsigned boot are security boundaries.

GitHub-hosted jobs test source. They have no access to carbon, sascha, Docker
sockets on those hosts, or release keys. Private component checkout uses separate
read-only deploy keys scoped to individual repositories. Never expose those keys
to fork PR workflows, use pull_request_target to execute PR code, or register the
existing build hosts as general untrusted self-hosted runners.
