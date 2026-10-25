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

## Publication controls

Source and release reviews must check for credentials, private signing material,
customer data, and generated datastore or firmware-variable images. Upstream
test fixtures must be distinguished from deployment credentials and verified
against pinned upstream sources before any exception is accepted.

CI uses public component checkouts. Development build caches contain explicitly
selected dependency and compiler outputs; keys and deployment state are excluded.
The [release policy](releases.md) defines the assets and qualification required
for firmware publication.
