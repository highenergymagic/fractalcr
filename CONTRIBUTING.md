# Contributing

Start from a recursive checkout of the integration repository. Read
[architecture](docs/architecture.md), [building](docs/building.md) and
[dependency maintenance](docs/dependencies.md).

For a component change, create a branch inside its checkout, commit and push it,
then update the corresponding gitlink in an integration branch. Include both
links in the integration PR. Do not merge a component change into a release
until the integration revision containing it passes relevant tests.

Run the application checks on Linux with the Go version in `go.mod`:

```sh
go test -mod=vendor ./cmd/... ./internal/... ./pkg/...
go vet -mod=vendor ./cmd/... ./internal/... ./pkg/...
python3 -m unittest discover -s tools/tests
```

Avoid recursive `./...` over the integration checkout: the OE layer contains Go
sources for separate upstream modules. `bash tools/check-services.sh` runs C tests in a builder
with the checkout mounted at `/src`. Use the VM qualification tools for runtime
changes. Keep generated keys, artifacts, caches and interactive disks out of Git.

The Python tooling suite needs `cryptography`; see [host prerequisites](docs/building.md).
For documentation changes, check relative links both in a recursive checkout
and on GitHub: links crossing a submodule boundary need the destination
repository URL. Use example accounts and explicit host placeholders rather than
maintainer-specific machines. Keep dated test evidence separate from user steps.

Use small commits describing the resulting behavior. Tests should cover the
failure or trust boundary being changed. Runtime claims must identify exact
image hashes; build success alone is not runtime qualification.

## Documentation standards

Write for an operator or contributor who has only the public source and release
artifacts. Use an appliance configuration-guide style:

- Begin with the task and its purpose. State the required privilege, command
  mode, hardware, and starting configuration before the procedure.
- Use numbered steps for ordered actions. Provide commands in copyable blocks,
  explain placeholders, and identify whether commands run on the appliance,
  in ROMMON, or on a build workstation.
- Follow configuration procedures with verification commands and the expected
  state. Include recovery steps when an operation can interrupt access or boot.
- Put destructive consequences beside the command that causes them. State
  restrictions as specific behavior, affected features, and operator actions.
- Keep operator procedures, developer architecture, and qualification references
  separate. Link to the relevant reference instead of repeating its detail.
- Use example accounts, documentation addresses, and explicit host/path
  placeholders. Omit private machine names, conversation references, internal
  iteration numbers, planning diaries, and unrelated product comparisons.
- Describe the current implementation directly. Avoid phrases such as "now
  supports" or "in this change" outside versioned release notes. Identify the
  tested revision and image in qualification reports, not private evidence paths.
- Do not invent output, hardware properties, compatibility, or completed tests.
  Keep known restrictions visible in release status and the relevant procedure.
- Check headings, command modes, examples, and links before publishing. Links
  between component repositories must use their public GitHub URLs.
