#!/usr/bin/env bats

load helpers

IMAGE=kubernetes/pause

function teardown() {
	cleanup_test
}

@test "run container in pod with image ID" {
	start_crio
	run crioctl pod run --config "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	sed -e "s/%VALUE%/$REDIS_IMAGEID/g" "$TESTDATA"/container_config_by_imageid.json > "$TESTDIR"/ctr_by_imageid.json
	run crioctl ctr create --config "$TESTDIR"/ctr_by_imageid.json --pod "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_crio
}

@test "image pull" {
	start_crio "" "" --no-pause-image
	run crioctl image pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_images
	stop_crio
}

@test "image pull and list by digest" {
	start_crio "" "" --no-pause-image
	run crioctl image pull nginx@sha256:4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	echo "$output"
	[ "$status" -eq 0 ]

	run crioctl image list --quiet nginx@sha256:4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run crioctl image list --quiet nginx@4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run crioctl image list --quiet @4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	run crioctl image list --quiet 4aacdcf186934dcb02f642579314075910f1855590fd3039d8fa4c9f96e48315
	[ "$status" -eq 0 ]
	echo "$output"
	[ "$output" != "" ]

	cleanup_images
	stop_crio
}

@test "image list with filter" {
	start_crio "" "" --no-pause-image
	run crioctl image pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	run crioctl image list --quiet "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		run crioctl image remove --id "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	run crioctl image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		echo "$id"
		status=1
	done
	cleanup_images
	stop_crio
}

@test "image list/remove" {
	start_crio "" "" --no-pause-image
	run crioctl image pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	run crioctl image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" != "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		run crioctl image remove --id "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	run crioctl image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" = "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		echo "$id"
		status=1
	done
	cleanup_images
	stop_crio
}

@test "image status/remove" {
	start_crio "" "" --no-pause-image
	run crioctl image pull "$IMAGE"
	echo "$output"
	[ "$status" -eq 0 ]
	run crioctl image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" != "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		run crioctl image status --id "$id"
		echo "$output"
		[ "$status" -eq 0 ]
		[ "$output" != "" ]
		run crioctl image remove --id "$id"
		echo "$output"
		[ "$status" -eq 0 ]
	done
	run crioctl image list --quiet
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" = "" ]
	printf '%s\n' "$output" | while IFS= read -r id; do
		echo "$id"
		status=1
	done
	cleanup_images
	stop_crio
}
