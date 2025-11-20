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
ARTIFACT_IMAGE_SUBPATH="$ARTIFACT_REPO:subpath"

@test "should be able to pull and list an OCI artifact" {
	start_crio
	cleanup_images
	crictl pull $ARTIFACT_IMAGE

	# Should get listed as filtered artifact
	run crictl images -q $ARTIFACT_IMAGE
	[ "$output" != "" ]

	# Should be available on the whole list
	crictl images | grep -qE "$ARTIFACT_REPO.*singlefile"
}

@test "should be able to pull and list an OCI artifact with shortname" {
	CONTAINER_REGISTRIES_CONF_DIR="$TESTDIR/containers/registries.conf.d"
	mkdir -p "$CONTAINER_REGISTRIES_CONF_DIR"
	printf 'unqualified-search-registries = ["quay.io"]' >> "$CONTAINER_REGISTRIES_CONF_DIR/99-registry.conf"

	IMAGE=crio/artifact:singlefile
	CONTAINER_REGISTRIES_CONF_DIR=$CONTAINER_REGISTRIES_CONF_DIR start_crio
	cleanup_images
	crictl pull $IMAGE

	# Should get listed as filtered artifact
	run crictl images -q $IMAGE
	[ "$output" != "" ]

	# Should be available on the whole list
	crictl images | grep -qE "quay.io/crio/artifact.*singlefile"
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

@test "should be able to inspect an OCI artifact with other references" {
	CONTAINER_REGISTRIES_CONF_DIR="$TESTDIR/containers/registries.conf.d"
	mkdir -p "$CONTAINER_REGISTRIES_CONF_DIR"
	printf 'unqualified-search-registries = ["quay.io"]' >> "$CONTAINER_REGISTRIES_CONF_DIR/99-registry.conf"

	CONTAINER_REGISTRIES_CONF_DIR=$CONTAINER_REGISTRIES_CONF_DIR start_crio
	crictl pull $ARTIFACT_IMAGE

	# canonical name
	digestedRef=$(crictl inspecti $ARTIFACT_IMAGE | jq -r '.status.repoDigests[0]')
	crictl inspecti "$digestedRef"

	# digest (long and short)
	imageId=$(crictl inspecti $ARTIFACT_IMAGE | jq -r '.status.id')
	crictl inspecti "$imageId"
	crictl inspecti "${imageId:0:12}"

	# shortname
	crictl inspecti crio/artifact:singlefile
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

@test "should return error when the OCP Artifact Mount is disabled" {
	ARTIFACT_CONFIG="$CRIO_CONFIG_DIR/00-disable-artifact.conf"
	cat << EOF > "$ARTIFACT_CONFIG"
[crio.image]
oci_artifact_mount_support = false
EOF
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
	run ! crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json"

	rm -f "$ARTIFACT_CONFIG"
}

@test "should be able to mount OCI Artifact with sub path" {
	start_crio
	crictl pull $ARTIFACT_IMAGE_SUBPATH
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE_SUBPATH "$ARTIFACT_IMAGE_SUBPATH" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE_SUBPATH },
      image_sub_path: "subpath"
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	crictl start "$ctr_id"

	# The artifact should get mounted with the correct sub path
	run crictl exec --sync "$ctr_id" cat /root/artifact/2
	[[ "$output" == "2" ]]

	run crictl exec --sync "$ctr_id" cat /root/artifact/3
	[[ "$output" == "3" ]]
}

@test "should fail to mount OCI Artifact with sub path if not existing" {
	start_crio
	crictl pull $ARTIFACT_IMAGE_SUBPATH
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	jq --arg ARTIFACT_IMAGE_SUBPATH "$ARTIFACT_IMAGE_SUBPATH" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE_SUBPATH },
      image_sub_path: "subpath-not-existing"
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	run ! crictl create "$pod_id" "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json"

	[[ "$output" == *"ImageVolumeMountFailed"*"does not exist in OCI artifact volume"* ]]
}

@test "should pull multi-architecture image" {
	start_crio

	# TODO(bitoku): use an image in quay.io/crio once quay.io supports multiarch artifacts
	# This version doesn't have to be updated. It's specified only to keep the test consistent.
	MULTIARCH_ARTIFACT="ghcr.io/cri-o/bundle:v1.32.4"
	crictl pull "$MULTIARCH_ARTIFACT"

	jq --arg ARTIFACT_IMAGE "$MULTIARCH_ARTIFACT" \
		'.mounts = [ {
      container_path: "/root/artifact",
      image: { image: $ARTIFACT_IMAGE },
    } ] |
    .command = ["sleep", "3600"]' \
		"$TESTDATA"/container_config.json > "$TESTDIR/container_config.json"
	ctr_id=$(crictl run "$TESTDIR/container_config.json" "$TESTDATA/sandbox_config.json")
	run crictl exec "$ctr_id" sha256sum /root/artifact/cri-o/bin/crio

	# Architecture-specific hash expectations
	if [[ "$ARCH" == "aarch64" ]]; then
		[[ "$output" == *"f18a492aeef00b307d6962c876de4839148c34e73035ba619e848298dc849d3a"* ]]
	else
		[[ "$output" == *"ae5d192303e5f9a357c6ea39308338956b62b8830fd05f0460796db2215c2b35"* ]]
	fi
}
