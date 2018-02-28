#!/usr/bin/env bats

load helpers

function teardown() {
	cleanup_test
}

@test "pid_namespace_mode_pod_test" {
	start_crio
	pidNamespaceMode=$(cat "$TESTDATA"/sandbox_config.json | python -c 'import json,sys;obj=json.load(sys.stdin);obj["linux"]["security_context"]["namespace_options"]["pid"] = 0; json.dump(obj, sys.stdout)')
	echo "$pidNamespaceMode" > "$TESTDIR"/sandbox_pidnamespacemode_config.json
	run crictl runp "$TESTDIR"/sandbox_pidnamespacemode_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	pod_id="$output"
	run crictl create "$pod_id" "$TESTDATA"/container_redis.json "$TESTDIR"/sandbox_pidnamespacemode_config.json
	echo "$output"
	[ "$status" -eq 0 ]
	ctr_id="$output"
	run crictl start "$ctr_id"
	[ "$status" -eq 0 ]

	run crictl exec --sync "$ctr_id" cat /proc/1/cmdline
	echo "$output"
	[ "$status" -eq 0 ]
	[[ "$output" =~ pause ]]

	run crictl stopp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	run crictl rmp "$pod_id"
	echo "$output"
	[ "$status" -eq 0 ]
	cleanup_ctrs
	cleanup_pods
	stop_crio
}
