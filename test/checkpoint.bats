#!/usr/bin/env bats

load helpers

function setup() {
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi
	has_criu
	setup_test
}

function teardown() {
	cleanup_test
}

@test "checkpoint and restore one container into a new pod (drop infra:true)" {
	CONTAINER_DROP_INFRA_CTR=true CONTAINER_ENABLE_CRIU_SUPPORT=true start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	BIND_MOUNT_FILE=$(mktemp)
	BIND_MOUNT_DIR=$(mktemp -d)
	jq ". +{mounts:[{\"container_path\":\"/etc/issue\",\"host_path\":\"$BIND_MOUNT_FILE\"},{\"container_path\":\"/data\",\"host_path\":\"$BIND_MOUNT_DIR\"}]}" "$TESTDATA"/container_sleep.json > "$TESTDATA"/checkpoint.json
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/checkpoint.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	crictl rm -f "$ctr_id"
	crictl rmp -f "$pod_id"
	rm -f "$BIND_MOUNT_FILE"
	rmdir "$BIND_MOUNT_DIR"
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# Replace original container with checkpoint image
	RESTORE_JSON=$(mktemp)
	jq ".image.image=\"$TESTDIR/cp.tar\"" "$TESTDATA"/container_sleep.json > "$RESTORE_JSON"
	ctr_id=$(crictl create "$pod_id" "$RESTORE_JSON" "$TESTDATA"/sandbox_config.json)
	rm -f "$RESTORE_JSON"
	rm -f "$TESTDATA"/checkpoint.json
	crictl start "$ctr_id"
	restored=$(crictl inspect --output go-template --template "{{(index .info.restored)}}" "$ctr_id")
	[[ "$restored" == "true" ]]
	rm -f "$BIND_MOUNT_FILE"
	rmdir "$BIND_MOUNT_DIR"
}

@test "checkpoint and restore one container into a new pod (drop infra:false)" {
	CONTAINER_DROP_INFRA_CTR=false CONTAINER_ENABLE_CRIU_SUPPORT=true start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	crictl rm -f "$ctr_id"
	crictl rmp -f "$pod_id"
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# Replace original container with checkpoint image
	RESTORE_JSON=$(mktemp)
	jq ".image.image=\"$TESTDIR/cp.tar\"" "$TESTDATA"/container_sleep.json > "$RESTORE_JSON"
	ctr_id=$(crictl create "$pod_id" "$RESTORE_JSON" "$TESTDATA"/sandbox_config.json)
	rm -f "$RESTORE_JSON"
	crictl start "$ctr_id"
}

@test "checkpoint and restore one container into a new pod using --export to OCI image" {
	has_buildah
	CONTAINER_DROP_INFRA_CTR=false CONTAINER_ENABLE_CRIU_SUPPORT=true start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	crictl rm -f "$ctr_id"
	crictl rmp -f "$pod_id"
	newimage=$(run_buildah from scratch)
	run_buildah add "$newimage" "$TESTDIR"/cp.tar /
	run_buildah config --annotation io.kubernetes.cri-o.annotations.checkpoint.name=sleeper "$newimage"
	run_buildah commit "$newimage" "checkpoint-image:tag1"
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# Replace original container with checkpoint image
	RESTORE_JSON=$(mktemp)
	jq ".image.image=\"localhost/checkpoint-image:tag1\"" "$TESTDATA"/container_sleep.json > "$RESTORE_JSON"
	ctr_id=$(crictl create "$pod_id" "$RESTORE_JSON" "$TESTDATA"/sandbox_config.json)
	rm -f "$RESTORE_JSON"
	crictl start "$ctr_id"
}

@test "checkpoint and restore one container into a new pod using --export to OCI image using repoDigest" {
	has_buildah
	CONTAINER_DROP_INFRA_CTR=false CONTAINER_ENABLE_CRIU_SUPPORT=true start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	crictl rm -f "$ctr_id"
	crictl rmp -f "$pod_id"
	newimage=$(run_buildah from scratch)
	run_buildah add "$newimage" "$TESTDIR"/cp.tar /
	run_buildah config --annotation io.kubernetes.cri-o.annotations.checkpoint.name=sleeper "$newimage"
	run_buildah commit "$newimage" "checkpoint-image:tag1"
	# Kubernetes uses the repoDigest to references images.
	repo_digest=$(crictl inspecti --output go-template --template "{{(index .status.repoDigests 0)}}" "localhost/checkpoint-image:tag1")
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# Replace original container with checkpoint image
	RESTORE_JSON=$(mktemp)
	jq ".image.image=\"$repo_digest\"" "$TESTDATA"/container_sleep.json > "$RESTORE_JSON"
	ctr_id=$(crictl create "$pod_id" "$RESTORE_JSON" "$TESTDATA"/sandbox_config.json)
	rm -f "$RESTORE_JSON"
	crictl start "$ctr_id"
}
