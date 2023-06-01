#!/usr/bin/env bats
# TODO: We need to modify these tests to be based on parsing mountinfo instead.
load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "image volume ignore" {
	CONTAINER_IMAGE_VOLUMES="ignore" start_crio

	jq '	  .command = ["bin/sleep", "600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container_image_volume.json

	ctr_id=$(crictl run "$TESTDIR"/container_image_volume.json "$TESTDATA"/sandbox_config.json)
	run ! crictl exec --sync "$ctr_id" ls /imagevolume
	[[ "$output" == *"ls: cannot access '/imagevolume': No such file or directory"* ]]
}

@test "image volume bind" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	CONTAINER_IMAGE_VOLUMES="bind" start_crio

	jq '	  .command = ["bin/sleep", "600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container_image_volume.json

	ctr_id=$(crictl run "$TESTDIR"/container_image_volume.json "$TESTDATA"/sandbox_config.json)
	output=$(crictl exec --sync "$ctr_id" touch /imagevolume/test_file)
	[ "$output" = "" ]
}

@test "image volume user mkdir" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	CONTAINER_IMAGE_VOLUMES="mkdir" start_crio

	jq '	  .command = ["bin/sleep", "600"]
		| .linux.security_context.run_as_user.value = 1000' \
		"$TESTDATA"/container_config.json > "$TESTDIR"/container_image_volume.json

	ctr_id=$(crictl run "$TESTDIR"/container_image_volume.json "$TESTDATA"/sandbox_config.json)
	output=$(crictl exec --sync "$ctr_id" touch /imagevolume/test_file)
	[ "$output" = "" ]
	output=$(crictl exec --sync "$ctr_id" id)
	[[ "$output" == *"uid=1000(1000) gid=0(root) groups=0(root)"* ]]
}
