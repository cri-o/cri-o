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
    id="$output"

    # when
    run crictl stats -o json
    echo "$output"
    [ "$status" -eq 0 ]

    # then
    jq -e '.stats[0].attributes.id = "'"$id"'"' <<< "$output"
    jq -e '.stats[0].cpu.timestamp > 0' <<< "$output"
    jq -e '.stats[0].cpu.usageCoreNanoSeconds.value > 0' <<< "$output"
    jq -e '.stats[0].memory.timestamp > 0' <<< "$output"
    jq -e '.stats[0].memory.workingSetBytes.value > 0' <<< "$output"
}

@test "container stats" {
    # given
    container2config=$(cat "$TESTDATA"/container_redis.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["name"] = ["podsandbox1-redis2"];obj["metadata"]["name"] = "podsandbox1-redis2"; json.dump(obj, sys.stdout)')
    echo "$container2config" > "$TESTDIR"/container_redis2.json
    run crictl runp "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    pod_id="$output"
    run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    ctr1_id="$output"
    run crictl create "$pod_id" "$TESTDIR"/container_redis2.json "$TESTDATA"/sandbox_config.json
    echo "$output"
    [ "$status" -eq 0 ]
    ctr2_id="$output"
    run crictl start "$ctr1_id"
    echo "$output"
    [ "$status" -eq 0 ]
    run crictl start "$ctr2_id"
    echo "$output"
    [ "$status" -eq 0 ]

    # when
    run crictl stats -o json "$ctr1_id"
    echo "$output"
    [ "$status" -eq 0 ]
    ctr1_stats_JSON="$output"

    run crictl stats -o json "$ctr2_id"
    echo "$output"
    [ "$status" -eq 0 ]
    ctr2_stats_JSON="$output"

    run echo $ctr1_stats_JSON | jq -e '.stats[0].memory.workingSetBytes.value'
    [ "$status" -eq 0 ]
    ctr1_memory_bytes="$output"
    run echo $ctr2_stats_JSON | jq -e '.stats[0].memory.workingSetBytes.value'
    [ "$status" -eq 0 ]
    ctr2_memory_bytes="$output"

    run echo $ctr1_memory_bytes != $ctr2_memory_bytes
    [ "$status" -eq 0 ]
}
