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
	id=$(crictl run "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)

	# when
	output=$(crictl stats -o json)

	# then
	jq -e '.stats[0].attributes.id = "'"$id"'"' <<< "$output"
	jq -e '.stats[0].cpu.timestamp > 0' <<< "$output"
	jq -e '.stats[0].cpu.usageCoreNanoSeconds.value > 0' <<< "$output"
	jq -e '.stats[0].memory.timestamp > 0' <<< "$output"
	jq -e '.stats[0].memory.workingSetBytes.value > 0' <<< "$output"
}

@test "container stats" {
	# given
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	ctr1_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr1_id"

	ctr2_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
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

	# Test if the label filtering works
	[ "$(crictl stats | wc -l)" == 3 ]
	[ "$(crictl stats --label tier=backend | wc -l)" == 2 ]
}

@test "pod stats" {
	# given
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"

	# when
	json=$(crictl statsp --id "$pod_id" -o json)
	echo "$json"
	jq -e '.stats[0].attributes.id == "'"$pod_id"'"' <<< "$json"

	# Check that the container is listed in the statsp response
	jq -e '.stats[0].linux.containers[0].attributes.id == "'"$ctr_id"'"' <<< "$json"
}
