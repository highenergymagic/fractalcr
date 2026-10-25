# Source distribution and reporting

FractalCR source is published in the integration repository and three pinned
component repositories. See [Repository architecture](architecture.md) for the
repository map and recursive checkout procedure.

## Source packages

The release `source.tar.gz` includes the component revisions and a file-hash
manifest. Generate this package from a clean recursive checkout:

```sh
python3 tools/source-archive.py --output out/fractalcr-source.tar.gz
```

The exporter rejects uncommitted changes, mismatched component revisions, and
private-key material. GitHub's automatic source ZIP does not include submodules.

## Security reporting

Use [private vulnerability reporting](../SECURITY.md) for security findings.
Ordinary bugs can be reported through the integration repository's issue tracker.
Include the source revision or image hash and sanitized reproduction details.

## Source and build artifacts

The source archive contains tracked files from the integration repository and
its pinned components. CI caches retain selected dependency and compiler outputs.
Signing keys, datastore images, and firmware-variable stores are excluded from
the source archive and cache paths.

Build volumes and VM test directories contain generated machine state and may
contain private test credentials. The firmware release package selects its
downloadable assets separately; see [Publishing development releases](publishing.md).
