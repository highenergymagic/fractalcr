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

CI uses isolated workers and public component checkouts. Production signing
credentials must remain outside CI. Never supply signing keys to fork workflows,
execute pull-request code through `pull_request_target`, or give untrusted
contributions access to privileged build hosts.
