#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "image volume ignore" {
	CONTAINER_IMAGE_VOLUMES=ignore start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	image_volume_config=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["image"]["image"] = "quay.io/crio/image-volume-test"; obj["command"] = ["/bin/sleep", "600"]; json.dump(obj, sys.stdout)')
	echo "$image_volume_config" > "$TESTDIR"/container_image_volume.json
	run crictl create "$pod_id" "$TESTDIR"/container_image_volume.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" ls /imagevolume
	echo "$output"
	[ "$status" -ne 0 ]
	[[ "$output" =~ "ls: /imagevolume: No such file or directory" ]]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "image volume bind" {
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	CONTAINER_IMAGE_VOLUMES=bind start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	image_volume_config=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["image"]["image"] = "quay.io/crio/image-volume-test"; obj["command"] = ["/bin/sleep", "600"]; json.dump(obj, sys.stdout)')
	echo "$image_volume_config" > "$TESTDIR"/container_image_volume.json
	run crictl create "$pod_id" "$TESTDIR"/container_image_volume.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" touch /imagevolume/test_file
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" = "" ]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
}

@test "image volume user mkdir" {
	if test -n "$UID_MAPPINGS"; then
		skip "userNS enabled"
	fi
	IMAGE_VOLUMES=mkdir start_crio
	run crictl runp "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	image_volume_config=$(cat "$TESTDATA"/container_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["image"]["image"] = "quay.io/crio/image-volume-test"; obj["command"] = ["/bin/sleep", "600"]; obj["linux"]["security_context"]["run_as_user"]["value"] = 1000; json.dump(obj, sys.stdout)')
	echo "$image_volume_config" > "$TESTDIR"/container_image_volume.json
	run crictl create "$pod_id" "$TESTDIR"/container_image_volume.json "$TESTDATA"/sandbox_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl exec --sync "$ctr_id" touch /imagevolume/test_file
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" = "" ]
	run crictl exec --sync "$ctr_id" id
	echo "$output"
	[ "$status" -eq 0 ]
	[ "$output" = "uid=1000 gid=0(root)" ]
	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_crio
}
