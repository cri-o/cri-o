#!/usr/bin/env bats

load helpers

IMAGE=kubernetes/pause

function teardown() {
	cleanup_test
}

@test "run container in pod with image ID" {
	start_ocid
	run ocic pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	sed -e "s/%VALUE%/$REDIS_IMAGEID/g" "$TESTDATA"/container_config_by_imageid.json > "$TESTDIR"/ctr_by_imageid.json
	run ocic ctr create --config "$TESTDIR"/ctr_by_imageid.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_ocid
}

@test "image pull" {
	start_ocid "" "" --no-pause-image
	run ocic image pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_images
	stop_ocid
}

@test "image pull and list by digest" {
	start_ocid "" "" --no-pause-image
	run ocic image pull nginx@sha256:4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	echo "$output"
	[ "$status" -eq 0 ]

	run ocic image list --quiet nginx@sha256:4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run ocic image list --quiet nginx@4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run ocic image list --quiet @4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run ocic image list --quiet 4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	cleanup_images
	stop_ocid
}

@test "image list with filter" {
	start_ocid "" "" --no-pause-image
	run ocic image pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic image list --quiet "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		run ocic image remove --id "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	run ocic image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		echo "$id"
		status=1
	done
	cleanup_images
	stop_ocid
}

@test "image list/remove" {
	start_ocid "" "" --no-pause-image
	run ocic image pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" != "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		run ocic image remove --id "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	run ocic image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" = "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		echo "$id"
		status=1
	done
	cleanup_images
	stop_ocid
}

@test "image status/remove" {
	start_ocid "" "" --no-pause-image
	run ocic image pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	run ocic image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" != "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		run ocic image status --id "$id"
		echo "$output"
		[ "$status" -eq 0 ]
		[ "$output" != "" ]
		run ocic image remove --id "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	run ocic image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" = "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		echo "$id"
		status=1
	done
	cleanup_images
	stop_ocid
}
