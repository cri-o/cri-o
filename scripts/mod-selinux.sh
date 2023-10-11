#!/bin/bash

set -euxo pipefail

cat <<EOF >local.te
module local 1.0;

require {
	type container_runtime_tmpfs_t;
	type container_t;
	class file { entrypoint execute map read };
}

#============= container_t ==============
allow container_t container_runtime_tmpfs_t:file entrypoint;
allow container_t container_runtime_tmpfs_t:file map;
allow container_t container_runtime_tmpfs_t:file { execute read };
EOF

# Compile the module.
checkmodule -M -m -o local.mod local.te

# Create the package.
semodule_package -o local.pp -m local.mod

# Load the module into the kernel.
sudo semodule -i local.pp
