#!/usr/bin/env bash
set -e

TEST_USERNS=${TEST_USERNS:-}
TEST_KEEP_ON_FAILURE=${TEST_KEEP_ON_FAILURE:-}

cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")"

if [[ -n "$TEST_USERNS" ]]; then
    echo "Enabled user namespace testing"
    export \
        CONTAINER_UID_MAPPINGS="0:100000:100000" \
        CONTAINER_GID_MAPPINGS="0:200000:100000"

    # Needed for RHEL
    if [[ -w /proc/sys/user/max_user_namespaces ]]; then
        echo 15000 >/proc/sys/user/max_user_namespaces
    fi
fi

# Load the helpers.
. helpers.bash

function execute() {
    echo >&2 ++ "$@"
    time "$@"
}

# Tests to run. Default is "." (i.e. the current directory).
TESTS=("${@:-.}")

# The number of parallel jobs to execute tests
export JOBS=${JOBS:-$(($(nproc --all) * 4))}

# Run the tests.
execute bats --jobs "$JOBS" --tap "${TESTS[@]}"
# Set this var to run irqbalance tests
export TEST_SERIAL="Yes"
execute bats --tap ./irqbalance.bats
