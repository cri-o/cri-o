#!/usr/bin/env bash

cat > /dev/stderr << EOF
WARNING: device mapper is obsoleted and is not compiled in.

If you see this from a build, please modify it to not use
this ($0) file, as it will be removed from the sources soon!
EOF

# TODO: remove this file once https://github.com/containers/storage/pull/1622
# is merged and a new c/storage is vendored here, and all build systems stop
# using this file.
echo exclude_graphdriver_devicemapper
