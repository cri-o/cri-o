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

CONTAINER_PATH=/volume
IMAGE=quay.io/crio/artifact:v1

@test "OCI image volume mount lifecycle" {
	if [[ "$TEST_USERNS" == "1" ]]; then
		skip "test fails in a user namespace"
	fi

	start_crio

	# Prepull the artifact
	crictl_pull "$IMAGE"

	# Set mounts in the same way as the kubelet would do
	jq --arg IMAGE "$IMAGE" --arg CONTAINER_PATH "$CONTAINER_PATH" \
		'.mounts = [{
			host_path: "",
			container_path: $CONTAINER_PATH,
			image: { image: $IMAGE, user_specified_image: $IMAGE },
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
	IMAGE_ID=$(crictl inspecti quay.io/crio/artifact:v1 | jq -e -r .status.id)
	IMAGE_MOUNT_ID=$(crictl inspect "$CTR_ID" | jq -e -r '.status.mounts[0].image.image')
	[[ "$IMAGE_ID" == "$IMAGE_MOUNT_ID" ]]

	# KEP-5365: the mount's image_ref should match the image ID reported by PullImage.
	# crictl < 1.36 does not know about ImageSpec.image_ref yet.
	if check_crictl_version "1.36"; then
		IMAGE_MOUNT_REF=$(crictl inspect "$CTR_ID" | jq -e -r '.status.mounts[0].image.imageRef')
		[[ "$IMAGE_MOUNT_REF" == "$IMAGE_ID" ]]
	fi

	# Remove the container
	crictl rm -f "$CTR_ID"

	# Image removal should work now
	crictl rmi $IMAGE
}

@test "OCI image volume mount reports image ID for a regular image" {
	if [[ "$TEST_USERNS" == "1" ]]; then
		skip "test fails in a user namespace"
	fi

	# crictl < 1.36 does not know about ImageSpec.image_ref yet.
	requires_crictl "1.36"

	start_crio

	# Use an image different from the container's rootfs image, so that a
	# wrong image ID (e.g. the container image's) cannot pass the test.
	VOLUME_IMAGE=quay.io/crio/hello-wasm:latest

	# Set mounts in the same way as the kubelet would do
	jq --arg IMAGE "$VOLUME_IMAGE" --arg CONTAINER_PATH "$CONTAINER_PATH" \
		'.mounts = [{
			host_path: "",
			container_path: $CONTAINER_PATH,
			image: { image: $IMAGE, user_specified_image: $IMAGE },
			readonly: true
		}]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container.json"

	CTR_ID=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# The kubelet garbage collection expects the image ID set in the container status mount
	IMAGE_ID=$(crictl inspecti "$VOLUME_IMAGE" | jq -e -r .status.id)
	IMAGE_MOUNT_ID=$(crictl inspect "$CTR_ID" | jq -e -r '.status.mounts[0].image.image')
	[[ "$IMAGE_MOUNT_ID" == "$IMAGE_ID" ]]

	# KEP-5365: the mount's image_ref should match the image ID reported by PullImage
	IMAGE_MOUNT_REF=$(crictl inspect "$CTR_ID" | jq -e -r '.status.mounts[0].image.imageRef')
	[[ "$IMAGE_MOUNT_REF" == "$IMAGE_ID" ]]
}

@test "OCI image volume SELinux" {
	if ! is_selinux_enforcing; then
		skip "not enforcing"
	fi

	# RHEL/CentOS 7's container-selinux package replaces container_file_t with svirt_sandbox_file_t
	# under the hood. This causes the annotation to not work correctly.
	if is_rhel_7; then
		skip "fails on RHEL 7 or earlier"
	fi

	start_crio

	# Prepull the artifact
	crictl_pull "$IMAGE"

	# Build a second sandbox using a different level
	jq '.metadata.name = "sb-1" |
		.metadata.uid = "new-uid" |
		.linux.security_context.selinux_options.level = "s0:c200,c100"' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	# Set mounts in the same way as the kubelet would do
	jq --arg IMAGE "$IMAGE" --arg CONTAINER_PATH "$CONTAINER_PATH" \
		'.mounts = [{
			host_path: "",
			container_path: $CONTAINER_PATH,
			image: { image: $IMAGE, user_specified_image: $IMAGE },
			readonly: true
		}]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container.json"

	CTR_ID1=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")
	CTR_ID2=$(crictl run "$TESTDIR/container.json" "$TESTDIR/sandbox.json")

	# Assert the right labels
	crictl exec -s "$CTR_ID1" ls -Z "$CONTAINER_PATH" | grep -q "s0:c4,c5"
	crictl exec -s "$CTR_ID2" ls -Z "$CONTAINER_PATH" | grep -q "s0:c100,c200"
}

@test "OCI image volume does not exist locally" {
	start_crio

	# Set mounts in the same way as the kubelet would do
	jq --arg IMAGE "$IMAGE" --arg CONTAINER_PATH "$CONTAINER_PATH" \
		'.mounts = [{
			host_path: "",
			container_path: $CONTAINER_PATH,
			image: { image: $IMAGE, user_specified_image: $IMAGE },
			readonly: true
		}]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container.json"

	run ! crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json"
}

@test "OCI image volume mount with sub path" {
	if [[ "$TEST_USERNS" == "1" ]]; then
		skip "test fails in a user namespace"
	fi

	start_crio

	# Prepull the artifact
	crictl_pull "$IMAGE"

	# Set mounts in the same way as the kubelet would do
	jq --arg IMAGE "$IMAGE" --arg CONTAINER_PATH "$CONTAINER_PATH" \
		'.mounts = [{
			host_path: "",
			container_path: $CONTAINER_PATH,
			image: { image: $IMAGE, user_specified_image: $IMAGE },
			image_sub_path: "dir",
			readonly: true
		}]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container.json"

	CTR_ID=$(crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json")

	# Assert mount availability within the sub path
	[[ $(crictl exec "$CTR_ID" cat "$CONTAINER_PATH/file") == 1 ]]
}

@test "OCI image volume mount with invalid sub path" {
	if [[ "$TEST_USERNS" == "1" ]]; then
		skip "test fails in a user namespace"
	fi

	start_crio

	# Prepull the artifact
	crictl_pull "$IMAGE"

	# Set mounts in the same way as the kubelet would do
	jq --arg IMAGE "$IMAGE" --arg CONTAINER_PATH "$CONTAINER_PATH" \
		'.mounts = [{
			host_path: "",
			container_path: $CONTAINER_PATH,
			image: { image: $IMAGE, user_specified_image: $IMAGE },
			image_sub_path: "foo-bar",
			readonly: true
		}]' \
		"$TESTDATA"/container_sleep.json > "$TESTDIR/container.json"

	run ! crictl run "$TESTDIR/container.json" "$TESTDATA/sandbox_config.json"
	grep -q "ImageVolumeMountFailed: sub path .* does not exist in image volume" "$CRIO_LOG"
}
