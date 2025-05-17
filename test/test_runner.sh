#!/usr/bin/env bash
set -xe

TEST_USERNS=${TEST_USERNS:-}
TEST_KEEP_ON_FAILURE=${TEST_KEEP_ON_FAILURE:-}

if [ -n "$GOCOVERDIR" ]; then
    # It's used to make coverage profiles. https://go.dev/doc/build-cover
    mkdir -p "$GOCOVERDIR"
fi

cd "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")"

if [[ "$TEST_USERNS" == "1" ]]; then
    echo "Enabled user namespace testing"
    export \
        CONTAINER_UID_MAPPINGS="0:100000:100000" \
        CONTAINER_GID_MAPPINGS="0:200000:100000"

    # Needed for RHEL
    if [[ -w /proc/sys/user/max_user_namespaces ]]; then
        echo 15000 >/proc/sys/user/max_user_namespaces
    fi
fi

# Preload images.
(
    . common.sh
    get_images
)

function execute() {
    echo >&2 ++ "$@"
    time "$@"
}

# Tests to run. Default is "." (i.e. the current directory).
TESTS=("${@:-.}")

# Only run critest if requested
if [[ "$RUN_CRITEST" == "1" ]]; then
    TESTS=(critest.bats)
fi

# The number of parallel jobs to execute tests
export JOBS=${JOBS:-$(nproc --all)}

bats --version

# Run the tests.
execute bats --jobs "$JOBS" --tap "${TESTS[@]}"
