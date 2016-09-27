#!/usr/bin/env bats

load helpers

function teardown() {
	stop_ocid
	cleanup_test
}

# PR#59
@test "pod release name on remove" {
	skip "cannot be run in a container yet"

	start_ocid
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]
	id="$output"
	run ocic pod stop --id "$id"
	[ "$status" -eq 0 ]
	sleep 5 # FIXME: there's a race between container kill and delete below
	run ocic pod remove --id "$id"
	[ "$status" -eq 0 ]
	run ocic pod create --config "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 0 ]

	# TODO: cleanup all the stuff from runc, meaning list pods and stop remove them
	# pod list + pod stop + pod remove in cleanup
}
