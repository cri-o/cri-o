#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "image volume ignore" {
	CONTAINER_IMAGE_VOLUMES="ignore" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .image.image = "quay.io/crio/image-volume-test"
		| .command = ["bin/sleep", "600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container_image_volume.json

	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container_image_volume.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	run crictl exec --sync "$ctr_id" ls /imagevolume
	[ "$status" -ne 0 ]
	[[ "$output" == *"ls: /imagevolume: No such file or directory"* ]]
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "image volume bind" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	CONTAINER_IMAGE_VOLUMES="bind" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	jq '	  .image.image = "quay.io/crio/image-volume-test"
		| .command = ["bin/sleep", "600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container_image_volume.json

	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container_image_volume.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	output=$(crictl exec --sync "$ctr_id" touch /imagevolume/test_file)
	[ "$output" = "" ]
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "image volume user mkdir" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	CONTAINER_IMAGE_VOLUMES="mkdir" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq '	  .image.image = "quay.io/crio/image-volume-test"
		| .command = ["bin/sleep", "600"]
		| .linux.security_context.run_as_user.value = 1000' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container_image_volume.json

	ctr_id=$(crictl create "$pod_id" "$TESTDIR"/container_image_volume.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	output=$(crictl exec --sync "$ctr_id" touch /imagevolume/test_file)
	[ "$output" = "" ]
	output=$(crictl exec --sync "$ctr_id" id)
	[[ "$output" == *"uid=1000 gid=0(root)"* ]]
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}
