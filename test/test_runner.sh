#!/usr/bin/env bash
set -e

TEST_USERNS=${TEST_USERNS:-}

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
    eval "$@"
}

# Tests to run. Defaults to all.
TESTS=${*:-.}

# The number of parallel jobs to execute
export JOBS=${JOBS:-$(($(nproc --all) * 4))}

# Run the tests.
execute time bats --jobs "$JOBS" --tap "$TESTS"
