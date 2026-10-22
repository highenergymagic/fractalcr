#!/bin/bash
set -eo pipefail
cd /work/sources/openembedded-core
source oe-init-build-env /work/build >/dev/null
exec bitbake "$@"
