#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

# PR#59
@test "pod release name on remove" {
	# this test requires docker, thus it can't yet be run in a container
	if [ "$TRAVIS" = "true" ]; then # instead of $TRAVIS, add a function is_containerized to skip here
		skip "cannot yet run this test in a container, use sudo make localintegration"
	fi

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]
	echo "$output"
	id="$output"
	run ocic pod stop --id "$id"
	[ "$status" -eq 0 ]
	sleep 1 # FIXME: there's a race between container kill and delete below
	run ocic pod remove --id "$id"
	[ "$status" -eq 0 ]
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]
	stop_ocid
	cleanup_pods
}
