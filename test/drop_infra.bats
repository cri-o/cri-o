#!/usr/bin/env bats

load helpers

function setup() {
	# TODO: drop this skip once userns works with pinned namespaces
	if test -n "$CONTAINER_UID_MAPPINGS"; then
		skip "userNS enabled"
	fi

	setup_test
	CONTAINER_DROP_INFRA_CTR=true start_crio
}

function teardown() {
	cleanup_test
}

@test "test infra ctr dropped" {
	jq '.linux.security_context.namespace_options.pid = 1' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	output=$(runtime list || true)
	[[ ! "$output" = *"$pod_id"* ]]
}

@test "test infra ctr not dropped" {
	jq '.linux.security_context.namespace_options.pid = 0' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)

	output=$(runtime list)
	[[ "$output" = *"$pod_id"* ]]
}

@test "test infra ctr dropped status" {
	jq '.linux.security_context.namespace_options.pid = 1' \
		"$TESTDATA"/sandbox_config.json > "$TESTDIR"/sandbox_no_infra.json
	pod_id=$(crictl runp "$TESTDIR"/sandbox_no_infra.json)
	output=$(crictl inspectp "$pod_id" | jq .info)
	[[ "$output" != "{}" ]]
}
