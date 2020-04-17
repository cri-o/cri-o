#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
    setup_test
    start_crio
}

function teardown() {
    cleanup_test
}

@test "stats" {
    # given
    run crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
    [ "$status" -eq 0 ]

    # when
    run crictl stats -o json
    echo "$output"
    [ "$status" -eq 0 ]

    # then
    JSON="$output"
    echo $JSON | jq -e '.stats[0].attributes.id != ""'
    [ "$status" -eq 0 ]

    echo $JSON | jq -e '.stats[0].cpu.timestamp > 0'
    [ "$status" -eq 0 ]

    echo $JSON | jq -e '.stats[0].cpu.usageCoreNanoSeconds.value > 0'
    [ "$status" -eq 0 ]

    echo $JSON | jq -e '.stats[0].memory.timestamp > 0'
    [ "$status" -eq 0 ]

    echo $JSON | jq -e '.stats[0].memory.workingSetBytes.value > 0'
    [ "$status" -eq 0 ]
}
