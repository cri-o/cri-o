#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "ctr check shared /dev/shm" {
	start_crio
	pod_id=$(crictl runp "$TESTDATA"/sandbox_config.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_sleep.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl exec --sync "$ctr_id" touch /dev/shm/testdata

	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDATA"/sandbox_config.json)
	crictl start "$ctr_id"
	crictl exec --sync "$ctr_id" ls /dev/shm/testdata
}
