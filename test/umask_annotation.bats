#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "check umask is changed" {
	create_runtime_with_allowed_annotation "umask" "io.kubernetes.cri-o.umask"
	start_crio
	# Run base container to ensure it creates at all
	pod_id=$(crictl runp <(jq '.annotations."io.kubernetes.cri-o.umask" = "077"' "$TESTDATA"/sandbox_config.json))

	# Run multi-container pod to ensure that they all work together
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)

	# Start the container by its ID
	crictl start "$ctr_id"

	# Confirm that the new umask is applied
	[[ $(crictl exec "$ctr_id" grep Umask /proc/1/status) == *"77"* ]]
}
