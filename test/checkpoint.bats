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

@test "checkpoint and restore one container into a new pod using --export" {
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
