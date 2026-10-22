# Repository and trust boundaries

The integration repository is the release manifest. Its Git tree pins three
component commits, while `os/sources.json` pins upstream OE metadata. A tag in
the integration repository therefore identifies the complete first-party tree.

| Repository | Checkout path | Responsibility |
| --- | --- | --- |
| highenergymagic/fractalcr | root | Go management, integration, tests, release orchestration, documentation |
| highenergymagic/fractalcr-rommon | boot | Buildroot recovery monitor, CIP boot kernel, image verification |
| highenergymagic/fractalcr-supervisor | service | Shared PID 1, service graph, health and control protocol |
| highenergymagic/meta-fractalcr | os/meta-fractal | OE distro, machine, kernel and appliance recipes |

The component repositories are intentionally checked out at the existing paths.
Use the integration checkout to build and qualify them together. The layer
expects management and supervisor sources to be supplied by `os/tools/setup.sh`;
it is not yet a standalone generic distro layer. Cross-component changes require
component commits followed by an integration commit updating their gitlinks.
GitHub's automatically generated source zip does not include submodules.

```sh
gh auth setup-git
git clone --recurse-submodules https://github.com/highenergymagic/fractalcr.git
cd fractalcr
git submodule update --init --recursive
```

Firmware authenticates ROMMON's EFI image against enrolled Secure Boot keys.
ROMMON embeds an image-envelope public key and verifies the complete system
payload, copies it into a sealed memfd, and passes that descriptor to
`kexec_file_load`. Its kernel independently requires a trusted PE signature.
The system kernel contains the initramfs; neither an external initrd nor an
operator-supplied kernel command line is accepted. Optional Secure Boot keeps
both image checks, but removes firmware's authentication of ROMMON itself.

The supervisor is shared by ROMMON and the OS. Configuration, networking,
storage, Docker, SSH and the console are separate services. Persistent data and
startup configuration live on the datastore; the root image is read-only.

Privilege 15 is an appliance administrator and can run native Docker commands,
mount host paths, or explicitly start the privileged debug container. It is
equivalent to host administration, not a tenant boundary. Kata can isolate
workloads, but cannot protect the host against an administrator who deliberately
requests privileged host access. Physical serial/firmware access is trusted for
recovery and password reset.
