#!/usr/bin/env bats
# vim: set syntax=sh:

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "image fs info with default settings should return matching container_filesystem and image_filesystem" {
	start_crio
	output=$(crictl imagefsinfo)
	[ "$output" != "" ]

	container_output=$(jq -e '.status.containerFilesystems[0]' <<< "$output")
	image_output=$(jq -e '.status.imageFilesystems[0]' <<< "$output")
	# if these are the same we can directly compare.
	[ "$container_output" = "$image_output" ]
}

@test "image fs info with imagestore set should return different filesystems" {
	CONTAINER_IMAGESTORE="$TESTDIR/imagestore" start_crio
	output=$(crictl imagefsinfo)
	[ "$output" != "" ]

	container_output=$(jq -e '.status.containerFilesystems[0]' <<< "$output")
	image_output=$(jq -e '.status.imageFilesystems[0]' <<< "$output")
	[ "$container_output" != "$image_output" ]
}

# This is the only test we have for integration tests
# Integration tests overwrite locations for graphroot
@test "testing metrics for container_storage with graphroot different from default" {

	HOST="127.0.0.1"
	PORT=$(free_port)

	CONTAINER_ENABLE_METRICS=true CONTAINER_METRICS_PORT=$PORT start_crio
	metric_type=$(curl -sf "http://${HOST}:${PORT}/metrics" | grep container_storage | awk '{print $2}')
	[ "$metric_type" == 2 ]
}
