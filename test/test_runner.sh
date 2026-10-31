#!/usr/bin/env bash
set -xe

TEST_USERNS=${TEST_USERNS:-}
TEST_KEEP_ON_FAILURE=${TEST_KEEP_ON_FAILURE:-}
# Directory to write JUnit reports, outputs of retried tests and the list of
# tests leaking processes to.
export TEST_REPORT_DIR=${TEST_REPORT_DIR:-}

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

# Tests to run. Default is "." (i.e. the current directory).
TESTS=("${@:-.}")

# Run the parallel and serial tests by default.
RUN_PARALLEL=1
RUN_SERIAL=1
# Additional arguments for the parallel bats run.
PARALLEL_ARGS=()

# TEST_SHARD selects a part of the test suite, which allows CI to split it
# across multiple jobs:
# - "<index>/<total>" runs a subset of the parallel tests. The parallel tests
#   of all files get distributed round-robin, which splits the tests of every
#   file evenly across the shards, so slow files do not need to be balanced.
# - "serial" runs all serial tests.
TEST_SHARD=${TEST_SHARD:-}
if [[ -n "$TEST_SHARD" ]]; then
    if [[ $# -ne 0 || "$RUN_CRITEST" == "1" ]]; then
        echo "TEST_SHARD can't be combined with test arguments or RUN_CRITEST" >&2
        exit 1
    fi

    if [[ "$TEST_SHARD" == serial ]]; then
        RUN_PARALLEL=0
    else
        RUN_SERIAL=0
        SHARD_INDEX=${TEST_SHARD%/*}
        SHARD_TOTAL=${TEST_SHARD#*/}
        if ! [[ "$TEST_SHARD" =~ ^[0-9]+/[0-9]+$ &&
            "$SHARD_INDEX" -ge 1 && "$SHARD_INDEX" -le "$SHARD_TOTAL" ]]; then
            echo "Invalid TEST_SHARD: $TEST_SHARD, expected <index>/<total> or serial" >&2
            exit 1
        fi

        # Select every SHARD_TOTAL-th parallel test, which are the ones not
        # tagged as crio:serial, as "<file>\t<name>".
        mapfile -t SHARD_TESTS < <(
            awk -v idx="$SHARD_INDEX" -v total="$SHARD_TOTAL" '
                FNR == 1 { serial_file = 0; serial_test = 0 }
                /^# bats file_tags=.*crio:serial/ { serial_file = 1 }
                /^# bats test_tags=.*crio:serial/ { serial_test = 1 }
                /^@test / {
                    if (!serial_file && !serial_test && count++ % total == idx - 1) {
                        name = $0
                        sub(/^@test "/, "", name)
                        sub(/" \{$/, "", name)
                        print FILENAME "\t" name
                    }
                    serial_test = 0
                }' ./*.bats
        )
        if [[ ${#SHARD_TESTS[@]} -eq 0 ]]; then
            echo "No tests left for shard $TEST_SHARD" >&2
            exit 1
        fi
        mapfile -t TESTS < <(printf '%s\n' "${SHARD_TESTS[@]%%$'\t'*}" | uniq)
        # Run only the selected tests of the files by matching their names.
        PARALLEL_ARGS+=(--filter "^($(printf '%s\n' "${SHARD_TESTS[@]#*$'\t'}" |
            sed 's/[][\.*+?(){}|^$]/\\&/g' | paste -sd '|'))\$")
        echo "Running shard $TEST_SHARD with ${#SHARD_TESTS[@]} tests of the files: ${TESTS[*]}"
    fi
fi

# Only run critest if requested
if [[ "$RUN_CRITEST" == "1" ]]; then
    TESTS=(critest.bats)
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

# The number of parallel jobs to execute tests
export JOBS=${JOBS:-$(($(nproc --all) * 2))}
# The maximum number of additional attempts that will be made on a failed test
# before it is finally considered failed, see BATS_TEST_RETRIES in helpers.bash.
export TEST_RETRIES=${TEST_RETRIES:-0}

bats --version

# The --allow-empty-suite flag got introduced in bats v1.14.0, while the CI
# machine images may still ship an older version.
BATS_ARGS=()
if bats --help 2>&1 | grep -qF -- --allow-empty-suite; then
    BATS_ARGS+=(--allow-empty-suite)
fi

# Run bats with the arguments $2... and write its report to the directory $1
# in TEST_REPORT_DIR, if set. Only the outputs of failed attempts of retried
# tests ("<number>-try<N>-<test>.log") get kept, with the characters artifact
# uploads do not allow replaced, since the JUnit report contains the rest.
function run_bats() {
    local dir="$TEST_REPORT_DIR/$1" status=0 file target
    shift
    if [[ -z "$TEST_REPORT_DIR" ]]; then
        execute bats "$@"
        return
    fi

    mkdir -p "$dir/outputs"
    execute bats --report-formatter junit --output "$dir" \
        --gather-test-outputs-in "$dir/outputs" "$@" || status=$?
    find "$dir/outputs" -type f ! -name '[0-9]*-try[0-9]*-*' -delete
    for file in "$dir"/outputs/*; do
        target="${file%/*}/$(basename "$file" | tr '":<>|*?' _)"
        if [[ -f "$file" && "$file" != "$target" ]]; then
            mv "$file" "$target"
        fi
    done
    return "$status"
}

# Run the tests.
if [[ "$RUN_PARALLEL" == "1" ]]; then
    run_bats parallel --jobs "$JOBS" --tap "${BATS_ARGS[@]}" "${TESTS[@]}" --filter-tags '!crio:serial' "${PARALLEL_ARGS[@]}"
fi
if [[ "$RUN_SERIAL" == "1" ]]; then
    run_bats serial --tap "${BATS_ARGS[@]}" "${TESTS[@]}" --filter-tags 'crio:serial'
fi
