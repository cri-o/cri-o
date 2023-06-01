#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "check /dev/shm is changed" {
	create_runtime_with_allowed_annotation "shmsize" "io.kubernetes.cri-o.ShmSize"
	start_crio
	# Run base container to ensure it creates at all
	pod_id=$(crictl runp <(jq '.annotations."io.kubernetes.cri-o.ShmSize" = "16Mi"' "$TESTDATA"/sandbox_config.json))

	# Run multi-container pod to ensure that they all work together
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start the container by its ID
	crictl start "$ctr_id"

	# Confirm that the new size is applied
	df=$(crictl exec --sync "$ctr_id" df | grep /dev/shm)
	[[ "$df" == *'16384'* ]]
}

@test "check /dev/shm fails with incorrect values" {
	create_runtime_with_allowed_annotation "shmsize" "io.kubernetes.cri-o.ShmSize"
	start_crio
	# Ensure pod fails if /dev/shm size is negative
	run ! crictl runp <(jq '.annotations."io.kubernetes.cri-o.ShmSize" = "-1"' "$TESTDATA"/sandbox_config.json)

	# Ensure pod fails if /dev/shm size is not a size
	run ! crictl runp <(jq '.annotations."io.kubernetes.cri-o.ShmSize" = "notanumber"' "$TESTDATA"/sandbox_config.json)
}
