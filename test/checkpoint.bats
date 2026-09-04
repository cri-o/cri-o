#!/usr/bin/env bats

load helpers

function setup() {
	unset SIGNATURE_POLICY
	if [[ $RUNTIME_TYPE == pod ]]; then
		skip "not yet supported by conmonrs"
	fi

	setup_test
	has_criu
}

function teardown() {
	cleanup_test
}

@test "checkpoint and restore a pod with two containers" {
	if is_using_crun; then
		skip "Pod checkpoint restore coverage currently requires runc"
	fi

	CONTAINER_DROP_INFRA_CTR=false CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="checkpoint_restore" start_crio

	sandbox_config="$TESTDIR/sandbox.json"
	container_one="$TESTDIR/container-one.json"
	container_two="$TESTDIR/container-two.json"
	cp "$TESTDATA/sandbox_config.json" "$sandbox_config"
	jq '.metadata.name = "pod-checkpoint-one"' \
		"$TESTDATA/container_sleep.json" > "$container_one"
	jq '.metadata.name = "pod-checkpoint-two"' \
		"$TESTDATA/container_sleep.json" > "$container_two"

	pod_id=$(crictl runp "$sandbox_config")
	ctr_one=$(crictl create "$pod_id" "$container_one" "$sandbox_config")
	ctr_two=$(crictl create "$pod_id" "$container_two" "$sandbox_config")
	crictl start "$ctr_one"
	crictl start "$ctr_two"

	checkpoint_dir="$TESTDIR/pod-checkpoint"
	mkdir -m 700 "$checkpoint_dir"
	"$CHECKPOD_BINARY" checkpoint \
		--endpoint "unix://$CRIO_SOCKET" \
		--sandbox "$pod_id" \
		--containers "$ctr_one,$ctr_two" \
		--output "$checkpoint_dir"

	crictl rmp -f "$pod_id"
	restore_sandbox="$TESTDIR/restore-sandbox.json"
	jq '.metadata.uid = "redhat-test-crio-restored" | .metadata.attempt = 2' \
		"$sandbox_config" > "$restore_sandbox"

	restore_response=$("$CHECKPOD_BINARY" restore \
		--endpoint "unix://$CRIO_SOCKET" \
		--checkpoint "$checkpoint_dir" \
		--sandbox-config "$restore_sandbox" \
		--container-configs "$container_one,$container_two")

	restored_pod=$(jq -r '.pod_sandbox_id' <<< "$restore_response")
	[[ -n "$restored_pod" ]]
	[[ $(jq '.restored_containers | length' <<< "$restore_response") -eq 2 ]]

	while read -r restored_container; do
		crictl start "$restored_container"
		[[ $(crictl inspect --output go-template --template '{{.status.state}}' "$restored_container") == "CONTAINER_RUNNING" ]]
	done < <(jq -r '.restored_containers[].container_id' <<< "$restore_response")
}

@test "checkpoint and restore one container into a new pod (drop infra:true)" {
	if is_using_crun; then
		skip "not supported by crun: https://github.com/containers/crun/issues/1207"
	fi

	CONTAINER_DROP_INFRA_CTR=true CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="checkpoint_restore" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	BIND_MOUNT_FILE=$(mktemp)
	BIND_MOUNT_DIR=$(mktemp -d)
	jq ". +{mounts:[{\"container_path\":\"/etc/issue\",\"host_path\":\"$BIND_MOUNT_FILE\"}, \
		{\"container_path\":\"/data\",\"host_path\":\"$BIND_MOUNT_DIR\"}]} \
		|.command=[\"/bin/bash\"] \
		|.args=[\"-c\",\"while true; do echo -n 'hello: '; date; sleep 0.5;done\"]" \
		"$TESTDATA"/container_sleep.json > "$TESTDATA"/checkpoint.json
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/checkpoint.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	LOG_CONTENT_BEFORE=$(crictl logs "$ctr_id")
	LINES_BEFORE=$(echo "$LOG_CONTENT_BEFORE" | wc -l)
	# Just remember the first line
	LOG_CONTENT_BEFORE=$(echo "$LOG_CONTENT_BEFORE" | head -1)
	crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	crictl rm -f "$ctr_id"
	crictl rmp -f "$pod_id"
	rm -f "$BIND_MOUNT_FILE"
	rmdir "$BIND_MOUNT_DIR"
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# Replace original container with checkpoint image
	RESTORE_JSON=$(mktemp)
	RESTORE2_JSON=$(mktemp)
	jq ".image.image=\"$TESTDIR/cp.tar\"" "$TESTDATA"/container_sleep.json > "$RESTORE_JSON"
	# This should fail because the bind mounts are not part of the create request
	run crictl create "$pod_id" "$RESTORE_JSON" "$TESTDATA"/sandbox_config.json
	[ "$status" -eq 1 ]
	jq ". +{mounts:[{\"container_path\":\"/etc/issue\",\"host_path\":\"$BIND_MOUNT_FILE\"},{\"container_path\":\"/data\",\"host_path\":\"$BIND_MOUNT_DIR\"}]}" "$RESTORE_JSON" > "$RESTORE2_JSON"
	ctr_id=$(crictl create "$pod_id" "$RESTORE2_JSON" "$TESTDATA"/sandbox_config.json)
	rm -f "$RESTORE_JSON"
	rm -f "$RESTORE2_JSON"
	rm -f "$TESTDATA"/checkpoint.json
	crictl start "$ctr_id"
	restored=$(crictl inspect --output go-template --template "{{(index .info.restored)}}" "$ctr_id")
	[[ "$restored" == "true" ]]
	# Sleeping here for a second to verify that logging still works.
	# The container creates a log line every 0.5 seconds. Waiting 1 second
	# should give us at least one line.
	sleep 1
	LOG_CONTENT_AFTER=$(crictl logs "$ctr_id")
	LINES_AFTER=$(echo "$LOG_CONTENT_AFTER" | wc -l)
	if [ "$LINES_BEFORE" -ge "$LINES_AFTER" ]; then
		echo "number of lines after checkpointing ($LINES_AFTER) " \
			"should be larger than before checkpointing ($LINES_BEFORE)"
		false
	fi
	[[ "$LOG_CONTENT_AFTER" == *"$LOG_CONTENT_BEFORE"* ]]
	rm -f "$BIND_MOUNT_FILE"
	rmdir "$BIND_MOUNT_DIR"
}

@test "checkpoint and restore one container into a new pod (drop infra:false)" {
	CONTAINER_DROP_INFRA_CTR=false CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="checkpoint_restore" start_crio
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
	CONTAINER_DROP_INFRA_CTR=false CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="checkpoint_restore" start_crio
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
	CONTAINER_DROP_INFRA_CTR=false CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="checkpoint_restore" start_crio
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

@test "checkpoint and restore one container into a new pod with a new name" {
	CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="checkpoint_restore" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# Add Kubernetes like annotations
	START_CONTAINER_JSON_1=$(mktemp)
	jq '
			.labels."io.kubernetes.container.name" = "podsandbox-sleep"
		|	.labels."io.kubernetes.pod.name" = "podsandbox1" ' \
		"$TESTDATA"/container_sleep.json > "$START_CONTAINER_JSON_1"
	ctr_id=$(crictl create "$pod_id" "$START_CONTAINER_JSON_1" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	crictl rm -f "$ctr_id"
	crictl rmp -f "$pod_id"
	# Replace original container with checkpoint image
	RESTORE_CONTAINER_JSON_1=$(mktemp)
	RESTORE_CONTAINER_JSON_2=$(mktemp)
	RESTORE_SANDBOX_JSON=$(mktemp)
	jq ".image.image=\"$TESTDIR/cp.tar\"" "$TESTDATA"/container_sleep.json > "$RESTORE_CONTAINER_JSON_1"
	# rename pod and container
	jq '.metadata.name="restoresandbox2"' "$TESTDATA"/sandbox_config.json > "$RESTORE_SANDBOX_JSON"
	jq '
			.metadata.name = "restored-sleep-container"
		|	.labels."io.kubernetes.container.name" = "restored-sleep-container"
		|	.labels."io.kubernetes.pod.name" = "restoresandbox2" ' \
		"$RESTORE_CONTAINER_JSON_1" > "$RESTORE_CONTAINER_JSON_2"
	pod_id=$(crictl runp "$RESTORE_SANDBOX_JSON")
	ctr_id=$(crictl create "$pod_id" "$RESTORE_CONTAINER_JSON_2" "$RESTORE_SANDBOX_JSON")
	rm -f "$RESTORE_CONTAINER_JSON_1"
	rm -f "$RESTORE_CONTAINER_JSON_2"
	rm -f "$RESTORE_SANDBOX_JSON"
	rm -f "$START_CONTAINER_JSON_1"
	crictl start "$ctr_id"
	container_name=$(crictl inspect --output go-template --template '{{(index .status.labels "io.kubernetes.container.name" )}}' "$ctr_id")
	pod_name=$(crictl inspect --output go-template --template '{{(index .status.labels "io.kubernetes.pod.name" )}}' "$ctr_id")
	[[ "$container_name" == "restored-sleep-container" ]]
	[[ "$pod_name" == "restoresandbox2" ]]
}

@test "checkpoint and restore: /etc/passwd uses Kubernetes run_as_user on restore" {
	CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="checkpoint_restore" start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# Create container with run_as_user=1001
	START_JSON=$(mktemp)
	jq '.linux.security_context.run_as_user.value = 1001
		| .command=["/bin/sh"]
		| .args=["-c","sleep inf"]' \
		"$TESTDATA"/container_sleep.json > "$START_JSON"
	ctr_id=$(crictl create "$pod_id" "$START_JSON" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	# Verify the UID of the running process
	run crictl exec "$ctr_id" id
	[[ "$output" == *"uid=1001"* ]]
	# Verify /etc/passwd contains entry for UID 1001
	run crictl exec "$ctr_id" cat /etc/passwd
	[[ "$output" == *"1001"* ]]
	# Checkpoint the container
	crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	crictl rm -f "$ctr_id"
	crictl rmp -f "$pod_id"
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	RESTORE_JSON=$(mktemp)
	jq '.image.image="'"$TESTDIR"'/cp.tar"
		| .linux.security_context.run_as_user.value = 1001' \
		"$TESTDATA"/container_sleep.json > "$RESTORE_JSON"
	ctr_id=$(crictl create "$pod_id" "$RESTORE_JSON" "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	# Verify that the container was restored
	restored=$(crictl inspect --output go-template --template "{{(index .info.restored)}}" "$ctr_id")
	[[ "$restored" == "true" ]]
	# Verify the UID is still 1001
	run crictl exec "$ctr_id" id
	[[ "$output" == *"uid=1001"* ]]
	run crictl exec "$ctr_id" cat /etc/passwd
	[[ "$output" == *"1001"* ]]
	# Cleanup
	rm -f "$START_JSON"
	rm -f "$RESTORE_JSON"
}

@test "checkpoint_only level allows checkpoint but blocks restore" {
	CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="checkpoint_only" start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	# Checkpointing is allowed.
	crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	crictl rm -f "$ctr_id"
	crictl rmp -f "$pod_id"

	# Restore is disabled, so the checkpoint archive is treated as a regular
	# image reference, which cannot be pulled. The create must fail.
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	RESTORE_JSON=$(mktemp)
	jq ".image.image=\"$TESTDIR/cp.tar\"" "$TESTDATA"/container_sleep.json > "$RESTORE_JSON"
	run ! crictl create "$pod_id" "$RESTORE_JSON" "$TESTDATA"/sandbox_config.json
	rm -f "$RESTORE_JSON"
}

@test "none level blocks checkpoint" {
	CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="none" start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	run ! crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	[[ "$output" == *"checkpoint/restore support not available"* ]]
}

@test "deprecated enable_criu_support=false disables checkpoint and overrides checkpoint_restore field" {
	CONTAINER_ENABLE_CRIU_SUPPORT=false CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="checkpoint_restore" start_crio

	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	run ! crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	[[ "$output" == *"checkpoint/restore support not available"* ]]
}

@test "invalid container_level_enabled fails to start crio" {
	setup_crio

	export CONTAINER_CHECKPOINT_RESTORE_CONTAINER_LEVEL_ENABLED="bogus"
	run ! "$CRIO_BINARY_PATH" -c "$CRIO_CONFIG" -d "$CRIO_CONFIG_DIR"
	[[ "$output" == *"container_level_enabled"* ]]
}
