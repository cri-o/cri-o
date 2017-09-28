#!/bin/bash

set -ex

# Set by 'runscript' role
DISTRO="$1"
ARTIFACTS="$2"

# Make sure it's not running
( systemctl is-active cri-o && systemctl stop cri-o ) || true

# FIXME: This should use the installed integration-tests package (not avail. on RHEL yet)
make test-binaries  # bin2img, copyimg, checkseccomp
# Override defaults in test/helpers.bash
export CRIO_ROOT=$(realpath "$PWD/..")
export CRIO_BINARY='/usr/bin/crio'
export CONMON_BINARY='/usr/libexec/crio/conmon'
export PAUSE_BINARY='/usr/libexec/crio/pause'
export CRIO_CNI_PLUGIN='/usr/libexec/cni/'

if [ "$DISTRO" == "RedHat" ] || [ "$DISTRO" == "Fedora" ]
then
    export STORAGE_OPTIONS='--storage-driver=overlay --storage-opt overlay.override_kernel_check=1'
else
    export export STORAGE_OPTIONS='--storage-driver=overlay'
fi

./test/test_runner.sh | tee /tmp/artifacts/integration_results.txt"
