#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "check /dev/shm is changed" {
	start_crio
	replace_config "enable_custom_shm_size" "true"
	restart_crio

	run crictl runp "$TESTDATA"/sandbox_config_shmsize.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config_shmsize.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" df -h | grep /dev/shm | awk '{print $2}'
	echo "$output"
	[[ "$output" == "16384" ]]
}
