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
elif [[ $# -eq 0 && "${CRIO_TEST_SELECTION:-1}" == "1" ]]; then
    # Change based test selection, see hack/ci/select-bats.sh. The selection
    # can be precomputed by the caller via CRIO_BATS_FILES (space separated
    # file names or "ALL"). It falls back to the full suite whenever the
    # change under test cannot be determined.
    if [[ -n "${CRIO_BATS_FILES:-}" ]]; then
        SELECTION=$CRIO_BATS_FILES
    else
        SELECTION=$(../hack/ci/select-bats.sh | tr '\n' ' ')
    fi
    SELECTION=$(echo "$SELECTION" | xargs)
    if [[ "$SELECTION" == "ALL" ]]; then
        echo "Running the full integration test suite"
    elif [[ -z "$SELECTION" ]]; then
        echo "No integration test is affected by the change, nothing to run"
        exit 0
    else
        read -ra TESTS <<<"$SELECTION"
        echo "Running the selected integration tests: ${TESTS[*]}"
    fi
fi

# The number of parallel jobs to execute tests
export JOBS=${JOBS:-$(($(nproc --all) * 2))}
# The maximum number of additional attempts that will be made on a failed test before it is finally considered failed.
# https://bats-core.readthedocs.io/en/stable/writing-tests.html#special-variables
export BATS_TEST_RETRIES=1

bats --version

# The --allow-empty-suite flag got introduced in bats v1.14.0, while the CI
# machine images may still ship an older version.
BATS_ARGS=()
if bats --help 2>&1 | grep -qF -- --allow-empty-suite; then
    BATS_ARGS+=(--allow-empty-suite)
fi

# Write JUnit reports to BATS_REPORT_DIR (if set) in addition to the TAP
# output, so that CI systems can record per test results and durations.
BATS_REPORT_DIR=${BATS_REPORT_DIR:-}
if [[ -n "$BATS_REPORT_DIR" ]] && ! bats --help 2>&1 | grep -qF -- --report-formatter; then
    echo "bats does not support --report-formatter, not writing JUnit reports"
    BATS_REPORT_DIR=
fi

function report_args() {
    if [[ -n "$BATS_REPORT_DIR" ]]; then
        mkdir -p "$BATS_REPORT_DIR/$1"
        echo --report-formatter junit --output "$BATS_REPORT_DIR/$1"
    fi
}

function collect_reports() {
    local phase
    for phase in parallel serial; do
        if [[ -n "$BATS_REPORT_DIR" && -f "$BATS_REPORT_DIR/$phase/report.xml" ]]; then
            mv "$BATS_REPORT_DIR/$phase/report.xml" "$BATS_REPORT_DIR/junit_bats_$phase.xml"
            rmdir "$BATS_REPORT_DIR/$phase" 2>/dev/null || true
        fi
    done
}
trap collect_reports EXIT

# Run the tests.
# shellcheck disable=SC2046
execute bats --jobs "$JOBS" --tap "${BATS_ARGS[@]}" $(report_args parallel) "${TESTS[@]}" --filter-tags '!crio:serial'
# shellcheck disable=SC2046
execute bats --tap "${BATS_ARGS[@]}" $(report_args serial) "${TESTS[@]}" --filter-tags 'crio:serial'
