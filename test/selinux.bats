#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "selinux label level=s0 is sufficient" {
	start_crio

	jq '	  .linux.security_context.selinux_options = {"level": "s0"}' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox.json

	pod_id=$(crictl runp "$TESTDIR"/sandbox.json)
	ctr_id=$(crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox.json)
	crictl start "$ctr_id"
}
