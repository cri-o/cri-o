#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
	start_crio
}

function teardown() {
	cleanup_test
}

function run_userns_container_test() {
	# Create sandbox config with userns options using jq
	jq '.linux.security_context.namespace_options.userns_options = {
		"mode": 0,
		"uids": [{"container_id": 0, "host_id": 100000, "length": 100000}],
		"gids": [{"container_id": 0, "host_id": 200000, "length": 100000}]
	}' "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_userns.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_userns.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_userns.json)
	crictl start "$ctr_id"
	state=$(crictl inspect "$ctr_id")
	pid=$(echo "$state" | jq .info.pid)
	grep 100000 /proc/"$pid"/uid_map
	grep 200000 /proc/"$pid"/gid_map
}

@test "ctr_userns run container" {
	run_userns_container_test
}

@test "ctr_userns run container with drop_infra_ctr disabled" {
	CONTAINER_DROP_INFRA_CTR=false restart_crio
	run_userns_container_test
}

function run_userns_restart_test() {
	# Create sandbox config with userns options using jq
	jq '.linux.security_context.namespace_options.userns_options = {
		"mode": 0,
		"uids": [{"container_id": 0, "host_id": 100000, "length": 100000}],
		"gids": [{"container_id": 0, "host_id": 200000, "length": 100000}]
	}' "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_userns.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_userns.json)
	restart_crio
	crictl inspectp "$pod_id" | jq -e '.status.state == "SANDBOX_READY"'
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_userns.json)
	crictl start "$ctr_id"
	state=$(crictl inspect "$ctr_id")
	pid=$(echo "$state" | jq .info.pid)
	grep 100000 /proc/"$pid"/uid_map
	grep 200000 /proc/"$pid"/gid_map
}

# This test follows a similar pattern to the one above but adds
# an extra step of crio restart.
@test "ctr_userns mappings persist after crio restart" {
	run_userns_restart_test
}

@test "ctr_userns mappings persist after crio restart with drop_infra_ctr disabled" {
	CONTAINER_DROP_INFRA_CTR=false restart_crio
	run_userns_restart_test
}

function run_userns_kill_restart_test() {
	# Create sandbox config with userns options using jq
	jq '.linux.security_context.namespace_options.userns_options = {
		"mode": 0,
		"uids": [{"container_id": 0, "host_id": 100000, "length": 100000}],
		"gids": [{"container_id": 0, "host_id": 200000, "length": 100000}]
	}' "$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_userns.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox_userns.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_userns.json)
	crictl start "$ctr_id"

	restart_crio

	crictl stop "$ctr_id"
	runtime delete -f "$ctr_id"
	crictl rm "$ctr_id"

	new_ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDIR"/sandbox_userns.json)
	crictl start "$new_ctr_id"

	state=$(crictl inspect "$new_ctr_id")
	pid=$(echo "$state" | jq .info.pid)
	grep 100000 /proc/"$pid"/uid_map
	grep 200000 /proc/"$pid"/gid_map
}

@test "ctr_userns mappings persist after container kill and restart" {
	run_userns_kill_restart_test
}

@test "ctr_userns mappings persist after container kill and restart with drop_infra_ctr disabled" {
	CONTAINER_DROP_INFRA_CTR=false restart_crio
	run_userns_kill_restart_test
}
