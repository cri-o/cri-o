#!/usr/bin/env bats

load helpers

@test "Check for valid pod netns CIDR" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	if [ ! -f "$OCID_CNI_PLUGIN/bridge" ]; then
		skip "missing CNI bridge plugin, please install it"
	fi

	if [ ! -f "$OCID_CNI_PLUGIN/host-local" ]; then
		skip "missing CNI host-local IPAM, please install it"
	fi

	prepare_network_conf $POD_CIDR

	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	check_pod_cidr $pod_id

	cleanup_pods
	cleanup_network_conf
	stop_ocid
}

@test "Ping pod netns from the host" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	if [ ! -f "$OCID_CNI_PLUGIN/bridge" ]; then
		skip "missing CNI bridge plugin, please install it"
	fi

	if [ ! -f "$OCID_CNI_PLUGIN/host-local" ]; then
		skip "missing CNI host-local IPAM, please install it"
	fi

	prepare_network_conf $POD_CIDR

	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"

	ping_pod $pod_id

	cleanup_pods
	cleanup_network_conf
	stop_ocid
}
