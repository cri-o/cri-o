#!/usr/bin/env bats

load helpers

function setup() {
    setup_test
    start_crio
}

function teardown() {
    cleanup_test
}

function run_crio_status() {
    run ${CRIO_STATUS_BINARY_PATH} $@
}

@test "status should fail if no subcommand is provided" {
    # when
    run_crio_status
    echo "$output"

    # then
    [ "$status" -eq 1 ]
}

@test "status should succeed to retrieve the config" {
    # when
    run_crio_status --socket=${CRIO_SOCKET} config
    echo "$output"

    # then
    [ "$status" -eq 0 ]
    [[ "$output" == *"[crio]"* ]]
}

@test "status should fail to retrieve the config with invalid socket" {
    # when
    run_crio_status --socket wrong.sock c
    echo "$output"

    # then
    [ "$status" -eq 1 ]
}

@test "status should succeed to retrieve the info" {
    # when
    run_crio_status --socket=${CRIO_SOCKET} info
    echo "$output"

    # then
    [ "$status" -eq 0 ]
    [[ "$output" == *"storage driver"* ]]
}

@test "status should fail to retrieve the info with invalid socket" {
    # when
    run_crio_status --socket wrong.sock i
    echo "$output"

    # then
    [ "$status" -eq 1 ]
}

@test "succeed to retrieve the container info" {
    # given
    run crictl runp "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod="$output"
    run crictl create "$pod" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    ctr="$output"
    run crictl start "$ctr"
    echo "$output"
    [ "$status" -eq 0 ]

    # when
    run_crio_status --socket=${CRIO_SOCKET} containers --id "$ctr"
    echo "$output"

    # then
    [ "$status" -eq 0 ]
    [[ "$output" == *"sandbox: $pod"* ]]
}

@test "should fail to retrieve the container info without ID" {
    # when
    run_crio_status --socket=${CRIO_SOCKET} containers
    echo "$output"

    # then
    [ "$status" -eq 1 ]
}

@test "should fail to retrieve the container with invalid socket" {
    # when
    run_crio_status --socket wrong.sock s
    echo "$output"

    # then
    [ "$status" -eq 1 ]
}
