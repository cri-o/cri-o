#!/usr/bin/env bats

load helpers

function setup() {
	# TODO: drop this skip once userns works with CONTAINER_MANAGE_NS_LIFECYCLE=true
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	setup_test
	CONTAINER_MANAGE_NS_LIFECYCLE=true CONTAINER_DROP_INFRA_CTR=true start_crio
}

function teardown() {
	cleanup_test
}

@test "test infra ctr dropped" {
	jq '.linux.security_context.namespace_options.pid = 1' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	output=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list)
	[[ ! "$output" = *"$pod_id"* ]]
}

@test "test infra ctr not dropped" {
	jq '.linux.security_context.namespace_options.pid = 0' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	output=$("$CONTAINER_RUNTIME" --root "$RUNTIME_ROOT" list)
	[[ "$output" = *"$pod_id"* ]]
}
