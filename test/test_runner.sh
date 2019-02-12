#!/usr/bin/env bash
set -e

TEST_USERNS=${TEST_USERNS:-}

cd "$(dirname "$(readlink -f "$BASH_SOURCE")")"

if [[ -n "$TEST_USERNS" ]]; then
    export UID_MAPPINGS="0:100000:100000"
    export GID_MAPPINGS="0:200000:100000"

    # Needed for RHEL
    if [[ -w /proc/sys/user/max_user_namespaces ]]; then
        echo 15000 > /proc/sys/user/max_user_namespaces
    fi
fi

# Load the helpers.
. helpers.bash

function execute() {
	>&2 echo "++ $@"
	eval "$@"
}

# Tests to run. Defaults to all.
TESTS=${@:-.}

# Run the tests.
execute time bats --tap $TESTS
