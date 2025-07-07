#!/usr/bin/env bats
# vim:set ft=bash :

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

ARTIFACT_REPO=quay.io/sohankunkerkar/artifact
ARTIFACT_IMAGE="$ARTIFACT_REPO:singlefile"
ARTIFACT_IMAGE_MULTI="$ARTIFACT_REPO:multiplefiles"

@test "should cleanup artifacts when container is removed" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Create a container with artifact mount
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

	# Verify artifact is mounted and accessible
	run crictl exec --sync "$ctr_id" cat /root/artifact/artifact.txt
	[[ "$output" == "hello artifact" ]]

	# Stop and remove the container
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify artifact directories are cleaned up
	# The cleanup should have removed the extracted artifact directories
	# But the parent directories should remain
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"
}

@test "should cleanup artifacts when container creation fails" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Create a container with invalid artifact mount to trigger failure
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.mounts = [ {
      container_path: "/root/original-ks.cfg",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"

	# This should fail because we're trying to mount to a file path
	run ! crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json"

	# Verify artifact directories are cleaned up even on failure
	# Check only the CRI-O extracted artifacts directories, not the parent artifact storage
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"

	# Clean up the pod
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}

@test "should cleanup artifacts when pod is removed" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Create a pod with multiple containers using artifacts
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)

	# Create first container
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.mounts = [ {
      container_path: "/root/artifact1",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container1_config.json"
	ctr1_id=$(crictl create "$pod_id" "$TESTDIR/container1_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr1_id"

	# Create second container with same artifact
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.metadata.name = "container2" |
		.mounts = [ {
      container_path: "/root/artifact2",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container2_config.json"
	ctr2_id=$(crictl create "$pod_id" "$TESTDIR/container2_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr2_id"

	# Verify artifacts are mounted in both containers
	run crictl exec --sync "$ctr1_id" cat /root/artifact1/artifact.txt
	[[ "$output" == "hello artifact" ]]
	run crictl exec --sync "$ctr2_id" cat /root/artifact2/artifact.txt
	[[ "$output" == "hello artifact" ]]

	# Remove the entire pod (this should cleanup all containers and their artifacts)
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify artifact directories are cleaned up
	# But the parent directories should remain
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"
}

@test "should cleanup artifacts when container is force removed" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Create a container with artifact mount
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

	# Verify artifact is mounted
	run crictl exec --sync "$ctr_id" cat /root/artifact/artifact.txt
	[[ "$output" == "hello artifact" ]]

	# Force remove the container (without stopping first)
	crictl rm -f "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify artifact directories are cleaned up
	# But the parent directories should remain
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"
}

@test "should cleanup artifacts when container creation fails after artifact mount" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Create a container with artifact mount but command that will fail during runtime
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["sh", "-c", "exit 1"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"

	# Create the container (this should succeed)
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")

	# Start the container (this should succeed, but container will exit with code 1)
	crictl start "$ctr_id"
	run crictl inspect "$ctr_id"
	[[ "$(crictl inspect "$ctr_id" | jq .status.exitCode)" -eq 1 ]]

	# Remove the failed container
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify artifact directories are cleaned up
	# But the parent directories should remain
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"
}

@test "should cleanup artifacts when multiple artifacts are used" {
	start_crio
	crictl pull $ARTIFACT_IMAGE
	crictl pull $ARTIFACT_IMAGE_MULTI

	# Create a container with multiple artifact mounts
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" --arg ARTIFACT_IMAGE_MULTI "$ARTIFACT_IMAGE_MULTI" \
		'.mounts = [
      {
        container_path: "/root/artifact1",
        image: { image: $ARTIFACT_IMAGE },
      },
      {
        container_path: "/root/artifact2",
        image: { image: $ARTIFACT_IMAGE_MULTI },
      }
    ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr_id"

	# Verify both artifacts are mounted
	run crictl exec --sync "$ctr_id" cat /root/artifact1/artifact.txt
	[[ "$output" == "hello artifact" ]]
	run crictl exec --sync "$ctr_id" cat /root/artifact2/artifact.txt
	[[ "$output" == "hello artifact" ]]
	run crictl exec --sync "$ctr_id" cat /root/artifact2/artifact.sh
	[[ "$output" == *"echo hello artifact"* ]]

	# Remove the container
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify all artifact directories are cleaned up
	# But the parent directories should remain
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"
}

@test "should cleanup artifacts when container is killed" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Create a container with artifact mount
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

	# Verify artifact is mounted
	run crictl exec --sync "$ctr_id" cat /root/artifact/artifact.txt
	[[ "$output" == "hello artifact" ]]

	# Kill the container
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify artifact directories are cleaned up
	# But the parent directories should remain
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"
}

@test "should cleanup artifacts when server restarts and container is removed" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Create a container with artifact mount
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

	# Verify artifact is mounted
	run crictl exec --sync "$ctr_id" cat /root/artifact/artifact.txt
	[[ "$output" == "hello artifact" ]]

	# Stop CRI-O
	stop_crio

	# Restart CRI-O
	start_crio

	# The container should still be running and artifact should still be accessible
	run crictl exec --sync "$ctr_id" cat /root/artifact/artifact.txt
	[[ "$output" == "hello artifact" ]]

	# Now remove the container
	crictl stop "$ctr_id"
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify artifact directories are cleaned up after container removal
	# But the parent directories should remain
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"
}

@test "should cleanup artifacts when container exits normally" {
	start_crio
	crictl pull $ARTIFACT_IMAGE

	# Create a container with artifact mount that exits quickly
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE "$ARTIFACT_IMAGE" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["echo", "hello artifact"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr_id"

	# Wait for container to exit
	sleep 2

	# Verify container has exited
	run crictl inspect "$ctr_id"
	[[ "$output" == *"EXITED"* ]]

	# Remove the container
	crictl rm "$ctr_id"
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"

	# Verify artifact directories are cleaned up
	# But the parent directories should remain
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"
}

@test "should cleanup artifacts when subpath mount fails" {
	start_crio
	ARTIFACT_IMAGE_SUBPATH="$ARTIFACT_REPO:subpath"
	crictl pull $ARTIFACT_IMAGE_SUBPATH

	# Create a container with invalid subpath to trigger failure
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE_SUBPATH "$ARTIFACT_IMAGE_SUBPATH" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE_SUBPATH },
      image_sub_path: "subpath-not-existing"
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"

	# This should fail because the subpath doesn't exist
	run ! crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json"

	# Verify artifact directories are cleaned up even on failure
	# But the parent directories should remain
	run find "$TESTDIR/crio/extracted-artifacts" -mindepth 1 -type d 2> /dev/null || true
	[[ "$output" == "" ]]

	# Check that the parent directories still exist (they should)
	run test -d "$TESTDIR/crio/extracted-artifacts"
	run test -d "$TESTDIR/crio/artifacts"

	# Clean up the pod
	crictl stopp "$pod_id"
	crictl rmp "$pod_id"
}
