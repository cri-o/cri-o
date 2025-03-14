#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

ARTIFACT_REPO=quay.io/crio/artifact
ARTIFACT_IMAGE="$ARTIFACT_REPO:singlefile"

@test "should be able to pull and list an OCI artifact" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Should get listed as filtered artifact
	crictl images -q $ARTIFACT_IMAGE
	[ "$output" != "" ]

	# Should be available on the whole list
	crictl images | grep -qE "$ARTIFACT_REPO.*singlefile"
}
@test "should be able to inspect an OCI artifact" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	crictl inspecti $ARTIFACT_IMAGE |
		jq -e '
		(.status.pinned == true) and
		(.status.repoDigests | length == 1) and
		(.status.repoTags | length == 1) and
		(.status.size != "0")'
}

@test "should be able to remove an OCI artifact" {
	start_crio
	crictl pull $ARTIFACT_IMAGE
	crictl rmi $ARTIFACT_IMAGE

	[ "$(crictl images -q $ARTIFACT_IMAGE | wc -l)" == 0 ]
}

@test "should be able to mount OCI Artifact" {
	start_crio
	crictl pull $ARTIFACT_IMAGE
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr_id"

	# The artifact should be mounted
	run crictl exec --sync "$ctr_id" cat /root/artifact/artifact.txt
	[[ "$output" == "hello artifact" ]]

	# The mount should be read-only
	run ! crictl exec --sync "$ctr_id" sh -c "echo 'test' > /root/artifact/artifact.txt"
	[[ "$output" == *"Read-only file system"* ]]
}

@test "should be able to mount artifact with multiple files on directory" {
	start_crio
	IMAGE="$ARTIFACT_REPO:multiplefiles"
	crictl pull $IMAGE
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$IMAGE" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr_id"

	# The artifacts should be mounted
	run crictl exec --sync "$ctr_id" cat /root/artifact/artifact.txt
	[[ "$output" == "hello artifact" ]]
	run crictl exec --sync "$ctr_id" cat /root/artifact/artifact.sh
	[[ "$output" == *"echo hello artifact" ]]
}

@test "should be able to relabel selinux label" {
	skip_if_selinux_disabled
	skip_if_vm_runtime
	start_crio
	crictl pull $ARTIFACT_IMAGE
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE },
      selinux_relabel: true,
    } ] |
    .command = ["sleep", "3600"] |
    .linux.security_context.selinux_options = {"level": "s0:c100,c200"}' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr_id"

	# The artifact should be mounted
	run crictl exec --sync "$ctr_id" ls -laZ /root/artifact/artifact.txt
	[[ "$output" == *"s0:c100,c200"* ]]
}

@test "should return error when mounting artifact on file path" {
	start_crio
	crictl pull "$ARTIFACT_IMAGE"
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.mounts = [ {
      container_path: "/root/original-ks.cfg",
      image: { image: $ARTIFACT_IMAGE },
    } ]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	run ! crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json"
}

@test "should return error when running executable" {
	start_crio
	IMAGE="$ARTIFACT_REPO:exec"
	crictl pull $IMAGE
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$IMAGE" \
		'.mounts = [ {
        container_path: "/root/artifact",
        image: { image: $ARTIFACT_IMAGE },
      } ] |
      .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr_id"

	run ! crictl exec --sync "$ctr_id" /root/artifact/artifact.sh
}

@test "should return error when removing image that is in use" {
	start_crio
	crictl pull "$ARTIFACT_IMAGE"
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr_id"
	run ! crictl rmi "$ARTIFACT_IMAGE"
	# After the container stopped, it should be able to be removed.
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl rmi "$ARTIFACT_IMAGE"
}
