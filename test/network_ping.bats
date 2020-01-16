#!/usr/bin/env bats

load helpers

function setup() {
    export CONTAINER_DEFAULT_SYSCTLS='net.ipv4.ping_group_range=0   2147483647'
    setup_test
}

function teardown() {
    stop_crio
    rm -r "$TESTDIR" || true
}

@test "Ping pod from the host" {
    start_crio
    run crictl runp "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"

    run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0  ]
    ctr_id="$output"

    ping_pod $ctr_id

    crictl rmp -f $pod_id
}

@test "Ping pod from another pod" {
    start_crio
    run crictl runp "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod1_id="$output"
    run crictl create "$pod1_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0  ]
    ctr1_id="$output"

    temp_sandbox_conf cni_test

    run crictl runp "$TESTDIR"/sandbox_config_cni_test.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod2_id="$output"
    run crictl create "$pod2_id" "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox_config_cni_test.json
    echo "$output"
    [ "$status" -eq 0  ]
    ctr2_id="$output"

    ping_pod_from_pod $ctr1_id $ctr2_id
    ping_pod_from_pod $ctr2_id $ctr1_id

    crictl rmp -f $pod1_id $pod2_id
}
