# Security policy

This project is under development. It has functional qualification and targeted
security tests; it has not passed an independent comprehensive security audit.
There are no production-supported release versions yet.

Report vulnerabilities privately using [GitHub's vulnerability report form](https://github.com/highenergymagic/fractalcr/security/advisories/new).
The component repositories also accept private reports through their Security
pages. Please include affected commit/image hashes, impact, reproduction steps
and relevant sanitized logs. Do not report vulnerabilities in public issues or
include credentials, signing keys or customer data. There is no guaranteed
response time or paid support commitment for this development preview.

See [the security review](docs/security-review.md), [trust boundaries](docs/architecture.md)
and [release policy](docs/releases.md). Keep proof-of-concept artifacts in
disposable VMs. Physical access and privilege 15 are trusted administration;
workload escape, unauthenticated SSH access, lower-privilege escalation and
unsigned boot are security boundaries.

GitHub-hosted jobs test source. They have no access to carbon, sascha, Docker
sockets on those hosts, or release keys. Public component checkouts need no deploy keys. Never supply signing keys to
fork PR workflows, use pull_request_target to execute PR code, or register the
existing build hosts as general untrusted self-hosted runners.
