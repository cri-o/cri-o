#!/usr/bin/env bats

load helpers

function setup() {
	export CONTAINER_MANAGE_NS_LIFECYCLE=true
	setup_test
	start_crio
}

function teardown() {
	cleanup_test
}

@test "test infra ctr dropped" {
	temp_config=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["security_context"]["namespace_options"]["pid"] = 1; json.dump(obj, sys.stdout)')
	echo "$temp_config" > "$TESTDIR"/sandbox_no_infra.json
	run crictl runp "$TESTDIR"/sandbox_no_infra.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run "$CONTAINER_RUNTIME" list
	echo "$output"
	[ "$status" -eq 0 ]
	[[ ! "$output" =~ "$pod_id" ]]
}

@test "test infra ctr not dropped" {
	temp_config=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["security_context"]["namespace_options"]["pid"] = 0; json.dump(obj, sys.stdout)')
	echo "$temp_config" > "$TESTDIR"/sandbox_no_infra.json
	run crictl runp "$TESTDIR"/sandbox_no_infra.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	run "$CONTAINER_RUNTIME" list
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ "$pod_id" ]]
}
