#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	requires_crictl "1.31"
	setup_test
}

function teardown() {
	cleanup_test
}

@test "OCI image volume mount lifecycle" {
	start_crio

	CONTAINER_PATH=/volume
	IMAGE=quay.io/crio/artifact:v1

	# Prepull the artifact
	crictl pull "$IMAGE"

	# Set mounts in the same way as the kubelet would do
	jq --arg IMAGE "$IMAGE" --arg CONTAINER_PATH "$CONTAINER_PATH" \
		'.mounts = [{
			host_path: "",
			container_path: $CONTAINER_PATH,
			image: { image: $IMAGE },
			readonly: true
		}]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container.json"

	CTR_ID=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# Assert mount availability
	[[ $(crictl exec "$CTR_ID" cat "$CONTAINER_PATH/dir/file") == 1 ]]
	[[ $(crictl exec "$CTR_ID" cat "$CONTAINER_PATH/file") == 2 ]]

	# Image removal should be blocked
	run ! crictl rmi $IMAGE

	# The kubelet garbage collection expects the image ID set in the container status mount
	IMAGE_ID=$(crictl inspecti quay.io/crio/artifact:v1 | jq -e .status.id)
	IMAGE_MOUNT_ID=$(crictl inspect "$CTR_ID" | jq -e '.status.mounts[0].image.image')
	[[ "$IMAGE_ID" == "$IMAGE_MOUNT_ID" ]]

	# Remove the container
	crictl rm -f "$CTR_ID"

	# Image removal should work now
	crictl rmi $IMAGE
}
