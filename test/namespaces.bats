#!/usr/bin/env bats

load helpers

function setup() {
	setup_test
}

function teardown() {
	cleanup_test
}

@test "pid_namespace_mode_pod_test" {
	start_crio

	pod_config="$TESTDIR"/sandbox_config.json
	jq '	  .linux.security_context.namespace_options = {
			pid: 0,
			host_network: false,
			host_pid: false,
			host_ipc: false
		}' \
		"$TESTDATA"/sandbox_config.json > "$pod_config"
	pod_id=$(crictl runp "$pod_config")

	ctr_config="$TESTDIR"/config.json
	jq '	  del(.linux.security_context.namespace_options)' \
		"$TESTDATA"/container_redis.json > "$ctr_config"
	ctr_id=$(crictl create "$pod_id" "$ctr_config" "$pod_config")
	crictl start "$ctr_id"

	output=$(crictl exec --sync "$ctr_id" cat /proc/1/cmdline)
	[[ "$output" == *"pause"* ]]
}
