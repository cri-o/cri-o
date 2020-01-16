#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
    setup_test
}

function teardown() {
    cleanup_test
}

@test "config dir should succeed" {
    # given
    setup_crio

    printf "[crio.runtime]\npids_limit = 1234\n" > "$CRIO_CONFIG_DIR"/00-default
    printf "[crio.runtime]\npids_limit = 5678\n" > "$CRIO_CONFIG_DIR"/01-overwrite

    # when
    start_crio_no_setup
    run ${CRIO_STATUS_BINARY_PATH} --socket=${CRIO_SOCKET} config
    echo "$output"

    # then
    [ "$status" -eq 0 ]
    [[ "$output" =~ "pids_limit = 5678" ]]
}

@test "config dir should fail with invalid option" {
    # given
    printf "[crio.runtime]\nlog_level = info\n" > "$CRIO_CONFIG"
    printf "[crio.runtime]\nlog_level = wrong\n" > "$CRIO_CONFIG_DIR"/00-default

    # when
    "$CRIO_BINARY_PATH" -c "$CRIO_CONFIG" -d "$CRIO_CONFIG_DIR" &> >(tee "$CRIO_LOG") || true
    RES=$(cat "$CRIO_LOG")

    # then
    [[ "$RES" =~ "unable to decode configuration" ]]
}
