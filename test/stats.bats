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
    python -c 'import json,sys;obj=json.load(sys.stdin);obj["name"] = ["podsandbox1-sleep2"];obj["metadata"]["name"] = "podsandbox1-sleep2"; json.dump(obj, sys.stdout)' \
        < "$TESTDATA"/container_sleep.json > "$TESTDIR"/container_sleep2.json

    pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

    ctr1_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
    crictl start "$ctr1_id"

    ctr2_id=$(crictl create "$pod_id" "$TESTDIR"/container_sleep2.json "$TESTDATA"/sandbox_config.json)
    crictl start "$ctr2_id"

    # when
    json=$(crictl stats -o json "$ctr1_id")
    echo "$json"
    jq -e '.stats[0].attributes.id == "'"$ctr1_id"'"' <<< "$json"
    ctr1_mem=$(jq -e '.stats[0].memory.workingSetBytes.value' <<< "$json")

    json=$(crictl stats -o json "$ctr2_id")
    echo "$json"
    jq -e '.stats[0].attributes.id == "'"$ctr2_id"'"' <<< "$json"
    ctr2_mem=$(jq -e '.stats[0].memory.workingSetBytes.value' <<< "$json")

    # Assuming the two containers can't have exactly same memory usage
    echo "checking $ctr1_mem != $ctr2_mem"
    [ "$ctr1_mem" != "$ctr2_mem" ]
}
