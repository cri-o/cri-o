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
	jq ".image.image=\"$TESTDIR/cp.tar\"" "$TESTDATA"/container_sleep.json > "$TESTDATA"/restore.json
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/restore.json "$TESTDATA"/sandbox_config.json)
	rm -f "$TESTDATA"/restore.json
	rm -f "$TESTDATA"/checkpoint.json
	crictl start "$ctr_id"
	rm -f "$BIND_MOUNT_FILE"
	rmdir "$BIND_MOUNT_DIR"
}

@test "checkpoint and restore one container into a new pod (drop infra:false)" {
	CONTAINER_DROP_INFRA_CTR=false CONTAINER_ENABLE_CRIU_SUPPORT=true start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl checkpoint --export="$TESTDIR"/cp.tar "$ctr_id"
	crictl rmp -f "$pod_id"
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	# Replace original container with checkpoint image
	jq ".image.image=\"$TESTDIR/cp.tar\"" "$TESTDATA"/container_sleep.json > "$TESTDATA"/restore.json
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/restore.json "$TESTDATA"/sandbox_config.json)
	rm -f "$TESTDATA"/restore.json
	crictl start "$ctr_id"
	crictl rmp -f "$pod_id"
}
