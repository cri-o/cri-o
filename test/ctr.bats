#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

@test "ctr remove" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]
	echo "$output"
	pod_id="$output"
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run ocic ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr remove --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod stop --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	stop_ocid
	cleanup_pods
}

@test "ctr lifecycle" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]
	echo "$output"
	pod_id="$output"
	run ocic pod list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr create --config "$TESTDATA"/container_redis.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr start --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr stop --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr status --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr remove --id "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod stop --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod remove --id "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic pod list
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic ctr list
	echo "$output"
	[ "$status" -eq 0 ]
	stop_ocid
	cleanup_pods
}
