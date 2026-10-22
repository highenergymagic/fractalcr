# Contributing

Start from a recursive checkout of the integration repository. Read
[architecture](docs/architecture.md), [building](docs/building.md) and
[dependency maintenance](docs/dependencies.md).

For a component change, create a branch inside its checkout, commit and push it,
then update the corresponding gitlink in an integration branch. Include both
links in the integration PR. Do not merge a component change into a release
until the integration revision containing it passes relevant tests.

Run `go test -mod=vendor ./...`, `go vet -mod=vendor ./...`, and
`python3 -m unittest discover -s tools/tests`. Linux-specific management tests
must also run on Linux. `bash tools/check-services.sh` runs C tests in a builder
with the checkout mounted at `/src`. Use the VM qualification tools for runtime
changes. Keep generated keys, artifacts, caches and interactive disks out of Git.

Use small commits describing the resulting behavior. Tests should cover the
failure or trust boundary being changed. Runtime claims must identify exact
image hashes; build success alone is not runtime qualification.
